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
		api.PUT("/child/update-name/:id", handlers.UpdateChildName(db))
		api.PUT("/user/update-email/:uid", handlers.UpdateUserEmail(db))
		api.POST("/story/generate", handlers.GenerateStory(db))
		api.POST("/audio/generate_audio", handlers.GeneratedAudio(db))
	}
	return r
}
