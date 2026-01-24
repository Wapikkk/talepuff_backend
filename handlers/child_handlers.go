package handlers

import (
	"net/http"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetChildInfo(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("uid")
		var user models.User
		var child models.Child

		// User berdasarkan firebase_uid
		if err := db.Where("firebase_uid = ?", uid).First(&user).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan"})
			return
		}

		// Child berdasarkan user_id
		if err := db.Where("user_id = ?", user.ID).First(&child).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Data anak tidak ditemukan"})
			return
		}

		// Kirim data anak ke Flutter
		c.JSON(http.StatusOK, child)
	}
}
