package dto

type RegisterRequest struct {
	FirebaseUID string   `json:"firebase_uid" binding:"required"`
	Email       string   `json:"email" binding:"required"`
	ChildName   string   `json:"child_name" binding:"required"`
	Age         int      `json:"age"`
	Gender      string   `json:"gender"`
	Interests   []string `json:"interests"`
}

type UpdateEmailRequest struct {
	Email string `json:"email" binding:"required"`
}
