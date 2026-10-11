package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/model"
)

// ===== P0 回归: 精确模式不得丢弃旧料直配根 =====

// precise 模式 + 与旧料等长的零件: preAssignExactScraps 产出的直配根必须出现在最终方案里。
// 旧实现在 precise 分支漏 append(fixed), 等长零件从方案中消失 (照单下料会漏切)。
func TestBarCutPreciseKeepsDirectAssignedScraps(t *testing.T) {
	resp := map[string]any{
		"bars": []map[string]any{
			{"source": "material", "material_index": 0, "pattern": []int{1}, "count": 1},
		},
		"unplaced":   []map[string]any{},
		"iterations": 3,
		"elapsed_ms": 50,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewCutService(srv.URL)
	result, err := s.BarCut(1, model.BarRequest{
		Items:        model.BarItemList{{Length: 100, Spec: "1"}, {Length: 100, Spec: "1"}},
		// 旧料 label 必须与零件 spec 同名才会被该规格组认领
		Materials:    []model.BarMaterial{{Label: "1", Length: 100}, {Label: "1", Length: 300}},
		NewMaterials: []model.BarMaterial{{Label: "1", Length: 600}},
		Mode:         model.BarModePrecise,
	})
	if err != nil {
		t.Fatalf("精确求解失败: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("应产出 2 根 (直配旧料 100 + sidecar 新料 600), 实得 %d 根: %+v", len(result.Results), result.Results)
	}
	// 第一根 = 与零件等长的旧料直配根
	first := result.Results[0]
	if first.TotalLength != 100 || len(first.Cuts) != 1 || first.Cuts[0] != 100 || first.MaterialType != "1" {
		t.Fatalf("直配旧料根不符: %+v", first)
	}
	// 第二根 = sidecar 新料根
	second := result.Results[1]
	if second.TotalLength != 600 || len(second.Cuts) != 1 || second.Cuts[0] != 100 {
		t.Fatalf("sidecar 新料根不符: %+v", second)
	}
	// 零件守恒: 2 件 100 全部切出
	cutTotal := 0
	for _, r := range result.Results {
		cutTotal += len(r.Cuts)
	}
	if cutTotal != 2 {
		t.Fatalf("应切出 2 件, 实得 %d", cutTotal)
	}
}

// 切割损耗 ≥ 最短材料规格长度属非法输入 (每根最多切 1 件且大件全不可行), 直接报错
func TestBarCutLossMustBeBelowMinSpec(t *testing.T) {
	s := NewCutService("")
	_, err := s.BarCut(1, model.BarRequest{
		Items:             model.IntItems(100),
		NewMaterialLength: 600,
		Loss:              600,
	})
	if err == nil || !strings.Contains(err.Error(), "切割损耗") {
		t.Fatalf("损耗 ≥ 最短规格长度应报错, 实得: %v", err)
	}
}

// ===== P1: 2D 锯缝 =====

// checkPlaneLayout 校验排版结果: 零件不重叠、不超出板材; kerf > 0 时同排/同列相邻零件间距 ≥ kerf
func checkPlaneLayout(t *testing.T, results []model.BinResult, kerf float64) {
	t.Helper()
	const eps = 1e-6
	for _, br := range results {
		for _, p := range br.Pieces {
			if p.X < -eps || p.Y < -eps || p.X+p.W > br.MaterialWidth+eps || p.Y+p.H > br.MaterialHeight+eps {
				t.Fatalf("板 %d 零件 %s 超出板材: (%v,%v,%v,%v) vs %vx%v",
					br.BinID, p.Label, p.X, p.Y, p.W, p.H, br.MaterialWidth, br.MaterialHeight)
			}
		}
		for i := 0; i < len(br.Pieces); i++ {
			for j := i + 1; j < len(br.Pieces); j++ {
				a, b := br.Pieces[i], br.Pieces[j]
				xOverlap := a.X < b.X+b.W-eps && a.X+a.W > b.X+eps
				yOverlap := a.Y < b.Y+b.H-eps && a.Y+a.H > b.Y+eps
				if xOverlap && yOverlap {
					t.Fatalf("板 %d 零件重叠: %s 与 %s", br.BinID, a.Label, b.Label)
				}
				if kerf <= 0 {
					continue
				}
				// y 向重叠 (同一行) 时要求 x 向间隙 ≥ kerf; x 向重叠 (同一列) 时要求 y 向间隙 ≥ kerf
				if yOverlap {
					gap := b.X - (a.X + a.W)
					if gap < 0 {
						gap = a.X - (b.X + b.W)
					}
					if gap < kerf-eps && (a.X+a.W <= b.X+eps || b.X+b.W <= a.X+eps) {
						t.Fatalf("板 %d 同行零件间距 %v 小于锯缝 %v: %s 与 %s", br.BinID, gap, kerf, a.Label, b.Label)
					}
				}
				if xOverlap {
					gap := b.Y - (a.Y + a.H)
					if gap < 0 {
						gap = a.Y - (b.Y + b.H)
					}
					if gap < kerf-eps && (a.Y+a.H <= b.Y+eps || b.Y+b.H <= a.Y+eps) {
						t.Fatalf("板 %d 同列零件间距 %v 小于锯缝 %v: %s 与 %s", br.BinID, gap, kerf, a.Label, b.Label)
					}
				}
			}
		}
	}
}

// Guillotine/MaxRects 带锯缝: 零件按膨胀尺寸装箱 — 180×60 的板 kerf=0 时 3 件 60×60 一板排下,
// kerf=5 时一排只放得下 2 件, 第 3 件开新板; 输出零件保持真实宽高
func TestPlaneCutKerfExpandsFootprint(t *testing.T) {
	cases := []struct {
		strategy string
	}{
		{"Guillotine"},
		{"MaxRects"},
	}
	for _, tc := range cases {
		req := model.BinRequest{
			Items:    []model.Item{{Label: "A", Width: 60, Height: 60, Quantity: 3}},
			Width:    180,
			Height:   60,
			Strategy: tc.strategy,
		}
		s := NewCutService("")
		noKerf, err := s.PlaneCut(req)
		if err != nil {
			t.Fatalf("%s 无锯缝求解失败: %v", tc.strategy, err)
		}
		if len(noKerf.Results) != 1 || len(noKerf.Results[0].Pieces) != 3 {
			t.Fatalf("%s 无锯缝应 1 板 3 件, 实得 %+v", tc.strategy, noKerf.Results)
		}

		req.Kerf = 5
		withKerf, err := s.PlaneCut(req)
		if err != nil {
			t.Fatalf("%s 带锯缝求解失败: %v", tc.strategy, err)
		}
		pieceTotal := 0
		for _, br := range withKerf.Results {
			pieceTotal += len(br.Pieces)
		}
		if pieceTotal != 3 {
			t.Fatalf("%s 带锯缝应全部切出 3 件, 实得 %d", tc.strategy, pieceTotal)
		}
		if len(withKerf.Results) != 2 {
			t.Fatalf("%s 带锯缝 5cm 应开 2 板 (一排只容 2 件), 实得 %d 板", tc.strategy, len(withKerf.Results))
		}
		for _, br := range withKerf.Results {
			for _, p := range br.Pieces {
				if p.W != 60 || p.H != 60 {
					t.Fatalf("%s 输出零件应保持真实宽高 60×60, 实得 %vx%v", tc.strategy, p.W, p.H)
				}
			}
		}
		checkPlaneLayout(t, withKerf.Results, 5)
	}
}

// ===== P1: 平面最大空矩形 (余料入库候选) =====

// 单件 40×40 置于 100×100 板角: 最大空矩形面积应为 6000
func TestLargestEmptyRectSinglePiece(t *testing.T) {
	r := largestEmptyRect(100, 100, []model.Piece{{X: 0, Y: 0, W: 40, H: 40}})
	if r == nil || r.W*r.H != 6000 {
		t.Fatalf("最大空矩形面积应为 6000, 实得 %+v", r)
	}
}

// 左右两条竖料夹出中间通长空隙: 最大空矩形应为中间 20×100
func TestLargestEmptyRectMiddleGap(t *testing.T) {
	r := largestEmptyRect(100, 100, []model.Piece{
		{X: 0, Y: 0, W: 40, H: 100},
		{X: 60, Y: 0, W: 40, H: 100},
	})
	if r == nil || r.X != 40 || r.Y != 0 || r.W != 20 || r.H != 100 {
		t.Fatalf("应找到中间 20×100 空隙, 实得 %+v", r)
	}
}

// 件数超上限时不计算 (防御 O(n²) 膨胀); 空板/零尺寸板返回 nil
func TestLargestEmptyRectGuards(t *testing.T) {
	if largestEmptyRect(100, 100, nil) != nil {
		t.Fatal("空板应返回 nil")
	}
	if largestEmptyRect(0, 100, []model.Piece{{X: 0, Y: 0, W: 1, H: 1}}) != nil {
		t.Fatal("零尺寸板应返回 nil")
	}
	pieces := make([]model.Piece, 151)
	for i := range pieces {
		pieces[i] = model.Piece{X: float64(i % 10), Y: float64(i / 10), W: 0.5, H: 0.5}
	}
	if largestEmptyRect(100, 100, pieces) != nil {
		t.Fatal("件数超上限应返回 nil")
	}
}

// PlaneCut 结果应携带每板最大空矩形
func TestPlaneCutAttachesScrapRect(t *testing.T) {
	req := model.BinRequest{
		Items:    []model.Item{{Label: "A", Width: 40, Height: 40, Quantity: 1}},
		Width:    100,
		Height:   100,
		Strategy: "Guillotine",
	}
	s := NewCutService("")
	resp, err := s.PlaneCut(req)
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("应 1 板, 实得 %d", len(resp.Results))
	}
	rect := resp.Results[0].ScrapRect
	// 残料区是 L 形, 最大"矩形"为 100×60=6000 (不是残料总面积 8400)
	if rect == nil || rect.W*rect.H != 6000 {
		t.Fatalf("100×100 板切 40×40 件后最大空矩形面积应 6000, 实得 %+v", rect)
	}
}

// ===== P1: 输入上限 =====

// 平面切割展开件数超上限直接报错 (不再静默展开巨量对象)
func TestPlaneCutRejectsTooManyPieces(t *testing.T) {
	req := model.BinRequest{
		Items:    []model.Item{{Label: "A", Width: 10, Height: 10, Quantity: planeMaxPieces + 1}},
		Width:    100,
		Height:   100,
		Strategy: "Guillotine",
	}
	s := NewCutService("")
	_, err := s.PlaneCut(req)
	if err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("展开件数超上限应报错, 实得: %v", err)
	}
}

// 产品单件数上限: 单件 Count > 9999 拒绝保存
func TestValidateProductSpecCountCap(t *testing.T) {
	spec := model.CutProductSpec{Items: []model.CutProductItem{{
		Type: "glass", Width: 100, Height: 100, Series: "5", Count: 10000,
		Grid: model.CutWindowGrid{Cols: []float64{100}, Rows: []float64{100}, Cells: [][]int{{1}}},
	}}}
	if err := validateProductSpec(spec); err == nil || !strings.Contains(err.Error(), "数量过多") {
		t.Fatalf("Count 10000 应报数量过多, 实得: %v", err)
	}
}
