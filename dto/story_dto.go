package dto

type GenerateStoryRequest struct {
	ChildID       uint   `json:"child_id"`
	CharacterName string `json:"character_name"`
	Theme         string `json:"theme"`
	Mood          string `json:"mood"`
	LengthMinutes int    `json:"length_minutes"`
	Language      string `json:"language"`
	Description   string `json:"description"`
}

type PythonResponse struct {
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
