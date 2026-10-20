package models

import (
	"gorm.io/gorm"
)

type Story struct {
	gorm.Model
	ChildID       uint   `json:"child_id" gorm:"index"`
	CharacterName string `json:"character_name"`
	Theme         string `json:"theme"`
	Mood          string `json:"mood"`
	Description   string `json:"description" gorm:"type:text"`
	Language      string `json:"language"`
	LengthMinutes int    `json:"length_minutes"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	AudioURL      string `json:"audio_url"`
	VoiceTone     string `json:"voice_tone"`
	IsFavorite    bool   `json:"is_favorite" gorm:"not null;default:false"`
	PlayCount     int    `json:"play_count" gorm:"not null;default:0"`
}
