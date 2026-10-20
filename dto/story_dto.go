package dto

import "time"

type GenerateStoryRequest struct {
	ChildID       uint   `json:"child_id"`
	CharacterName string `json:"character_name"`
	Theme         string `json:"theme"`
	Mood          string `json:"mood"`
	LengthMinutes int    `json:"length_minutes"`
	Language      string `json:"language"`
	Title         string `json:"title"`
	Description   string `json:"description"`
}

type PythonResponse struct {
	Status   string `json:"status"`
	Title    string `json:"title"`
	Response string `json:"response"`
}

type AudiGeneratedRequest struct {
	StoryID   uint   `json:"story_id" binding:"required"`
	VoiceTone string `json:"voice_tone" binding:"required"`
}

type PythonAudioResponse struct {
	Status   string `json:"status"`
	AudioURL string `json:"audio_url"`
}

type StoryDTO struct {
	ID            uint      `json:"id"`
	ChildID       uint      `json:"child_id"`
	CharacterName string    `json:"character_name"`
	Theme         string    `json:"theme"`
	Mood          string    `json:"mood"`
	Language      string    `json:"language"`
	LengthMinutes int       `json:"length_minutes"`
	Title         string    `json:"title"`
	Content       string    `json:"content,omitempty"`
	Description   string    `json:"description,omitempty"`
	AudioURL      string    `json:"audio_url"`
	VoiceTone     string    `json:"voice_tone"`
	IsFavorite    bool      `json:"is_favorite"`
	PlayCount     int       `json:"play_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type StoryListResponse struct {
	Stories []StoryDTO `json:"stories"`
	HasMore bool       `json:"has_more"`
}

type SetFavoriteRequest struct {
	Favorite *bool `json:"favorite" binding:"required"`
}
