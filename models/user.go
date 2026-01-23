package models

import "time"

type User struct {
	ID          uint      `gorm:"primaryKey"`
	FirebaseUID string    `gorm:"uniqueIndex;not null" json:"firebase_uid"`
	Email       string    `gorm:"uniqueIndex:nor null" json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}
