package router

import (
	"talepuff_backend/auth"
	"talepuff_backend/handlers"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(db *gorm.DB, verifier auth.Verifier) *gin.Engine {
	r := gin.Default()
	r.Static("/uploads", "./uploads")

	api := r.Group("/api", auth.Middleware(verifier))
	{
		api.POST("/register", handlers.RegisterUser(db))
		api.GET("/child/:uid", handlers.GetChildInfo(db))
		api.POST("/child/upload-photo/:id", handlers.UploadChildPhotoHandler(db))
		api.DELETE("/child/:id/photo", handlers.DeleteChildPhoto(db))
		api.PUT("/child/:id/mascot", handlers.UpdateChildMascot(db))
		api.PUT("/child/update-name/:id", handlers.UpdateChildName(db))
		api.PUT("/user/update-email/:uid", handlers.UpdateUserEmail(db))
		api.POST("/story/generate", handlers.GenerateStory(db))
		api.GET("/stories", handlers.ListStories(db))
		api.GET("/stories/:id", handlers.GetStory(db))
		api.PUT("/stories/:id/favorite", handlers.SetStoryFavorite(db))
		api.DELETE("/stories/:id", handlers.DeleteStory(db))
		api.POST("/audio/generate_audio", handlers.GeneratedAudio(db))
		api.POST("/feedback", handlers.SubmitFeedback(db))
	}
	return r
}
