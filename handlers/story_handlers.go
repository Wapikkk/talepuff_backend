package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"
	"talepuff_backend/validation"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	maxStoredMoodRunes        = 200
	maxStoredDescriptionRunes = 1000
)

var aiClient = &http.Client{Timeout: 3 * time.Minute}

var errAIDecode = errors.New("failed to read the AI service response")

func aiBaseURL() string {
	if url := strings.TrimRight(strings.TrimSpace(os.Getenv("AI_SERVICE_URL")), "/"); url != "" {
		return url
	}
	return "http://localhost:8000"
}

func callAI(path string, payload, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := aiClient.Post(aiBaseURL()+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("AI service answered %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errAIDecode
	}
	return nil
}

func truncateRunes(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

func storyDTO(story models.Story, full bool) dto.StoryDTO {
	result := dto.StoryDTO{
		ID:            story.ID,
		ChildID:       story.ChildID,
		CharacterName: story.CharacterName,
		Theme:         story.Theme,
		Mood:          story.Mood,
		Language:      story.Language,
		LengthMinutes: story.LengthMinutes,
		Title:         story.Title,
		AudioURL:      story.AudioURL,
		VoiceTone:     story.VoiceTone,
		IsFavorite:    story.IsFavorite,
		PlayCount:     story.PlayCount,
		CreatedAt:     story.CreatedAt,
	}
	if full {
		result.Content = story.Content
		result.Description = story.Description
	}
	return result
}

func GenerateStory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.GenerateStoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		language, err := validation.Language(req.Language)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.Language = language

		if _, err := findOwnedChild(db, auth.IdentityFrom(c).UID, req.ChildID); err != nil {
			lookupFailed(c, err, "Child profile not found")
			return
		}

		var aiResult dto.PythonResponse
		if err := callAI("/generate_story", req, &aiResult); err != nil {
			if errors.Is(err, errAIDecode) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read AI response"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to AI service"})
			return
		}

		newStory := models.Story{
			ChildID:       req.ChildID,
			CharacterName: req.CharacterName,
			Theme:         req.Theme,
			Mood:          truncateRunes(req.Mood, maxStoredMoodRunes),
			Description:   truncateRunes(req.Description, maxStoredDescriptionRunes),
			Language:      req.Language,
			LengthMinutes: req.LengthMinutes,
			Title:         aiResult.Title,
			Content:       aiResult.Response,
		}

		if err := db.Create(&newStory).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save story to database"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Story generated successfully",
			"data":    storyDTO(newStory, true),
		})
	}
}

func GeneratedAudio(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.AudiGeneratedRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		voiceTone, err := validation.VoiceTone(req.VoiceTone)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		story, err := findOwnedStory(db, auth.IdentityFrom(c).UID, req.StoryID)
		if err != nil {
			lookupFailed(c, err, "Story not found")
			return
		}

		payload := map[string]string{"story_text": story.Content, "voice_tone": voiceTone}
		var aiResult dto.PythonAudioResponse
		if err := callAI("/generate_audio", payload, &aiResult); err != nil {
			if errors.Is(err, errAIDecode) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TTS response"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to contact TTS service"})
			return
		}

		if err := db.Model(&models.Story{}).Where("id = ?", story.ID).
			Updates(map[string]any{"audio_url": aiResult.AudioURL, "voice_tone": voiceTone}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save audio"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":   "Audio generated successfully",
			"audio_url": aiResult.AudioURL,
		})
	}
}
