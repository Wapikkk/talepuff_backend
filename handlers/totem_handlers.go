package handlers

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"talepuff_backend/auth"
	"talepuff_backend/models"
	"talepuff_backend/totem"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Sama dengan SAFE_AUDIO_NAME di layanan AI: hanya WAV buatan layanan itu yang boleh dikirim ke totem.
var safeAudioName = regexp.MustCompile(`^story_\d+_[A-Za-z]+\.wav$`)

// totemAudioURL menyusun alamat yang bisa dijangkau totem. URL di database memakai localhost
// (alamat layanan AI dari sudut pandang laptop), jadi hanya nama berkasnya yang dipakai.
func totemAudioURL(storedURL string) (string, bool) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AUDIO_PUBLIC_BASE_URL")), "/")
	name := audioFileName(storedURL)
	if base == "" || !safeAudioName.MatchString(name) {
		return "", false
	}
	return base + "/audio/" + name, true
}

func totemEnabled(hub totem.Hub) bool {
	_, off := hub.(totem.Disabled)
	return !off
}

// totemFail menjawab galat dengan `code` yang tetap, supaya aplikasi tidak perlu mencocokkan teks.
func totemFail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": message, "code": code})
}

func totemUnavailable(c *gin.Context) {
	totemFail(c, http.StatusServiceUnavailable, "totem_unavailable", "The totem service is not set up on the server")
}

func totemNotPaired(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		totemFail(c, http.StatusNotFound, "totem_not_paired", "No totem is linked to this account")
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
}

func ownedDevice(db *gorm.DB, uid string) (models.Device, error) {
	var device models.Device
	err := db.Select("devices.*").
		Joins("JOIN users ON users.id = devices.user_id").
		Where("users.firebase_uid = ?", uid).
		First(&device).Error
	return device, err
}

func GetTotem(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		device, err := ownedDevice(db, auth.IdentityFrom(c).UID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"paired": false})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"paired":       true,
			"device_id":    device.DeviceID,
			"volume_limit": device.VolumeLimit,
			"bedtime":      bedtimeJSON(device),
			"status":       hub.Status(device.DeviceID),
		})
	}
}

func PairTotem(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !totemEnabled(hub) {
			totemUnavailable(c)
			return
		}
		var req struct {
			DeviceID string `json:"device_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		id := strings.ToLower(strings.TrimSpace(req.DeviceID))
		if !totem.ValidDeviceID(id) {
			totemFail(c, http.StatusBadRequest, "invalid_device_id", "That is not a valid totem id")
			return
		}

		var user models.User
		if err := db.Where("firebase_uid = ?", auth.IdentityFrom(c).UID).First(&user).Error; err != nil {
			lookupFailed(c, err, "User not found")
			return
		}
		// Hanya totem yang sedang terlihat di broker yang bisa ditautkan, jadi id asal tebak tidak bisa "diklaim".
		if !hub.Known(id) {
			totemFail(c, http.StatusNotFound, "totem_not_visible", "We can't see this totem. Make sure it is on and connected to Wi-Fi")
			return
		}

		var existing models.Device
		err := db.Where("device_id = ?", id).First(&existing).Error
		switch {
		case err == nil && existing.UserID != user.ID:
			totemFail(c, http.StatusConflict, "totem_taken", "This totem is already linked to another account")
			return
		case err == nil:
			c.JSON(http.StatusOK, gin.H{"device_id": id})
			return
		case !errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// Satu akun, satu totem: menautkan totem baru menggantikan yang lama.
		res := db.Model(&models.Device{}).Where("user_id = ?", user.ID).Update("device_id", id)
		if res.Error == nil && res.RowsAffected == 0 {
			res.Error = db.Create(&models.Device{DeviceID: id, UserID: user.ID}).Error
		}
		if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
			totemFail(c, http.StatusConflict, "totem_taken", "This totem is already linked to another account")
			return
		}
		if res.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link the totem"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"device_id": id})
	}
}

func UnpairTotem(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		device, err := ownedDevice(db, auth.IdentityFrom(c).UID)
		if err != nil {
			totemNotPaired(c, err)
			return
		}
		if err := db.Delete(&device).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unlink the totem"})
			return
		}
		_ = hub.Send(device.DeviceID, totem.Command{Action: "stop"})
		c.JSON(http.StatusOK, gin.H{"message": "Totem unlinked"})
	}
}

func validVolume(v *int) bool { return v == nil || (*v >= 0 && *v <= 100) }

// playVolume memilih volume untuk perintah play dan memotongnya ke batas orang tua. Tanpa permintaan
// eksplisit dipakai volume totem saat ini (bila diketahui), supaya batas yang lebih rendah tetap berlaku.
func playVolume(requested *int, current, limit int) int {
	volume := limit
	switch {
	case requested != nil:
		volume = *requested
	case current > 0:
		volume = current
	}
	return min(volume, limit)
}

// deviceForCommand mengembalikan totem milik pengguna yang siap menerima perintah, atau menjawab galatnya.
func deviceForCommand(c *gin.Context, db *gorm.DB, hub totem.Hub) (models.Device, bool) {
	if !totemEnabled(hub) {
		totemUnavailable(c)
		return models.Device{}, false
	}
	device, err := ownedDevice(db, auth.IdentityFrom(c).UID)
	if err != nil {
		totemNotPaired(c, err)
		return device, false
	}
	if !hub.Status(device.DeviceID).Online {
		totemFail(c, http.StatusConflict, "totem_offline", "Your totem is offline")
		return device, false
	}
	return device, true
}

func sendFailed(c *gin.Context, err error) {
	if errors.Is(err, totem.ErrNotConfigured) {
		totemUnavailable(c)
		return
	}
	totemFail(c, http.StatusServiceUnavailable, "totem_unreachable", "We can't reach the totem right now. Please try again")
}

func PlayOnTotem(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			StoryID uint `json:"story_id"`
			Volume  *int `json:"volume"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.StoryID == 0 || !validVolume(req.Volume) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		story, err := findOwnedStory(db, auth.IdentityFrom(c).UID, req.StoryID)
		if err != nil {
			lookupFailed(c, err, "Story not found")
			return
		}
		if story.AudioURL == "" {
			totemFail(c, http.StatusConflict, "story_no_voice", "This story has no voice yet")
			return
		}
		device, ok := deviceForCommand(c, db, hub)
		if !ok {
			return
		}
		audioURL, ok := totemAudioURL(story.AudioURL)
		if !ok {
			totemFail(c, http.StatusServiceUnavailable, "totem_unavailable", "The audio address for the totem is not set up on the server")
			return
		}

		storyID := story.ID
		volume := playVolume(req.Volume, hub.Status(device.DeviceID).Volume, device.VolumeLimit)
		if err := hub.Send(device.DeviceID, totem.Command{Action: "play", URL: audioURL, StoryID: &storyID, Volume: &volume}); err != nil {
			sendFailed(c, err)
			return
		}
		db.Model(&models.Story{}).Where("id = ?", story.ID).UpdateColumn("play_count", gorm.Expr("play_count + 1"))
		recordPlay(db, story, device.DeviceID)
		c.JSON(http.StatusOK, gin.H{"message": "Sent to the totem"})
	}
}

func CommandTotem(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Action string `json:"action"`
			Value  *int   `json:"value"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		switch req.Action {
		case "pause", "resume", "stop":
			req.Value = nil
		case "volume":
			if req.Value == nil || !validVolume(req.Value) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Volume must be between 0 and 100"})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown action"})
			return
		}

		device, ok := deviceForCommand(c, db, hub)
		if !ok {
			return
		}
		if req.Action == "volume" {
			capped := min(*req.Value, device.VolumeLimit)
			req.Value = &capped
		}
		if err := hub.Send(device.DeviceID, totem.Command{Action: req.Action, Value: req.Value}); err != nil {
			sendFailed(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Sent to the totem"})
	}
}

func SetTotemVolumeLimit(db *gorm.DB, hub totem.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Limit *int `json:"limit"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Limit == nil || !validVolume(req.Limit) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "The limit must be between 0 and 100"})
			return
		}
		device, err := ownedDevice(db, auth.IdentityFrom(c).UID)
		if err != nil {
			totemNotPaired(c, err)
			return
		}
		if err := db.Model(&device).Update("volume_limit", *req.Limit).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the volume limit"})
			return
		}
		// Bila totem sedang bersuara lebih keras dari batas baru, turunkan sekarang.
		if status := hub.Status(device.DeviceID); status.Online && status.Volume > *req.Limit {
			_ = hub.Send(device.DeviceID, totem.Command{Action: "volume", Value: req.Limit})
		}
		c.JSON(http.StatusOK, gin.H{"volume_limit": *req.Limit})
	}
}
