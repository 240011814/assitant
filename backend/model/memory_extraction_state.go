package model

import "time"

const (
	MemoryExtractionStatusPending    = "pending"
	MemoryExtractionStatusProcessing = "processing"
	MemoryExtractionStatusDone       = "done"
	MemoryExtractionStatusFailed     = "failed"
	MemoryExtractionStatusSkipped    = "skipped"
)

// MemoryExtractionState 会话级抽取进度
type MemoryExtractionState struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	HistoryID     uint       `json:"history_id" gorm:"uniqueIndex"`
	UserID        uint       `json:"user_id"`
	LastSortOrder int        `json:"last_sort_order"`
	Status        string     `json:"status" gorm:"size:20"`
	Error         string     `json:"error" gorm:"type:text"`
	ExtractedAt   *time.Time `json:"extracted_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (MemoryExtractionState) TableName() string {
	return "memory_extraction_states"
}
