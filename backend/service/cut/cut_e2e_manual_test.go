package cut

import (
	"fmt"
	"os"
	"testing"
	"time"

	"backend/model"
)

// 端到端手动验证: 需要本地起 sidecar 才会执行
//   cd baostock && PYTHONPATH=. ./.venv/Scripts/python.exe -m uvicorn main:app --port 8100
//   CUT_E2E_SIDECAR=http://127.0.0.1:8100 go test ./service/ -run TestPlaneCutPreciseE2E -v
// 复现 "二维几十个件, 2 分钟算不出来": 混合规格 ~40 件, 板 200×160。
func TestPlaneCutPreciseE2E(t *testing.T) {
	baseURL := os.Getenv("CUT_E2E_SIDECAR")
	if baseURL == "" {
		t.Skip("未设置 CUT_E2E_SIDECAR, 跳过端到端验证")
	}
	s := NewCutService(baseURL)
	req := model.BinRequest{
		Items: []model.Item{
			{Label: "A", Width: 120, Height: 80, Quantity: 8},
			{Label: "B", Width: 100, Height: 60, Quantity: 6},
			{Label: "C", Width: 80, Height: 50, Quantity: 12},
			{Label: "D", Width: 60, Height: 45, Quantity: 10},
			{Label: "E", Width: 150, Height: 90, Quantity: 5},
		},
		Width:    200,
		Height:   160,
		Strategy: "Precise",
	}
	start := time.Now()
	// 对照组: MaxRects 启发式 (精确模式的 warm start 来源)
	heu, heuErr := s.maxRectsCut(req)
	if heuErr != nil {
		t.Fatalf("启发式失败: %v", heuErr)
	}
	t.Logf("启发式 MaxRects: 用板 %d, 利用率 %v%% (耗时 %v)", len(heu.Results), heu.Summary.Utilization, time.Since(start))
	resp, err := s.PlaneCut(req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("精确求解失败 (耗时 %v): %v", elapsed, err)
	}
	placed := 0
	var pieceArea, boardArea float64
	for _, r := range resp.Results {
		placed += len(r.Pieces)
		boardArea += r.MaterialWidth * r.MaterialHeight
		var used float64
		for _, p := range r.Pieces {
			used += p.W * p.H
		}
		t.Logf("板%d %vx%v 件数=%d 已用面积=%.0f 利用率=%v%%", r.BinID, r.MaterialWidth, r.MaterialHeight, len(r.Pieces), used, r.Utilization)
		pieceArea += used
	}
	unplaced := 0
	for _, u := range resp.Unplaced {
		unplaced += u.Quantity
	}
	t.Logf("耗时 %v, 用板 %d, 排入 %d 件, 未排入 %d 件, 零件总面积=%.0f, 板总面积=%.0f, 汇总利用率=%v%%",
		elapsed, len(resp.Results), placed, unplaced, pieceArea, boardArea, resp.Summary.Utilization)
	if placed+unplaced != 41 {
		t.Fatalf("件数守恒失败: placed=%d unplaced=%d 应 41", placed, unplaced)
	}
	fmt.Printf("E2E: %v 用板=%d 排入=%d 未排=%d 利用率=%v%%\n",
		elapsed, len(resp.Results), placed, unplaced, resp.Summary.Utilization)
}
