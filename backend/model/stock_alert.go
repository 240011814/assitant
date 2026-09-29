package model

import "time"

// 预警规则类型
const (
	AlertRulePriceAbove     = "price_above"      // 收盘价 >= 阈值
	AlertRulePriceBelow     = "price_below"      // 收盘价 <= 阈值
	AlertRuleChangePctAbove = "change_pct_above" // 当日涨跌幅 >= 阈值(百分比)
	AlertRuleChangePctBelow = "change_pct_below" // 当日涨跌幅 <= 阈值(百分比)
)

var AlertRuleTypes = map[string]bool{
	AlertRulePriceAbove:     true,
	AlertRulePriceBelow:     true,
	AlertRuleChangePctAbove: true,
	AlertRuleChangePctBelow: true,
}

// StockAlertRule 自选股预警规则 (按最新交易日收盘数据评估)
type StockAlertRule struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	UserID             uint       `json:"user_id"`
	Code               string     `json:"code"`
	Name               string     `json:"name"`
	RuleType           string     `json:"rule_type"`
	Threshold          float64    `json:"threshold"`
	Enabled            bool       `json:"enabled"`
	LastTriggeredAt    *time.Time `json:"last_triggered_at"`
	LastTriggeredValue *float64   `json:"last_triggered_value"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (StockAlertRule) TableName() string {
	return "stock_alert_rules"
}

// CreateStockAlertRuleRequest 新建预警规则
type CreateStockAlertRuleRequest struct {
	Code      string  `json:"code" binding:"required"`
	Name      string  `json:"name"`
	RuleType  string  `json:"rule_type" binding:"required"`
	Threshold float64 `json:"threshold"`
}

// UpdateStockAlertRuleRequest 更新预警规则 ( nil 字段不更新)
type UpdateStockAlertRuleRequest struct {
	Name      *string  `json:"name"`
	RuleType  *string  `json:"rule_type"`
	Threshold *float64 `json:"threshold"`
	Enabled   *bool    `json:"enabled"`
}

// StockAlertTrigger 触发记录 (推送内容 + 前端展示)
type StockAlertTrigger struct {
	Rule      StockAlertRule `json:"rule"`
	Code      string         `json:"code"`
	StockName string         `json:"stock_name"`
	Price     float64        `json:"price"`
	ChangePct float64        `json:"change_pct"`
	TradeDate string         `json:"trade_date"`
	Message   string         `json:"message"`
}
