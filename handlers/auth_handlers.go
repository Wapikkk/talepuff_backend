package handlers

import (
	"net/http"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type RegisterRequest struct {
	FirebaseUID string   `json:"firebase_uid" binding:"required"`
	Email       string   `json:"email" binding:"required"`
	ChildName   string   `json:"child_name" binding:"required"`
	Age         int      `json:"age"`
	Gender      string   `json:"gender"`
	Interests   []string `json:"interests"`
}

func RegisterUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			user := models.User{
				FirebaseUID: req.FirebaseUID,
				Email:       req.Email,
			}

			if err := tx.Where(models.User{FirebaseUID: req.FirebaseUID}).FirstOrCreate(&user).Error; err != nil {
				return err
			}

			child := models.Child{
				UserID:   user.ID,
				Name:     req.ChildName,
				Age:      uint(req.Age),
				Gender:   req.Gender,
				Interest: pq.StringArray(req.Interests),
			}

			if err := tx.Where(models.Child{UserID: user.ID}).Assign(child).FirstOrCreate(&child).Error; err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save data"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User and Child data saved successfully!"})
	}
}
