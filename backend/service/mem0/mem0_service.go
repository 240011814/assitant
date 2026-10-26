package service

import (
	interfaces "backend/interface"
	"backend/model"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Mem0ServiceImpl struct {
	enabled bool
	apiKey  string
	baseURL string
	client  *http.Client
	db *gorm.DB
}

func NewMem0Service(timeout time.Duration,db *gorm.DB) *Mem0ServiceImpl {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Mem0ServiceImpl{
		baseURL: "https://api.mem0.ai/v1",
		client:  &http.Client{Timeout: timeout},
		db: db,
	}
}

func (s *Mem0ServiceImpl) LoadFromTool() {
	var tool model.AITool
	if err := s.db.Where("name = ?", "mem0_memory").First(&tool).Error; err != nil {
		s.enabled = false
		return
	}
	if !tool.Enabled || tool.ConfigJSON == "" {
		s.enabled = false
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(tool.ConfigJSON), &cfg); err != nil {
		s.enabled = false
		return
	}
	apiKey, _ := cfg["api_key"].(string)
	baseURL, _ := cfg["base_url"].(string)
	if apiKey == "" {
		s.enabled = false
		return
	}
	s.enabled = true
	s.apiKey = apiKey
	if baseURL != "" {
		s.baseURL = baseURL
	}
}

func (s *Mem0ServiceImpl) IsConfigured() bool {
	return s != nil && s.enabled && s.apiKey != ""
}

func (s *Mem0ServiceImpl) GetEnabled() bool {
	return s != nil && s.enabled
}

func (s *Mem0ServiceImpl) v3BaseURL() string {
	base := strings.TrimRight(s.baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base[:len(base)-3] + "/v3"
	}
	return base + "/v3"
}

func (s *Mem0ServiceImpl) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+s.apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mem0 API error: status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (s *Mem0ServiceImpl) AddMemory(userID uint, messages []interfaces.Mem0Message, metadata map[string]any) (*interfaces.Mem0AddResponse, error) {
	if !s.IsConfigured() {
		return nil, nil
	}
	reqBody := map[string]any{
		"messages": messages,
		"user_id":  strconv.FormatUint(uint64(userID), 10),
	}
	if len(metadata) > 0 {
		reqBody["metadata"] = metadata
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.v3BaseURL()+"/memories/add/", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}
	var result interfaces.Mem0AddResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	log.Printf("mem0 v3 add memory user=%d event_id=%s status=%s", userID, result.EventID, result.Status)
	return &result, nil
}

func (s *Mem0ServiceImpl) SearchMemories(userID uint, query string, topK int) ([]interfaces.Mem0Memory, error) {
	if !s.IsConfigured() {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	body := map[string]any{
		"query": query,
		"filters": map[string]string{
			"user_id": strconv.FormatUint(uint64(userID), 10),
		},
		"top_k": topK,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.v3BaseURL()+"/memories/search/", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	respBody, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}
	var result struct {
		Results []interfaces.Mem0Memory `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Results, nil
}

func (s *Mem0ServiceImpl) ListMemories(userID uint, page, pageSize int) (*interfaces.Mem0ListResponse, error) {
	if !s.IsConfigured() {
		return nil, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	body := map[string]any{
		"filters": map[string]string{
			"user_id": strconv.FormatUint(uint64(userID), 10),
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	url := fmt.Sprintf("%s/memories/?page=%d&page_size=%d", s.v3BaseURL(), page, pageSize)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	respBody, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}
	var result interfaces.Mem0ListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

func (s *Mem0ServiceImpl) DeleteMemory(memoryID string) error {
	if !s.IsConfigured() {
		return nil
	}
	url := fmt.Sprintf("%s/memories/%s/", s.baseURL, memoryID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+s.apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mem0 API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	log.Printf("mem0 delete memory id=%s", memoryID)
	return nil
}



