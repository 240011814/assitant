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
	"identity", "goals", "skills", "projects",
	"preferences", "habits", "learning_topics", "constraints",
}

var dimensionLabels = map[string]string{
	"identity":        "身份",
	"goals":           "目标",
	"skills":          "技能",
	"projects":        "项目",
	"preferences":     "偏好",
	"habits":          "习惯",
	"learning_topics": "学习主题",
	"constraints":     "约束",
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

func (s *UserMemoryService) ListExperiences(userID uint, page, pageSize int, category, keyword string) (int64, []model.UserExperience, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := DB.Model(&model.UserExperience{}).Where("user_id = ?", userID)
	if category != "" {
		q = q.Where("category = ?", category)
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
		Category:    req.Category,
		Title:       req.Title,
		Content:     req.Content,
		TimeRange:   req.TimeRange,
		OccurredAt:  req.OccurredAt,
		Tags:        string(tags),
		MemoryLevel: req.MemoryLevel,
		Evidence:    req.Evidence,
		Status:      req.Status,
	}
	return s.AddExperience(userID, exp)
}

func (s *UserMemoryService) UpdateExperience(userID, id uint, req model.UpdateUserExperienceRequest) error {
	updates := map[string]interface{}{"is_user_edited": true}
	if req.Category != nil {
		updates["category"] = *req.Category
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
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	TimeRange   string   `json:"time_range"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	MemoryLevel string   `json:"memory_level"`
	Confidence  float64  `json:"confidence"`
	Evidence    string   `json:"evidence"`
}

type extractionResult struct {
	IsSubstantial bool                 `json:"is_substantial"`
	Experience    *extractedExperience `json:"experience"`
	ProfileFacts  []model.ProfileFact  `json:"profile_facts"`
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
	res, err := s.extractWithLLM(sess.Title, transcript, cfg.Model)
	if err != nil {
		s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusFailed, 0, err.Error())
		return err
	}

	if !res.IsSubstantial || res.Experience == nil || strings.TrimSpace(res.Experience.Title) == "" {
		// 无实质内容: 不产生经历, 但仍贡献画像事实
		if len(res.ProfileFacts) > 0 {
			if err := s.MergeProfileFacts(sess.UserID, res.ProfileFacts); err != nil {
				log.Printf("[UserMemory] 画像合并失败 user=%d err=%v", sess.UserID, err)
			}
		}
		s.setState(sess.HistoryID, sess.UserID, model.MemoryExtractionStatusSkipped, sess.MaxSortOrder, "")
		return nil
	}

	if err := s.upsertSessionExperience(sess, res.Experience); err != nil {
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

func (s *UserMemoryService) upsertSessionExperience(sess eligibleSession, exp *extractedExperience) error {
	normalizeExtractedExperience(exp)

	hid := sess.HistoryID
	tags, _ := json.Marshal(exp.Tags)
	occurred := sess.CreatedAt

	fields := map[string]interface{}{
		"category":     exp.Category,
		"title":        exp.Title,
		"content":      exp.Content,
		"time_range":   exp.TimeRange,
		"tags":         string(tags),
		"confidence":   exp.Confidence,
		"memory_level": exp.MemoryLevel,
		"evidence":     exp.Evidence,
		"occurred_at":  occurred,
		"status":       exp.Status,
	}

	var existing model.UserExperience
	err := DB.Where("history_id = ?", hid).First(&existing).Error
	if err == nil {
		if existing.IsUserEdited {
			return nil
		}
		return DB.Model(&model.UserExperience{}).Where("id = ?", existing.ID).Updates(fields).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	record := &model.UserExperience{
		UserID:      sess.UserID,
		HistoryID:   &hid,
		Category:    exp.Category,
		Title:       exp.Title,
		Content:     exp.Content,
		TimeRange:   exp.TimeRange,
		OccurredAt:  &occurred,
		Tags:        string(tags),
		Confidence:  exp.Confidence,
		MemoryLevel: exp.MemoryLevel,
		Evidence:    exp.Evidence,
		Status:      exp.Status,
	}
	return DB.Create(record).Error
}

// normalizeExtractedExperience 校正 LLM 输出: 合法分类/进度状态/记忆等级, 置信度范围
func normalizeExtractedExperience(exp *extractedExperience) {
	if !validExperienceStatus(exp.Status) {
		exp.Status = model.ExperienceStatusUnknown
	}
	if !validMemoryLevel(exp.MemoryLevel) {
		exp.MemoryLevel = model.MemoryLevelLongTerm
	}
	if !validCategory(exp.Category) {
		exp.Category = "experience"
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
	case model.ExperienceStatusOngoing, model.ExperienceStatusCompleted,
		model.ExperienceStatusAbandoned, model.ExperienceStatusUnknown:
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

func validCategory(s string) bool {
	switch s {
	case "identity", "goal", "project", "skill", "preference",
		"habit", "experience", "challenge", "decision":
		return true
	}
	return false
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

【一、is_substantial：是否构成可记录的用户经历】
true（满足其一）：
- 用户已经发生或正在进行中的项目 / 工作
- 用户长期或明确的学习目标
- 用户明确的技能、能力
- 用户的长期兴趣、习惯
- 用户做出的重要决策，或正在面对的困难
false（满足其一）：
- 单纯询问知识、请求解释概念
- 请求建议、方案对比，但没有行动
- 假设性讨论（例如「如果…会怎样」）
- 只是好奇/感兴趣，但没有行动或计划
- 寒暄、纯事实问答
关键区分：只有「已发生、正在持续、或用户明确计划执行」的事才算经历；提问/咨询/感兴趣不算。
- 正例：「我最近开始用 Go 写股票选股工具」→ 经历
- 反例：「Go 适合做量化吗？」→ 咨询，不是经历
- 反例：「怎么学习英语？」→ 咨询，不是经历

【二、memory_level：记忆等级】
- core：长期稳定、基本不变，如职业身份、技术栈、长期目标、沟通偏好
- long_term：预计保留半年以上，如正在进行的项目/学习、中期目标
- temporary：临时信息（今天想买什么、这周关注什么、当前任务上下文），不进入画像

【三、experience：is_substantial 为 true 时输出，否则为 null】
{
  "category": "identity|goal|project|skill|preference|habit|experience|challenge|decision",
  "title": "不超过20字，概括这段经历",
  "content": "客观总结2~4句，只写用户做了什么、进展、结果",
  "time_range": "时间范围，如 2026 / 2026-03~2026-05；不确定则为空字符串",
  "status": "ongoing|completed|abandoned|unknown",
  "memory_level": "core|long_term|temporary",
  "tags": ["关键词"],
  "confidence": 0.8,
  "evidence": "最能代表这条经历的用户原话摘要（不超过50字）"
}
category 取值：identity 身份 / goal 长期目标 / project 项目 / skill 技能 / preference 偏好 / habit 习惯 / experience 一般经历 / challenge 困难 / decision 重大决策。

【四、profile_facts：稳定画像事实（数组，可为空）】
只记录满足以下之一、且与用户本人相关的信息：
1. 用户明确表达，如「我喜欢…」「我的目标是…」「我长期使用…」「我习惯…」「我是…」
2. 在对话中反复出现且前后一致的事实
不要记录：一次性需求、当前任务上下文、AI 的推测、临时状态。
- 反例：「用户最近研究股票」（临时/上下文）
- 正例：「用户正在学习股票投资，并计划开发选股工具」（有明确目标）
每条格式：
{"content":"一句关于用户的稳定事实","memory_level":"core|long_term|temporary","confidence":0.9,"evidence":"用户原话摘要"}

【五、confidence 规则】
- 0.9~1.0：用户明确陈述
- 0.7~0.9：多轮推断但较可靠
- 0.4~0.7：弱推断
- 小于 0.4：不要记录

【六、输出】
{
  "is_substantial": true,
  "experience": { ... } 或 null,
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

【dimensions 固定维度】（没有内容则省略该键，不要新增其它键；多值用「、」连接为一个字符串）
- identity：身份（职业、角色）
- goals：目标
- skills：技能
- projects：项目
- preferences：偏好
- habits：习惯
- learning_topics：学习主题
- constraints：约束/限制

【输出】只输出 JSON，不要解释，不要 markdown 代码块：
{"summary":"一段话整体画像，不超过200字","dimensions":{"identity":"","goals":"","skills":"","projects":"","preferences":"","habits":"","learning_topics":"","constraints":""},"tags":["标签"],"confidence":0.8}`

func (s *UserMemoryService) extractWithLLM(title, transcript, modelOverride string) (*extractionResult, error) {
	userPrompt := fmt.Sprintf("会话标题：%s\n\n对话内容：\n%s", title, transcript)
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
