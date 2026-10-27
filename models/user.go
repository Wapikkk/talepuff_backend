package models

import "time"

// User hanya memetakan akun Firebase ke profil di backend. Email SENGAJA tidak disimpan: tidak ada
// fitur yang membacanya, dan salinannya menjadi usang saat pengguna mengganti email di Firebase.
// Kolom `email` lama (bila ada di database yang sudah berjalan) dibiarkan dan tidak lagi dipakai.
type User struct {
	ID          uint      `gorm:"primaryKey"`
	FirebaseUID string    `gorm:"uniqueIndex;not null" json:"firebase_uid"`
	CreatedAt   time.Time `json:"created_at"`
}
