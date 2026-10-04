package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RAG (文档语义检索): 向量经 VectorStore 接口存取 (当前实现 ClickHouse document_chunks,
// Go 直写主数据, 与股票复制表角色无关; 可替换为 Qdrant/Milvus/MySQL 等),
// 嵌入走 OpenAI 兼容 /embeddings 接口 (本地 Ollama: base_url=http://127.0.0.1:11434/v1, model=bge-m3;
// 任何 OpenAI 兼容嵌入服务均可, 换服务只改配置)。
// 配置存 system_config rag_* 键 (S3 同款热刷新); 存储未启用或嵌入未配置时索引/检索自动降级关闭
const (
	ragDefaultChunkSize  = 500 // 切块字符数 (rune)
	ragDefaultOverlap    = 80  // 相邻块重叠字符数
	ragDefaultTopK       = 5
	ragChunkSizeMax      = 2000
	ragOverlapMax        = 500
	ragSearchTopKMax     = 20
	ragMaxBatchEmbed     = 16 // 单次 /embeddings 请求携带的文本数
	ragEmbedTimeout      = 60 * time.Second
	ragSearchTimeout     = 60 * time.Second
	ragTestEmbedTimeout  = 30 * time.Second
)

// ragConfig RAG 运行配置快照 (cacheMu 保护, 读时取快照)
type ragConfig struct {
	Enabled    bool
	BaseURL    string
	Model      string
	APIKey     string
	ChunkSize  int
	Overlap    int
	TopK       int
}

// RagService 文档向量嵌入与检索门面。只负责: rag_* 配置热刷新 / OpenAI 兼容嵌入调用 /
// 切块 / 向量存储访问 (经 VectorStore 接口, 后端可替换)。索引管线在 UserDocumentService
type RagService struct {
	configSvc  *SystemConfigService
	store      VectorStore
	mu         sync.RWMutex
	cfg        ragConfig
	dims       int // 首次成功嵌入探测到的向量维度 (0=未知, 仅观测用)
	httpClient *http.Client
}

// NewRagService 构造并加载一次配置 (system_config 中尚无 rag_* 键时为全默认值);
// store 为向量存储实现, 允许 nil (检索/索引整体降级关闭)
func NewRagService(configSvc *SystemConfigService, store VectorStore) *RagService {
	s := &RagService{
		configSvc:  configSvc,
		store:      store,
		httpClient: &http.Client{Timeout: ragEmbedTimeout},
	}
	s.RefreshConfig()
	return s
}

// Available 向量存储当前是否可用 (ragReady 用; 嵌入配置完整性由 ready 校验)
func (s *RagService) Available() bool {
	return s.store != nil && s.store.Available()
}

// InsertChunks 向量存储写入口 (索引管线用; 存储不可用返回明确错误)
func (s *RagService) InsertChunks(ctx context.Context, rows []ChunkRow) error {
	if !s.Available() {
		return errors.New("向量存储不可用")
	}
	return s.store.InsertChunks(ctx, rows)
}

// DeleteDocumentChunks 向量存储删除口 (重建/删除文档用; 存储不可用返回明确错误)
func (s *RagService) DeleteDocumentChunks(ctx context.Context, userID uint, documentID uint) error {
	if !s.Available() {
		return errors.New("向量存储不可用")
	}
	return s.store.DeleteDocumentChunks(ctx, userID, documentID)
}

// RefreshConfig 从 system_config 读取 rag_* 键 (与 S3/SMTP 同款热刷新, 配置页保存后触发)
func (s *RagService) RefreshConfig() {
	getVal := func(key string) string {
		v, _ := s.configSvc.GetValue(key)
		return v
	}
	isTrue := func(v string) bool { return v == "1" || strings.EqualFold(v, "true") }
	getInt := func(key string, def, min, max int) int {
		if v := getVal(key); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				if n < min {
					n = min
				}
				if n > max {
					n = max
				}
				return n
			}
		}
		return def
	}

	cfg := ragConfig{
		Enabled:   isTrue(getVal("rag_enabled")),
		BaseURL:   strings.TrimSpace(getVal("rag_embedding_base_url")),
		Model:     strings.TrimSpace(getVal("rag_embedding_model")),
		APIKey:    getVal("rag_embedding_api_key"),
		ChunkSize: getInt("rag_chunk_size", ragDefaultChunkSize, 100, ragChunkSizeMax),
		Overlap:   getInt("rag_chunk_overlap", ragDefaultOverlap, 0, ragOverlapMax),
		TopK:      getInt("rag_top_k", ragDefaultTopK, 1, ragSearchTopKMax),
	}
	if cfg.Overlap >= cfg.ChunkSize {
		cfg.Overlap = cfg.ChunkSize / 5
	}

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// snapshot 取配置快照 (锁内拷贝, 避免与 RefreshConfig 并发竞争)
func (s *RagService) snapshot() ragConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Enabled RAG 总开关 (不含 CH 可用性判断, 那是调用方的事)
func (s *RagService) Enabled() bool { return s.snapshot().Enabled }

// Dims 已探测到的向量维度 (0=未知)
func (s *RagService) Dims() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dims
}

// ready 校验配置完整性并返回快照
func (s *RagService) ready() (ragConfig, error) {
	cfg := s.snapshot()
	if !cfg.Enabled {
		return cfg, errors.New("RAG 未开启 (系统配置 -> 文档 RAG)")
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return cfg, errors.New("嵌入模型未配置 (需填写 Base URL 与模型名)")
	}
	return cfg, nil
}

// ChunkText 按字符数切块 (rune 级, 防止切断多字节字符), 相邻块带 overlap。
// 窗口后 30% 内优先在换行/句末符号处断开, 减少句子被拦腰截断
func ChunkText(text string, size, overlap int) []string {
	runes := []rune(text)
	if size <= 0 {
		size = ragDefaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 5
	}
	isCut := func(i int) bool {
		switch runes[i] {
		case '\n':
			return true
		case '。', '！', '？', '；', '!', '?', ';':
			return true
		case '.', ',':
			// 英文句点/逗号后跟空白才算句子边界 (避免 3.14 / 1,000 被切开)
			return i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n'
		}
		return false
	}
	chunks := make([]string, 0, len(runes)/max(size-overlap, 1)+1)
	start := 0
	for start < len(runes) {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		} else {
			// 在窗口后 30% 内从右往左找断句点
			searchFrom := start + (size*7)/10
			cut := -1
			for i := end - 1; i >= searchFrom && i > start; i-- {
				if isCut(i) {
					cut = i + 1
					break
				}
			}
			if cut > start {
				end = cut
			}
		}
		if chunk := strings.TrimSpace(string(runes[start:end])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1 // 防御: 保证窗口前进
		}
		start = next
	}
	return chunks
}

// ============ 嵌入客户端 (OpenAI 兼容 /embeddings) ============

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 批量嵌入文本 (内部按 ragMaxBatchEmbed 分批), 返回与 texts 等长的向量数组
func (s *RagService) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	cfg, err := s.ready()
	if err != nil {
		return nil, err
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += ragMaxBatchEmbed {
		end := min(start+ragMaxBatchEmbed, len(texts))
		vecs, err := s.embedBatch(ctx, cfg.BaseURL, cfg.Model, cfg.APIKey, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	if len(out) > 0 && len(out[0]) > 0 {
		s.mu.Lock()
		if s.dims == 0 {
			s.dims = len(out[0])
		}
		s.mu.Unlock()
	}
	return out, nil
}

// embedBatch 单次 /embeddings 请求 (POST {base_url}/embeddings, body: {model, input: [...]})
func (s *RagService) embedBatch(ctx context.Context, baseURL, model, apiKey string, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{"model": model, "input": texts})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求嵌入服务失败: %w", err)
	}
	defer resp.Body.Close()
	var body embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if body.Error != nil {
			msg = body.Error.Message
		}
		return nil, fmt.Errorf("嵌入服务返回 HTTP %d: %s", resp.StatusCode, msg)
	}
	if body.Error != nil && body.Error.Message != "" {
		return nil, fmt.Errorf("嵌入服务错误: %s", body.Error.Message)
	}
	if len(body.Data) != len(texts) {
		return nil, fmt.Errorf("嵌入服务返回 %d 条向量, 期望 %d 条", len(body.Data), len(texts))
	}
	// index 全为 0 的服务 (部分实现不带下标) 按返回顺序对位
	byIndex := false
	for _, d := range body.Data {
		if d.Index != 0 {
			byIndex = true
			break
		}
	}
	result := make([][]float32, len(texts))
	for i, d := range body.Data {
		pos := i
		if byIndex {
			pos = d.Index
		}
		if pos < 0 || pos >= len(result) {
			return nil, fmt.Errorf("嵌入响应下标越界: %d", pos)
		}
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("嵌入服务返回空向量 (第 %d 条)", i)
		}
		result[pos] = d.Embedding
	}
	return result, nil
}

// TestEmbedding 用显式连接参数测试嵌入服务连通性 (配置页「测试连接」), 返回向量维度。
// 独立于当前配置快照, 便于保存前验证
func (s *RagService) TestEmbedding(ctx context.Context, baseURL, model, apiKey string) (int, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		return 0, errors.New("Base URL 与模型名不能为空")
	}
	vecs, err := s.embedBatch(ctx, baseURL, model, apiKey, []string{"连接测试"})
	if err != nil {
		return 0, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return 0, errors.New("嵌入服务返回空向量")
	}
	return len(vecs[0]), nil
}

// ============ 向量检索 (经 VectorStore 接口) ============

// Search 在用户的文档向量索引中检索与 query 最相关的片段:
// 查询向量化 (OpenAI 兼容嵌入) 后交给存储实现检索, 返回按相似度降序的命中。
// 按 embeddingModel=当前配置模型 过滤由存储实现负责: 换嵌入模型后旧向量不参与检索,
// 需对文档重建索引才会重新可见
func (s *RagService) Search(ctx context.Context, userID uint, query string, documentIDs []uint, topK int) ([]VectorHit, error) {
	if !s.Available() {
		return nil, errors.New("向量存储不可用 (检索不可用)")
	}
	cfg, err := s.ready()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, ragSearchTimeout)
	defer cancel()
	qvec, err := s.embedBatch(ctx, cfg.BaseURL, cfg.Model, cfg.APIKey, []string{query})
	if err != nil {
		return nil, fmt.Errorf("查询向量化失败: %w", err)
	}
	if topK <= 0 {
		topK = cfg.TopK
	}
	if topK > ragSearchTopKMax {
		topK = ragSearchTopKMax
	}
	return s.store.SearchChunks(ctx, userID, qvec[0], cfg.Model, documentIDs, topK)
}
