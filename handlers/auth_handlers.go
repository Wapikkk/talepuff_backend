package handlers

import (
	"errors"
	"net/http"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"
	"talepuff_backend/validation"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

var errProfileExists = errors.New("profile already exists")

func RegisterUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		uid := auth.IdentityFrom(c).UID
		if req.FirebaseUID != "" && req.FirebaseUID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "firebase_uid does not match the authenticated user"})
			return
		}

		childName, err := validation.ChildName(req.ChildName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			user := models.User{FirebaseUID: uid}
			if err := tx.Where(models.User{FirebaseUID: uid}).FirstOrCreate(&user).Error; err != nil {
				return err
			}

			var existing models.Child
			switch err := tx.Where("user_id = ?", user.ID).First(&existing).Error; {
			case err == nil:
				return errProfileExists
			case !errors.Is(err, gorm.ErrRecordNotFound):
				return err
			}

			child := models.Child{
				UserID:    user.ID,
				Name:      childName,
				Age:       &req.Age,
				Gender:    req.Gender,
				Interests: pq.StringArray(req.Interests),
			}
			return tx.Create(&child).Error
		})

		switch {
		case errors.Is(err, errProfileExists):
			c.JSON(http.StatusConflict, gin.H{"error": "Profile already exists for this account"})
		case errors.Is(err, gorm.ErrDuplicatedKey):
			// Dua pendaftaran serentak untuk uid yang sama: yang kalah menabrak indeks unik firebase_uid.
			c.JSON(http.StatusConflict, gin.H{"error": "Profile already exists for this account"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save data"})
		default:
			c.JSON(http.StatusOK, gin.H{"message": "User and Child data saved successfully!"})
		}
	}
}
