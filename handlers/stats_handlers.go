package handlers

import (
	"net/http"
	"strconv"
	"sync"
	"talepuff_backend/auth"
	"talepuff_backend/models"
	"talepuff_backend/totem"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var Now = time.Now

const (
	statsDays        = 7
	listenWriteEvery = 5 * time.Second
	listenWindow     = 24 * time.Hour
)

func recordPlay(db *gorm.DB, story models.Story, deviceID string) {
	db.Create(&models.StoryPlay{StoryID: story.ID, ChildID: story.ChildID, DeviceID: deviceID, StartedAt: Now()})
}

func RecordListening(db *gorm.DB) func(deviceID string, status totem.Status) {
	var mu sync.Mutex
	lastWrite := map[string]time.Time{}

	return func(deviceID string, status totem.Status) {
		if status.StoryID == nil || status.Position <= 0 {
			return
		}
		switch status.State {
		case "playing", "paused", "idle":
		default:
			return
		}
		position := status.Position
		if status.Duration > 0 {
			position = min(position, status.Duration)
		}

		mu.Lock()
		if status.State == "playing" && Now().Sub(lastWrite[deviceID]) < listenWriteEvery {
			mu.Unlock()
			return
		}
		lastWrite[deviceID] = Now()
		mu.Unlock()

		db.Exec(`UPDATE story_plays SET listened_seconds = ?
			WHERE id = (SELECT id FROM story_plays WHERE device_id = ? AND story_id = ? AND started_at > ? ORDER BY started_at DESC LIMIT 1)
			AND listened_seconds < ?`,
			position, deviceID, *status.StoryID, Now().Add(-listenWindow), position)
	}
}

type dayStats struct {
	Date    string `json:"date"`
	Stories int    `json:"stories"`
	Plays   int    `json:"plays"`
	Seconds int    `json:"seconds"`
}

func WeeklyStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		childID, err := strconv.ParseUint(c.Query("child_id"), 10, 64)
		if err != nil || childID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "child_id is required"})
			return
		}
		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(childID))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}

		now := Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		since := today.AddDate(0, 0, -(statsDays - 1))

		days := make([]dayStats, statsDays)
		index := map[string]int{}
		for i := range days {
			date := since.AddDate(0, 0, i).Format("2006-01-02")
			days[i].Date = date
			index[date] = i
		}
		bucket := func(t time.Time) (int, bool) {
			i, ok := index[t.In(now.Location()).Format("2006-01-02")]
			return i, ok
		}

		var created []time.Time
		if err := db.Model(&models.Story{}).Where("child_id = ? AND created_at >= ?", child.ID, since).Pluck("created_at", &created).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		var plays []models.StoryPlay
		if err := db.Where("child_id = ? AND started_at >= ?", child.ID, since).Find(&plays).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		var total dayStats
		for _, t := range created {
			if i, ok := bucket(t); ok {
				days[i].Stories++
				total.Stories++
			}
		}
		for _, play := range plays {
			if i, ok := bucket(play.StartedAt); ok {
				days[i].Plays++
				days[i].Seconds += play.ListenedSeconds
				total.Plays++
				total.Seconds += play.ListenedSeconds
			}
		}
		c.JSON(http.StatusOK, gin.H{"days": days, "totals": gin.H{"stories": total.Stories, "plays": total.Plays, "seconds": total.Seconds}})
	}
}
