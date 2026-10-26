package model

import "time"

// UserCourseTraining 用户课程训练记录
type UserCourseTraining struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	UserID         uint       `json:"user_id" gorm:"not null;index"`
	CourseID       uint       `json:"course_id" gorm:"not null;index"`
	TrainingStatus string     `json:"training_status" gorm:"size:20;not null;default:'not_started';index"`
	TrainingCount  int        `json:"training_count" gorm:"not null;default:0"`
	LastTrainedAt  *time.Time `json:"last_trained_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TrainingStatus 训练状态常量
const (
	TrainingStatusNotStarted = "not_started"
	TrainingStatusInProgress = "in_progress"
	TrainingStatusCompleted  = "completed"
)

// UpdateTrainingStatusRequest 更新训练状态请求
type UpdateTrainingStatusRequest struct {
	TrainingStatus string `json:"training_status" binding:"required,oneof=not_started in_progress completed"`
}

// UserCourseTrainingResponse 用户课程训练记录响应
type UserCourseTrainingResponse struct {
	TrainingStatus string     `json:"training_status"`
	TrainingCount  int        `json:"training_count"`
	LastTrainedAt  *time.Time `json:"last_trained_at"`
}
