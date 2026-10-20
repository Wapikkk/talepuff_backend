package handlers

import (
	"net/http"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"
	"talepuff_backend/validation"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const feedbackLimitPerHour = 5

func SubmitFeedback(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.FeedbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Input"})
			return
		}

		category, err := validation.FeedbackCategory(req.Category)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		message, err := validation.FeedbackMessage(req.Message)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var user models.User
		if err := db.Where("firebase_uid = ?", auth.IdentityFrom(c).UID).First(&user).Error; err != nil {
			lookupFailed(c, err, "User not found")
			return
		}

		var recent int64
		since := time.Now().Add(-time.Hour)
		if err := db.Model(&models.Feedback{}).Where("user_id = ? AND created_at > ?", user.ID, since).Count(&recent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if recent >= feedbackLimitPerHour {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many feedback messages. Please try again later."})
			return
		}

		feedback := models.Feedback{
			UserID:     user.ID,
			Category:   category,
			Message:    message,
			AppVersion: validation.AppVersion(req.AppVersion),
		}
		if err := db.Create(&feedback).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save feedback"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Thank you for your feedback!"})
	}
}
