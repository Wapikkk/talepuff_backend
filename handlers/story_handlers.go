package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GenerateStory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.GenerateStoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, err := findOwnedChild(db, auth.IdentityFrom(c).UID, req.ChildID); err != nil {
			lookupFailed(c, err, "Child profile not found")
			return
		}

		pythonPayload, _ := json.Marshal(req)
		pythonAPIURL := "http://localhost:8000/generate_story"
		resp, err := http.Post(pythonAPIURL, "application/json", bytes.NewBuffer(pythonPayload))
		if err != nil || resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to AI service"})
			return
		}
		defer resp.Body.Close()

		var aiResult dto.PythonResponse
		if err := json.NewDecoder(resp.Body).Decode(&aiResult); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read AI response"})
			return
		}

		newStory := models.Story{
			ChildID:       req.ChildID,
			CharacterName: req.CharacterName,
			Theme:         req.Theme,
			Language:      req.Language,
			Title:         aiResult.Title,
			Content:       aiResult.Response,
		}

		if err := db.Create(&newStory).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save story to database"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Story generated successfully",
			"data":    newStory,
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

		story, err := findOwnedStory(db, auth.IdentityFrom(c).UID, req.StoryID)
		if err != nil {
			lookupFailed(c, err, "Story not found")
			return
		}

		pythonPayload := map[string]string{
			"story_text": story.Content,
			"voice_tone": req.VoiceTone,
		}
		pythonReqBody, _ := json.Marshal(pythonPayload)

		resp, err := http.Post("http://localhost:8000/generate_audio", "application/json", bytes.NewBuffer(pythonReqBody))
		if err != nil || resp.StatusCode != 200 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to contact TTS service"})
			return
		}
		defer resp.Body.Close()

		var pythonResp dto.PythonAudioResponse
		if err := json.NewDecoder(resp.Body).Decode(&pythonResp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TTS response"})
			return
		}

		story.AudioURL = pythonResp.AudioURL
		db.Save(&story)

		c.JSON(http.StatusOK, gin.H{
			"message":   "Audio generated successfully",
			"audio_url": story.AudioURL,
		})
	}
}
