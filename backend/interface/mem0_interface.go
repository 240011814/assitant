package interfaces

type Mem0Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Mem0Memory struct {
	ID         string   `json:"id"`
	Memory     string   `json:"memory"`
	Score      *float64 `json:"score,omitempty"`
	UserID     string   `json:"user_id,omitempty"`
	Metadata   any      `json:"metadata,omitempty"`
	Categories []string `json:"categories,omitempty"`
	CreatedAt  string   `json:"created_at,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

type Mem0AddResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
	EventID string `json:"event_id"`
}

type Mem0ListResponse struct {
	Count    int          `json:"count"`
	Next     *string      `json:"next"`
	Previous *string      `json:"previous"`
	Results  []Mem0Memory `json:"results"`
}

type Mem0Service interface {
	IsConfigured() bool
	GetEnabled() bool
	LoadFromTool()
	AddMemory(userID uint, messages []Mem0Message, metadata map[string]any) (*Mem0AddResponse, error)
	SearchMemories(userID uint, query string, topK int) ([]Mem0Memory, error)
	ListMemories(userID uint, page, pageSize int) (*Mem0ListResponse, error)
	DeleteMemory(memoryID string) error
}
