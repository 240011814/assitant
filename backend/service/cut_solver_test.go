package service

import (
	"encoding/json"
	"io"
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

	s := NewCutService("")
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	items := []aggItem{{length: 10, demand: 3}, {length: 7, demand: 2}}
	demand := []int{3, 2}
	results, usedRest, err := s.solvePreciseGroup(client, items, demand,
		[]float64{60}, []string{"6m"}, []float64{25, 25}, []string{"余料A", "余料B"}, 1, 5, 1)
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

	s := NewCutService("")
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	items := []aggItem{{length: 10, demand: 3}}
	_, _, err := s.solvePreciseGroup(client, items, []int{3}, []float64{60}, []string{""}, []float64{25, 25}, []string{"", ""}, 0, 5, 1)
	if err == nil {
		t.Fatal("旧料超卖应报错")
	}
}

// 未配置求解地址时, precise 模式自动回退快速算法, 结果照常产出
func TestBarCutPreciseFallbackWithoutSolver(t *testing.T) {
	s := NewCutService("")
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

// 回归 (真实订单): 20×185.5 + 24×98.6, kerf 0.2, 600 新料。
// 旧实现每个模式只开一根, 高利用率模式 (6×98.6) 用两根后剩余零件被低利用率模式零散
// 吸收 → 13 根/利用率 77.9%; 模式复用后 11 根/92.07%。
// 下界: 零件总长 6076.4 / 600 = 10.13 → ≥11 根, 故 11 根为最优。
func TestBarCutGreedyReusesHighUtilizationPatterns(t *testing.T) {
	s := NewCutService("")
	items := model.BarItemList{}
	for i := 0; i < 20; i++ {
		items = append(items, model.BarItem{Length: 185.5, Spec: "1"})
	}
	for i := 0; i < 24; i++ {
		items = append(items, model.BarItem{Length: 98.6, Spec: "1"})
	}
	resp, err := s.BarCut(1, model.BarRequest{
		Items:        items,
		NewMaterials: []model.BarMaterial{{Label: "1", Length: 600}},
		Loss:         0.2,
	})
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	if resp.Summary.MaterialCount != 11 {
		t.Fatalf("应开 11 根新料, 实际 %d 根 (summary=%+v)", resp.Summary.MaterialCount, resp.Summary)
	}
	long, short := 0, 0
	for _, r := range resp.Results {
		for _, c := range r.Cuts {
			switch c {
			case 185.5:
				long++
			case 98.6:
				short++
			}
		}
	}
	if long != 20 || short != 24 {
		t.Fatalf("零件应全部切出: 185.5×%d (应 20), 98.6×%d (应 24)", long, short)
	}
	if resp.Summary.Utilization < 92 {
		t.Fatalf("利用率应 ≥92%%, 实际 %v%%", resp.Summary.Utilization)
	}
}

// 回归: 通用组多规格下, 兜底开料应选能装下零件的更大规格; 旧实现从 materialLens[0]
// (最短规格) 起选, 装不下时 cuts=0 直接 break, 剩余需求被静默丢弃
func TestBarCutLeftoverFallsBackToLargerSpec(t *testing.T) {
	s := NewCutService("")
	resp, err := s.BarCut(1, model.BarRequest{
		Items: model.IntItems(500, 500, 500, 500, 500),
		NewMaterials: []model.BarMaterial{
			{Label: "short", Length: 100},
			{Label: "long", Length: 600},
		},
	})
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	if len(resp.Results) != 5 {
		t.Fatalf("5 件 500 应全部切出 (开 5 根 600 新料), 实际 %d 根: %+v", len(resp.Results), resp.Results)
	}
	totalCut := 0
	for _, r := range resp.Results {
		totalCut += len(r.Cuts)
	}
	if totalCut != 5 {
		t.Fatalf("期望切出 5 件, 实际 %d", totalCut)
	}
}

// 回归 (线上反馈): 贪心按利用率吃模式把 102 提前耗尽后, 尾部剩 [130×2] 与 [60×6] 各占一根
// 整料 (29 根 / 89.59%)。低利用率料重排应把 [130×2]+[60×6]+[130×4] 三根合并为两根, 降到 28 根。
func TestBarCutResidualRepack(t *testing.T) {
	req := model.BarRequest{
		NewMaterialLength: 600,
		NewMaterials:      []model.BarMaterial{{Length: 600}, {Label: "1", Length: 600}},
		Loss:              0.2,
		UtilizationWeight: 4,
	}
	addItems := func(length float64, n int) {
		for i := 0; i < n; i++ {
			req.Items = append(req.Items, model.BarItem{Length: length, Spec: "1"})
		}
	}
	addItems(185, 36)
	addItems(102, 24)
	addItems(130, 36)
	addItems(60, 30)

	s := NewCutService("")
	resp, err := s.BarCut(1, req)
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	if len(resp.Results) > 28 {
		t.Fatalf("期望 ≤28 根 (优化前 29 根为次优解), 实得 %d", len(resp.Results))
	}
	// 零件守恒 + 每根不超容量 (重排不得丢件/超切)
	want := map[float64]int{185: 36, 102: 24, 130: 36, 60: 30}
	got := map[float64]int{}
	for _, r := range resp.Results {
		used := 0.0
		for i, c := range r.Cuts {
			got[c]++
			used += c
			if i > 0 {
				used += 0.2
			}
		}
		if used > r.TotalLength+1e-6 {
			t.Fatalf("料 #%d 超容量: used=%v total=%v", r.Index, used, r.TotalLength)
		}
	}
	for l, n := range want {
		if got[l] != n {
			t.Fatalf("长度 %v 切割数不符: 期望 %d, 实得 %d", l, n, got[l])
		}
	}
}

// 回归 (线上反馈): 无旧料时 Scraps 为 nil 切片, Marshal 成 "scraps":null, pydantic 的
// list 字段拒绝 null → sidecar 422, 精确模式一直静默回退快速模式 (两种方案结果一样)。
// 请求体三个集合字段必须始终是数组, 不允许 null。
func TestSolvePreciseGroupPayloadArraysNotNull(t *testing.T) {
	raw := map[string]json.RawMessage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"bars": []any{}, "unplaced": []any{}, "iterations": 0, "elapsed_ms": 0})
	}))
	defer srv.Close()

	s := NewCutService("")
	client := &cutSolverClient{baseURL: srv.URL, httpClient: srv.Client()}
	items := []aggItem{{length: 185, demand: 1}}
	_, _, err := s.solvePreciseGroup(client, items, []int{1}, []float64{600}, []string{"1"}, nil, nil, 0.2, 4, 1)
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	for _, key := range []string{"items", "materials", "scraps"} {
		if v, ok := raw[key]; !ok || string(v) == "null" {
			t.Fatalf("请求体 %s 应为数组, 实得: %s", key, v)
		}
	}
}
