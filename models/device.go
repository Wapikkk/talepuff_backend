package models

import "time"

// Device menautkan satu totem (DeviceID dari firmware) ke satu akun.
type Device struct {
	ID       uint   `gorm:"primaryKey"`
	DeviceID string `gorm:"uniqueIndex;not null" json:"device_id"`
	UserID   uint   `gorm:"uniqueIndex;not null" json:"-"`
	// VolumeLimit (0-100) adalah batas dari orang tua; backend tidak pernah mengirim volume di atasnya.
	VolumeLimit int `gorm:"not null;default:50" json:"volume_limit"`
	// Cerita malam otomatis: jam mengikuti zona waktu server. BedtimeLastRun (YYYY-MM-DD) mencegah putar dua kali sehari.
	BedtimeEnabled bool      `gorm:"not null;default:false" json:"-"`
	BedtimeHour    int       `gorm:"not null;default:20" json:"-"`
	BedtimeMinute  int       `gorm:"not null;default:0" json:"-"`
	BedtimeLastRun string    `gorm:"not null;default:''" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}
