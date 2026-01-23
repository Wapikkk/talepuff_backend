package models

import (
	"time"

	pq "github.com/lib/pq"
)

type Child struct {
	ID        uint           `gorm:"primaryKey"`
	UserID    uint           `gorm:"not null"`
	Name      string         `gorm:"not null" json:"name"`
	Age       uint           `json:"age"`
	Gender    string         `json:"gender"`
	Interest  pq.StringArray `gorm:"type:text[]" json:"interest"`
	CreatedAt time.Time      `json:"created_at"`
}
