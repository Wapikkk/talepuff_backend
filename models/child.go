package models

import (
	"time"

	pq "github.com/lib/pq"
)

type Child struct {
	ID        uint           `gorm:"primaryKey"`
	UserID    uint           `gorm:"not null"`
	Name      string         `gorm:"not null" json:"name"`
	Age       *int           `json:"age"`
	Gender    string         `json:"gender"`
	Interests pq.StringArray `gorm:"type:text[]" json:"interests"`
	CreatedAt time.Time      `json:"created_at"`
}
