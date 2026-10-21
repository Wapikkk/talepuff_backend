package handlers

import (
	"errors"
	"net/http"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func findOwnedChild(db *gorm.DB, uid string, childID uint) (models.Child, error) {
	var child models.Child
	err := db.Select("children.*").
		Joins("JOIN users ON users.id = children.user_id").
		Where("children.id = ? AND users.firebase_uid = ?", childID, uid).
		First(&child).Error
	return child, err
}

func findOwnedStory(db *gorm.DB, uid string, storyID uint) (models.Story, error) {
	var story models.Story
	err := db.Select("stories.*").
		Joins("JOIN children ON children.id = stories.child_id").
		Joins("JOIN users ON users.id = children.user_id").
		Where("stories.id = ? AND users.firebase_uid = ?", storyID, uid).
		First(&story).Error
	return story, err
}

func findOwnedJob(db *gorm.DB, uid string, jobID uint) (models.StoryJob, error) {
	var job models.StoryJob
	err := db.Select("story_jobs.*").
		Joins("JOIN children ON children.id = story_jobs.child_id").
		Joins("JOIN users ON users.id = children.user_id").
		Where("story_jobs.id = ? AND users.firebase_uid = ?", jobID, uid).
		First(&job).Error
	return job, err
}

func lookupFailed(c *gin.Context, err error, notFoundMessage string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": notFoundMessage})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
}
