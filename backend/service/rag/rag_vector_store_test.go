package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeVectorStore 验证 RagService -> VectorStore 的委托语义 (换存储后端时行为契约不变)
type fakeVectorStore struct {
	mu        sync.Mutex
	available bool
	inserted  [][]ChunkRow
	deleted   [][2]uint // {userID, documentID}
	searches  []fakeSearchCall
	hits      []VectorHit
}

type fakeSearchCall struct {
	userID  uint
	vec     []float32
	model   string
	docIDs  []uint
	topK    int
}

func (f *fakeVectorStore) Available() bool { return f.available }

func (f *fakeVectorStore) InsertChunks(_ context.Context, rows []ChunkRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]ChunkRow, len(rows))
	copy(cp, rows)
	f.inserted = append(f.inserted, cp)
	return nil
}

func (f *fakeVectorStore) DeleteDocumentChunks(_ context.Context, userID, documentID uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, [2]uint{userID, documentID})
	return nil
}

func (f *fakeVectorStore) SearchChunks(_ context.Context, userID uint, queryVec []float32, embeddingModel string, documentIDs []uint, topK int) ([]VectorHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches = append(f.searches, fakeSearchCall{userID: userID, vec: queryVec, model: embeddingModel, docIDs: documentIDs, topK: topK})
	return f.hits, nil
}

func newStoreTestRagService(store VectorStore, embedURL string) *RagService {
	s := &RagService{httpClient: &http.Client{}, store: store}
	s.mu.Lock()
	s.cfg = ragConfig{Enabled: true, BaseURL: embedURL, Model: "m1", ChunkSize: 500, Overlap: 80, TopK: 5}
	s.mu.Unlock()
	return s
}

func TestRagSearchDelegatesToStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEmbedResponse(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float32{7, 7}}},
		})
	}))
	defer srv.Close()

	store := &fakeVectorStore{available: true, hits: []VectorHit{
		{DocumentID: 3, ChunkIndex: 2, Score: 0.91, Content: "片段"},
	}}
	s := newStoreTestRagService(store, srv.URL)

	hits, err := s.Search(context.Background(), 9, "查询内容", []uint{3}, 0) // topK=0 → 用配置默认 5
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "片段" {
		t.Fatalf("应原样返回存储层命中, got %v", hits)
	}
	if len(store.searches) != 1 {
		t.Fatalf("应恰好一次存储检索, got %d", len(store.searches))
	}
	call := store.searches[0]
	if call.userID != 9 || call.model != "m1" || call.topK != 5 || len(call.docIDs) != 1 || call.docIDs[0] != 3 {
		t.Fatalf("检索参数透传不符: %+v", call)
	}
	if len(call.vec) != 2 || call.vec[0] != 7 {
		t.Fatalf("查询向量应为嵌入结果, got %v", call.vec)
	}
}

func TestRagStoreUnavailableNoEmbedCall(t *testing.T) {
	// 存储不可用时应先失败, 不发起嵌入请求 (嵌入服务地址故意指向不可达端口)
	s := newStoreTestRagService(nil, "http://127.0.0.1:1")
	if s.Available() {
		t.Fatal("store=nil 时 Available 应为 false")
	}
	if _, err := s.Search(context.Background(), 1, "查询", nil, 0); err == nil {
		t.Fatal("存储不可用时 Search 应报错")
	}
	if err := s.InsertChunks(context.Background(), []ChunkRow{{UserID: 1, DocumentID: 1}}); err == nil {
		t.Fatal("存储不可用时 InsertChunks 应报错")
	}
	if err := s.DeleteDocumentChunks(context.Background(), 1, 1); err == nil {
		t.Fatal("存储不可用时 DeleteDocumentChunks 应报错")
	}
}

func TestRagInsertDeleteDelegate(t *testing.T) {
	store := &fakeVectorStore{available: true}
	s := newStoreTestRagService(store, "http://unused")

	rows := []ChunkRow{
		{UserID: 1, DocumentID: 2, ChunkIndex: 0, Content: "a", Embedding: []float32{1}, EmbeddingModel: "m1", Dims: 1},
		{UserID: 1, DocumentID: 2, ChunkIndex: 1, Content: "b", Embedding: []float32{2}, EmbeddingModel: "m1", Dims: 1},
	}
	if err := s.InsertChunks(context.Background(), rows); err != nil {
		t.Fatalf("InsertChunks 失败: %v", err)
	}
	if err := s.DeleteDocumentChunks(context.Background(), 1, 2); err != nil {
		t.Fatalf("DeleteDocumentChunks 失败: %v", err)
	}
	if len(store.inserted) != 1 || len(store.inserted[0]) != 2 || store.inserted[0][1].Content != "b" {
		t.Fatalf("写入委托不符: %+v", store.inserted)
	}
	if len(store.deleted) != 1 || store.deleted[0] != [2]uint{1, 2} {
		t.Fatalf("删除委托不符: %+v", store.deleted)
	}
}
