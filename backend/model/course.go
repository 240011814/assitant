package model

import "time"

// Course 课程包
type Course struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id"`
	Title       string    `json:"title" gorm:"size:200;not null"`
	Description string    `json:"description" gorm:"size:500"`
	Tags        string    `json:"tags" gorm:"size:500;default:''"`
	IsPublic    bool      `json:"is_public" gorm:"default:false"`
	ItemCount   int       `json:"item_count" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CourseItem 课程条目（英语句子+翻译）
type CourseItem struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	CourseID           uint      `json:"course_id" gorm:"index;not null"`
	EnglishSentence    string    `json:"english_sentence" gorm:"type:text;not null"`
	ChineseTranslation string    `json:"chinese_translation" gorm:"type:text"`
	SortOrder          int       `json:"sort_order" gorm:"default:0"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CreateCourseRequest 创建课程包请求
type CreateCourseRequest struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IsPublic    bool     `json:"is_public"`
}

// UpdateCourseRequest 更新课程包请求
type UpdateCourseRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IsPublic    *bool    `json:"is_public"`
}

// CreateCourseItemRequest 创建课程条目请求
type CreateCourseItemRequest struct {
	EnglishSentence    string `json:"english_sentence" binding:"required"`
	ChineseTranslation string `json:"chinese_translation"`
	SortOrder          int    `json:"sort_order"`
}

// UpdateCourseItemRequest 更新课程条目请求
type UpdateCourseItemRequest struct {
	EnglishSentence    string `json:"english_sentence"`
	ChineseTranslation string `json:"chinese_translation"`
	SortOrder          *int   `json:"sort_order"`
}

// BatchCreateCourseItemsRequest 批量创建课程条目请求
type BatchCreateCourseItemsRequest struct {
	Items []CreateCourseItemRequest `json:"items" binding:"required,min=1"`
}

// BatchDeleteCourseItemsRequest 批量删除课程条目请求
type BatchDeleteCourseItemsRequest struct {
	Ids []uint `json:"ids" binding:"required,min=1"`
}

// CourseResponse 课程包响应（包含条目数量）
type CourseResponse struct {
	Course
	Items []CourseItem `json:"items,omitempty"`
}
