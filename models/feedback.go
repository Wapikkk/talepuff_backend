package models

import "gorm.io/gorm"

type Feedback struct {
	gorm.Model
	UserID     uint   `gorm:"not null;index"`
	Category   string `gorm:"not null"`
	Message    string `gorm:"type:text;not null"`
	AppVersion string
}
