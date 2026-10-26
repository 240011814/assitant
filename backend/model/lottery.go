package model

import (
	"time"
)

// LotteryActivity 抽奖活动实体
type LotteryActivity struct {
	ID                  uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                string    `gorm:"size:100;not null" json:"name"`
	Description         string    `gorm:"type:text" json:"description"`
	StartTime           time.Time `gorm:"not null" json:"startTime"`
	EndTime             time.Time `gorm:"not null" json:"endTime"`
	Status              int       `gorm:"default:0" json:"status"`   // 0-未开始, 1-进行中, 2-已结束
	DrawMode            int       `gorm:"default:0" json:"drawMode"` // 0-转盘, 1-原神抽卡
	MaxParticipants     int       `gorm:"default:0" json:"maxParticipants"`
	CurrentParticipants int       `gorm:"default:0" json:"currentParticipants"`
	DailyLimit          int       `gorm:"default:0" json:"dailyLimit"`
	TotalLimit          int       `gorm:"default:0" json:"totalLimit"`
	CreatedBy           uint      `gorm:"not null" json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (LotteryActivity) TableName() string {
	return "lottery_activity"
}

// LotteryPrize 奖品实体
type LotteryPrize struct {
	ID                 uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ActivityID         uint      `gorm:"not null;index" json:"activityId"`
	Name               string    `gorm:"size:100;not null" json:"name"`
	Description        string    `gorm:"type:text" json:"description"`
	ImageURL           string    `gorm:"size:500" json:"imageUrl"`
	PrizeType          int       `gorm:"default:0" json:"prizeType"`  // 0-实物, 1-虚拟
	PrizeLevel         int       `gorm:"default:0" json:"prizeLevel"` // 0-未设置, 1-特等奖, 2-一等奖, 3-二等奖, 4-三等奖
	PrizeValue         float64   `gorm:"type:decimal(10,2);default:0" json:"prizeValue"`
	TotalCount         int       `gorm:"default:0" json:"totalCount"`
	RemainingCount     int       `gorm:"default:0" json:"remainingCount"`
	Probability        float64   `gorm:"type:decimal(5,4);default:0" json:"probability"`
	DisplayProbability float64   `gorm:"type:decimal(5,4);default:0" json:"displayProbability"`
	SortOrder          int       `gorm:"default:0" json:"sortOrder"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (LotteryPrize) TableName() string {
	return "lottery_prize"
}

// LotteryRecord 抽奖记录实体
type LotteryRecord struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ActivityID uint      `gorm:"not null;index" json:"activityId"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	UserName   string    `gorm:"size:100" json:"userName"`
	PrizeID    *uint     `json:"prizeId"` // NULL-未中奖
	PrizeName  string    `gorm:"size:100" json:"prizeName"`
	IsWinner   bool      `gorm:"default:false" json:"isWinner"`
	CreatedAt  time.Time `json:"createdAt"`
}

// TableName 指定表名
func (LotteryRecord) TableName() string {
	return "lottery_record"
}

// ==================== 请求结构体 ====================

// CreateLotteryActivityRequest 创建抽奖活动请求
type CreateLotteryActivityRequest struct {
	Name            string    `json:"name" binding:"required"`
	Description     string    `json:"description"`
	StartTime       time.Time `json:"startTime" binding:"required"`
	EndTime         time.Time `json:"endTime" binding:"required"`
	DrawMode        int       `json:"drawMode"`
	MaxParticipants int       `json:"maxParticipants"`
	DailyLimit      int       `json:"dailyLimit"`
	TotalLimit      int       `json:"totalLimit"`
}

// UpdateLotteryActivityRequest 更新抽奖活动请求
type UpdateLotteryActivityRequest struct {
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	Status          *int      `json:"status"`
	DrawMode        *int      `json:"drawMode"`
	MaxParticipants *int      `json:"maxParticipants"`
	DailyLimit      *int      `json:"dailyLimit"`
	TotalLimit      *int      `json:"totalLimit"`
}

// CreateLotteryPrizeRequest 创建奖品请求
type CreateLotteryPrizeRequest struct {
	Name               string  `json:"name" binding:"required"`
	Description        string  `json:"description"`
	ImageURL           string  `json:"imageUrl"`
	PrizeType          int     `json:"prizeType"`
	PrizeLevel         int     `json:"prizeLevel"`
	PrizeValue         float64 `json:"prizeValue"`
	TotalCount         int     `json:"totalCount" binding:"required"`
	Probability        float64 `json:"probability"`
	DisplayProbability float64 `json:"displayProbability"`
	SortOrder          int     `json:"sortOrder"`
}

// UpdateLotteryPrizeRequest 更新奖品请求
type UpdateLotteryPrizeRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	ImageURL           string   `json:"imageUrl"`
	PrizeType          *int     `json:"prizeType"`
	PrizeLevel         *int     `json:"prizeLevel"`
	PrizeValue         *float64 `json:"prizeValue"`
	TotalCount         *int     `json:"totalCount"`
	Probability        *float64 `json:"probability"`
	DisplayProbability *float64 `json:"displayProbability"`
	SortOrder          *int     `json:"sortOrder"`
}

// DrawLotteryRequest 抽奖请求
type DrawLotteryRequest struct {
	UserName string `json:"userName" binding:"required"`
}

// ==================== 响应结构体 ====================

// LotteryActivityResponse 抽奖活动响应
type LotteryActivityResponse struct {
	LotteryActivity
	PrizeCount int `json:"prizeCount"`
}

// LotteryRecordResponse 抽奖记录响应
type LotteryRecordResponse struct {
	LotteryRecord
	UserName     string `json:"userName"`
	ActivityName string `json:"activityName"`
}

// LotteryWinnerResponse 中奖名单响应
type LotteryWinnerResponse struct {
	ID           uint      `json:"id"`
	ActivityID   uint      `json:"activityId"`
	ActivityName string    `json:"activityName"`
	UserID       uint      `json:"userId"`
	UserName     string    `json:"userName"`
	PrizeName    string    `json:"prizeName"`
	PrizeType    int       `json:"prizeType"`
	PrizeValue   float64   `json:"prizeValue"`
	CreatedAt    time.Time `json:"createdAt"`
}
