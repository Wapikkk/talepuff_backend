package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"talepuff_backend/auth"
	"talepuff_backend/dto"
	"talepuff_backend/models"
	"talepuff_backend/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ErrAIUnavailable = "ai_unavailable"
	ErrAIFailed      = "ai_failed"
	ErrVoiceFailed   = "voice_failed"
	ErrSaveFailed    = "save_failed"
	ErrInterrupted   = "interrupted"
)

var (
	jobSlots = make(chan struct{}, 1)
	jobsWG   sync.WaitGroup
)

func StartStoryJob(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.GenerateStoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		language, err := validation.Language(req.Language)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.Language = language

		voiceTone, err := validation.VoiceTone(req.VoiceTone)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.VoiceTone = voiceTone

		if _, err := findOwnedChild(db, auth.IdentityFrom(c).UID, req.ChildID); err != nil {
			lookupFailed(c, err, "Child profile not found")
			return
		}

		job := models.StoryJob{ChildID: req.ChildID, Status: models.JobQueued}
		if err := db.Create(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				var active models.StoryJob
				db.Where("child_id = ? AND status NOT IN ?", req.ChildID, []string{models.JobDone, models.JobFailed}).First(&active)
				c.JSON(http.StatusConflict, gin.H{"error": "A story is already being created", "job_id": active.ID})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start the story"})
			return
		}

		jobsWG.Add(1)
		go runStoryJob(db, job.ID, req)

		c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": job.Status})
	}
}

func GetStoryJob(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job id"})
			return
		}
		job, err := findOwnedJob(db, auth.IdentityFrom(c).UID, uint(id))
		if err != nil {
			lookupFailed(c, err, "Job not found")
			return
		}

		result := dto.StoryJobDTO{ID: job.ID, Status: job.Status, ErrorCode: job.ErrorCode, IdeaDropped: job.IdeaDropped, NameReplaced: job.NameReplaced}
		if job.StoryID != nil {
			var story models.Story
			if err := db.First(&story, *job.StoryID).Error; err == nil {
				full := storyDTO(story, true)
				result.Story = &full
			}
		}
		c.JSON(http.StatusOK, result)
	}
}

func FailInterruptedJobs(db *gorm.DB) {
	err := db.Model(&models.StoryJob{}).
		Where("status NOT IN ?", []string{models.JobDone, models.JobFailed}).
		Updates(map[string]any{"status": models.JobFailed, "error_code": ErrInterrupted}).Error
	if err != nil {
		log.Printf("could not close interrupted story jobs: %v", err)
	}
}

func runStoryJob(db *gorm.DB, jobID uint, req dto.GenerateStoryRequest) {
	defer jobsWG.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("story job %d crashed: %v", jobID, recovered)
			failJob(db, jobID, ErrAIFailed)
		}
	}()

	jobSlots <- struct{}{}
	defer func() { <-jobSlots }()

	updateJob(db, jobID, map[string]any{"status": models.JobWritingStory})

	var aiResult dto.PythonResponse
	if err := callAI("/generate_story", req, &aiResult); err != nil {
		log.Printf("story job %d: writing failed: %v", jobID, err)
		failJob(db, jobID, aiErrorCode(err))
		return
	}

	story := models.Story{
		ChildID:       req.ChildID,
		CharacterName: req.CharacterName,
		Theme:         req.Theme,
		Mood:          truncateRunes(req.Mood, maxStoredMoodRunes),
		Description:   truncateRunes(req.Description, maxStoredDescriptionRunes),
		Language:      req.Language,
		LengthMinutes: req.LengthMinutes,
		Title:         aiResult.Title,
		Content:       aiResult.Response,
		VoiceTone:     req.VoiceTone,
	}
	if err := db.Create(&story).Error; err != nil {
		log.Printf("story job %d: saving the story failed: %v", jobID, err)
		failJob(db, jobID, ErrSaveFailed)
		return
	}

	updateJob(db, jobID, map[string]any{
		"status":        models.JobCreatingVoice,
		"story_id":      story.ID,
		"idea_dropped":  containsFlag(aiResult.Flags, "idea_dropped"),
		"name_replaced": containsFlag(aiResult.Flags, "name_replaced"),
	})

	var audio dto.PythonAudioResponse
	payload := map[string]string{"story_text": story.Content, "voice_tone": req.VoiceTone}
	if err := callAI("/generate_audio", payload, &audio); err != nil {
		log.Printf("story job %d: creating the voice failed: %v", jobID, err)
		failJob(db, jobID, ErrVoiceFailed)
		return
	}
	if err := db.Model(&models.Story{}).Where("id = ?", story.ID).
		Updates(map[string]any{"audio_url": audio.AudioURL, "sentences": encodeSentences(audio.Sentences)}).Error; err != nil {
		log.Printf("story job %d: saving the voice failed: %v", jobID, err)
		failJob(db, jobID, ErrSaveFailed)
		return
	}

	updateJob(db, jobID, map[string]any{"status": models.JobDone})
}

func aiErrorCode(err error) string {
	var statusErr *aiStatusError
	if errors.As(err, &statusErr) || errors.Is(err, errAIDecode) {
		return ErrAIFailed
	}
	return ErrAIUnavailable
}

func containsFlag(flags []string, wanted string) bool {
	for _, flag := range flags {
		if flag == wanted {
			return true
		}
	}
	return false
}

func updateJob(db *gorm.DB, jobID uint, values map[string]any) {
	if err := db.Model(&models.StoryJob{}).Where("id = ?", jobID).Updates(values).Error; err != nil {
		log.Printf("story job %d: could not record %v: %v", jobID, values, err)
	}
}

func failJob(db *gorm.DB, jobID uint, code string) {
	updateJob(db, jobID, map[string]any{"status": models.JobFailed, "error_code": code})
}
