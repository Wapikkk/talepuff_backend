package main

import (
	"context"
	"fmt"
	"log"
	"talepuff_backend/auth"
	"talepuff_backend/handlers"
	"talepuff_backend/models"
	"talepuff_backend/router"

	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Gagal memuat file .env (salin .env.example menjadi .env lalu isi nilainya): ", err)
	}

	dsn := os.Getenv("DB_URL")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatal("Gagal koneksi database:", err)
	}

	fmt.Println("Running Database Migration...")
	if err := db.AutoMigrate(
		&models.User{},
		&models.Child{},
		&models.Story{},
		&models.StoryJob{},
		&models.Feedback{},
	); err != nil {
		log.Fatal("Migrasi database gagal: ", err)
	}

	handlers.FailInterruptedJobs(db)

	verifier, err := auth.NewFirebaseVerifier(context.Background(), os.Getenv("FIREBASE_PROJECT_ID"))
	if err != nil {
		log.Fatal("Gagal menyiapkan verifikasi token Firebase: ", err)
	}

	r := router.New(db, verifier)

	fmt.Println("Server is running on port 8080...")
	os.MkdirAll("./uploads/profiles", os.ModePerm)
	r.Run("0.0.0.0:8080")
}
