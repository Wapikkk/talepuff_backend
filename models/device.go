package models

import "time"

// Device menautkan satu totem (DeviceID dari firmware) ke satu akun.
type Device struct {
	ID        uint      `gorm:"primaryKey"`
	DeviceID  string    `gorm:"uniqueIndex;not null" json:"device_id"`
	UserID    uint      `gorm:"uniqueIndex;not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
