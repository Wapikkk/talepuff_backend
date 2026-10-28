package router

import (
	"talepuff_backend/auth"
	"talepuff_backend/handlers"
	"talepuff_backend/totem"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// hub boleh nil: perintah totem lalu dijawab 503.
func New(db *gorm.DB, verifier auth.Verifier, hub totem.Hub) *gin.Engine {
	if hub == nil {
		hub = totem.Disabled{}
	}
	r := gin.Default()
	r.Static("/uploads", "./uploads")
	r.GET("/api/health", handlers.Health)

	api := r.Group("/api", auth.Middleware(verifier))
	{
		api.POST("/register", handlers.RegisterUser(db))
		api.GET("/child/:uid", handlers.GetChildInfo(db))
		api.POST("/child/upload-photo/:id", handlers.UploadChildPhotoHandler(db))
		api.DELETE("/child/:id/photo", handlers.DeleteChildPhoto(db))
		api.PUT("/child/:id/mascot", handlers.UpdateChildMascot(db))
		api.PUT("/child/update-name/:id", handlers.UpdateChildName(db))
		api.POST("/story/generate", handlers.StartStoryJob(db))
		api.GET("/story/jobs/:id", handlers.GetStoryJob(db))
		api.GET("/stories", handlers.ListStories(db))
		api.GET("/stories/:id", handlers.GetStory(db))
		api.PUT("/stories/:id/favorite", handlers.SetStoryFavorite(db))
		api.DELETE("/stories/:id", handlers.DeleteStory(db))
		api.POST("/audio/generate_audio", handlers.GeneratedAudio(db))
		api.POST("/feedback", handlers.SubmitFeedback(db))
		api.GET("/totem", handlers.GetTotem(db, hub))
		api.POST("/totem/pair", handlers.PairTotem(db, hub))
		api.DELETE("/totem", handlers.UnpairTotem(db, hub))
		api.POST("/totem/play", handlers.PlayOnTotem(db, hub))
		api.POST("/totem/command", handlers.CommandTotem(db, hub))
	}
	return r
}
