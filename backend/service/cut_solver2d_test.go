package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/model"
)

// mock sidecar: 解码请求后由 mutate 构造响应
func newCut2dMockServer(t *testing.T, mutate func(req *cut2dSolverRequest) any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cut2dSolverRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解码请求失败: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mutate(&req))
	}))
}

// 把请求里的 initial_solution 原样作为求解结果回显 (等价于"未改进启发式", 且几何有效)
func echoSolution(req *cut2dSolverRequest) cut2dSolverResponse {
	return cut2dSolverResponse{
		Pieces:    req.InitialSolution,
		Status:    "optimal",
		ElapsedMS: 1,
	}
}

func cut2dTestRequest() model.BinRequest {
	return model.BinRequest{
		Items: []model.Item{
			{Label: "小件", Width: 60, Height: 40, Quantity: 2},
			{Label: "中件", Width: 80, Height: 50, Quantity: 1},
		},
		Width:    200,
		Height:   200,
		Strategy: "Precise",
	}
}

// 精确求解组装: mock 回显启发式解, 应组装出与启发式等价的布局 (板数/件数/坐标/宽高互换)
func TestSolvePlanePreciseAssembly(t *testing.T) {
	srv := newCut2dMockServer(t, func(req *cut2dSolverRequest) any {
		if len(req.Boards) == 0 || len(req.InitialSolution) == 0 {
			t.Errorf("请求应携带候选板与 initial_solution")
		}
		return echoSolution(req)
	})
	defer srv.Close()

	s := NewCutService("")
	req := cut2dTestRequest()
	fallback, err := s.maxRectsCut(req)
	if err != nil {
		t.Fatalf("启发式失败: %v", err)
	}
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	resp := s.solvePlanePrecise(client, req, fallback)

	placed := 0
	for _, r := range resp.Results {
		placed += len(r.Pieces)
		if r.Utilization <= 0 {
			t.Errorf("板利用率未计算: %+v", r)
		}
	}
	if placed != 3 {
		t.Fatalf("应排入 3 件, 实际 %d", placed)
	}
	// 组装的宽高必须与启发式一致 (含旋转互换口径)
	heuPieces := map[string]model.Piece{}
	for _, r := range fallback.Results {
		for _, p := range r.Pieces {
			heuPieces[p.Label] = p
		}
	}
	for _, r := range resp.Results {
		for _, p := range r.Pieces {
			hp, ok := heuPieces[p.Label]
			if !ok {
				t.Fatalf("结果件 %s 在启发式解中不存在", p.Label)
			}
			if p.W != hp.W || p.H != hp.H || p.Rotated != hp.Rotated {
				t.Fatalf("件 %s 宽高/旋转不符: (%v,%v,%v) vs (%v,%v,%v)", p.Label, p.W, p.H, p.Rotated, hp.W, hp.H, hp.Rotated)
			}
		}
	}
}

// oversized 未排入应被接受 (与启发式同口径), 其余零件照常组装
func TestSolvePlanePreciseOversizedAccepted(t *testing.T) {
	srv := newCut2dMockServer(t, func(req *cut2dSolverRequest) any {
		resp := echoSolution(req)
		resp.Unplaced = []cut2dUnplaced{{ItemIndex: 0, Count: 1, Reason: "oversized"}}
		return resp
	})
	defer srv.Close()

	s := NewCutService("")
	req := model.BinRequest{
		Items: []model.Item{
			{Label: "大件", Width: 3000, Height: 2000, Quantity: 1}, // 超出所有板
			{Label: "小件", Width: 400, Height: 300, Quantity: 2},
		},
		Width:    1200,
		Height:   1000,
		Strategy: "Precise",
	}
	fallback, err := s.maxRectsCut(req)
	if err != nil {
		t.Fatalf("启发式失败: %v", err)
	}
	client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
	resp := s.solvePlanePrecise(client, req, fallback)

	placed := 0
	for _, r := range resp.Results {
		placed += len(r.Pieces)
	}
	if placed != 2 {
		t.Fatalf("应排入 2 件小件, 实际 %d", placed)
	}
	if len(resp.Unplaced) != 1 || resp.Unplaced[0].Reason != "oversized" {
		t.Fatalf("大件应进入未排入清单 (oversized): %+v", resp.Unplaced)
	}
}

// 求解服务 500 / "unfit" 未排入 / 解不完备 / 规模超限 → 一律回退启发式结果
func TestSolvePlanePreciseFallback(t *testing.T) {
	s := NewCutService("")
	req := cut2dTestRequest()
	fallback, err := s.maxRectsCut(req)
	if err != nil {
		t.Fatalf("启发式失败: %v", err)
	}

	t.Run("HTTP500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
		if resp := s.solvePlanePrecise(client, req, fallback); resp != fallback {
			t.Fatal("HTTP 500 应回退启发式结果")
		}
	})

	t.Run("UnfitUnplaced", func(t *testing.T) {
		srv := newCut2dMockServer(t, func(req *cut2dSolverRequest) any {
			resp := echoSolution(req)
			resp.Unplaced = []cut2dUnplaced{{ItemIndex: 0, Count: 1, Reason: "unfit"}}
			return resp
		})
		defer srv.Close()
		client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
		if resp := s.solvePlanePrecise(client, req, fallback); resp != fallback {
			t.Fatal("存在 unfit 未排入应回退启发式结果")
		}
	})

	t.Run("Incomplete", func(t *testing.T) {
		srv := newCut2dMockServer(t, func(req *cut2dSolverRequest) any {
			resp := echoSolution(req)
			if len(resp.Pieces) > 0 {
				resp.Pieces = resp.Pieces[:len(resp.Pieces)-1] // 丢一件
			}
			return resp
		})
		defer srv.Close()
		client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
		if resp := s.solvePlanePrecise(client, req, fallback); resp != fallback {
			t.Fatal("解不完备应回退启发式结果")
		}
	})

	t.Run("TooManyPieces", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_ = json.NewEncoder(w).Encode(echoSolution(&cut2dSolverRequest{}))
		}))
		defer srv.Close()
		bigReq := model.BinRequest{
			Items:  []model.Item{{Label: "件", Width: 10, Height: 10, Quantity: cut2dMaxPieces + 1}},
			Width:  6000,
			Height: 4000,
		}
		bigFallback, err := s.maxRectsCut(bigReq)
		if err != nil {
			t.Fatalf("启发式失败: %v", err)
		}
		client := &cutSolverClient{baseURL: srv.URL, httpClient: &http.Client{}}
		if resp := s.solvePlanePrecise(client, bigReq, bigFallback); resp != bigFallback {
			t.Fatal("展开件数超限应回退启发式结果")
		}
		if called {
			t.Fatal("超限时不应调用求解服务")
		}
	})
}

// 未配置求解地址时 Precise 策略整体回退 MaxRects, 结果照常产出
func TestPlaneCutPreciseFallbackWithoutSolver(t *testing.T) {
	s := NewCutService("")
	resp, err := s.PlaneCut(model.BinRequest{
		Items: []model.Item{
			{Label: "件A", Width: 400, Height: 300, Quantity: 3},
			{Label: "件B", Width: 200, Height: 200, Quantity: 2},
		},
		Width:    1000,
		Height:   800,
		Strategy: "Precise",
	})
	if err != nil {
		t.Fatalf("Precise 回退路径不应报错: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("回退后应有结果")
	}
	total := 0
	for _, r := range resp.Results {
		total += len(r.Pieces)
	}
	if total != 5 {
		t.Fatalf("应排入 5 件, 实际 %d", total)
	}
}
