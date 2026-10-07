package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend/model"
)

// ===== OR-Tools 精确求解 sidecar 客户端 (cut_api/, 与 baostock 同款 sidecar 模式) =====
// 求解地址取 Baostock ENV `BAOSTOCK_API_URL` (sidecar 路由与行情同进程; 空 = 未启用精确模式)。
// 精确模式下 sidecar 失败/超时/解不完备时 BarCut 直接报错给前端, 不静默回退内置快速算法。

const (
	cutSolverTimeout = 20 * time.Second
	cutSolverBudget  = 8000 // 传给 sidecar 的求解预算 ms (含列生成 + 整数化)
)

type cutSolverClient struct {
	baseURL    string
	httpClient *http.Client
}

// solverClient 按 Baostock ENV 注入的地址构建客户端; 未配置返回 nil
func (s *CutService) solverClient() *cutSolverClient {
	url := strings.TrimSpace(s.baostockURL)
	if url == "" {
		return nil
	}
	return &cutSolverClient{baseURL: strings.TrimRight(url, "/"), httpClient: &http.Client{Timeout: cutSolverTimeout}}
}

type cutSolverRequest struct {
	Kerf              float64             `json:"kerf"`
	Items             []cutSolverItem     `json:"items"`
	Materials         []cutSolverMaterial `json:"materials"`
	Scraps            []cutSolverScrap    `json:"scraps"`
	TimeLimitMS       int                 `json:"time_limit_ms"`
	UtilizationWeight float64             `json:"utilization_weight"` // stage2 利用率偏好权重 (与前端滑杆同源)
}

type cutSolverItem struct {
	Length float64 `json:"length"`
	Demand int     `json:"demand"`
}

type cutSolverMaterial struct {
	Label  string  `json:"label"`
	Length float64 `json:"length"`
}

type cutSolverScrap struct {
	Length float64 `json:"length"`
}

type cutSolverBar struct {
	Source        string   `json:"source"` // "material" | "scrap"
	MaterialIndex *int     `json:"material_index"`
	ScrapLength   *float64 `json:"scrap_length"`
	Pattern       []int    `json:"pattern"` // 每种零件的切割数 (按 items 下标)
	Count         int      `json:"count"`   // 同模式同来源根数
}

type cutSolverUnplaced struct {
	ItemIndex int `json:"item_index"`
	Count     int `json:"count"`
}

type cutSolverResponse struct {
	Bars       []cutSolverBar      `json:"bars"`
	Unplaced   []cutSolverUnplaced `json:"unplaced"`
	Iterations int                 `json:"iterations"`
	ElapsedMS  int                 `json:"elapsed_ms"`
}

// solvePreciseGroup 对一个规格组调 sidecar 精确求解。
// 返回组装好的结果与被消费旧料下标 (相对 restScraps); 任何异常返回 error 由调用方直接报给前端。
func (s *CutService) solvePreciseGroup(client *cutSolverClient, items []aggItem, demand []int,
	materialLens []float64, materialLabels []string, restScraps []float64, restLabels []string,
	kerf float64, utilWeight float64, startIdx int) ([]model.BarResult, []int, error) {

	// 三个集合显式初始化为空数组: nil 切片会被 Marshal 成 null, pydantic 的 list 字段
	// 不接受 null, 会 422 (线上问题: 无旧料时 scraps=null 导致精确模式始终回退)
	reqBody := cutSolverRequest{
		Kerf:              kerf,
		Items:             []cutSolverItem{},
		Materials:         []cutSolverMaterial{},
		Scraps:            []cutSolverScrap{},
		TimeLimitMS:       cutSolverBudget,
		UtilizationWeight: utilWeight,
	}
	for t := range items {
		reqBody.Items = append(reqBody.Items, cutSolverItem{Length: items[t].length, Demand: demand[t]})
	}
	for i, l := range materialLens {
		label := ""
		if i < len(materialLabels) {
			label = materialLabels[i]
		}
		reqBody.Materials = append(reqBody.Materials, cutSolverMaterial{Label: label, Length: l})
	}
	for _, sc := range restScraps {
		reqBody.Scraps = append(reqBody.Scraps, cutSolverScrap{Length: sc})
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cutSolverTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/cut1d/solve", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("请求求解服务失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("求解服务返回 HTTP %d: %s", resp.StatusCode, truncateRunes(string(body), 200))
	}
	var out cutSolverResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, nil, fmt.Errorf("解析求解响应失败: %w", err)
	}
	if len(out.Unplaced) > 0 {
		return nil, nil, fmt.Errorf("求解结果存在 %d 个未放置零件", len(out.Unplaced))
	}

	// 旧料按长度排队 (保持输入顺序), 每消耗一根出队一个物理下标
	scrapQueue := make(map[float64][]int, len(restScraps))
	for idx, sc := range restScraps {
		scrapQueue[sc] = append(scrapQueue[sc], idx)
	}

	var results []model.BarResult
	var usedRest []int
	idx := startIdx
	for _, bar := range out.Bars {
		if len(bar.Pattern) != len(items) {
			return nil, nil, fmt.Errorf("求解响应模式长度不符")
		}
		pieceCount := 0
		for _, q := range bar.Pattern {
			if q < 0 {
				return nil, nil, fmt.Errorf("求解响应模式含负数")
			}
			pieceCount += q
		}
		if bar.Count <= 0 || pieceCount == 0 {
			continue
		}

		var capacity float64
		var materialType string
		switch bar.Source {
		case "material":
			if bar.MaterialIndex == nil || *bar.MaterialIndex < 0 || *bar.MaterialIndex >= len(materialLens) {
				return nil, nil, fmt.Errorf("求解响应材料下标越界")
			}
			capacity = materialLens[*bar.MaterialIndex]
			if *bar.MaterialIndex < len(materialLabels) {
				materialType = materialLabels[*bar.MaterialIndex]
			}
		case "scrap":
			if bar.ScrapLength == nil {
				return nil, nil, fmt.Errorf("求解响应旧料缺长度")
			}
			length := *bar.ScrapLength
			if _, ok := scrapQueue[length]; !ok || len(scrapQueue[length]) == 0 {
				return nil, nil, fmt.Errorf("旧料长度 %v 数量不足", length)
			}
			capacity = float64(length)
			materialType = "库存余料"
		default:
			return nil, nil, fmt.Errorf("未知求解来源: %s", bar.Source)
		}

		cutLengths := make([]float64, 0, pieceCount)
		for t, q := range bar.Pattern {
			for i := 0; i < q; i++ {
				cutLengths = append(cutLengths, items[t].length)
			}
		}
		used := s.dot(bar.Pattern, items) + kerf*float64(max(0, pieceCount-1))
		if used > capacity+1e-6 {
			return nil, nil, fmt.Errorf("求解结果超出材料容量 (防御)")
		}

		for k := 0; k < bar.Count; k++ {
			if bar.Source == "scrap" {
				length := *bar.ScrapLength
				queue := scrapQueue[length]
				if len(queue) == 0 {
					return nil, nil, fmt.Errorf("旧料长度 %v 数量不足", length)
				}
				scrapIdx := queue[0]
				scrapQueue[length] = queue[1:]
				usedRest = append(usedRest, scrapIdx)
				if scrapIdx < len(restLabels) && restLabels[scrapIdx] != "" {
					materialType = restLabels[scrapIdx]
				}
			}
			results = append(results, model.BarResult{
				Index:        idx,
				TotalLength:  capacity,
				Cuts:         cutLengths,
				Used:         round2(used),
				Remaining:    round2(capacity - used),
				MaterialType: materialType,
			})
			idx++
		}
	}
	return results, usedRest, nil
}
