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
	VoiceTone     string `json:"voice_tone"`
}

type PythonResponse struct {
	Status   string   `json:"status"`
	Title    string   `json:"title"`
	Response string   `json:"response"`
	Flags    []string `json:"flags"`
}

type AudiGeneratedRequest struct {
	StoryID   uint   `json:"story_id" binding:"required"`
	VoiceTone string `json:"voice_tone" binding:"required"`
}

// Sentence adalah satu kalimat yang dibacakan, dengan waktu mulai/selesai (milidetik) dalam audio.
type Sentence struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

type PythonAudioResponse struct {
	Status    string     `json:"status"`
	AudioURL  string     `json:"audio_url"`
	Sentences []Sentence `json:"sentences"`
}

type StoryDTO struct {
	ID            uint       `json:"id"`
	ChildID       uint       `json:"child_id"`
	CharacterName string     `json:"character_name"`
	Theme         string     `json:"theme"`
	Mood          string     `json:"mood"`
	Language      string     `json:"language"`
	LengthMinutes int        `json:"length_minutes"`
	Title         string     `json:"title"`
	Content       string     `json:"content,omitempty"`
	Description   string     `json:"description,omitempty"`
	AudioURL      string     `json:"audio_url"`
	Sentences     []Sentence `json:"sentences,omitempty"`
	VoiceTone     string     `json:"voice_tone"`
	IsFavorite    bool       `json:"is_favorite"`
	PlayCount     int        `json:"play_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

type StoryListResponse struct {
	Stories []StoryDTO `json:"stories"`
	HasMore bool       `json:"has_more"`
}

type SetFavoriteRequest struct {
	Favorite *bool `json:"favorite" binding:"required"`
}

type StoryJobDTO struct {
	ID           uint      `json:"id"`
	Status       string    `json:"status"`
	Story        *StoryDTO `json:"story,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	IdeaDropped  bool      `json:"idea_dropped"`
	NameReplaced bool      `json:"name_replaced"`
}
