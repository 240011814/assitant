package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ============ ChunkText ============

func TestChunkTextShortText(t *testing.T) {
	chunks := ChunkText("短文本", 500, 80)
	if len(chunks) != 1 || chunks[0] != "短文本" {
		t.Fatalf("短文本应产出单块, got %v", chunks)
	}
}

func TestChunkTextEmpty(t *testing.T) {
	if chunks := ChunkText("", 500, 80); len(chunks) != 0 {
		t.Fatalf("空文本应产出 0 块, got %v", chunks)
	}
}

func TestChunkTextSizesAndOverlap(t *testing.T) {
	// 无换行/句点的纯汉字, 走硬切: 每块 <= size 且相邻块有 overlap 重叠
	text := strings.Repeat("字", 1200)
	chunks := ChunkText(text, 500, 80)
	if len(chunks) < 3 {
		t.Fatalf("1200 字应切出 >=3 块, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := len([]rune(c)); n > 500 {
			t.Fatalf("块 %d 超过 500 字: %d", i, n)
		}
	}
	// 相邻块重叠: 块 i 尾部内容应出现在块 i+1 头部
	tail := []rune(chunks[0])[len([]rune(chunks[0]))-40:]
	if !strings.HasPrefix(chunks[1], string(tail)) {
		t.Fatalf("相邻块应带 overlap 重叠, 块0尾部=%q 块1头部=%q", string(tail), chunks[1][:min(40, len(chunks[1]))])
	}
}

func TestChunkTextPrefersParagraphBreak(t *testing.T) {
	// 窗口后段有换行时, 应在换行处断开而不是拦腰硬切
	line := strings.Repeat("甲", 60)
	text := strings.Join([]string{line, line, line, line, line, line, line, line, line}, "\n")
	chunks := ChunkText(text, 500, 80)
	if len(chunks) < 2 {
		t.Fatalf("多行文本应切出多块, got %d", len(chunks))
	}
	// 非末块应以整行边界收尾 (末字符是行内容, 不会切在行中间)
	for i, c := range chunks {
		if i == len(chunks)-1 {
			continue
		}
		runes := []rune(c)
		if runes[len(runes)-1] != '甲' {
			t.Fatalf("块 %d 应在整行边界结束, 尾字符=%q", i, string(runes[len(runes)-1]))
		}
	}
}

func TestChunkTextOverlapClamped(t *testing.T) {
	// overlap >= size 时自动收敛, 不应死循环
	chunks := ChunkText(strings.Repeat("字", 1000), 100, 500)
	if len(chunks) == 0 {
		t.Fatal("应产出切块")
	}
}

func TestChunkTextMultibyteNotCut(t *testing.T) {
	// rune 级切块: 中文字符不应被切成无效字节
	text := strings.Repeat("中文测试", 300) // 1200 rune
	chunks := ChunkText(text, 300, 50)
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "中文测试") {
		t.Fatal("切块后内容应保持原字符完整")
	}
}

// ============ 嵌入客户端 ============

func newTestRagService(baseURL string) *RagService {
	s := &RagService{httpClient: &http.Client{}}
	s.mu.Lock()
	s.cfg = ragConfig{Enabled: true, BaseURL: baseURL, Model: "test-embed", ChunkSize: 500, Overlap: 80, TopK: 5}
	s.mu.Unlock()
	return s
}

func writeEmbedResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func TestEmbedBatchSplitAndOrder(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("请求体解析失败: %v", err)
			return
		}
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()
		if len(req.Input) > 16 {
			t.Errorf("单次请求批量超限: %d", len(req.Input))
		}
		if req.Model != "test-embed" {
			t.Errorf("model 应透传, got %q", req.Model)
		}
		// 每条向量用请求序号+输入下标标记, 验证顺序映射
		data := make([]map[string]any, 0, len(req.Input))
		for i := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float32{float32(n), float32(i), 3}})
		}
		writeEmbedResponse(w, http.StatusOK, map[string]any{"data": data})
	}))
	defer srv.Close()

	s := newTestRagService(srv.URL)
	texts := make([]string, 20) // 超过单批 16, 应拆两次请求
	for i := range texts {
		texts[i] = "文本" + strings.Repeat("x", i)
	}
	vecs, err := s.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 2 {
		t.Fatalf("20 条文本应拆 2 次请求, got %d", requests)
	}
	if len(vecs) != 20 {
		t.Fatalf("应返回 20 条向量, got %d", len(vecs))
	}
	// 顺序映射: 第 0 条来自第 1 次请求下标 0; 第 16 条来自第 2 次请求下标 0
	if vecs[0][0] != 1 || vecs[0][1] != 0 {
		t.Fatalf("第 0 条向量标记不符: %v", vecs[0])
	}
	if vecs[16][0] != 2 || vecs[16][1] != 0 {
		t.Fatalf("第 16 条向量标记不符 (应来自第二次请求): %v", vecs[16])
	}
}

func TestEmbedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEmbedResponse(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": "模型不存在"},
		})
	}))
	defer srv.Close()

	s := newTestRagService(srv.URL)
	_, err := s.Embed(context.Background(), []string{"测试"})
	if err == nil || !strings.Contains(err.Error(), "模型不存在") {
		t.Fatalf("应透出服务端错误信息, got %v", err)
	}
}

func TestEmbedPositionalFallback(t *testing.T) {
	// 部分服务不带 index 下标 (全 0), 应按返回顺序对位
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEmbedResponse(w, http.StatusOK, map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float32{1, 1}},
				{"index": 0, "embedding": []float32{2, 2}},
			},
		})
	}))
	defer srv.Close()

	s := newTestRagService(srv.URL)
	vecs, err := s.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	if vecs[0][0] != 1 || vecs[1][0] != 2 {
		t.Fatalf("无下标时应按顺序对位, got %v / %v", vecs[0], vecs[1])
	}
}

func TestTestEmbeddingReturnsDims(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEmbedResponse(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": make([]float32, 1024)}},
		})
	}))
	defer srv.Close()

	s := newTestRagService(srv.URL)
	dims, err := s.TestEmbedding(context.Background(), srv.URL, "bge-m3", "")
	if err != nil {
		t.Fatalf("TestEmbedding 失败: %v", err)
	}
	if dims != 1024 {
		t.Fatalf("维度应为 1024, got %d", dims)
	}
}

func TestTestEmbeddingEmptyParams(t *testing.T) {
	s := newTestRagService("http://unused")
	if _, err := s.TestEmbedding(context.Background(), "", "m", ""); err == nil {
		t.Fatal("空 BaseURL 应报错")
	}
	if _, err := s.TestEmbedding(context.Background(), "http://x", "", ""); err == nil {
		t.Fatal("空模型应报错")
	}
}
