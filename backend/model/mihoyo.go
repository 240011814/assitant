package model

import (
	"time"
)

// 米哈游 (原神) 每日签到
//
// 登录态说明: 扫码登录成功后保存 stoken/cookie_token/ltoken,
// 这些字段等同于账号登录态, 全部带 json:"-" 永不随接口下发。
// cookie_token/ltoken 过期时用 stoken 自动续期, 无需重新扫码。

// TaskNameMihoyoGenshinCheckin 原神每日签到系统任务 (job_definitions.task_name)
const TaskNameMihoyoGenshinCheckin = "mihoyo.genshin_checkin"

// ===== 签到结果状态 =====
const (
	MihoyoCheckinStatusSuccess = "success" // 签到成功
	MihoyoCheckinStatusAlready = "already" // 今天已经签到过
	MihoyoCheckinStatusBlocked = "blocked" // 触发 geetest 验证码被拦截
	MihoyoCheckinStatusFailed  = "failed"  // 接口报错
	MihoyoCheckinStatusNoRole  = "no_role" // 没有原神(国服)角色
)

// MihoyoCredential 用户绑定的米哈游登录凭据 (扫码登录写入, 每用户一条)
type MihoyoCredential struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"not null;uniqueIndex:uk_mihoyo_user" json:"userId"`
	MihoyoUID   string    `gorm:"column:mihoyo_uid;size:32" json:"mihoyoUid"` // 米游社 aid
	MihoyoMid   string    `gorm:"column:mihoyo_mid;size:64" json:"mihoyoMid"`
	Nickname    string    `gorm:"size:64" json:"nickname"`
	CookieToken string    `gorm:"type:text" json:"-"`
	Ltoken      string    `gorm:"type:text" json:"-"`
	Stoken      string    `gorm:"type:text" json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (MihoyoCredential) TableName() string {
	return "mihoyo_credentials"
}

// ===== 接口 DTO =====

// MihoyoQRStartData 开始扫码登录: 返回二维码内容与轮询凭证
type MihoyoQRStartData struct {
	Ticket    string `json:"ticket"`
	QRUrl     string `json:"qrUrl"`
	ExpiresIn int    `json:"expiresIn"` // 二维码有效期(秒)
}

// MihoyoQRStatusData 扫码状态 (Confirmed 时后端已保存凭据)
type MihoyoQRStatusData struct {
	Status   string `json:"status"` // Created / Scanned / Confirmed / Expired
	Message  string `json:"message,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	UID      string `json:"uid,omitempty"`
	Bound    bool   `json:"bound"`
}

// MihoyoBindStatusData 绑定状态
type MihoyoBindStatusData struct {
	Bound    bool   `json:"bound"`
	Nickname string `json:"nickname"`
	UID      string `json:"uid"`
	Mid      string `json:"mid"`
}

// MihoyoCheckinResult 单个角色的签到结果
type MihoyoCheckinResult struct {
	GameUID  string `json:"gameUid"`
	Nickname string `json:"nickname"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

// MihoyoCheckinStatusRole 签到信息查询单角色状态 (查询不触发签到)
type MihoyoCheckinStatusRole struct {
	GameUID      string `json:"gameUid"`
	Nickname     string `json:"nickname"`
	Signed       bool   `json:"signed"` // 今日已签
	TotalSignDay int    `json:"totalSignDay"`
	AwardName    string `json:"awardName"` // 今日奖励
	AwardCnt     string `json:"awardCnt"`
	AwardIcon    string `json:"awardIcon"`
}

// MihoyoCheckinStatusData 签到信息查询结果
type MihoyoCheckinStatusData struct {
	Bound bool                      `json:"bound"`
	Roles []MihoyoCheckinStatusRole `json:"roles"`
}
