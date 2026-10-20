package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"talepuff_backend/auth"
	"talepuff_backend/models"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	maxPhotoBytes    = 5 << 20
	profileDir       = "uploads/profiles"
	profileURLPrefix = "/uploads/profiles/"
)

var photoExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

func UploadChildPhotoHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid child id"})
			return
		}
		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPhotoBytes+(1<<20))
		file, err := c.FormFile("photo")
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Photo is too large (max 5 MB)"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read the photo"})
			return
		}
		if file.Size > maxPhotoBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Photo is too large (max 5 MB)"})
			return
		}

		extension, err := imageExtension(file)
		if err != nil {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Only JPEG, PNG or WebP photos are accepted"})
			return
		}

		name := fmt.Sprintf("child-%d-%d%s", child.ID, time.Now().UnixNano(), extension)
		savePath := filepath.Join(profileDir, name)
		if err := c.SaveUploadedFile(file, savePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the photo"})
			return
		}

		dbPath := profileURLPrefix + name
		if err := db.Model(&models.Child{}).Where("id = ?", child.ID).Update("profile_photo_url", dbPath).Error; err != nil {
			_ = os.Remove(savePath) // jangan tinggalkan berkas yatim
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update database"})
			return
		}

		removeProfileFile(child.ProfilePhotoURL) // foto lama sudah tidak dipakai
		c.JSON(http.StatusOK, gin.H{"message": "Upload berhasil", "url": dbPath})
	}
}

func DeleteChildPhoto(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid child id"})
			return
		}
		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}

		if err := db.Model(&models.Child{}).Where("id = ?", child.ID).Update("profile_photo_url", "").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update database"})
			return
		}
		removeProfileFile(child.ProfilePhotoURL)
		c.JSON(http.StatusOK, gin.H{"message": "Photo removed"})
	}
}

func imageExtension(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	extension, ok := photoExtensions[http.DetectContentType(head[:n])]
	if !ok {
		return "", errors.New("unsupported image type")
	}
	return extension, nil
}

func removeProfileFile(photoURL string) {
	if !strings.HasPrefix(photoURL, profileURLPrefix) {
		return
	}
	name := filepath.Base(photoURL)
	if name == "." || name == string(filepath.Separator) {
		return
	}
	if err := os.Remove(filepath.Join(profileDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("photo: gagal menghapus %s: %v", name, err)
	}
}
