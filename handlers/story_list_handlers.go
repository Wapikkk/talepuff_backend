package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultStoryPageSize = 30
	maxStoryPageSize     = 100
	maxSearchRunes       = 100
)

const storySummaryColumns = "id, created_at, child_id, character_name, theme, mood, language, length_minutes, title, audio_url, voice_tone, is_favorite, play_count"

func likePattern(query string) string {
	escaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + escaper.Replace(query) + "%"
}

func intQuery(c *gin.Context, name string, fallback, min, max int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}

func ListStories(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		childID, err := strconv.ParseUint(c.Query("child_id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "child_id is required"})
			return
		}
		tab := c.DefaultQuery("tab", "recent")
		if tab != "recent" && tab != "favorites" && tab != "popular" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tab must be one of: recent, favorites, popular"})
			return
		}
		search := strings.TrimSpace(c.Query("q"))
		if utf8.RuneCountInString(search) > maxSearchRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "q is too long"})
			return
		}
		limit, ok := intQuery(c, "limit", defaultStoryPageSize, 1, maxStoryPageSize)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		offset, ok := intQuery(c, "offset", 0, 0, 1_000_000)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset is invalid"})
			return
		}

		child, err := findOwnedChild(db, auth.IdentityFrom(c).UID, uint(childID))
		if err != nil {
			lookupFailed(c, err, "Child not found")
			return
		}

		query := db.Model(&models.Story{}).Select(storySummaryColumns).Where("child_id = ?", child.ID)
		switch tab {
		case "favorites":
			query = query.Where("is_favorite = ?", true).Order("created_at DESC, id DESC")
		case "popular":
			query = query.Where("play_count > 0").Order("play_count DESC, created_at DESC, id DESC")
		default:
			query = query.Order("created_at DESC, id DESC")
		}
		if search != "" {
			query = query.Where(`title ILIKE ? ESCAPE '\'`, likePattern(search))
		}

		var stories []models.Story
		if err := query.Limit(limit + 1).Offset(offset).Find(&stories).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		hasMore := len(stories) > limit
		if hasMore {
			stories = stories[:limit]
		}

		items := make([]dto.StoryDTO, 0, len(stories))
		for _, story := range stories {
			items = append(items, storyDTO(story, false))
		}
		c.JSON(http.StatusOK, dto.StoryListResponse{Stories: items, HasMore: hasMore})
	}
}

func GetStory(db *gorm.DB) gin.HandlerFunc {
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
		c.JSON(http.StatusOK, gin.H{"data": storyDTO(story, true)})
	}
}

func SetStoryFavorite(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story id"})
			return
		}
		var input dto.SetFavoriteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "favorite (true or false) is required"})
			return
		}
		story, err := findOwnedStory(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Story not found")
			return
		}

		if err := db.Model(&models.Story{}).Where("id = ?", story.ID).Update("is_favorite", *input.Favorite).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update database"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"is_favorite": *input.Favorite})
	}
}
