package model

import (
	"time"
)

// Vocabulary 生词实体
type Vocabulary struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          uint       `gorm:"not null;index" json:"userId"`
	Word            string     `gorm:"size:100;not null;index" json:"word"`
	Phonetic        string     `gorm:"size:100" json:"phonetic"`
	Definition      string     `gorm:"type:text" json:"definition"`
	Example         string     `gorm:"type:text" json:"example"`
	SourceContext   string     `gorm:"type:text" json:"sourceContext"`
	ConfusingWords  string     `gorm:"type:text" json:"confusingWords"`
	IsMastered      bool       `gorm:"default:false" json:"isMastered"`
	SrsBox          int        `gorm:"not null;default:0" json:"srsBox"` // 旧 Leitner 盒子, 保留兼容, FSRS 后不再更新
	NextReviewAt    *time.Time `json:"nextReviewAt"`                     // 下次复习时间 (NULL=未进入复习流程)
	LastReviewedAt  *time.Time `json:"lastReviewedAt"`                   // 最近复习时间
	FsrsStability   float64    `gorm:"not null;default:0" json:"fsrsStability"`
	FsrsDifficulty  float64    `gorm:"not null;default:0" json:"fsrsDifficulty"`
	FsrsState       int        `gorm:"not null;default:0" json:"fsrsState"` // 0=New 1=Learning 2=Review 3=Relearning
	FsrsReps        int        `gorm:"not null;default:0" json:"fsrsReps"`
	FsrsLapses      int        `gorm:"not null;default:0" json:"fsrsLapses"`
	AssistExample   string     `gorm:"type:text" json:"assistExample"`       // 遗忘干预: AI 新例句
	AssistMnemonic  string     `gorm:"type:text" json:"assistMnemonic"`      // 遗忘干预: AI 助记提示
	SourceKind      string     `gorm:"size:20" json:"sourceKind"`            // 来源: reading=分级阅读 / rss=RSS 条目 (空=手动)
	SourceRef       uint       `gorm:"not null;default:0" json:"sourceRef"`  // 来源文档/条目 ID (回链原文)
	Encounters      int        `gorm:"not null;default:0" json:"encounters"` // 阅读中再次相遇次数
	ContextSentence string     `gorm:"type:text" json:"contextSentence"`     // 加入生词本时所在的原句 (语境复习方向)
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// FSRS 状态常量 (与 go-fsrs State 对齐)
const (
	FsrsStateNew        = 0
	FsrsStateLearning   = 1
	FsrsStateReview     = 2
	FsrsStateRelearning = 3
)

// ReviewDirection 复习方向: 双向卡 (en2zh 再认 / zh2en 主动回忆 / listen 听音辨词 / context 原文语境)
const (
	ReviewDirectionEn2Zh   = "en2zh"
	ReviewDirectionZh2En   = "zh2en"
	ReviewDirectionListen  = "listen"
	ReviewDirectionContext = "context"
)

// TableName 指定表名
func (Vocabulary) TableName() string {
	return "vocabulary"
}

// CreateVocabularyRequest 创建生词请求
type CreateVocabularyRequest struct {
	Word            string `json:"word" binding:"required"`
	Phonetic        string `json:"phonetic"`
	Definition      string `json:"definition"`
	Example         string `json:"example"`
	SourceContext   string `json:"sourceContext"`
	ConfusingWords  string `json:"confusingWords"`
	SourceKind      string `json:"sourceKind"`      // reading / rss (空=手动)
	SourceRef       uint   `json:"sourceRef"`       // 来源文档/条目 ID
	ContextSentence string `json:"contextSentence"` // 加入时所在原句
}

// ReadingDailyStat 阅读行为日统计 (按日聚合, UPSERT 递增)
type ReadingDailyStat struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint   `gorm:"not null;uniqueIndex:uk_rds_user_date,priority:1" json:"userId"`
	StatDate    string `gorm:"type:date;not null;uniqueIndex:uk_rds_user_date,priority:2" json:"statDate"` // YYYY-MM-DD
	Opens       int    `gorm:"not null;default:0" json:"opens"`
	Lookups     int    `gorm:"not null;default:0" json:"lookups"`
	WordsAdded  int    `gorm:"not null;default:0" json:"wordsAdded"`
	ReadSeconds int    `gorm:"not null;default:0" json:"readSeconds"`
}

// TableName 指定表名
func (ReadingDailyStat) TableName() string {
	return "reading_daily_stats"
}

// VocabularyAssessment 词汇量评估记录
type VocabularyAssessment struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index:idx_va_user,priority:1" json:"userId"`
	CEFR      string    `gorm:"size:4;not null" json:"cefr"` // A1..C2
	EstSize   int       `gorm:"not null" json:"estSize"`     // 估算词汇量
	Detail    string    `gorm:"type:text" json:"-"`          // 各频段答题明细 JSON
	CreatedAt time.Time `gorm:"index:idx_va_user,priority:2" json:"createdAt"`
}

// TableName 指定表名
func (VocabularyAssessment) TableName() string {
	return "vocabulary_assessments"
}

// UpdateVocabularyRequest 更新生词请求
type UpdateVocabularyRequest struct {
	Phonetic       string `json:"phonetic"`
	Definition     string `json:"definition"`
	Example        string `json:"example"`
	ConfusingWords string `json:"confusingWords"`
	IsMastered     *bool  `json:"isMastered"`
}

// WordBank 分级锚定词库 (word_bank 表): 自适应词汇量测试的考词来源。
// 词的难度由分级数据锚定, 不由 LLM 判断; 种子 = 每档典型词, 后续可整档扩充。
type WordBank struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Word      string    `gorm:"size:50;not null;uniqueIndex:uk_word_bank_word" json:"word"`
	CEFR      string    `gorm:"size:4;not null;index:idx_word_bank_cefr,priority:1" json:"cefr"` // A1..C2
	Source    string    `gorm:"size:30;not null;default:'band-seed'" json:"source"`
	Enabled   bool      `gorm:"not null;default:true;index:idx_word_bank_cefr,priority:2" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

// TableName 指定表名
func (WordBank) TableName() string {
	return "word_bank"
}

// VocabTestItem 考题缓存 (vocab_test_items 表, 按词全局复用, 与 dictionary_cache 同模式):
// 正确释义 + 3 个干扰项由 LLM 生成一次后永久复用, 选项顺序在生成时打乱落库。
type VocabTestItem struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Word              string    `gorm:"size:50;not null;uniqueIndex:uk_vti_word" json:"word"`
	CEFR              string    `gorm:"size:4;not null" json:"cefr"`
	CorrectDefinition string    `gorm:"size:255;not null" json:"correctDefinition"`
	Distractors       string    `gorm:"size:900;not null" json:"-"`  // 3 个干扰释义 JSON 数组
	OptionsJSON       string    `gorm:"size:1200;not null" json:"-"` // 打乱后的 4 选项
	Answer            int       `gorm:"not null" json:"-"`           // 正确选项下标 0-3
	Model             string    `gorm:"size:100;not null;default:''" json:"model"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (VocabTestItem) TableName() string {
	return "vocab_test_items"
}
