package models

import (
	"gorm.io/gorm"
)

type Story struct {
	gorm.Model
	ChildID       uint   `json:"child_id"`
	CharacterName string `json:"character_name"`
	Theme         string `json:"theme"`
	Language      string `json:"language"`
	Content       string `json:"content"`
}
