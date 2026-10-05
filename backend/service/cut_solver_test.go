package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/model"
)

// 精确求解组装: mock sidecar 返回固定解, 验证 BarResult 组装/旧料出队/多根展开
func TestSolvePreciseGroupAssembly(t *testing.T) {
	resp := map[string]any{
		"bars": []map[string]any{
			{"source": "material", "material_index": 0, "pattern": []int{3, 0}, "count": 1},
			{"source": "material", "material_index": 0, "pattern": []int{1, 1}, "count": 1},
			{"source": "scrap", "scrap_length": 25, "pattern": []int{0, 2}, "count": 2},
		},
		"unplaced":   []map[string]any{},
		"iterations": 7,
		"elapsed_ms": 120,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["kerf"] == nil || req["time_limit_ms"] == nil {
			t.Errorf("请求应携带 kerf/time_limit_ms")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewCutService(nil)
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	items := []aggItem{{length: 10, demand: 3}, {length: 7, demand: 2}}
	demand := []int{3, 2}
	results, usedRest, err := s.solvePreciseGroup(client, items, demand,
		[]float64{60}, []string{"6m"}, []int{25, 25}, []string{"余料A", "余料B"}, 1, 5, 1)
	if err != nil {
		t.Fatalf("精确求解失败: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("应组装 4 根材料, got %d", len(results))
	}
	// 新料: 3×10 + 2×kerf = 32
	if results[0].Index != 1 || results[0].MaterialType != "6m" || results[0].Used != 32 || results[0].Remaining != 28 {
		t.Fatalf("第一根新料组装不符: %+v", results[0])
	}
	// 旧料按输入顺序出队, 类型取各自 label
	if results[2].TotalLength != 25 || results[2].MaterialType != "余料A" {
		t.Fatalf("第一根旧料不符: %+v", results[2])
	}
	if results[3].MaterialType != "余料B" {
		t.Fatalf("第二根旧料应取到下一根的 label: %+v", results[3])
	}
	if len(usedRest) != 2 || usedRest[0] != 0 || usedRest[1] != 1 {
		t.Fatalf("旧料消费下标不符: %v", usedRest)
	}
}

// 旧料数量不足 (sidecar 超卖) 应报错并触发调用方回退
func TestSolvePreciseGroupScrapShortage(t *testing.T) {
	resp := map[string]any{
		"bars":     []map[string]any{{"source": "scrap", "scrap_length": 25, "pattern": []int{2}, "count": 3}},
		"unplaced": []map[string]any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewCutService(nil)
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	items := []aggItem{{length: 10, demand: 3}}
	_, _, err := s.solvePreciseGroup(client, items, []int{3}, []float64{60}, []string{""}, []int{25, 25}, []string{"", ""}, 0, 5, 1)
	if err == nil {
		t.Fatal("旧料超卖应报错")
	}
}

// 未配置求解地址时, precise 模式自动回退快速算法, 结果照常产出
func TestBarCutPreciseFallbackWithoutSolver(t *testing.T) {
	s := NewCutService(nil)
	resp, err := s.BarCut(1, model.BarRequest{
		Items:             model.BarItemList{{Length: 2000}, {Length: 1500}},
		NewMaterialLength: 6000,
		Mode:              model.BarModePrecise,
	})
	if err != nil {
		t.Fatalf("precise 回退路径不应报错: %v", err)
	}
	if len(resp.Results) == 0 || resp.Summary.MaterialCount == 0 {
		t.Fatalf("回退后应有结果: %+v", resp.Summary)
	}
}
