package dto

type RegisterRequest struct {
	FirebaseUID string   `json:"firebase_uid"`
	Email       string   `json:"email" binding:"required"`
	ChildName   string   `json:"child_name" binding:"required"`
	Age         int      `json:"age"`
	Gender      string   `json:"gender"`
	Interests   []string `json:"interests"`
}

type FeedbackRequest struct {
	Category   string `json:"category" binding:"required"`
	Message    string `json:"message" binding:"required"`
	AppVersion string `json:"app_version"`
}

type UpdateEmailRequest struct {
	Email string `json:"email" binding:"required"`
}
