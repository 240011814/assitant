package service

import (
	iface "backend/interface"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"backend/model"

	"gorm.io/gorm"
)

// StockAlertService 自选股预警: 规则 CRUD + 按最新交易日收盘数据评估 + Telegram/邮件推送
// 评估基于收盘数据 (baostock 无盘中实时行情), 定时任务在每日行情同步后运行
type StockAlertService struct {
	notifiers []iface.Notifier
}

func NewStockAlertService(notifiers ...iface.Notifier) *StockAlertService {
	return &StockAlertService{notifiers: notifiers}
}

// ListAlerts 用户自己的预警规则列表
func (s *StockAlertService) ListAlerts(userID uint) ([]model.StockAlertRule, error) {
	var list []model.StockAlertRule
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&list).Error
	return list, err
}

// CreateAlert 新建规则
func (s *StockAlertService) CreateAlert(userID uint, req model.CreateStockAlertRuleRequest) (*model.StockAlertRule, error) {
	if !model.AlertRuleTypes[req.RuleType] {
		return nil, errors.New("不支持的预警类型")
	}
	if err := validateAlertCode(req.Code); err != nil {
		return nil, err
	}
	rule := model.StockAlertRule{
		UserID:    userID,
		Code:      req.Code,
		Name:      req.Name,
		RuleType:  req.RuleType,
		Threshold: req.Threshold,
		Enabled:   true,
	}
	if err := DB.Create(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// UpdateAlert 更新规则 (仅本人)
func (s *StockAlertService) UpdateAlert(userID, id uint, req model.UpdateStockAlertRuleRequest) error {
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.RuleType != nil {
		if !model.AlertRuleTypes[*req.RuleType] {
			return errors.New("不支持的预警类型")
		}
		updates["rule_type"] = *req.RuleType
	}
	if req.Threshold != nil {
		updates["threshold"] = *req.Threshold
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		return nil
	}
	result := DB.Model(&model.StockAlertRule{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("规则不存在")
	}
	return nil
}

// DeleteAlert 删除规则 (仅本人)
func (s *StockAlertService) DeleteAlert(userID, id uint) error {
	result := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.StockAlertRule{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("规则不存在")
	}
	return nil
}

func validateAlertCode(code string) error {
	if len(code) != 9 || !strings.HasPrefix(code, "sh.") && !strings.HasPrefix(code, "sz.") && !strings.HasPrefix(code, "bj.") {
		return errors.New("股票代码格式错误, 应为 sh.XXXXXX / sz.XXXXXX / bj.XXXXXX")
	}
	return nil
}

// latestQuote 某只证券的最新日K (评估用)
type latestQuote struct {
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	Close     *float64  `gorm:"column:close"`
	ChangePct *float64  `gorm:"column:change_pct"`
	TradeDate time.Time `gorm:"column:trade_date"`
}

// EvaluateAll 评估所有启用的预警规则, 触发并推送。返回触发条数
// 防重: 同一规则仅在"最新行情交易日"晚于上次触发日时评估 (行情未更新/停牌/周末不重复推送)
func (s *StockAlertService) EvaluateAll() (int, error) {
	var rules []model.StockAlertRule
	if err := DB.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return 0, err
	}
	if len(rules) == 0 {
		return 0, nil
	}

	codes := make([]string, 0, len(rules))
	seen := make(map[string]bool)
	for _, r := range rules {
		if !seen[r.Code] {
			seen[r.Code] = true
			codes = append(codes, r.Code)
		}
	}

	// 批量取每个 code 的最新日K + 股票名
	var quotes []latestQuote
	err := DB.Table("stock_daily AS sd").
		Select("sd.code, si.name, sd.close, sd.change_pct, sd.trade_date").
		Joins("JOIN (SELECT code, MAX(trade_date) AS md FROM stock_daily WHERE frequency = 'daily' AND code IN ? GROUP BY code) t ON t.code = sd.code AND t.md = sd.trade_date", codes).
		Joins("LEFT JOIN stock_info si ON si.code = sd.code").
		Where("sd.frequency = 'daily'").
		Find(&quotes).Error
	if err != nil {
		return 0, err
	}
	quoteMap := make(map[string]latestQuote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Code] = q
	}

	now := time.Now()
	triggered := 0
	perUser := make(map[uint][]model.StockAlertTrigger)

	for i := range rules {
		rule := &rules[i]
		quote, ok := quoteMap[rule.Code]
		if !ok || quote.Close == nil {
			continue
		}

		// 行情交易日必须晚于上次触发日, 否则跳过 (同一天的行情只触发一次)
		if rule.LastTriggeredAt != nil {
			last := time.Date(rule.LastTriggeredAt.Year(), rule.LastTriggeredAt.Month(), rule.LastTriggeredAt.Day(), 0, 0, 0, 0, time.Local)
			quoteDay := time.Date(quote.TradeDate.Year(), quote.TradeDate.Month(), quote.TradeDate.Day(), 0, 0, 0, 0, time.Local)
			if !quoteDay.After(last) {
				continue
			}
		}

		hit, message := evaluateRule(rule, quote)
		if !hit {
			continue
		}

		value := *quote.Close
		if strings.HasSuffix(rule.RuleType, "pct") && quote.ChangePct != nil {
			value = *quote.ChangePct
		}
		dbUpdates := map[string]interface{}{
			"last_triggered_at":    now,
			"last_triggered_value": value,
		}
		if err := DB.Model(&model.StockAlertRule{}).Where("id = ?", rule.ID).Updates(dbUpdates).Error; err != nil {
			log.Printf("[StockAlert] 更新触发状态失败 rule=%d: %v", rule.ID, err)
			continue
		}
		rule.LastTriggeredAt = &now

		perUser[rule.UserID] = append(perUser[rule.UserID], model.StockAlertTrigger{
			Rule:      *rule,
			Code:      rule.Code,
			StockName: quote.Name,
			Price:     derefOrZero(quote.Close),
			ChangePct: derefOrZero(quote.ChangePct),
			TradeDate: quote.TradeDate.Format("2006-01-02"),
			Message:   message,
		})
		triggered++
	}

	// 按用户推送
	for userID, triggers := range perUser {
		s.notifyUser(userID, triggers)
	}
	return triggered, nil
}

// evaluateRule 判断单条规则是否触发, 返回 (是否触发, 描述消息)
func evaluateRule(rule *model.StockAlertRule, quote latestQuote) (bool, string) {
	name := quote.Name
	if name == "" {
		name = rule.Code
	}
	price := derefOrZero(quote.Close)
	changePct := derefOrZero(quote.ChangePct)
	date := quote.TradeDate.Format("2006-01-02")

	switch rule.RuleType {
	case model.AlertRulePriceAbove:
		if price >= rule.Threshold {
			return true, fmt.Sprintf("【股票预警】%s (%s)\n%s 收盘价 %.2f 元, 已突破阈值 %.2f 元", name, rule.Code, date, price, rule.Threshold)
		}
	case model.AlertRulePriceBelow:
		if price <= rule.Threshold {
			return true, fmt.Sprintf("【股票预警】%s (%s)\n%s 收盘价 %.2f 元, 已跌破阈值 %.2f 元", name, rule.Code, date, price, rule.Threshold)
		}
	case model.AlertRuleChangePctAbove:
		if changePct >= rule.Threshold {
			return true, fmt.Sprintf("【股票预警】%s (%s)\n%s 涨幅 %+.2f%%, 达到阈值 %+.2f%%", name, rule.Code, date, changePct, rule.Threshold)
		}
	case model.AlertRuleChangePctBelow:
		if changePct <= rule.Threshold {
			return true, fmt.Sprintf("【股票预警】%s (%s)\n%s 跌幅 %+.2f%%, 达到阈值 %+.2f%%", name, rule.Code, date, changePct, rule.Threshold)
		}
	}
	return false, ""
}

// notifyUser 按用户配置的通知渠道推送 (渠道未配置/不可用时跳过)
func (s *StockAlertService) notifyUser(userID uint, triggers []model.StockAlertTrigger) {
	channels := s.getUserChannels(userID)
	if len(channels) == 0 {
		return
	}

	subject := fmt.Sprintf("股票预警触发 (%d 条)", len(triggers))
	var body strings.Builder
	for _, t := range triggers {
		body.WriteString(t.Message)
		body.WriteString("\n\n")
	}
	msg := iface.NotifyMessage{Subject: subject, Body: strings.TrimSpace(body.String())}

	var user model.User
	if err := DB.First(&user, userID).Error; err != nil {
		log.Printf("[StockAlert] 推送失败: 用户 %d 不存在", userID)
		return
	}

	for _, channel := range channels {
		var target string
		switch channel {
		case "email":
			target = user.Email
		case "telegram":
			if user.TelegramChatID == nil {
				continue
			}
			target = fmt.Sprintf("%d", *user.TelegramChatID)
		default:
			continue
		}
		if target == "" {
			continue
		}
		for _, n := range s.notifiers {
			if n.Name() != channel {
				continue
			}
			if err := n.Send(target, msg); err != nil {
				if !errors.Is(err, ErrBotNotStarted) {
					log.Printf("[StockAlert] 推送失败 user=%d channel=%s: %v", userID, channel, err)
				}
			}
		}
	}
}

// getUserChannels 读用户通知渠道偏好 (JSON 数组), 未配置默认 email
func (s *StockAlertService) getUserChannels(userID uint) []string {
	var pref model.UserPreference
	if err := DB.Where("user_id = ? AND pref_key = ?", userID, "notification_channels").First(&pref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{"email"}
		}
		return nil
	}
	var channels []string
	if err := json.Unmarshal(pref.PrefValue, &channels); err != nil {
		return nil
	}
	return channels
}

func derefOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
