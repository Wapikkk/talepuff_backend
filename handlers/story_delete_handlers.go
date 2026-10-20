package handlers

import (
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"talepuff_backend/auth"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func DeleteStory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story id"})
			return
		}
		story, err := findOwnedStory(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Story not found")
			return
		}

		if err := db.Delete(&models.Story{}, story.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete story"})
			return
		}

		removeStoryAudio(story.AudioURL)
		c.JSON(http.StatusOK, gin.H{"message": "Story deleted"})
	}
}

func removeStoryAudio(audioURL string) {
	name := audioFileName(audioURL)
	if name == "" {
		return
	}
	request, err := http.NewRequest(http.MethodDelete, aiBaseURL()+"/generated_audio/"+url.PathEscape(name), nil)
	if err != nil {
		log.Printf("audio cleanup: %v", err)
		return
	}
	response, err := aiClient.Do(request)
	if err != nil {
		log.Printf("audio cleanup for %s failed: %v", name, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		log.Printf("audio cleanup for %s: AI service answered %d", name, response.StatusCode)
	}
}

func audioFileName(audioURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(audioURL))
	if err != nil || parsed.Path == "" {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" {
		return ""
	}
	return name
}
