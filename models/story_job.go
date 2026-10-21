package models

import "gorm.io/gorm"

const (
	JobQueued        = "queued"
	JobWritingStory  = "writing_story"
	JobCreatingVoice = "creating_voice"
	JobDone          = "done"
	JobFailed        = "failed"
)

type StoryJob struct {
	gorm.Model
	ChildID      uint   `gorm:"not null;uniqueIndex:idx_one_active_job_per_child,where:status <> 'done' AND status <> 'failed'"`
	Status       string `gorm:"not null;default:queued;index"`
	StoryID      *uint
	ErrorCode    string
	IdeaDropped  bool `gorm:"not null;default:false"`
	NameReplaced bool `gorm:"not null;default:false"`
}
