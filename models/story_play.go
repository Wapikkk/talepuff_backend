package models

import "time"

type StoryPlay struct {
	ID              uint      `gorm:"primaryKey"`
	StoryID         uint      `gorm:"index;not null"`
	ChildID         uint      `gorm:"index;not null"`
	DeviceID        string    `gorm:"not null;default:''"`
	StartedAt       time.Time `gorm:"index;not null"`
	ListenedSeconds int       `gorm:"not null;default:0"`
}
