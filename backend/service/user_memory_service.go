package service

import (
	"backend/model"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// MemoryExtractionConfig 画像/经历抽取配置(来自 system_config)
type MemoryExtractionConfig struct {
	Enabled         bool
	Model           string
	IdleMinutes     int
	MinUserMessages int
}

// UserMemoryService 用户画像与经历: 会话级抽取、画像合并、注入与 CRUD
type UserMemoryService struct {
	agentSvc *AIAgentService
	sysCfg   *SystemConfigService
	mu       sync.Mutex
}

func NewUserMemoryService(agentSvc *AIAgentService, sysCfg *SystemConfigService) *UserMemoryService {
	return &UserMemoryService{agentSvc: agentSvc, sysCfg: sysCfg}
}

func (s *UserMemoryService) getConfig() MemoryExtractionConfig {
	cfg := MemoryExtractionConfig{
		Enabled:         true,
		IdleMinutes:     15,
		MinUserMessages: 6,
	}
	if s.sysCfg == nil {
		return cfg
	}
	if v, err := s.sysCfg.GetValue("memory_extraction_enabled"); err == nil {
		cfg.Enabled = v != "false"
	}
	if v, err := s.sysCfg.GetValue("memory_extraction_model"); err == nil {
		cfg.Model = v
	}
	if v, err := s.sysCfg.GetValue("memory_session_idle_minutes"); err == nil {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.IdleMinutes = n
		}
	}
	if v, err := s.sysCfg.GetValue("memory_min_min_user_messages"); err == nil {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MinUserMessages = n
		}
	}
	return cfg
}

// ============ 画像 ============

func (s *UserMemoryService) GetOrCreatePortrait(userID uint) (*model.UserPortrait, error) {
	p := model.UserPortrait{UserID: userID}
	err := DB.Where("user_id = ?", userID).
		Attrs(model.UserPortrait{ExtractionEnabled: true, Dimensions: "{}", Tags: "[]"}).
		FirstOrCreate(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *UserMemoryService) GetPortrait(userID uint) (*model.UserPortrait, error) {
	var p model.UserPortrait
	if err := DB.Where("user_id = ?", userID).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// BuildProfilePrompt 构造注入对话的画像文本, 无画像/低置信/未启用时返回空串
func (s *UserMemoryService) BuildProfilePrompt(userID uint) string {
	p, err := s.GetPortrait(userID)
	if err != nil || p == nil || !p.ExtractionEnabled {
		return ""
	}

	var dims map[string]string
	_ = json.Unmarshal([]byte(p.Dimensions), &dims)
	var tags []string
	_ = json.Unmarshal([]byte(p.Tags), &tags)

	if strings.TrimSpace(p.Summary) == "" && len(dims) == 0 && len(tags) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## 用户画像（供参考，不要直接复述）\n")
	if strings.TrimSpace(p.Summary) != "" {
		b.WriteString(strings.TrimSpace(p.Summary) + "\n")
	}
	written := make(map[string]bool, len(dims))
	for _, k := range dimensionOrder {
		if v, ok := dims[k]; ok && strings.TrimSpace(v) != "" {
			b.WriteString(fmt.Sprintf("- %s：%s\n", dimensionLabel(k), strings.TrimSpace(v)))
			written[k] = true
		}
	}
	for k, v := range dims {
		if written[k] || strings.TrimSpace(v) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s：%s\n", k, strings.TrimSpace(v)))
	}
	if len(tags) > 0 {
		b.WriteString("标签：" + strings.Join(tags, "、") + "\n")
	}

	return b.String()
}

// dimensionOrder 固定画像维度顺序
var dimensionOrder = []string{
	"identity", "background", "goals", "skills", "projects",
	"learning_topics", "interests", "preferences", "habits",
	"communication_style", "language", "values", "risk_preference",
	"decision_style", "constraints",
}

var dimensionLabels = map[string]string{
	"identity":            "身份",
	"background":          "背景",
	"goals":               "目标",
	"skills":              "技能",
	"projects":            "项目",
	"learning_topics":     "学习主题",
	"interests":           "兴趣",
	"preferences":         "偏好",
	"habits":              "习惯",
	"communication_style": "沟通风格",
	"language":            "语言",
	"values":              "价值观",
	"risk_preference":     "风险偏好",
	"decision_style":      "决策风格",
	"constraints":         "约束",
}

func dimensionLabel(key string) string {
	if label, ok := dimensionLabels[key]; ok {
		return label
	}
	return key
}

func (s *UserMemoryService) UpdatePortrait(userID uint, req model.UpdateUserPortraitRequest) error {
	if _, err := s.GetOrCreatePortrait(userID); err != nil {
		return err
	}
	updates := map[string]interface{}{"is_user_edited": true}
	if req.Summary != nil {
		updates["summary"] = *req.Summary
	}
	if req.Dimensions != nil {
		b, _ := json.Marshal(*req.Dimensions)
		updates["dimensions"] = string(b)
	}
	if req.Tags != nil {
		b, _ := json.Marshal(*req.Tags)
		updates["tags"] = string(b)
	}
	if req.ExtractionEnabled != nil {
		updates["extraction_enabled"] = *req.ExtractionEnabled
	}
	return DB.Model(&model.UserPortrait{}).Where("user_id = ?", userID).Updates(updates).Error
}

// ============ 经历 CRUD ============

func (s *UserMemoryService) ListExperiences(userID uint, page, pageSize int, domain, keyword string) (int64, []model.UserExperience, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := DB.Model(&model.UserExperience{}).Where("user_id = ?", userID)
	if domain != "" {
		q = q.Where("domain = ?", domain)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("title LIKE ? OR content LIKE ? OR tags LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var list []model.UserExperience
	if err := q.Order("occurred_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return 0, nil, err
	}
	return total, list, nil
}

func (s *UserMemoryService) SearchExperiences(userID uint, query string, page, pageSize int) (int64, []model.UserExperience, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 10
	}
	q := DB.Model(&model.UserExperience{}).Where("user_id = ? AND memory_level <> ?", userID, model.MemoryLevelTemporary)
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("title LIKE ? OR content LIKE ? OR tags LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var list []model.UserExperience
	if err := q.Order("occurred_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return 0, nil, err
	}
	return total, list, nil
}

func (s *UserMemoryService) AddExperience(userID uint, exp *model.UserExperience) (*model.UserExperience, error) {
	exp.ID = 0
	exp.UserID = userID
	if exp.MemoryLevel == "" {
		exp.MemoryLevel = model.MemoryLevelLongTerm
	}
	if exp.Status == "" {
		exp.Status = model.ExperienceStatusUnknown
	}
	if exp.Importance < 1 || exp.Importance > 5 {
		exp.Importance = 3
	}
	exp.IsUserEdited = true
	if exp.OccurredAt == nil {
		now := time.Now()
		exp.OccurredAt = &now
	}
	if err := DB.Create(exp).Error; err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *UserMemoryService) CreateExperience(userID uint, req model.CreateUserExperienceRequest) (*model.UserExperience, error) {
	tags, _ := json.Marshal(req.Tags)
	exp := &model.UserExperience{
		UserID:      userID,
		Domain:      req.Domain,
		EventType:   req.EventType,
		Title:       req.Title,
		Content:     req.Content,
		TimeRange:   req.TimeRange,
		OccurredAt:  req.OccurredAt,
		Tags:        string(tags),
		MemoryLevel: req.MemoryLevel,
		Evidence:    req.Evidence,
		Status:      req.Status,
		Importance:  req.Importance,
	}
	return s.AddExperience(userID, exp)
}

func (s *UserMemoryService) UpdateExperience(userID, id uint, req model.UpdateUserExperienceRequest) error {
	updates := map[string]interface{}{"is_user_edited": true}
	if req.Domain != nil {
		updates["domain"] = *req.Domain
	}
	if req.EventType != nil {
		updates["event_type"] = *req.EventType
	}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.TimeRange != nil {
		updates["time_range"] = *req.TimeRange
	}
	if req.OccurredAt != nil {
		updates["occurred_at"] = *req.OccurredAt
	}
	if req.Tags != nil {
		b, _ := json.Marshal(*req.Tags)
		updates["tags"] = string(b)
	}
	if req.MemoryLevel != nil {
		updates["memory_level"] = *req.MemoryLevel
	}
	if req.Evidence != nil {
		updates["evidence"] = *req.Evidence
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Importance != nil {
		updates["importance"] = *req.Importance
	}
	res := DB.Model(&model.UserExperience{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *UserMemoryService) DeleteExperience(userID, id uint) error {
	res := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.UserExperience{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ============ 抽取 ============

type eligibleSession struct {
	HistoryID    uint      `gorm:"column:history_id"`
	UserID       uint      `gorm:"column:user_id"`
	Title        string    `gorm:"column:title"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UserMsgCount int       `gorm:"column:user_msg_count"`
	MaxSortOrder int       `gorm:"column:max_sort_order"`
	LastMsgAt    time.Time `gorm:"column:last_msg_at"`
}

type extractedExperience struct {
	Domain      string   `json:"domain"`
	EventType   string   `json:"event_type"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	TimeRange   string   `json:"time_range"`
	Status      string   `json:"status"`
	Importance  int      `json:"importance"`
	Tags        []string `json:"tags"`
	MemoryLevel string   `json:"memory_level"`
	Confidence  float64  `json:"confidence"`
	Evidence    string   `json:"evidence"`
}

type extractionResult struct {
	IsSubstantial bool                  `json:"is_substantial"`
	Experience    *extractedExperience  `json:"experience,omitempty"` // 兼容旧版单条输出
	Experiences   []extractedExperience `json:"experiences"`
	ProfileFacts  []model.ProfileFact   `json:"profile_facts"`
}

// experiences 汇总 experiences 与兼容字段 experience, 并过滤空标题
func (r *extractionResult) experiences() []extractedExperience {
	out := make([]extractedExperience, 0, len(r.Experiences)+1)
	for _, e := range r.Experiences {
		if strings.TrimSpace(e.Title) != "" {
			out = append(out, e)
		}
	}
	if r.Experience != nil && strings.TrimSpace(r.Experience.Title) != "" {
		out = append(out, *r.Experience)
	}
	return out
}

// RunExtraction 定时任务: 扫描满足条件的会话并抽取
func (s *UserMemoryService) RunExtraction() error {
	cfg := s.getConfig()
	if !cfg.Enabled {
		log.Println("[UserMemory] 抽取未启用, 跳过")
		return nil
	}
	if s.agentSvc == nil || s.agentSvc.activeModel == nil {
		return errors.New("AI 模型未配置, 无法抽取")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.collectEligible(0, cfg, true)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	log.Printf("[UserMemory] 本轮待抽取会话 %d 个", len(sessions))

	var firstErr error
	for _, sess := range sessions {
		if err := s.extractSession(sess, cfg); err != nil {
			log.Printf("[UserMemory] 会话 %d 抽取失败: %v", sess.HistoryID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ExtractUserSessions 手动触发某用户全部满足条件的会话抽取(忽略静默等待)
func (s *UserMemoryService) ExtractUserSessions(userID uint) (int, error) {
	cfg := s.getConfig()
	if !cfg.Enabled {
		return 0, errors.New("抽取未启用")
	}
	if s.agentSvc == nil || s.agentSvc.activeModel == nil {
		return 0, errors.New("AI 模型未配置, 无法抽取")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.collectEligible(userID, cfg, false)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, sess := range sessions {
		if err := s.extractSession(sess, cfg); err != nil {
			log.Printf("[UserMemory] 会话 %d 抽取失败: %v", sess.HistoryID, err)
			continue
		}
		done++
	}
	return done, nil
}

// collectEligible 收集可抽取会话; userID=0 表示全部用户; useIdle 是否要求会话已静默
func (s *UserMemoryService) collectEligible(userID uint, cfg MemoryExtractionConfig, useIdle bool) ([]eligibleSession, error) {
	var rows []eligibleSession

	sql := `SELECT h.id AS history_id, h.user_id, h.title, h.created_at,
       COUNT(CASE WHEN m.role = 'user' THEN 1 END) AS user_msg_count,
       MAX(m.sort_order) AS max_sort_order,
       MAX(m.created_at) AS last_msg_at
FROM training_histories h
JOIN training_messages m ON m.history_id = h.id`
	args := []interface{}{}
	if userID > 0 {
		sql += " WHERE h.user_id = ?"
		args = append(args, userID)
	}
	sql += " GROUP BY h.id, h.user_id, h.title, h.created_at HAVING user_msg_count > ?"
	args = append(args, cfg.MinUserMessages)
	if useIdle {
		sql += " AND last_msg_at <= ?"
		args = append(args, time.Now().Add(-time.Duration(cfg.IdleMinutes)*time.Minute))
	}

	if err := DB.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// 过滤: 正在处理/已完成且无新消息的会话
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.HistoryID)
	}
	var states []model.MemoryExtractionState
	DB.Where("history_id IN ?", ids).Find(&states)
	stateMap := make(map[uint]model.MemoryExtractionState, len(states))
	for _, st := range states {
		stateMap[st.HistoryID] = st
	}

	result := make([]eligibleSession, 0, len(rows))
	for _, r := range rows {
		st, ok := stateMap[r.HistoryID]
		if ok {
			if st.Status == model.MemoryExtractionStatusProcessing {
				continue
			}
			if (st.Status == model.MemoryExtractionStatusDone || st.Status == model.MemoryExtractionStatusSkipped) &&
				st.LastSortOrder >= r.MaxSortOrder {
				continue
			}
		}
		result = append(result, r)
		if len(result) >= 50 {
			break
		}
	}
	return result, nil
}

func (s *UserMemoryService) extractSession(sess eligibleSession, cfg MemoryExtractionConfig) error {
	var msgs []model.TrainingMessage
	if err := DB.Where("history_id = ?", sess.HistoryID).Order("sort_order ASC").Find(&msgs).Error; err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusProcessing, 0, "")

	transcript := buildTranscript(msgs)
	res, err := s.extractWithLLM(sess.Title, transcript, sess.CreatedAt, cfg.Model)
	if err != nil {
		s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusFailed, 0, err.Error())
		return err
	}

	exps := res.experiences()
	if !res.IsSubstantial || len(exps) == 0 {
		// 无实质内容: 不产生经历, 但仍贡献画像事实
		if len(res.ProfileFacts) > 0 {
			if err := s.MergeProfileFacts(sess.UserID, res.ProfileFacts); err != nil {
				log.Printf("[UserMemory] 画像合并失败 user=%d err=%v", sess.UserID, err)
			}
		}
		s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusSkipped, sess.MaxSortOrder, "")
		return nil
	}

	if err := s.syncSessionExperiences(sess, exps); err != nil {
		s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusFailed, 0, err.Error())
		return err
	}
	if len(res.ProfileFacts) > 0 {
		if err := s.MergeProfileFacts(sess.UserID, res.ProfileFacts); err != nil {
			log.Printf("[UserMemory] 画像合并失败 user=%d err=%v", sess.UserID, err)
		}
	}
	s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusDone, sess.MaxSortOrder, "")
	return nil
}

// syncSessionExperiences 以本轮抽取结果为准同步某会话的经历:
// 标题相同则更新, 新增则插入, 本轮不再出现的自动记录删除; 用户手动编辑过的不动
func (s *UserMemoryService) syncSessionExperiences(sess eligibleSession, exps []extractedExperience) error {
	hid := sess.HistoryID

	var existing []model.UserExperience
	if err := DB.Where("history_id = ? AND is_user_edited = ?", hid, false).Find(&existing).Error; err != nil {
		return err
	}
	existingByTitle := make(map[string]model.UserExperience, len(existing))
	for _, e := range existing {
		existingByTitle[e.Title] = e
	}

	kept := make(map[uint]bool, len(existing))
	processed := make(map[string]bool, len(exps))
	for i := range exps {
		exp := &exps[i]
		normalizeExtractedExperience(exp)
		title := strings.TrimSpace(exp.Title)
		if title == "" || processed[title] {
			continue
		}
		processed[title] = true
		if strings.TrimSpace(exp.TimeRange) == "" && !sess.CreatedAt.IsZero() {
			exp.TimeRange = sess.CreatedAt.Format("2006")
		}
		tags, _ := json.Marshal(exp.Tags)
		fields := map[string]interface{}{
			"domain":       exp.Domain,
			"event_type":   exp.EventType,
			"title":        exp.Title,
			"content":      exp.Content,
			"time_range":   exp.TimeRange,
			"tags":         string(tags),
			"confidence":   exp.Confidence,
			"importance":   exp.Importance,
			"memory_level": exp.MemoryLevel,
			"evidence":     exp.Evidence,
			"occurred_at":  sess.CreatedAt,
			"status":       exp.Status,
		}

		if e, ok := existingByTitle[exp.Title]; ok {
			if err := DB.Model(&model.UserExperience{}).Where("id = ?", e.ID).Updates(fields).Error; err != nil {
				return err
			}
			kept[e.ID] = true
			continue
		}

		record := &model.UserExperience{
			UserID:      sess.UserID,
			HistoryID:   &hid,
			Domain:      exp.Domain,
			EventType:   exp.EventType,
			Title:       exp.Title,
			Content:     exp.Content,
			TimeRange:   exp.TimeRange,
			OccurredAt:  &sess.CreatedAt,
			Tags:        string(tags),
			Confidence:  exp.Confidence,
			Importance:  exp.Importance,
			MemoryLevel: exp.MemoryLevel,
			Evidence:    exp.Evidence,
			Status:      exp.Status,
		}
		if err := DB.Create(record).Error; err != nil {
			return err
		}
	}

	// 删除本轮不再出现的自动记录
	for _, e := range existing {
		if kept[e.ID] {
			continue
		}
		if err := DB.Delete(&model.UserExperience{}, e.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// normalizeExtractedExperience 校正 LLM 输出: 合法分类/进度状态/记忆等级, 置信度范围
func normalizeExtractedExperience(exp *extractedExperience) {
	if !validExperienceStatus(exp.Status) {
		exp.Status = model.ExperienceStatusUnknown
	}
	if !validMemoryLevel(exp.MemoryLevel) {
		exp.MemoryLevel = model.MemoryLevelLongTerm
	}
	if !validDomain(exp.Domain) {
		exp.Domain = ""
	}
	if !validEventType(exp.EventType) {
		exp.EventType = ""
	}
	if exp.Importance < 1 || exp.Importance > 5 {
		exp.Importance = 3
	}
	if exp.Confidence < 0 {
		exp.Confidence = 0
	}
	if exp.Confidence > 1 {
		exp.Confidence = 1
	}
	if exp.Tags == nil {
		exp.Tags = []string{}
	}
}

func validExperienceStatus(s string) bool {
	switch s {
	case model.ExperienceStatusPlanned, model.ExperienceStatusOngoing,
		model.ExperienceStatusCompleted, model.ExperienceStatusAbandoned,
		model.ExperienceStatusPaused, model.ExperienceStatusUnknown:
		return true
	}
	return false
}

func validMemoryLevel(s string) bool {
	switch s {
	case model.MemoryLevelCore, model.MemoryLevelLongTerm, model.MemoryLevelTemporary:
		return true
	}
	return false
}

// 经历领域
var experienceDomains = map[string]bool{
	"career": true, "education": true, "project": true, "skill": true,
	"technology": true, "finance": true, "health": true, "lifestyle": true,
	"relationship": true, "community": true, "legal": true, "travel": true,
	"hobby": true, "habit": true, "personality": true, "preference": true,
	"achievement": true, "challenge": true, "other": true,
}

// 事件类型
var experienceEventTypes = map[string]bool{
	"start": true, "ongoing": true, "complete": true, "achieve": true,
	"fail": true, "abandon": true, "decide": true, "change": true,
	"participate": true, "publish": true, "compete": true, "volunteer": true,
	"relocate": true, "recover": true, "experiment": true, "maintain": true,
}

func validDomain(s string) bool {
	return experienceDomains[s]
}

func validEventType(s string) bool {
	return experienceEventTypes[s]
}

func (s *UserMemoryService) setState(historyID, userID uint, status string, lastSort int, errMsg string) {
	var st model.MemoryExtractionState
	err := DB.Where("history_id = ?", historyID).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		st = model.MemoryExtractionState{
			HistoryID: historyID,
			UserID:    userID,
			Status:    status,
		}
		if lastSort > 0 {
			st.LastSortOrder = lastSort
		}
		if errMsg != "" {
			st.Error = errMsg
		}
		if status == model.MemoryExtractionStatusDone {
			now := time.Now()
			st.ExtractedAt = &now
		}
		DB.Create(&st)
		return
	}
	updates := map[string]interface{}{"status": status, "error": errMsg}
	if lastSort > 0 {
		updates["last_sort_order"] = lastSort
	}
	if status == model.MemoryExtractionStatusDone {
		updates["extracted_at"] = time.Now()
	}
	DB.Model(&model.MemoryExtractionState{}).Where("history_id = ?", historyID).Updates(updates)
}

// ============ LLM ============

const extractSystemPrompt = `你是用户长期记忆抽取助手，负责从「用户与 AI 的一段对话」中判断是否值得写入长期记忆，并抽取用户经历与稳定画像事实。

【最高原则】
1. 只记录关于「用户本人」的信息，绝不记录 AI 提供的信息、解释或建议。
2. 不记录一次性的问答需求、知识咨询、假设性讨论、寒暄。
3. 不根据上下文猜测用户身份，一切以用户明确表达为准。
4. 只输出一个 JSON 对象，不要解释，不要 markdown 代码块。

【来源隔离（最重要，务必逐条检查）】
- 只允许抽取「用户」消息中的事实。对话中以「用户：」开头的是用户发言，「AI：」开头的是 AI 发言。
- 严禁抽取或改写 AI 的内容，包括：
  - AI 的建议（如「建议你每天练习英语」）
  - AI 的总结、解释、推测
  - 系统提示中的内容
- 反例：AI 说「建议你每天练习英语」→ 不得输出「用户每天练习英语」。
- 判定方法：问自己「这句话是用户本人说的吗？」只要不是用户亲口说的，就不要写。

【一、is_substantial：是否存在可记录的用户经历（仅当 experiences 非空时为 true）】
- 说明：is_substantial 只针对「经历」；画像事实 profile_facts 可独立输出，即使 is_substantial=false 也会被采用。
- 长期兴趣 / 习惯类信息优先放进 profile_facts，并须满足下方 a/b/c（详见【五】）。
true（满足其一）：
- 用户已发生或正在进行中的项目 / 工作
- 用户明确的学习目标，且有学习行动或计划
- 用户明确的技能、能力（有具体证据）
- 用户做出的重要决策，或正在面对的困难
- 会持续影响未来交互的长期偏好 / 习惯（如沟通方式、语言、固定规则），且满足 a/b/c 之一：
  a. 用户明确表示是长期、稳定的（如「我一直…」「我长期…」「我平时都…」）
  b. 在对话中被多次提及且前后一致
  c. 会影响未来的交互方式（如沟通偏好、回答习惯、语言偏好、约束条件）
false（满足其一）：
- 单纯询问知识、请求解释概念
- 请求建议、方案对比，但没有行动
- 假设性讨论（例如「如果…会怎样」）
- 只是顺口一提的喜好/好奇，且不满足上面 a/b/c
- 寒暄、纯事实问答
关键区分：只有「已发生、正在持续、或用户明确计划执行」的事才算经历；提问/咨询/随口一提不算。
- 正例：「我最近开始用 Go 写股票选股工具」→ 经历
- 反例：「Go 适合做量化吗？」→ 咨询，不是经历
- 反例：「怎么学习英语？」→ 咨询，不是经历
- 反例：「我喜欢看科幻电影」→ 顺口一提，不满足 a/b/c，不记录（除非用户说「我一直爱看科幻」或反复提到或会影响交互）

【二、memory_level：记忆等级】
- core：长期稳定、基本不变，如职业身份、技术栈、长期目标、沟通偏好
- long_term：预计保留半年以上，如正在进行的项目/学习、中期目标
- temporary：临时信息（今天想买什么、这周关注什么、当前任务上下文），不进入画像

【三、时间信息】
- 输入会给出「当前日期」「会话开始时间」，且对话内容每行带日期前缀。
- experience.time_range 必须尽量填写：优先用户明确提到的时间，其次会话时间/消息日期，格式如 "2026"、"2026-03"、"2026-03~2026-05"。
- 只有在对话完全没有任何时间线索时才留空字符串。

【四、experiences：可记录的经历数组（可为空；一段对话可能对应多条经历）】
- 从对话中识别 0~N 条互相独立的经历；每条对应一件不同的事，标题需能互相区分，不要用多条重复描述同一件事。
- 经历按「发生了什么」描述，不要按「用户是谁」分类（身份/目标/偏好等属于画像维度，不写进经历）。
- 若一条可记录经历都没有，输出空数组 []，并令 is_substantial 为 false。
- 每条格式：
{
  "domain": "career|education|project|skill|technology|finance|health|lifestyle|relationship|community|legal|travel|hobby|habit|personality|preference|achievement|challenge|other",
  "event_type": "start|ongoing|complete|achieve|fail|abandon|decide|change|participate|publish|compete|volunteer|relocate|recover|experiment|maintain",
  "title": "不超过20字，概括这段经历",
  "content": "客观总结2~4句，只写用户做了什么、进展、结果",
  "time_range": "时间范围，如 2026 / 2026-03~2026-05；不确定则为空字符串",
  "status": "planned|ongoing|completed|abandoned|paused|unknown",
  "importance": 3,
  "memory_level": "core|long_term|temporary",
  "tags": ["关键词"],
  "confidence": 0.8,
  "evidence": "用户原话片段（不超过50字）；无法引用用户原话则留空字符串"
}
- domain（领域）：career 职业/工作 / education 学习/教育 / project 项目/创造 / skill 技能/能力建设 / technology 技术偏好/技术栈 / finance 财务/投资 / health 健康 / lifestyle 生活方式 / relationship 人际关系/家庭/伴侣 / community 社群/组织 / legal 法律事务 / travel 旅行/迁移 / hobby 兴趣爱好 / habit 习惯/日常行为 / personality 性格/行为模式 / preference 偏好/选择倾向 / achievement 成就/荣誉 / challenge 困难/问题 / other 其他（仅当确实不属于以上任何一类时才用，不要滥用，优先归入最接近的类别）。
- 注意：domain 描述「经历所属领域」，不是静态属性；偏好/习惯/性格等静态特点仍写进 profile_facts，只有围绕它们的**具体事件**（如「开始培养早起习惯」「把技术栈从 Python 换成 Go」）才作为经历。
- event_type（事件类型）：start 开始 / ongoing 进行中 / complete 完成 / achieve 达成 / fail 失败 / abandon 放弃 / decide 决定 / change 转变 / participate 参与 / publish 发布 / compete 参赛 / volunteer 志愿 / relocate 迁移 / recover 康复 / experiment 尝试 / maintain 维持。
- importance（重要度）判断标准：
  1 = 一次性、小影响
  2 = 短期习惯、小项目
  3 = 持续半年以上的项目 / 学习目标
  4 = 职业方向变化、重大技能建设
  5 = 人生重大节点（职业转型、长期身份变化）
  拿不准时选 3；只有明确的重大节点才给 5，不要乱打分。
- evidence（证据）规则：
  - 必须来自用户明确表达的信息，优先直接抄用户原话片段（可截断），不要总结、不要改写。
  - 无法引用用户原话时留空字符串 ""，绝不允许编造、推断或补全。
  - 反例：用户「Go 怎么做股票系统？」→ evidence 写「用户正在开发股票系统」是推断（错误）；且这条本身也不应作为记忆。
- 示例：「从大厂离职做独立开发」→ domain=career, event_type=change, status=ongoing, importance=4, title=「离职转独立开发」。

【五、profile_facts：稳定画像事实（数组，可为空）】
只记录满足以下之一、且与用户本人相关的信息：
1. 用户明确表达，如「我喜欢…」「我的目标是…」「我长期使用…」「我习惯…」「我是…」
2. 在对话中反复出现且前后一致的事实
兴趣 / 习惯类额外门控（同【一】）：必须满足 a 明确长期 / b 多次提及 / c 影响交互，否则不记录。
不要记录：一次性需求、当前任务上下文、AI 的推测、临时状态。
- 反例：「用户最近研究股票」（临时/上下文）
- 正例：「用户正在学习股票投资，并计划开发选股工具」（有明确目标）
- 反例：「用户喜欢看科幻电影」（顺口一提，无长期/多次/影响交互，不记录）
每条格式：
{"content":"一句关于用户的稳定事实","memory_level":"core|long_term|temporary","confidence":0.9,"evidence":"用户原话片段；无法引用则留空"}
- evidence 规则同【四】：只能来自用户明确表达，禁止为了填字段而总结出不存在的事实。

【六、confidence 规则（禁止推断）】
- confidence 只能基于用户的明确表达：
  - 0.9~1.0：用户直接、明确陈述（如「我是…」「我喜欢…」「我正在…」）
  - 0.7~0.9：同一事实在对话中被用户多次明确陈述，且前后一致
- 严禁通过推断得出信息，以下一律不得写入（无论给多低的分）：
  - 根据用户行为推断身份 / 性格
  - 根据职业领域推测能力
  - 根据提问方向推测兴趣
- 例：用户问「Python 适合量化吗？」→ 不得输出「用户学习量化开发」。
- 只要无法追溯到用户明确表达，就舍弃该条，不要用低 confidence 蒙混写入。

【七、输出】
{
  "is_substantial": true,
  "experiences": [ ... ],
  "profile_facts": [ ... ]
}`

const mergeSystemPrompt = `你是用户画像维护助手。给定【当前画像】和【新增事实】，请合并生成最新的长期用户画像，重点防止画像膨胀与漂移。

【合并规则】
1. 只保留关于用户本人的、长期有效的信息，不要编造不存在的信息。
2. 信息优先级：长期稳定事实 > 当前项目/学习 > 临时状态。
3. 删除：已过期、重复、一次性需求、AI 推测、临时状态的信息。
4. 冲突处理：
   - 新事实明确改变旧状态 → 用新事实更新，例如「以前用 Python，现在主要用 Go」
   - 新事实只是补充 → 合并保留
   - 无法判断 → 保留旧信息，并适当降低 confidence
5. 无法归入固定维度的信息，写进 summary，不要新增维度键。
6. 只合并用户明确表达过的信息；不得从已有维度推导出新事实（例如从职业推断技能、从提问推断兴趣、把 AI 的建议当成用户事实）。无法追溯到用户明确表达的，一律不写。

【dimensions 固定维度】（没有内容则省略该键，不要新增其它键；多值用「、」连接为一个字符串）
- identity：当前身份（职业、角色）；只放当前身份，历史经历放 background
- background：教育、职业经历、行业、资历
- goals：目标
- skills：技能
- projects：项目
- learning_topics：正在学的学习主题
- interests：长期兴趣（未必正在学；与 learning_topics 区分：兴趣是「喜欢」，学习主题是「正在学」）
- preferences：偏好
- habits：习惯
- communication_style：沟通与回答偏好（如喜欢直接、简洁、要例子、不要客套；比 preferences 更具体，直接影响交互）
- language：语言偏好/母语/常用语言
- values：价值观、原则、优先级（长期稳定）
- risk_preference：风险偏好（如稳健/保守/激进）
- decision_style：决策风格（如数据驱动、先试再定；仅当用户明确陈述过才记，不要凭行为推断）
- constraints：约束/限制

【输出】只输出 JSON，不要解释，不要 markdown 代码块：
{"summary":"一段话整体画像，不超过800字","dimensions":{"identity":"","background":"","goals":"","skills":"","projects":"","learning_topics":"","interests":"","preferences":"","habits":"","communication_style":"","language":"","values":"","risk_preference":"","decision_style":"","constraints":""},"tags":["标签"],"confidence":0.8}`

func (s *UserMemoryService) extractWithLLM(title, transcript string, sessionTime time.Time, modelOverride string) (*extractionResult, error) {
	userPrompt := fmt.Sprintf(
		"会话标题：%s\n当前日期：%s\n会话开始时间：%s\n\n对话内容：\n%s",
		title,
		time.Now().Format("2006-01-02"),
		sessionTime.Format("2006-01-02 15:04"),
		transcript,
	)
	raw, err := s.agentSvc.GenerateText(modelOverride, extractSystemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}
	var res extractionResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
		return nil, fmt.Errorf("解析抽取结果失败: %w", err)
	}
	return &res, nil
}

// MergeProfileFacts 将新增事实合并进用户画像; 用户手动编辑过的画像不覆盖
// temporary 记忆等级的事实会被丢弃, 不进入画像
func (s *UserMemoryService) MergeProfileFacts(userID uint, facts []model.ProfileFact) error {
	facts = filterPortraitFacts(facts)
	if len(facts) == 0 {
		return nil
	}
	cfg := s.getConfig()
	if !cfg.Enabled {
		return nil
	}
	if s.agentSvc == nil || s.agentSvc.activeModel == nil {
		return errors.New("AI 模型未配置")
	}
	p, err := s.GetOrCreatePortrait(userID)
	if err != nil {
		return err
	}
	if p.IsUserEdited {
		return nil
	}

	current := map[string]interface{}{
		"summary":    p.Summary,
		"dimensions": json.RawMessage(orDefault(p.Dimensions, "{}")),
		"tags":       json.RawMessage(orDefault(p.Tags, "[]")),
	}
	curJSON, _ := json.Marshal(current)
	factsJSON, _ := json.Marshal(facts)
	userPrompt := fmt.Sprintf("【当前画像】\n%s\n\n【新增事实】\n%s", string(curJSON), string(factsJSON))

	raw, err := s.agentSvc.GenerateText(cfg.Model, mergeSystemPrompt, userPrompt)
	if err != nil {
		return err
	}
	var merged struct {
		Summary    string                     `json:"summary"`
		Dimensions map[string]json.RawMessage `json:"dimensions"`
		Tags       []string                   `json:"tags"`
		Confidence float64                    `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &merged); err != nil {
		return fmt.Errorf("解析画像合并结果失败: %w", err)
	}
	dimsMap := make(map[string]string, len(merged.Dimensions))
	for k, v := range merged.Dimensions {
		if s := normalizeDimensionValue(v); s != "" {
			dimsMap[k] = s
		}
	}
	dims, _ := json.Marshal(dimsMap)
	tags, _ := json.Marshal(merged.Tags)

	return DB.Model(&model.UserPortrait{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"summary":    merged.Summary,
		"dimensions": string(dims),
		"tags":       string(tags),
		"confidence": merged.Confidence,
	}).Error
}

// filterPortraitFacts 过滤画像事实: 去掉空内容与 temporary 等级
func filterPortraitFacts(facts []model.ProfileFact) []model.ProfileFact {
	out := make([]model.ProfileFact, 0, len(facts))
	for _, f := range facts {
		content := strings.TrimSpace(f.Content)
		if content == "" {
			continue
		}
		if f.MemoryLevel == model.MemoryLevelTemporary {
			continue
		}
		if !validMemoryLevel(f.MemoryLevel) {
			f.MemoryLevel = model.MemoryLevelLongTerm
		}
		f.Content = content
		out = append(out, f)
	}
	return out
}

// normalizeDimensionValue 兼容 LLM 输出字符串或字符串数组, 数组用「、」连接
func normalizeDimensionValue(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	switch s[0] {
	case '"':
		var v string
		if err := json.Unmarshal(raw, &v); err == nil {
			return strings.TrimSpace(v)
		}
	case '[':
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			return strings.TrimSpace(strings.Join(arr, "、"))
		}
		var anys []any
		if err := json.Unmarshal(raw, &anys); err == nil {
			parts := make([]string, 0, len(anys))
			for _, a := range anys {
				if str, ok := a.(string); ok && strings.TrimSpace(str) != "" {
					parts = append(parts, strings.TrimSpace(str))
				}
			}
			return strings.TrimSpace(strings.Join(parts, "、"))
		}
	}
	return ""
}

// ============ 定时任务 ============

func (s *UserMemoryService) RegisterCronTasks(js *JobScheduler) {
	if js == nil {
		return
	}
	js.RegisterTask("memory.extract_sessions", "抽取用户画像与经历(会话增量)", json.RawMessage(`{}`),
		func(_ *model.JobDefinition, _ json.RawMessage) error {
			return s.RunExtraction()
		})
}

// ============ 辅助 ============

func buildTranscript(msgs []model.TrainingMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		role := "用户"
		if m.Role == "assistant" {
			role = "AI"
		}
		if !m.CreatedAt.IsZero() {
			b.WriteString("[" + m.CreatedAt.Format("2006-01-02") + "] ")
		}
		b.WriteString(role + "：" + m.Content + "\n")
	}
	out := b.String()
	const maxLen = 12000
	runes := []rune(out)
	if len(runes) > maxLen {
		out = string(runes[:maxLen])
	}
	return out
}

// extractJSON 去掉 LLM 输出中可能包裹的 markdown 代码块, 并截取首个 JSON 对象
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "json"))
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 && j < len(s)-1 {
		s = s[:j+1]
	}
	return s
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
