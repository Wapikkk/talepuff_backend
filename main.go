package main

import (
	"fmt"
	"log"
	"talepuff_backend/handlers"
	"talepuff_backend/models"

	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dsn := os.Getenv("DB_URL")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Gagal koneksi database:", err)
	}

	fmt.Println("Running Database Migration...")
	db.AutoMigrate(&models.User{}, &models.Child{})

	r := gin.Default()

	api := r.Group("/api")
	{
		api.POST("/register", handlers.RegisterUser(db))
		api.GET("/child/:uid", handlers.GetChildInfo(db))
	}

	fmt.Println("Server is running on port 8080...")
	r.Run("0.0.0.0:8080")
}
