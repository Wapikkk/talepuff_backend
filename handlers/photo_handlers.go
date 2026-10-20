package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"talepuff_backend/auth"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func UploadChildPhotoHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		parsedID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid child id"})
			return
		}
		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(parsedID))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}
		childID := child.ID

		file, err := c.FormFile("photo")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Gagal mengambil file"})
			return
		}

		filename := fmt.Sprintf("%d-%s", time.Now().Unix(), file.Filename)
		savePath := filepath.Join("uploads", "profiles", filename)

		if err := c.SaveUploadedFile(file, savePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal simpan file di server"})
			return
		}

		dbPath := "/uploads/profiles/" + filename
		query := "UPDATE children SET profile_photo_url = ? WHERE id = ?"

		if err := db.Exec(query, dbPath, childID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal update database"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Upload berhasil",
			"url":     dbPath,
		})
	}
}
