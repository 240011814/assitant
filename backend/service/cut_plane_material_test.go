package service

import (
	"net/http"
	"strings"
	"testing"

	"backend/model"
)

// 零件指定材料类型后只能落在同名材料的板上; 通用零件可落任意板
func TestPlaneCutSpecBinding(t *testing.T) {
	s := NewCutService("")
	resp, err := s.PlaneCut(model.BinRequest{
		Items: []model.Item{
			{Label: "背板", Width: 100, Height: 50, Quantity: 2, Spec: "5mm背板"},
			{Label: "门板", Width: 100, Height: 50, Quantity: 2, Spec: "18mm门板"},
			{Label: "通用件", Width: 100, Height: 50, Quantity: 1},
		},
		NewMaterials: []model.Item{
			{Label: "5mm背板", Width: 200, Height: 200},
			{Label: "18mm门板", Width: 200, Height: 200},
		},
		Height:   200,
		Width:    200,
		Strategy: "Guillotine",
	})
	if err != nil {
		t.Fatalf("平面切割不应报错: %v", err)
	}
	for _, r := range resp.Results {
		if r.MaterialType != "5mm背板" && r.MaterialType != "18mm门板" {
			t.Fatalf("多规格下不应再开未命名新板材, 实际板类型: %s", r.MaterialType)
		}
		for _, p := range r.Pieces {
			switch {
			case strings.HasPrefix(p.Label, "背板"):
				if r.MaterialType != "5mm背板" {
					t.Fatalf("背板件 %s 落在了 %s 板上", p.Label, r.MaterialType)
				}
			case strings.HasPrefix(p.Label, "门板"):
				if r.MaterialType != "18mm门板" {
					t.Fatalf("门板件 %s 落在了 %s 板上", p.Label, r.MaterialType)
				}
			}
		}
	}
	placed := 0
	for _, r := range resp.Results {
		placed += len(r.Pieces)
	}
	if placed != 5 {
		t.Fatalf("应排入 5 件, 实际 %d", placed)
	}
	// 汇总按材料类型分组: 两种规格各一组
	if len(resp.Summary.ByMaterialType) != 2 {
		t.Fatalf("按材料类型分组应=2 组, 实际: %+v", resp.Summary.ByMaterialType)
	}
	for _, g := range resp.Summary.ByMaterialType {
		if g.Count != 1 || g.TotalArea != 40000 || g.Utilization <= 0 {
			t.Fatalf("分组统计口径不符: %+v", g)
		}
	}
}

// 引用未定义材料规格的零件应报错 (与一维口径一致)
func TestPlaneCutUndefinedSpec(t *testing.T) {
	s := NewCutService("")
	_, err := s.PlaneCut(model.BinRequest{
		Items:        []model.Item{{Label: "件", Width: 10, Height: 10, Spec: "不存在的规格"}},
		NewMaterials: []model.Item{{Label: "5mm背板", Width: 200, Height: 200}},
		Height:       200,
		Width:        200,
		Strategy:     "MaxRects",
	})
	if err == nil || !strings.Contains(err.Error(), "未定义的材料规格") {
		t.Fatalf("引用未定义材料规格应报错, 实际: %v", err)
	}
}

// 指定规格的零件超出自身规格的新板材 => 未排入 (oversized), 而非报错
func TestPlaneCutSpecOversized(t *testing.T) {
	s := NewCutService("")
	resp, err := s.PlaneCut(model.BinRequest{
		Items: []model.Item{
			{Label: "大件", Width: 300, Height: 200, Quantity: 1, Spec: "小板"},
			{Label: "小件", Width: 100, Height: 100, Quantity: 1, Spec: "小板"},
		},
		NewMaterials: []model.Item{{Label: "小板", Width: 200, Height: 200}},
		Height:       200,
		Width:        200,
		Strategy:     "MaxRects",
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(resp.Unplaced) != 1 || resp.Unplaced[0].Label != "大件" || resp.Unplaced[0].Reason != "oversized" {
		t.Fatalf("大件应未排入(oversized): %+v", resp.Unplaced)
	}
	placed := 0
	for _, r := range resp.Results {
		placed += len(r.Pieces)
	}
	if placed != 1 {
		t.Fatalf("小件应排入, 实际 %d", placed)
	}
}

// 同名旧料预留给指定规格的零件: 通用零件不得消耗被认领的旧料 (旧料尺寸与新板不同, 可按板尺寸区分来源)
func TestPlaneCutScrapReservedForSpec(t *testing.T) {
	s := NewCutService("")
	resp, err := s.PlaneCut(model.BinRequest{
		Items: []model.Item{
			{Label: "背板", Width: 500, Height: 400, Quantity: 1, Spec: "背板料"},
			{Label: "通用", Width: 500, Height: 400, Quantity: 1},
		},
		Materials:    []model.Item{{Label: "背板料", Width: 600, Height: 500, Quantity: 1}},
		NewMaterials: []model.Item{{Label: "背板料", Width: 610, Height: 510}},
		Height:       610,
		Width:        610,
		Strategy:     "MaxRects",
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("应开 2 块板 (旧料+新板), 实际: %+v", func() []model.BinResult {
			out := []model.BinResult{}
			out = append(out, resp.Results...)
			return out
		}())
	}
	for _, r := range resp.Results {
		isScrap := r.MaterialWidth == 600 && r.MaterialHeight == 500
		for _, p := range r.Pieces {
			if isScrap && p.Label != "背板" {
				t.Fatalf("旧料应预留给背板件, 实际排入 %s", p.Label)
			}
			if !isScrap && p.Label != "通用" {
				t.Fatalf("新板应排入通用件, 实际排入 %s", p.Label)
			}
		}
	}
}

// 精确模式请求应携带零件规格与候选板规格, 备用新板按规格补足
func TestSolvePlanePreciseSpecPayload(t *testing.T) {
	var got *cut2dSolverRequest
	srv := newCut2dMockServer(t, func(req *cut2dSolverRequest) any {
		cp := *req
		got = &cp
		return echoSolution(req)
	})
	defer srv.Close()

	s := NewCutService("")
	req := model.BinRequest{
		Items:        []model.Item{{Label: "背板", Width: 100, Height: 50, Quantity: 1, Spec: "背板料"}},
		Materials:    []model.Item{{Label: "背板料", Width: 200, Height: 200, Quantity: 1}},
		NewMaterials: []model.Item{{Label: "背板料", Width: 300, Height: 300}},
		Height:       300,
		Width:        300,
		Strategy:     "Precise",
	}
	fallback, err := s.maxRectsCut(req)
	if err != nil {
		t.Fatalf("启发式失败: %v", err)
	}
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	_ = s.solvePlanePrecise(client, req, fallback)

	if got == nil {
		t.Fatal("未捕获求解请求")
	}
	if len(got.Items) != 1 || got.Items[0].Spec != "背板料" {
		t.Fatalf("请求零件应携带规格: %+v", got.Items)
	}
	// 候选板: 启发式用旧料 1 张 + 备用新板 2 张 (面积下界 1 + 余量 1), 规格均为 背板料
	if len(got.Boards) != 3 {
		t.Fatalf("候选板应=3 (1 旧料 + 2 备用新板), 实际: %+v", got.Boards)
	}
	if !got.Boards[0].IsScrap || got.Boards[0].Spec != "背板料" {
		t.Fatalf("启发式旧料板应标记 is_scrap 且带规格: %+v", got.Boards[0])
	}
	for _, b := range got.Boards[1:] {
		if b.IsScrap || b.Spec != "背板料" || b.Width != 300 || b.Height != 300 {
			t.Fatalf("备用新板规格不符: %+v", b)
		}
	}
}
