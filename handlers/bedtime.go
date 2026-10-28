package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"talepuff_backend/auth"
	"talepuff_backend/models"
	"talepuff_backend/totem"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	bedtimeGrace        = 10 * time.Minute
	bedtimeTickInterval = 20 * time.Second
)

func bedtimeJSON(device models.Device) gin.H {
	return gin.H{"enabled": device.BedtimeEnabled, "hour": device.BedtimeHour, "minute": device.BedtimeMinute}
}

func SetTotemBedtime(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Enabled *bool `json:"enabled"`
			Hour    *int  `json:"hour"`
			Minute  *int  `json:"minute"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil || req.Hour == nil || req.Minute == nil ||
			*req.Hour < 0 || *req.Hour > 23 || *req.Minute < 0 || *req.Minute > 59 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Enabled, hour (0-23) and minute (0-59) are required"})
			return
		}
		device, err := ownedDevice(db, auth.IdentityFrom(c).UID)
		if err != nil {
			totemNotPaired(c, err)
			return
		}
		err = db.Model(&device).Updates(map[string]any{
			"bedtime_enabled":  *req.Enabled,
			"bedtime_hour":     *req.Hour,
			"bedtime_minute":   *req.Minute,
			"bedtime_last_run": "",
		}).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the schedule"})
			return
		}
		device.BedtimeEnabled, device.BedtimeHour, device.BedtimeMinute = *req.Enabled, *req.Hour, *req.Minute
		c.JSON(http.StatusOK, gin.H{"bedtime": bedtimeJSON(device)})
	}
}

func RunBedtimeScheduler(ctx context.Context, db *gorm.DB, hub totem.Hub) {
	ticker := time.NewTicker(bedtimeTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			RunBedtimeOnce(db, hub, now)
		}
	}
}

func RunBedtimeOnce(db *gorm.DB, hub totem.Hub, now time.Time) {
	var devices []models.Device
	if err := db.Where("bedtime_enabled = ?", true).Find(&devices).Error; err != nil {
		log.Printf("bedtime: reading schedules failed: %v", err)
		return
	}
	for _, device := range devices {
		runBedtime(db, hub, device, now)
	}
}

func runBedtime(db *gorm.DB, hub totem.Hub, device models.Device, now time.Time) {
	today := now.Format("2006-01-02")
	scheduled := time.Date(now.Year(), now.Month(), now.Day(), device.BedtimeHour, device.BedtimeMinute, 0, 0, now.Location())
	if device.BedtimeLastRun == today || now.Before(scheduled) || !now.Before(scheduled.Add(bedtimeGrace)) {
		return
	}

	done := func() {
		db.Model(&models.Device{}).Where("id = ?", device.ID).Update("bedtime_last_run", today)
	}

	status := hub.Status(device.DeviceID)
	if !status.Online {
		return
	}
	if status.State == "playing" || status.State == "paused" || status.State == "loading" {
		done()
		return
	}

	story, err := latestStoryWithAudio(db, device.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		done()
		return
	}
	if err != nil {
		log.Printf("bedtime: finding a story failed: %v", err)
		return
	}
	audioURL, ok := totemAudioURL(story.AudioURL)
	if !ok {
		log.Printf("bedtime: AUDIO_PUBLIC_BASE_URL is not set or story %d has an unusable audio file", story.ID)
		done()
		return
	}

	storyID := story.ID
	volume := playVolume(nil, status.Volume, device.VolumeLimit)
	if err := hub.Send(device.DeviceID, totem.Command{Action: "play", URL: audioURL, StoryID: &storyID, Volume: &volume}); err != nil {
		log.Printf("bedtime: sending to %s failed: %v", device.DeviceID, err)
		return
	}
	db.Model(&models.Story{}).Where("id = ?", story.ID).UpdateColumn("play_count", gorm.Expr("play_count + 1"))
	recordPlay(db, story, device.DeviceID)
	done()
}

func latestStoryWithAudio(db *gorm.DB, userID uint) (models.Story, error) {
	var story models.Story
	err := db.Select("stories.*").
		Joins("JOIN children ON children.id = stories.child_id").
		Where("children.user_id = ? AND stories.audio_url <> ''", userID).
		Order("stories.created_at DESC, stories.id DESC").
		First(&story).Error
	return story, err
}
