package handlers

import (
	"net/http"
	"strconv"
	"talepuff_backend/auth"
	"talepuff_backend/models"
	"talepuff_backend/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetChildInfo(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("uid")
		if uid != auth.IdentityFrom(c).UID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only read your own profile"})
			return
		}
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

func UpdateChildName(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid child id"})
			return
		}

		var input struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Input Data"})
			return
		}
		name, err := validation.ChildName(input.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}

		result := db.Model(&models.Child{}).Where("id = ?", child.ID).Update("name", name)
		switch {
		case result.Error != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update database"})
		case result.RowsAffected == 0:
			c.JSON(http.StatusNotFound, gin.H{"error": "Child not found"})
		default:
			c.JSON(http.StatusOK, gin.H{
				"message":  "Name updated successfully",
				"new_name": name,
			})
		}
	}
}
