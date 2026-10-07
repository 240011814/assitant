package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"backend/model"
)

// ===== 二维精确求解 sidecar 客户端 (cut_api/solver_2d, 与一维同款 sidecar 模式) =====
// 策略 Precise: 先跑 MaxRects 得到保底解, 再把启发式解作为完整 warm start 传给 sidecar
// CP-SAT (NoOverlap2D) 优化。未配置求解地址/规模超限/求解失败/解不完备/超时一律回退
// 启发式结果, 保证精确模式的产出永不劣于 MaxRects。

const (
	cut2dSolverTimeout = 150 * time.Second // HTTP 超时 = 求解预算 (2 分钟) + 组装余量
	cut2dSolverBudget  = 120000            // 传给 sidecar 的求解预算 ms (限时 2 分钟)
	cut2dMaxPieces     = 400               // 展开件数上限, 超过直接回退启发式
	cut2dMaxBoards     = 40                // 候选板材上限 (启发式用板 + 剩余旧料 + 备用新板)
	cut2dMaxPairs      = 9600              // 件×板 乘积上限 (模型规模防御)
	cut2dMaxExtraScrap = 12                // 启发式未消费旧料的候选追加上限
	cut2dNewBoardName  = "新板材"            // 与 buildPlaneMaterials 的新板标注一致
)

type cut2dSolverItem struct {
	Label  string  `json:"label"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Demand int     `json:"demand"`
	Spec   string  `json:"spec,omitempty"` // 材料类型约束: 非空时只能排入同 spec 的板
}

type cut2dSolverBoard struct {
	Label   string  `json:"label"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	IsScrap bool    `json:"is_scrap"`
	Spec    string  `json:"spec,omitempty"` // 板的材料类型 (旧料=来源类型名, 新板=规格名)
}

type cut2dSolverPlacement struct {
	Item    int     `json:"item"`  // items 下标
	Board   int     `json:"board"` // boards 下标
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Rotated bool    `json:"rotated"`
}

type cut2dSolverRequest struct {
	Items           []cut2dSolverItem      `json:"items"`
	Boards          []cut2dSolverBoard     `json:"boards"`
	InitialSolution []cut2dSolverPlacement `json:"initial_solution"`
	TimeLimitMS     int                    `json:"time_limit_ms"`
}

type cut2dUnplaced struct {
	ItemIndex int    `json:"item_index"`
	Count     int    `json:"count"`
	Reason    string `json:"reason"` // "oversized" | "unfit"
}

type cut2dSolverResponse struct {
	Pieces    []cut2dSolverPlacement `json:"pieces"`
	Unplaced  []cut2dUnplaced        `json:"unplaced"`
	Status    string                 `json:"status"` // optimal | feasible | failed
	ElapsedMS int                    `json:"elapsed_ms"`
}

// solvePlanePrecise 二维精确求解; 任何失败一律返回 fallback (启发式解), 调用方无需判错
func (s *CutService) solvePlanePrecise(client *cutSolverClient, req model.BinRequest, fallback *model.PlaneCutResponse) *model.PlaneCutResponse {
	resp, ok := s.trySolvePlanePrecise(client, req)
	if !ok {
		return fallback
	}
	return resp
}

func (s *CutService) trySolvePlanePrecise(client *cutSolverClient, req model.BinRequest) (*model.PlaneCutResponse, bool) {
	// 展开零件 (顺序与 sidecar 的展开一致: items 顺序 × quantity; label 规则同 expandItems)
	type expPiece struct {
		typeIdx int
		label   string
		spec    string
		w, h    float64
	}
	var expanded []expPiece
	for ti, it := range req.Items {
		q := it.Quantity
		if q < 1 {
			q = 1
		}
		for i := 0; i < q; i++ {
			label := it.Label
			if q > 1 {
				label = it.Label + "_" + strconv.Itoa(i+1)
			}
			expanded = append(expanded, expPiece{typeIdx: ti, label: label, spec: planeItemSpec(it), w: it.Width, h: it.Height})
		}
	}
	if len(expanded) == 0 || len(expanded) > cut2dMaxPieces {
		log.Printf("[Cut] 精确模式跳过: 展开件数 %d 超限 (> %d)", len(expanded), cut2dMaxPieces)
		return nil, false
	}

	// 候选板 = 启发式用板 (承载完整 warm start) + 未消费旧料 + 备用新板
	heu, err := s.maxRectsCut(req)
	if err != nil {
		log.Printf("[Cut] MaxRects 启发式失败: %v", err)
		return nil, false
	}
	type candBoard struct {
		label   string
		w, h    float64
		isScrap bool
	}
	var boards []candBoard
	// 启发式用板按 (类型名, 尺寸) + 剩余额度匹配旧料; 未匹配到的视为新板材
	// (多规格后新板名即规格名, 不能再按 "新板材" 字面区分)
	scrapAvail := make([]int, len(req.Materials))
	for i, m := range req.Materials {
		q := m.Quantity
		if q < 1 {
			q = 1
		}
		scrapAvail[i] = q
	}
	for _, r := range heu.Results {
		isScrap := false
		for i, m := range req.Materials {
			if scrapAvail[i] > 0 && planeScrapLabel(m) == r.MaterialType &&
				nearlyEq(m.Width, r.MaterialWidth) && nearlyEq(m.Height, r.MaterialHeight) {
				scrapAvail[i]--
				isScrap = true
				break
			}
		}
		boards = append(boards, candBoard{label: r.MaterialType, w: r.MaterialWidth, h: r.MaterialHeight, isScrap: isScrap})
	}
	// 剩余旧料候选 (上限内追加): 求解器可能做出比启发式更优的新旧取舍
	extra := 0
	for i, m := range req.Materials {
		for scrapAvail[i] > 0 && extra < cut2dMaxExtraScrap && len(boards) < cut2dMaxBoards {
			boards = append(boards, candBoard{label: planeScrapLabel(m), w: m.Width, h: m.Height, isScrap: true})
			scrapAvail[i]--
			extra++
		}
		if len(boards) >= cut2dMaxBoards {
			break
		}
	}
	// 备用新板: 每种规格补到 max(启发式用板数, 该规格专属零件面积下界) + 1 张
	// (+1 吸收通用件摊入; 未命名单一规格退化为旧口径 面积下界+2)。求解失败仍有 warm start 保底。
	specRows := planeSpecRows(req)
	if len(boards) < cut2dMaxBoards {
		usedNew := make(map[string]int)
		for _, b := range boards {
			if !b.isScrap {
				usedNew[b.label]++
			}
		}
		areaOfSpec := make(map[string]float64)
		hasGeneric := false
		for _, ep := range expanded {
			if ep.spec == "" {
				hasGeneric = true
			}
			areaOfSpec[ep.spec] += ep.w * ep.h
		}
		for _, sp := range specRows {
			if len(boards) >= cut2dMaxBoards {
				break
			}
			lb := int(math.Ceil(areaOfSpec[sp.name] / (sp.width * sp.height)))
			target := usedNew[sp.name] + 1
			if lb+1 > target {
				target = lb + 1
			}
			if hasGeneric {
				target++
			}
			for i := usedNew[sp.name]; i < target && len(boards) < cut2dMaxBoards; i++ {
				boards = append(boards, candBoard{label: sp.name, w: sp.width, h: sp.height})
			}
		}
	}
	if len(boards) == 0 || len(boards) > cut2dMaxBoards || len(expanded)*len(boards) > cut2dMaxPairs {
		log.Printf("[Cut] 精确模式跳过: 模型规模超限 (件 %d × 板 %d)", len(expanded), len(boards))
		return nil, false
	}

	// 启发式解 → hint: 按 label+尺寸匹配展开件 (启发式件按板序遍历, 同类型出现序的板下标
	// 单调不减, 与 sidecar 的对称性破除约束同向, hint 可直接受用)
	type heuPiece struct {
		label   string
		w, h    float64
		rotated bool
		board   int
		x, y    float64
	}
	var heuPieces []heuPiece
	for bi, r := range heu.Results {
		for _, p := range r.Pieces {
			heuPieces = append(heuPieces, heuPiece{label: p.Label, w: p.W, h: p.H, rotated: p.Rotated, board: bi, x: p.X, y: p.Y})
		}
	}
	matched := make([]bool, len(heuPieces))
	initial := make([]cut2dSolverPlacement, 0, len(heuPieces))
	for ti, ep := range expanded {
		for hi, hp := range heuPieces {
			if matched[hi] || hp.label != ep.label {
				continue
			}
			if (nearlyEq(ep.w, hp.w) && nearlyEq(ep.h, hp.h)) || (nearlyEq(ep.w, hp.h) && nearlyEq(ep.h, hp.w)) {
				matched[hi] = true
				initial = append(initial, cut2dSolverPlacement{Item: ti, Board: hp.board, X: hp.x, Y: hp.y, Rotated: hp.rotated})
				break
			}
		}
	}

	reqBody := cut2dSolverRequest{TimeLimitMS: cut2dSolverBudget, InitialSolution: initial}
	for _, it := range req.Items {
		q := it.Quantity
		if q < 1 {
			q = 1
		}
		reqBody.Items = append(reqBody.Items, cut2dSolverItem{Label: it.Label, Width: it.Width, Height: it.Height, Demand: q, Spec: planeItemSpec(it)})
	}
	for _, b := range boards {
		reqBody.Boards = append(reqBody.Boards, cut2dSolverBoard{Label: b.label, Width: b.w, Height: b.h, IsScrap: b.isScrap, Spec: b.label})
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[Cut] 精确模式请求序列化失败: %v", err)
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), cut2dSolverTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/cut2d/solve", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[Cut] 精确模式请求构建失败: %v", err)
		return nil, false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{Timeout: cut2dSolverTimeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[Cut] 精确求解失败 (回退 MaxRects): %v", err)
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Printf("[Cut] 精确求解响应读取失败: %v", err)
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Cut] 求解服务返回 HTTP %d: %s", resp.StatusCode, truncateRunes(string(body), 200))
		return nil, false
	}
	var out cut2dSolverResponse
	if err := json.Unmarshal(body, &out); err != nil {
		log.Printf("[Cut] 解析求解响应失败: %v", err)
		return nil, false
	}
	if out.Status != "optimal" && out.Status != "feasible" {
		log.Printf("[Cut] 精确求解无可用解 (回退 MaxRects): status=%s elapsed=%dms", out.Status, out.ElapsedMS)
		return nil, false
	}

	// 未排入: 只接受 oversized (与启发式口径一致); "放得下却未排" 视为解不完备, 整体回退
	var unplaced []model.UnplacedItem
	oversizedCount := 0
	for _, u := range out.Unplaced {
		if u.ItemIndex < 0 || u.ItemIndex >= len(req.Items) {
			log.Printf("[Cut] 精确求解响应非法: item_index 越界")
			return nil, false
		}
		if u.Reason != "oversized" {
			log.Printf("[Cut] 精确求解存在未排入零件 (回退 MaxRects): item=%d count=%d reason=%s", u.ItemIndex, u.Count, u.Reason)
			return nil, false
		}
		it := req.Items[u.ItemIndex]
		unplaced = append(unplaced, model.UnplacedItem{Label: it.Label, Width: it.Width, Height: it.Height, Quantity: u.Count, Reason: "oversized"})
		oversizedCount += u.Count
	}

	// 组装结果: 板序保持候选序, 空板跳过; 旋转件宽高互换
	pieceByBoard := make(map[int][]model.Piece)
	placedTotal := 0
	for _, p := range out.Pieces {
		if p.Board < 0 || p.Board >= len(boards) || p.Item < 0 || p.Item >= len(expanded) {
			log.Printf("[Cut] 精确求解响应非法: placement 越界")
			return nil, false
		}
		ep := expanded[p.Item]
		w, h := ep.w, ep.h
		if p.Rotated {
			w, h = ep.h, ep.w
		}
		pieceByBoard[p.Board] = append(pieceByBoard[p.Board], model.Piece{
			Label: ep.label, X: round2(p.X), Y: round2(p.Y), W: round2(w), H: round2(h), Rotated: p.Rotated,
		})
		placedTotal++
	}
	if placedTotal+oversizedCount != len(expanded) {
		log.Printf("[Cut] 精确求解解不完备 (回退 MaxRects): placed=%d oversized=%d expanded=%d", placedTotal, oversizedCount, len(expanded))
		return nil, false
	}

	var results []model.BinResult
	binID := 0
	for bi, b := range boards {
		pieces := pieceByBoard[bi]
		if len(pieces) == 0 {
			continue
		}
		br := model.BinResult{BinID: binID, MaterialType: b.label, MaterialWidth: b.w, MaterialHeight: b.h, Pieces: pieces}
		calculateUtilization(&br)
		results = append(results, br)
		binID++
	}
	return &model.PlaneCutResponse{Results: results, Unplaced: unplaced}, true
}

func nearlyEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}
