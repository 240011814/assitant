package model

type DashboardStats struct {
	TodayMessages     int64            `json:"today_messages"`
	TotalMessages     int64            `json:"total_messages"`
	TotalVocabulary   int64            `json:"total_vocabulary"`
	TotalNotes        int64            `json:"total_notes"`
	TotalFavorites    int64            `json:"total_favorites"`
	TrainingTrend     []TrendItem      `json:"training_trend"`
	TrainingTypeStats []TypeStatItem   `json:"training_type_stats"`
}

type TrendItem struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// AdminDashboardStats 系统概览 (管理员视角): 用户 / AI 用量 / 任务健康 / 操作审计
type AdminDashboardStats struct {
	TotalUsers         int64       `json:"total_users"`
	NewUsers7d         int64       `json:"new_users_7d"`
	RecentLogins24h    int64       `json:"recent_logins_24h"`
	MessagesToday      int64       `json:"messages_today"`
	TotalConversations int64       `json:"total_conversations"`
	EnabledJobs        int64       `json:"enabled_jobs"`
	FailedRuns24h      int64       `json:"failed_runs_24h"`
	OperationsToday    int64       `json:"operations_today"`
	FailedOpsToday     int64       `json:"failed_ops_today"`
	OpsTrend           []TrendItem `json:"ops_trend"`
}

type TypeStatItem struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}
