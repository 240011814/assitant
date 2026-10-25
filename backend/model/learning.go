package model

import (
	"time"
)

// ===== 词汇复习日志 =====

// VocabularyReviewLog 词汇复习日志: 支撑学习看板 (热力图/保持率/连续天数) 与每日新词限额计数
type VocabularyReviewLog struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint       `gorm:"not null;index:idx_vrl_user_date,priority:1" json:"userId"`
	VocabularyID uint       `gorm:"not null;index" json:"vocabularyId"`
	Direction    string     `gorm:"size:20;not null;default:'en2zh'" json:"direction"`
	Rating       int        `gorm:"not null" json:"rating"` // 1=Again 2=Hard 3=Good 4=Easy
	StateBefore  int        `gorm:"not null;default:0" json:"stateBefore"`
	Stability    float64    `gorm:"not null;default:0" json:"stability"`
	Difficulty   float64    `gorm:"not null;default:0" json:"difficulty"`
	Due          *time.Time `json:"due"`
	DurationMs   int        `gorm:"not null;default:0" json:"durationMs"`
	CreatedAt    time.Time  `gorm:"index:idx_vrl_user_date,priority:2" json:"createdAt"`
}

// TableName 指定表名
func (VocabularyReviewLog) TableName() string {
	return "vocabulary_review_logs"
}

// ===== 出题练习会话 =====

// QuizQuestion 单道题 (持久化为 JSON, Answer 只存服务端不下发前端)
type QuizQuestion struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // word2def / def2word / cloze / confusing / spell / translate
	WordID   uint     `json:"wordId,omitempty"`
	CourseID uint     `json:"courseId,omitempty"`
	Prompt   string   `json:"prompt"`
	Hint     string   `json:"hint,omitempty"` // 听写 TTS 文本 / 中文提示等
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"answer,omitempty"` // 正确答案: 随会话 JSON 落库, 返回前端前必须剥离
	Word     string   `json:"word,omitempty"`
	Done     bool     `json:"done"` // 已作答
}

// QuizSession 一次练习会话
type QuizSession struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index:idx_quiz_user,priority:1" json:"userId"`
	Questions string    `gorm:"type:longtext;not null" json:"-"`
	Total     int       `gorm:"not null;default:0" json:"total"`
	Answered  int       `gorm:"not null;default:0" json:"answered"`
	Correct   int       `gorm:"not null;default:0" json:"correct"`
	Status    string    `gorm:"size:20;not null;default:'active'" json:"status"`
	CreatedAt time.Time `gorm:"index:idx_quiz_user,priority:2" json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (QuizSession) TableName() string {
	return "quiz_sessions"
}

// QuizAnswerResult 答题结果
type QuizAnswerResult struct {
	QuestionID string `json:"questionId"`
	Correct    bool   `json:"correct"`
	Answer     string `json:"answer"`   // 正确答案
	Recorded   bool   `json:"recorded"` // 是否已计入错题本
}

// ===== 分级阅读 =====

// ReadingDoc 阅读文档 (纯文本, 段落以空行分隔)
type ReadingDoc struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           uint      `gorm:"not null;index:idx_reading_user,priority:1" json:"userId"`
	Title            string    `gorm:"size:200;not null" json:"title"`
	SourceType       string    `gorm:"size:20;not null;default:'text'" json:"sourceType"` // url/text
	SourceURL        string    `gorm:"size:500" json:"sourceUrl"`
	Content          string    `gorm:"type:mediumtext;not null" json:"content"`
	WordCount        int       `gorm:"not null;default:0" json:"wordCount"`
	DifficultWords   string    `gorm:"type:mediumtext" json:"-"` // JSON: [{word,meaning,reason,known}]
	UnknownRate      float64   `gorm:"not null;default:0" json:"unknownRate"`
	Status           string    `gorm:"size:20;not null;default:'pending'" json:"status"` // pending/annotating/annotated/failed
	EstUnknownRate   float64   `gorm:"not null;default:0" json:"estUnknownRate"`         // 导入时预估生词率 (生词本交集+长词启发, 免 LLM)
	AnnotateProgress int       `gorm:"not null;default:0" json:"annotateProgress"`       // 分块标注进度 0-100
	CreatedAt        time.Time `gorm:"index:idx_reading_user,priority:2" json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ReadingDoc) TableName() string {
	return "reading_docs"
}

// DifficultWord 标注生词条目
type DifficultWord struct {
	Word    string `json:"word"`
	Meaning string `json:"meaning"`
	Reason  string `json:"reason,omitempty"`
	Known   bool   `json:"known"` // 是否已在用户生词本
}

// DictionaryCache 点词即查的词典缓存 (LLM 结果全局复用)
type DictionaryCache struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Word        string    `gorm:"size:100;not null;uniqueIndex" json:"word"`
	Phonetic    string    `gorm:"size:100" json:"phonetic"`
	Definition  string    `gorm:"type:text" json:"definition"`
	Translation string    `gorm:"type:text" json:"translation"`
	Example     string    `gorm:"type:text" json:"example"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (DictionaryCache) TableName() string {
	return "dictionary_cache"
}

// ===== 慢 LLM 生成任务 =====

// AITextJob 慢 LLM 生成任务 (AI 写课文等): Cloudflare 对源站响应有 100s 上限,
// 此类操作必须异步执行, 任务行供前端轮询
type AITextJob struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"not null;index:idx_ai_text_job_user,priority:1" json:"userId"`
	Kind       string    `gorm:"size:30;not null" json:"kind"`                     // passage / vocab_test / rss_annotate
	Status     string    `gorm:"size:20;not null;default:'running'" json:"status"` // running/done/failed
	ResultID   uint      `gorm:"not null;default:0" json:"resultId"`
	ResultJSON string    `gorm:"type:longtext" json:"resultJson"` // 任务结果 JSON (词汇量测试题面等)
	Error      string    `gorm:"type:text" json:"error"`
	CreatedAt  time.Time `gorm:"index:idx_ai_text_job_user,priority:2" json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (AITextJob) TableName() string {
	return "ai_text_jobs"
}

// ===== 请求/响应结构 =====

// ReviewSubmitRequest FSRS 复习提交
type ReviewSubmitRequest struct {
	Rating     int    `json:"rating" binding:"required,min=1,max=4"` // 1=忘记 2=困难 3=良好 4=简单
	Direction  string `json:"direction"`
	DurationMs int    `json:"durationMs"`
}

// ReviewCardItem 复习卡 (词 + 本卡方向)
type ReviewCardItem struct {
	Direction string     `json:"direction"`
	Word      Vocabulary `json:"word"`
}

// ReviewSessionResponse 复习会话: 到期复习 + 新词 (受每日限额约束)
type ReviewSessionResponse struct {
	Reviews         []ReviewCardItem `json:"reviews"`
	NewWords        []ReviewCardItem `json:"newWords"`
	NewLimit        int              `json:"newLimit"`
	NewStartedToday int              `json:"newStartedToday"`
	EstimatedMin    int              `json:"estimatedMin"`
}

// CreateQuizSessionRequest 创建练习会话
type CreateQuizSessionRequest struct {
	Size   int      `json:"size"`   // 题目数量, 默认 10
	Types  []string `json:"types"`  // 为空则全部题型
	Source string   `json:"source"` // all / due / errorbook
}

// QuizAnswerRequest 提交答案
type QuizAnswerRequest struct {
	QuestionID string `json:"questionId" binding:"required"`
	Answer     string `json:"answer" binding:"required"`
	DurationMs int    `json:"durationMs"`
}

// SentencePracticeRequest 造句批改
type SentencePracticeRequest struct {
	WordID   uint   `json:"wordId" binding:"required"`
	Sentence string `json:"sentence" binding:"required"`
}

// SentencePracticeResult 造句批改结果
type SentencePracticeResult struct {
	Score    int      `json:"score"` // 0~100
	Correct  bool     `json:"correct"`
	Grammar  []string `json:"grammar"`  // 语法问题
	Improved string   `json:"improved"` // 改进版本
	Comment  string   `json:"comment"`  // 点评
	Word     string   `json:"word"`     // 目标词
}

// ReadingImportRequest 导入阅读材料
type ReadingImportRequest struct {
	Title      string `json:"title" binding:"required"`
	SourceType string `json:"sourceType" binding:"required,oneof=url text"`
	SourceURL  string `json:"sourceUrl"`
	Content    string `json:"content"`
}

// TranslateRequest 划词翻译 (ReaderView 选中文本)
type TranslateRequest struct {
	Text string `json:"text" binding:"required,min=2,max=2000"`
}

// VocabTestQuestion 词汇量测试单题 (LLM 按频段分层生成)
type VocabTestQuestion struct {
	ID      string   `json:"id"`
	Band    string   `json:"band"` // A1/A2/B1/B2/C1/C2
	Word    string   `json:"word"`
	Options []string `json:"options"` // 4 个中文释义 (含正确答案)
	Answer  int      `json:"answer"`  // 正确选项下标 (仅存服务端)
}

// VocabTestSubmitRequest 词汇量测试提交
type VocabTestSubmitRequest struct {
	JobID   int            `json:"jobId" binding:"required"`
	Answers map[string]int `json:"answers"` // 题目ID -> 选项下标
}

// GeneratePassageRequest AI 生成个性化课文 (把指定新词织进课文)
type GeneratePassageRequest struct {
	WordIDs       []uint `json:"wordIds"`       // 为空则自动取今日新词
	SentenceCount int    `json:"sentenceCount"` // 默认 8
}

// ImportWordItem 词书导入条目
type ImportWordItem struct {
	Word       string `json:"word" binding:"required"`
	Phonetic   string `json:"phonetic"`
	Definition string `json:"definition"`
	Example    string `json:"example"`
}

// ImportWordsResult 批量导入结果
type ImportWordsResult struct {
	Added   int      `json:"added"`
	Skipped int      `json:"skipped"`
	Failed  []string `json:"failed"`
}
