package usermemory

import (
	iface "backend/interface"
	"backend/model"
	"backend/service/ai/prompt"
	"backend/service/kernel"
	"backend/service/sys/systemconfig"
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

// DB 本包专用数据库句柄, 由 main.go 接线赋值 (usermemory.DB = db)。
var DB *gorm.DB

// MemoryExtractionConfig 画像/经历抽取配置(来自 system_config)
type MemoryExtractionConfig struct {
	Enabled         bool
	Model           string
	IdleMinutes     int
	MinUserMessages int
}

// UserMemoryService 用户画像与经历: 会话级抽取、画像合并、注入与 CRUD
// agentSvc 依赖窄接口 TextGenerator 而非具体 *AIAgentService, 切断 user→ai 的相互 import。
type UserMemoryService struct {
	agentSvc iface.TextGenerator
	sysCfg   *systemconfig.SystemConfigService
	mu       sync.Mutex
	// extracting 抽取运行标志 (由 mu 保护): 锁只覆盖标志的读写, 不覆盖抽取过程
	// —— 整轮抽取可达数十分钟, 若整程持锁, 手动触发请求会被挂死到 cron 那轮结束
	extracting bool
}

// tryLockExtraction 抢占本轮抽取资格; 已有进行中的一轮时返回 false (会话级防重由 DB 的 processing 状态兜底)
func (s *UserMemoryService) tryLockExtraction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extracting {
		return false
	}
	s.extracting = true
	return true
}

func (s *UserMemoryService) unlockExtraction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extracting = false
}

func NewUserMemoryService(agentSvc iface.TextGenerator, sysCfg *systemconfig.SystemConfigService) *UserMemoryService {
	s := &UserMemoryService{agentSvc: agentSvc, sysCfg: sysCfg}
	// 启动复位: 上个进程可能在 LLM 调用期间退出, processing 状态若不复位会被 collectEligible 永久跳过
	if err := DB.Model(&model.MemoryExtractionState{}).
		Where("status = ?", model.MemoryExtractionStatusProcessing).
		Updates(map[string]interface{}{
			"status": model.MemoryExtractionStatusFailed,
			"error":  "服务重启导致抽取中断, 已复位等待重试",
		}).Error; err != nil {
		log.Printf("[UserMemory] 复位卡在 processing 的抽取状态失败: %v", err)
	}
	return s
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
	if err := json.Unmarshal([]byte(p.Dimensions), &dims); err != nil && strings.TrimSpace(p.Dimensions) != "" {
		log.Printf("[user_memory] 画像 dimensions JSON 解析失败 userID=%d: %v", userID, err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(p.Tags), &tags); err != nil && strings.TrimSpace(p.Tags) != "" {
		log.Printf("[user_memory] 画像 tags JSON 解析失败 userID=%d: %v", userID, err)
	}

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
			fmt.Fprintf(&b, "- %s：%s\n", dimensionLabel(k), strings.TrimSpace(v))
			written[k] = true
		}
	}
	for k, v := range dims {
		if written[k] || strings.TrimSpace(v) == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s：%s\n", k, strings.TrimSpace(v))
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
	page = kernel.NormalizePage(page)
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
	page = kernel.NormalizePage(page)
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
	if s.agentSvc == nil || !s.agentSvc.HasActiveModel() {
		return errors.New("AI 模型未配置, 无法抽取")
	}

	if !s.tryLockExtraction() {
		return errors.New("上一轮抽取仍在进行中, 请稍后再试")
	}
	defer s.unlockExtraction()

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
	if s.agentSvc == nil || !s.agentSvc.HasActiveModel() {
		return 0, errors.New("AI 模型未配置, 无法抽取")
	}

	if !s.tryLockExtraction() {
		return 0, errors.New("上一轮抽取仍在进行中, 请稍后再试")
	}
	defer s.unlockExtraction()

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
	// 状态查询失败时必须报错跳过本轮: 拿不到状态会把已抽取过的会话当新会话重复抽取 (重复烧 LLM 费用)
	if err := DB.Where("history_id IN ?", ids).Find(&states).Error; err != nil {
		return nil, fmt.Errorf("查询抽取状态失败: %w", err)
	}
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
	res, err := s.extractWithLLM(sess.UserID, sess.Title, transcript, sess.CreatedAt, cfg.Model)
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
// (多表读改删, 包事务避免中途失败留下半同步状态)
func (s *UserMemoryService) syncSessionExperiences(sess eligibleSession, exps []extractedExperience) error {
	hid := sess.HistoryID

	return DB.Transaction(func(tx *gorm.DB) error {
		var existing []model.UserExperience
		if err := tx.Where("history_id = ? AND is_user_edited = ?", hid, false).Find(&existing).Error; err != nil {
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
				if err := tx.Model(&model.UserExperience{}).Where("id = ?", e.ID).Updates(fields).Error; err != nil {
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
			if err := tx.Create(record).Error; err != nil {
				return err
			}
		}

		// 删除本轮不再出现的自动记录
		for _, e := range existing {
			if kept[e.ID] {
				continue
			}
			if err := tx.Delete(&model.UserExperience{}, e.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
		if err := DB.Create(&st).Error; err != nil {
			log.Printf("[user_memory] 抽取状态落库失败 historyID=%d status=%s: %v", historyID, status, err)
		}
		return
	}
	updates := map[string]interface{}{"status": status, "error": errMsg}
	if lastSort > 0 {
		updates["last_sort_order"] = lastSort
	}
	if status == model.MemoryExtractionStatusDone {
		updates["extracted_at"] = time.Now()
	}
	if err := DB.Model(&model.MemoryExtractionState{}).Where("history_id = ?", historyID).Updates(updates).Error; err != nil {
		log.Printf("[user_memory] 抽取状态更新失败 historyID=%d status=%s: %v", historyID, status, err)
	}
}

// ============ LLM (系统提示词见 prompt_config.go, 可在配置页覆盖) ============
func (s *UserMemoryService) extractWithLLM(userID uint, title, transcript string, sessionTime time.Time, modelOverride string) (*extractionResult, error) {
	userPrompt := fmt.Sprintf(
		"会话标题：%s\n当前日期：%s\n会话开始时间：%s\n\n对话内容：\n%s",
		title,
		time.Now().Format("2006-01-02"),
		sessionTime.Format("2006-01-02 15:04"),
		transcript,
	)
	raw, err := s.agentSvc.GenerateText(userID, modelOverride, prompt.GetPrompt(prompt.PromptKeyMemoryExtract), userPrompt)
	if err != nil {
		return nil, err
	}
	var res extractionResult
	if err := json.Unmarshal([]byte(kernel.ExtractJSON(raw)), &res); err != nil {
		return nil, fmt.Errorf("解析抽取结果失败: %w", err)
	}
	return &res, nil
}

// MergeProfileFacts 将新增事实合并进用户画像; 用户手动编辑过的画像仍会被 AI 继续维护
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
	if s.agentSvc == nil || !s.agentSvc.HasActiveModel() {
		return errors.New("AI 模型未配置")
	}
	p, err := s.GetOrCreatePortrait(userID)
	if err != nil {
		return err
	}

	current := map[string]interface{}{
		"summary":    p.Summary,
		"dimensions": json.RawMessage(orDefault(p.Dimensions, "{}")),
		"tags":       json.RawMessage(orDefault(p.Tags, "[]")),
	}
	curJSON, _ := json.Marshal(current)
	factsJSON, _ := json.Marshal(facts)
	userPrompt := fmt.Sprintf("【当前画像】\n%s\n\n【新增事实】\n%s", string(curJSON), string(factsJSON))

	raw, err := s.agentSvc.GenerateText(userID, cfg.Model, prompt.GetPrompt(prompt.PromptKeyMemoryMerge), userPrompt)
	if err != nil {
		return err
	}
	var merged struct {
		Summary    string                     `json:"summary"`
		Dimensions map[string]json.RawMessage `json:"dimensions"`
		Tags       []string                   `json:"tags"`
		Confidence float64                    `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(kernel.ExtractJSON(raw)), &merged); err != nil {
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

// extractJSON 已抽到 backend/service/llmjson 包 (llmjson.ExtractJSON), 供 LLM JSON 宽容解析共用。

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
