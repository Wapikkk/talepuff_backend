package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"talepuff_backend/dto"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GenerateStory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDClaim, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid or missing token"})
			return
		}

		var currentUserID uint
		switch v := userIDClaim.(type) {
		case float64:
			currentUserID = uint(v)
		case uint:
			currentUserID = v
		case int:
			currentUserID = uint(v)
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse user ID from token"})
			return
		}

		var req dto.GenerateStoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var child models.Child
		if err := db.First(&child, "id = ? AND user_id = ?", req.ChildID, currentUserID).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Child profile not found or access denied"})
			return
		}

		pythonPayload, _ := json.Marshal(req)
		pythonAPIURL := "http://localhost:8000/generate"
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
