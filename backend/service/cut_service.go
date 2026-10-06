package service

import (
	"backend/model"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CutService struct {
	baostockURL string // Baostock sidecar 地址 (BAOSTOCK_API_URL), 兼任精确求解服务; 空 = 未启用精确模式
}

func NewCutService(baostockURL string) *CutService {
	return &CutService{baostockURL: baostockURL}
}

// ===== 一维切割算法（列生成 + 贪心分配）=====

// aggItem 聚合后的项目类型
type aggItem struct {
	length  float64 // 长度
	demand  int     // 需求数量
	indices []int   // 原始索引列表
}

// pattern 切割模式
type pattern struct {
	qty      []int   // 每种类型的数量
	used     float64 // 使用长度
	capacity float64 // 容量
	cuts     int     // 切割数
	isNew    bool    // 是否新料
	scrapIdx int     // 旧料索引
}

// BarCut 一维切割优化算法 (零件可指定归属材料规格: 按规格分组, 各组独立求解)
func (s *CutService) BarCut(userID uint, req model.BarRequest) (*model.BarCutResponse, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("切割项目不能为空")
	}

	// 材料规格: newMaterials 多规格优先, 为空时回退 newMaterialLength 单一规格 (兼容旧请求)
	// label -> 长度 (同名规格取首个定义)
	specLen := make(map[string]float64)
	specOrder := make([]string, 0, len(req.NewMaterials)+1)
	registerSpec := func(label string, length float64) error {
		if length <= 0 {
			return errors.New("新材料长度必须大于0")
		}
		if _, ok := specLen[label]; !ok {
			specLen[label] = length
			specOrder = append(specOrder, label)
		}
		return nil
	}
	if len(req.NewMaterials) > 0 {
		for _, m := range req.NewMaterials {
			if err := registerSpec(m.Label, m.Length); err != nil {
				return nil, err
			}
		}
	} else {
		if req.NewMaterialLength <= 0 {
			return nil, errors.New("新材料长度必须大于0")
		}
		if err := registerSpec("", req.NewMaterialLength); err != nil {
			return nil, err
		}
	}
	kerf := math.Max(0, req.Loss)

	// 旧料展开: 用户输入
	type scrapUnit struct {
		length float64
		label  string
	}
	var allScraps []scrapUnit
	for _, m := range req.Materials {
		if m.Length <= 0 {
			continue
		}
		// materials 每条即一根 (与旧语义一致)
		allScraps = append(allScraps, scrapUnit{length: m.Length, label: m.Label})
	}

	// 零件按规格分组: spec 非空的零件只从同名规格的材料上切, 空 spec 进通用组
	genericSpec := "\x00generic" // 内部占位: 空 spec 归入通用组 (通用组可用全部规格的新料)
	groupOrder := make([]string, 0, 4)
	groupItems := make(map[string][]float64)
	for _, it := range req.Items {
		if it.Length <= 0 {
			return nil, fmt.Errorf("切割项目长度必须大于0, 存在非法项: %v", it.Length)
		}
		spec := it.Spec
		if spec == "" {
			spec = genericSpec
		} else if _, ok := specLen[spec]; !ok {
			return nil, fmt.Errorf("尺寸长度 %v 引用了未定义的材料规格: %s", it.Length, spec)
		}
		if _, ok := groupItems[spec]; !ok {
			groupOrder = append(groupOrder, spec)
		}
		groupItems[spec] = append(groupItems[spec], it.Length)
	}
	// 通用组排到最后 (优先满足明确指定规格的零件)
	stableOrder := make([]string, 0, len(groupOrder))
	for _, g := range groupOrder {
		if g != genericSpec {
			stableOrder = append(stableOrder, g)
		}
	}
	if _, ok := groupItems[genericSpec]; ok {
		stableOrder = append(stableOrder, genericSpec)
	}

	// 旧料按 label 分配到组: 非通用组拿同名旧料; 其余 (无 label / 无人认领) 归通用组
	claimed := make([]bool, len(allScraps))
	var allResults []model.BarResult
	newIdx := 1

	for _, spec := range stableOrder {
		lengths := groupItems[spec]
		var groupSpecs []string
		if spec == genericSpec {
			groupSpecs = specOrder
		} else {
			groupSpecs = []string{spec}
		}

		// 组内材料: 新料规格
		var materialLens []float64
		var materialLabels []string
		maxLen := 0.0
		for _, name := range groupSpecs {
			l := specLen[name]
			materialLens = append(materialLens, l)
			materialLabels = append(materialLabels, name)
			if l > maxLen {
				maxLen = l
			}
		}

		// 组内校验: 零件必须能装进该组最长材料, 否则死循环
		for _, length := range lengths {
			if length > maxLen {
				specName := spec
				if spec == genericSpec {
					specName = "通用"
				}
				return nil, fmt.Errorf("切割项目长度 %v 超过材料规格 %s 的最长材料 %v, 无法切割", length, specName, maxLen)
			}
		}

		// 组内旧料: 非通用组认领同名旧料; 通用组拿全部未被认领的
		var groupScraps []float64
		var groupLabels []string
		for i, sc := range allScraps {
			if claimed[i] {
				continue
			}
			match := spec == genericSpec || sc.label == spec
			if spec == genericSpec {
				match = !claimed[i] && (sc.label == "" || !containsSpec(stableOrder[:len(stableOrder)-1], sc.label))
			}
			if !match {
				continue
			}
			claimed[i] = true
			groupScraps = append(groupScraps, sc.length)
			groupLabels = append(groupLabels, sc.label)
		}

		aggItems := s.aggregateItems(lengths)
		fixed, restIdxs, remainingDemand := s.preAssignExactScraps(aggItems, groupScraps, groupLabels, kerf)
		for i := range fixed {
			fixed[i].Index = newIdx
			newIdx++
		}
		restScraps := make([]float64, len(restIdxs))
		for i, idx := range restIdxs {
			restScraps[i] = groupScraps[idx]
		}

		// 精确模式: OR-Tools sidecar 列生成 (未配置地址或求解失败自动回退内置快速算法)
		var results []model.BarResult
		if req.Mode == model.BarModePrecise {
			if client := s.solverClient(); client != nil {
				precise, _, err := s.solvePreciseGroup(client, aggItems, remainingDemand, materialLens, materialLabels,
					restScraps, restLabelsFrom(groupScraps, groupLabels, restIdxs), kerf,
					math.Max(1, req.UtilizationWeight), newIdx)
				if err == nil {
					results = precise
					newIdx += len(results)
					allResults = append(allResults, results...)
					continue
				}
				log.Printf("[Cut] 精确求解失败 (回退快速模式): %v", err)
			} else {
				log.Printf("[Cut] 精确模式未配置求解地址 (BAOSTOCK_API_URL), 回退快速模式")
			}
		}

		var patterns []pattern
		for _, l := range materialLens {
			patterns = append(patterns, s.generateInitialPatterns(aggItems, remainingDemand, l, restScraps, kerf)...)
		}
		results, _ = s.solveGreedy(patterns, aggItems, remainingDemand, materialLens, materialLabels, restScraps, restLabelsFrom(groupScraps, groupLabels, restIdxs), kerf, newIdx)
		newIdx += len(results)
		allResults = append(allResults, fixed...)
		allResults = append(allResults, results...)
	}

	resp := &model.BarCutResponse{
		Results: allResults,
		Summary: summarizeBarResults(allResults),
	}
	return resp, nil
}

// precisePlaneCut 二维精确求解 (OR-Tools sidecar, cut_api/solver_2d): MaxRects 保底 + CP-SAT 优化。
// 未配置求解地址/规模超限/求解失败/超时一律回退 MaxRects, 结果不劣于启发式。
func (s *CutService) precisePlaneCut(req model.BinRequest) (*model.PlaneCutResponse, error) {
	fallback, err := s.maxRectsCut(req)
	if err != nil {
		return nil, err
	}
	client := s.solverClient()
	if client == nil {
		log.Printf("[Cut] 精确模式未配置求解地址 (BAOSTOCK_API_URL), 回退 MaxRects")
		return fallback, nil
	}
	return s.solvePlanePrecise(client, req, fallback), nil
}

// containsSpec 规格名列表包含判断
func containsSpec(specs []string, label string) bool {
	for _, s := range specs {
		if s == label {
			return true
		}
	}
	return false
}

// restLabelsFrom 从原下标映射出剩余旧料的类型名
func restLabelsFrom(scraps []float64, labels []string, restIdxs []int) []string {
	out := make([]string, len(restIdxs))
	for i, idx := range restIdxs {
		if idx < len(labels) {
			out[i] = labels[idx]
		}
	}
	return out
}

// summarizeBarResults 一维结果汇总: 材料总长/零件总长/利用率/可入库余料
func summarizeBarResults(results []model.BarResult) model.BarSummary {
	summary := model.BarSummary{}
	for _, r := range results {
		summary.MaterialCount++
		summary.TotalMaterialLength += r.TotalLength
		for _, c := range r.Cuts {
			summary.TotalCutLength += c
		}
		if r.Remaining > 0 {
			summary.ScrapCount++
			summary.TotalRemaining += r.Remaining
		}
	}
	if summary.TotalMaterialLength > 0 {
		summary.Utilization = round2(summary.TotalCutLength / summary.TotalMaterialLength * 100)
	}
	summary.TotalRemaining = round2(summary.TotalRemaining)
	summary.TotalCutLength = round2(summary.TotalCutLength)
	return summary
}

// aggregateItems 聚合相同长度的项目
func (s *CutService) aggregateItems(items []float64) []aggItem {
	typeMap := make(map[float64]*aggItem)
	order := []float64{}

	for i, item := range items {
		if _, exists := typeMap[item]; !exists {
			typeMap[item] = &aggItem{
				length:  item,
				demand:  0,
				indices: []int{},
			}
			order = append(order, item)
		}
		typeMap[item].demand++
		typeMap[item].indices = append(typeMap[item].indices, i)
	}

	result := make([]aggItem, 0, len(typeMap))
	for _, key := range order {
		result = append(result, *typeMap[key])
	}
	return result
}

// preAssignExactScraps 旧料直配预分配
// preAssignExactScraps 旧料直配预分配 (返回: 结果 / 剩余旧料的原下标 / 剩余需求)
func (s *CutService) preAssignExactScraps(items []aggItem, scraps []float64, scrapLabels []string, kerf float64) ([]model.BarResult, []int, []int) {
	var results []model.BarResult
	remainingDemand := make([]int, len(items))
	for i, item := range items {
		remainingDemand[i] = item.demand
	}

	// 标记已使用的旧料
	used := make([]bool, len(scraps))

	for t, item := range items {
		for i, scrap := range scraps {
			if used[i] {
				continue
			}
			// 精确匹配
			if item.length == scrap && remainingDemand[t] > 0 {
				used[i] = true
				remainingDemand[t]--

				results = append(results, model.BarResult{
					Index:        i + 1,
					TotalLength:  scrap,
					Cuts:         []float64{item.length},
					Used:         round2(item.length),
					Remaining:    0,
					MaterialType: scrapLabels[i],
				})

				// 移除已使用的索引
				if len(items[t].indices) > 0 {
					items[t].indices = items[t].indices[1:]
				}
				break
			}
		}
	}

	// 未使用旧料的原下标 (调用方据此追踪被消费的库存条目)
	var restIdxs []int
	for i := range scraps {
		if !used[i] {
			restIdxs = append(restIdxs, i)
		}
	}

	return results, restIdxs, remainingDemand
}

// generateInitialPatterns 生成初始切割模式
func (s *CutService) generateInitialPatterns(items []aggItem, demand []int, L float64, scraps []float64, kerf float64) []pattern {
	var patterns []pattern
	seen := make(map[string]bool)

	types := len(items)

	// 1. 单一类型模式
	for t := 0; t < types; t++ {
		maxPieces := int(L/items[t].length + 1e-9)
		maxPieces = min(maxPieces, demand[t])

		for p := 1; p <= maxPieces; p++ {
			used := float64(p)*items[t].length + kerf*float64(p-1)
			if used <= L+1e-9 {
				qty := make([]int, types)
				qty[t] = p
				key := s.patternKey(qty)
				if !seen[key] {
					seen[key] = true
					patterns = append(patterns, pattern{
						qty:      qty,
						used:     used,
						capacity: L,
						cuts:     p,
						isNew:    true,
						scrapIdx: -1,
					})
				}
			}
		}
	}

	// 2. 贪心模式（从大到小）
	if p := s.greedyPattern(items, demand, L, kerf, false, seen); p != nil {
		patterns = append(patterns, *p)
	}
	// 贪心模式（从小到大）
	if p := s.greedyPattern(items, demand, L, kerf, true, seen); p != nil {
		patterns = append(patterns, *p)
	}

	// 3. 混合模式
	if p := s.mixedPattern(items, demand, L, kerf, seen); p != nil {
		patterns = append(patterns, *p)
	}

	// 4. DP 背包精确模式 (替代原 DFS 全组合枚举: O(类型×容量) 无组合爆炸, 大订单不再放弃)
	patterns = append(patterns, s.dpPatternsForCapacity(items, demand, L, kerf, true, -1, seen)...)

	// 5. 旧料模式
	dpScrapQty := make(map[float64][]int) // 相同长度的旧料共享一次 DP 求解
	for idx, scrap := range scraps {
		if scrap <= 0 {
			continue
		}
		qty, ok := dpScrapQty[scrap]
		if !ok {
			if q, found := s.dpScrapBest(items, demand, scrap, kerf); found {
				qty = q
			} else {
				qty = nil
			}
			dpScrapQty[scrap] = qty
		}
		cuts := 0
		for _, q := range qty {
			cuts += q
		}
		if cuts > 0 {
			used := s.dot(qty, items) + kerf*float64(max(0, cuts-1))
			if used <= scrap+1e-6 {
				key := s.patternKey(qty) + "_scrap_" + itoa(idx)
				if !seen[key] {
					seen[key] = true
					patterns = append(patterns, pattern{
						qty:      qty,
						used:     used,
						capacity: scrap,
						cuts:     cuts,
						isNew:    false,
						scrapIdx: idx,
					})
				}
			}
		}
	}

	return patterns
}

// dpScrapBest 旧料容量的 DP 最优填充 (按长度缓存, DP 不可行时回退贪心)
func (s *CutService) dpScrapBest(items []aggItem, demand []int, scrapLen float64, kerf float64) ([]int, bool) {
	types := len(items)
	if types == 0 || types > dpMaxTypes {
		return nil, false
	}
	scale := dpScale(kerf, items)
	C := int(math.Round((scrapLen + kerf) * float64(scale)))
	if C <= 0 || C > dpMaxCapacity {
		return nil, false
	}
	weights := make([]int, types)
	caps := make([]int, types)
	for t := 0; t < types; t++ {
		weights[t] = int(math.Round((items[t].length + kerf) * float64(scale)))
		caps[t] = min(int(scrapLen/items[t].length+1e-9), demand[t])
		if weights[t] <= 0 || weights[t] > C {
			caps[t] = 0
		}
	}
	qty, ok := dpBestPattern(weights, caps, C)
	if !ok {
		return nil, false
	}
	// 浮点口径复核 (离散化舍入防御)
	cuts := 0
	for _, q := range qty {
		cuts += q
	}
	used := s.dot(qty, items) + kerf*float64(max(0, cuts-1))
	if cuts == 0 || used > scrapLen+1e-6 {
		return nil, false
	}
	return qty, true
}

// greedyPattern 贪心生成模式
func (s *CutService) greedyPattern(items []aggItem, demand []int, L float64, kerf float64, ascending bool, seen map[string]bool) *pattern {
	types := len(items)
	qty := make([]int, types)
	used := 0.0
	cuts := 0

	// 排序索引
	indices := make([]int, types)
	for i := range indices {
		indices[i] = i
	}
	if ascending {
		sort.Slice(indices, func(i, j int) bool {
			return items[indices[i]].length < items[indices[j]].length
		})
	} else {
		sort.Slice(indices, func(i, j int) bool {
			return items[indices[i]].length > items[indices[j]].length
		})
	}

	for _, id := range indices {
		left := demand[id]
		for left > 0 {
			next := used + items[id].length
			if cuts > 0 {
				next += kerf
			}
			if next <= L+1e-9 {
				qty[id]++
				cuts++
				used = next
				left--
			} else {
				break
			}
		}
	}

	if cuts > 0 {
		return &pattern{
			qty:      qty,
			used:     used,
			capacity: L,
			cuts:     cuts,
			isNew:    true,
			scrapIdx: -1,
		}
	}
	return nil
}

// mixedPattern 混合模式
func (s *CutService) mixedPattern(items []aggItem, demand []int, L float64, kerf float64, seen map[string]bool) *pattern {
	types := len(items)
	qty := make([]int, types)
	used := 0.0
	cuts := 0

	// 按长度降序排列
	indices := make([]int, types)
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return items[indices[i]].length > items[indices[j]].length
	})

	updated := true
	for updated {
		updated = false
		for _, id := range indices {
			if qty[id] >= demand[id] {
				continue
			}
			next := used + items[id].length
			if cuts > 0 {
				next += kerf
			}
			if next <= L+1e-9 {
				qty[id]++
				cuts++
				used = next
				updated = true
			}
		}
	}

	if cuts > 0 {
		return &pattern{
			qty:      qty,
			used:     used,
			capacity: L,
			cuts:     cuts,
			isNew:    true,
			scrapIdx: -1,
		}
	}
	return nil
}

// solveGreedy 贪心求解 (多材料规格: 新料 pattern 按各自容量利用率参与排序, 总长取 pattern.capacity)
// startIdx 为结果编号起点 (按规格分组求解时全局连续); 返回: 结果 / 被消费旧料在 scraps 中的下标
func (s *CutService) solveGreedy(patterns []pattern, items []aggItem, demand []int, materialLens []float64, materialLabels []string, scraps []float64, scrapLabels []string, kerf float64, startIdx int) ([]model.BarResult, []int) {
	types := len(items)
	remaining := make([]int, len(demand))
	copy(remaining, demand)

	// 新料规格名: 长度 -> label (供结果标注材料类型)
	lengthLabel := make(map[float64]string, len(materialLens))
	for i, l := range materialLens {
		if i < len(materialLabels) && materialLabels[i] != "" {
			lengthLabel[l] = materialLabels[i]
		}
	}

	var results []model.BarResult
	newIdx := startIdx

	// 按利用率排序模式
	sort.Slice(patterns, func(i, j int) bool {
		// 优先使用旧料
		if patterns[i].isNew != patterns[j].isNew {
			return !patterns[i].isNew
		}
		// 优先高利用率
		utilI := patterns[i].used / patterns[i].capacity
		utilJ := patterns[j].used / patterns[j].capacity
		return utilI > utilJ
	})

	// 跟踪旧料使用
	scrapUsed := make([]bool, len(scraps))

	for _, p := range patterns {
		// 同一模式按剩余需求连续开多根 (旧料一根一发: 消费后 scrapUsed 置位, canUse 自然转 false)
		for {
			// 检查是否可以使用此模式
			canUse := true
			for t := 0; t < types; t++ {
				if p.qty[t] > remaining[t] {
					canUse = false
					break
				}
			}

			// 检查旧料是否已使用
			if !p.isNew && p.scrapIdx >= 0 && scrapUsed[p.scrapIdx] {
				canUse = false
			}
			if !canUse {
				break
			}

			// 使用此模式
			cuts := []float64{}
			for t := 0; t < types; t++ {
				for i := 0; i < p.qty[t]; i++ {
					cuts = append(cuts, items[t].length)
				}
			}

			// 新料长度来自 pattern 所属材料规格 (capacity), 旧料取旧料原长
			totalLength := p.capacity
			materialType := lengthLabel[totalLength]
			if !p.isNew && p.scrapIdx >= 0 {
				totalLength = scraps[p.scrapIdx]
				materialType = ""
				if p.scrapIdx < len(scrapLabels) {
					materialType = scrapLabels[p.scrapIdx]
				}
				scrapUsed[p.scrapIdx] = true
			}

			results = append(results, model.BarResult{
				Index:        newIdx,
				TotalLength:  totalLength,
				Cuts:         cuts,
				Used:         round2(p.used),
				Remaining:    round2(p.capacity - p.used),
				MaterialType: materialType,
			})
			newIdx++

			// 更新需求
			for t := 0; t < types; t++ {
				remaining[t] -= p.qty[t]
			}

			if !p.isNew {
				break
			}
		}
	}

	// 处理剩余需求 (多材料: 每根新料选用"能容纳本类型零件的最小材料规格", 减少浪费)
	for t := 0; t < types; t++ {
		for remaining[t] > 0 {
			// 选能容纳当前零件的最小材料长度; 从最长规格起步 (BarCut 入口已校验装得下),
			// 避免 materialLens[0] 装不下时 cuts=0 静默丢件
			maxLen := materialLens[0]
			for _, l := range materialLens {
				if l > maxLen {
					maxLen = l
				}
			}
			curLen := maxLen
			for _, l := range materialLens {
				if l >= items[t].length && l < curLen {
					curLen = l
				}
			}
			L := curLen

			// 使用新材料
			qty := make([]int, types)
			used := 0.0
			cuts := 0

			// 尽可能多地放入
			for remaining[t] > 0 {
				next := used + items[t].length
				if cuts > 0 {
					next += kerf
				}
				if next <= L+1e-9 {
					qty[t]++
					remaining[t]--
					cuts++
					used = next
				} else {
					break
				}
			}

			// 尝试放入其他类型
			for t2 := 0; t2 < types; t2++ {
				if t2 == t {
					continue
				}
				for remaining[t2] > 0 {
					next := used + items[t2].length
					if cuts > 0 {
						next += kerf
					}
					if next <= L+1e-9 {
						qty[t2]++
						remaining[t2]--
						cuts++
						used = next
					} else {
						break
					}
				}
			}

			cutLengths := []float64{}
			for t2, q := range qty {
				for i := 0; i < q; i++ {
					cutLengths = append(cutLengths, items[t2].length)
				}
			}

			// 兜底守卫: 本轮一件都没放下 (入口已校验理论上不应发生), 跳过该类型防止死循环
			if cuts == 0 {
				break
			}

			results = append(results, model.BarResult{
				Index:        newIdx,
				TotalLength:  L,
				Cuts:         cutLengths,
				Used:         round2(used),
				Remaining:    round2(L - used),
				MaterialType: lengthLabel[L],
			})
			newIdx++
		}
	}

	// 收集被消费的旧料下标 (相对传入的 scraps)
	var usedScrapIdxs []int
	for idx, usedFlag := range scrapUsed {
		if usedFlag {
			usedScrapIdxs = append(usedScrapIdxs, idx)
		}
	}
	return results, usedScrapIdxs
}

// patternKey 生成模式的唯一键
func (s *CutService) patternKey(qty []int) string {
	key := ""
	for _, q := range qty {
		key += itoa(q) + ","
	}
	return key
}

// dot 计算点积
func (s *CutService) dot(qty []int, items []aggItem) float64 {
	sum := 0.0
	for i, q := range qty {
		sum += float64(q) * items[i].length
	}
	return sum
}

// ===== 二维切割算法=====

// PlaneCut 平面切割优化算法
func (s *CutService) PlaneCut(req model.BinRequest) (*model.PlaneCutResponse, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("切割项目不能为空")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return nil, errors.New("材料尺寸必须大于0")
	}

	var resp *model.PlaneCutResponse
	var err error
	switch req.Strategy {
	case "Guillotine":
		resp, err = s.guillotineCut(req)
	case "MaxRects":
		resp, err = s.maxRectsCut(req)
	case "Precise":
		// 精确模式 (OR-Tools sidecar): 内部已含 MaxRects 回退, 任何失败不阻断
		resp, err = s.precisePlaneCut(req)
	default:
		return nil, errors.New("不支持的切割策略: " + req.Strategy)
	}
	if err != nil {
		return nil, err
	}

	// 汇总统计 + 未排入件数量归并
	resp.Summary = summarizePlaneResults(resp.Results, resp.Unplaced)
	return resp, nil
}

// summarizePlaneResults 平面结果汇总
func summarizePlaneResults(results []model.BinResult, unplaced []model.UnplacedItem) model.PlaneSummary {
	summary := model.PlaneSummary{
		BinCount:      len(results),
		UnplacedCount: 0,
	}
	for _, u := range unplaced {
		summary.UnplacedCount += u.Quantity
	}
	for _, r := range results {
		summary.TotalArea += r.MaterialWidth * r.MaterialHeight
		for _, p := range r.Pieces {
			summary.UsedArea += p.W * p.H
		}
	}
	if summary.TotalArea > 0 {
		summary.Utilization = round2(summary.UsedArea / summary.TotalArea * 100)
	}
	summary.UsedArea = round2(summary.UsedArea)
	summary.TotalArea = round2(summary.TotalArea)
	return summary
}

// mergeUnplaced 按尺寸归并未排入件并标注原因
func mergeUnplaced(items []model.Item, reason string) []model.UnplacedItem {
	if len(items) == 0 {
		return nil
	}
	type key struct{ w, h float64 }
	counts := make(map[key]*model.UnplacedItem)
	var order []key
	for _, it := range items {
		k := key{it.Width, it.Height}
		if existing, ok := counts[k]; ok {
			existing.Quantity++
			continue
		}
		entry := &model.UnplacedItem{Label: it.Label, Width: it.Width, Height: it.Height, Quantity: 1, Reason: reason}
		counts[k] = entry
		order = append(order, k)
	}
	result := make([]model.UnplacedItem, 0, len(order))
	for _, k := range order {
		result = append(result, *counts[k])
	}
	return result
}

// ===== Guillotine 算法实现 =====

// FreeRectangle 空闲矩形
type FreeRectangle struct {
	X, Y, Width, Height float64
}

func (r FreeRectangle) Area() float64 {
	return r.Width * r.Height
}

// Placement 放置结果
type Placement struct {
	X, Y, W, H float64
	Rotated    bool
	RectIndex  int
}

// GuillotineBin Guillotine切割板材
type GuillotineBin struct {
	Width, Height float64
	FreeRects     []FreeRectangle
}

func NewGuillotineBin(width, height float64) *GuillotineBin {
	return &GuillotineBin{
		Width:     width,
		Height:    height,
		FreeRects: []FreeRectangle{{X: 0, Y: 0, Width: width, Height: height}},
	}
}

func (b *GuillotineBin) Insert(item model.Item) *Placement {
	w := item.Width
	h := item.Height
	eps := 1e-6

	var bestPlacement *Placement
	var bestRect FreeRectangle
	bestRectIndex := -1

	for i, rect := range b.FreeRects {
		candidates := []Placement{}

		// 不旋转
		if w <= rect.Width+eps && h <= rect.Height+eps {
			candidates = append(candidates, Placement{X: rect.X, Y: rect.Y, W: w, H: h, Rotated: false, RectIndex: i})
		}
		// 旋转
		if h <= rect.Width+eps && w <= rect.Height+eps {
			candidates = append(candidates, Placement{X: rect.X, Y: rect.Y, W: h, H: w, Rotated: true, RectIndex: i})
		}

		for _, cand := range candidates {
			if bestPlacement == nil {
				bestPlacement = &cand
				bestRect = rect
				bestRectIndex = i
			} else {
				candScore := b.placementScore(rect, cand)
				bestScore := b.placementScore(bestRect, *bestPlacement)
				cmp := compareScore(candScore, bestScore, eps)
				if cmp < 0 {
					bestPlacement = &cand
					bestRect = rect
					bestRectIndex = i
				}
			}
		}
	}

	if bestPlacement != nil {
		target := b.FreeRects[bestPlacement.RectIndex]
		b.FreeRects = append(b.FreeRects[:bestRectIndex], b.FreeRects[bestRectIndex+1:]...)

		placedW := bestPlacement.W
		placedH := bestPlacement.H

		var cut1, cut2 *FreeRectangle

		// 方案1：竖切（按宽度切）
		if target.Width > placedW+1e-9 {
			c1 := FreeRectangle{X: target.X + placedW, Y: target.Y, Width: target.Width - placedW, Height: placedH}
			c2 := FreeRectangle{X: target.X, Y: target.Y + placedH, Width: target.Width, Height: target.Height - placedH}
			cut1 = &c1
			cut2 = &c2
		}

		// 方案2：横切（按高度切）
		if target.Height > placedH+1e-9 {
			alt1 := FreeRectangle{X: target.X, Y: target.Y + placedH, Width: placedW, Height: target.Height - placedH}
			alt2 := FreeRectangle{X: target.X + placedW, Y: target.Y, Width: target.Width - placedW, Height: target.Height}
			if cut1 == nil || b.isMoreSquare(alt1, alt2, *cut1, *cut2) {
				cut1 = &alt1
				cut2 = &alt2
			}
		}

		if cut1 != nil && cut1.Area() > 0 {
			b.FreeRects = append(b.FreeRects, *cut1)
		}
		if cut2 != nil && cut2.Area() > 0 {
			b.FreeRects = append(b.FreeRects, *cut2)
		}

		return bestPlacement
	}

	return nil
}

func (b *GuillotineBin) placementScore(rect FreeRectangle, p Placement) [4]float64 {
	waste := rect.Area() - p.W*p.H
	leftoverW := rect.Width - p.W
	leftoverH := rect.Height - p.H
	minLeft := math.Min(leftoverW, leftoverH)
	maxLeft := math.Max(leftoverW, leftoverH)
	rotPref := 1.0
	if p.Rotated {
		rotPref = 0.0
	}
	return [4]float64{waste, minLeft, maxLeft, rotPref}
}

func compareScore(a, b [4]float64, eps float64) int {
	for i := 0; i < 3; i++ {
		diff := a[i] - b[i]
		if math.Abs(diff) > eps {
			if diff < 0 {
				return -1
			}
			return 1
		}
	}
	diff := a[3] - b[3]
	if math.Abs(diff) > eps {
		if diff < 0 {
			return -1
		}
		return 1
	}
	return 0
}

func (b *GuillotineBin) isMoreSquare(a1, a2, b1, b2 FreeRectangle) bool {
	ratioA := b1.getSquareRatio(a1) + b1.getSquareRatio(a2)
	ratioB := b1.getSquareRatio(b1) + b1.getSquareRatio(b2)
	return ratioA < ratioB
}

func (r FreeRectangle) getSquareRatio(fr FreeRectangle) float64 {
	if fr.Area() == 0 {
		return math.MaxFloat64
	}
	return math.Max(fr.Width, fr.Height) / math.Min(fr.Width, fr.Height)
}

// guillotineCut 刀切法切割
// planeMaterial 平面切割的可用材料实例 (旧料优先于新板材)
type planeMaterial struct {
	Name     string
	Width    float64
	Height   float64
	Priority int
}

// buildPlaneMaterials 构建材料实例列表: 旧料优先消费, 之后开备用新板材
func buildPlaneMaterials(req model.BinRequest) []planeMaterial {
	materials := []planeMaterial{}
	for _, m := range req.Materials {
		count := m.Quantity
		if count < 1 {
			count = 1
		}
		for i := 0; i < count; i++ {
			materials = append(materials, planeMaterial{Name: m.Label, Width: m.Width, Height: m.Height, Priority: 10})
		}
	}
	// 备用新板材上限: 防止极端输入无限开板; 用尽后零件进入"未排入"清单而非静默丢弃
	for i := 0; i < 100; i++ {
		materials = append(materials, planeMaterial{Name: "新板材", Width: req.Width, Height: req.Height, Priority: 0})
	}
	// 按优先级(旧料先) + 面积降序
	sort.Slice(materials, func(i, j int) bool {
		if materials[i].Priority != materials[j].Priority {
			return materials[i].Priority > materials[j].Priority
		}
		return materials[i].Width*materials[i].Height > materials[j].Width*materials[j].Height
	})
	return materials
}

// takePlaneMaterial 从列表中取出第一个能容纳该零件的材料 (旧料优先已由排序保证), 未找到返回 nil
func takePlaneMaterial(materials []planeMaterial, w, h float64) (*planeMaterial, []planeMaterial) {
	for i, m := range materials {
		if w <= m.Width && h <= m.Height {
			remaining := append(append([]planeMaterial{}, materials[:i]...), materials[i+1:]...)
			return &planeMaterial{Name: m.Name, Width: m.Width, Height: m.Height, Priority: m.Priority}, remaining
		}
	}
	return nil, materials
}

func (s *CutService) guillotineCut(req model.BinRequest) (*model.PlaneCutResponse, error) {
	var results []model.BinResult
	binID := 0

	// 展开所有项目
	allItems := s.expandItems(req.Items)

	// 过滤无效件 + 按面积降序排序
	validItems := []model.Item{}
	for _, item := range allItems {
		if item.Width > 0 && item.Height > 0 {
			validItems = append(validItems, item)
		}
	}
	sort.Slice(validItems, func(i, j int) bool {
		areaI := validItems[i].Width * validItems[i].Height
		areaJ := validItems[j].Width * validItems[j].Height
		if areaI != areaJ {
			return areaI > areaJ
		}
		return validItems[i].Height > validItems[j].Height
	})

	// 旧料优先, 用尽后开新板材 (旧实现完全忽略旧料入参)
	materials := buildPlaneMaterials(req)

	oversized := []model.Item{}
	exhausted := []model.Item{}
	bins := []*GuillotineBin{}

	for _, item := range validItems {
		placed := false

		// 尝试放入现有板材
		for i, bin := range bins {
			placement := bin.Insert(item)
			if placement != nil {
				results[i].Pieces = append(results[i].Pieces, createPieceFromPlacement(item, *placement))
				placed = true
				break
			}
		}

		// 若无板材可放, 取一块能容纳它的材料开新板
		if !placed {
			selected, rest := takePlaneMaterial(materials, item.Width, item.Height)
			if selected == nil {
				// 所有材料(含备用新板材)都放不下或已耗尽:
				// 新板材(含旋转)都容纳不下 => 零件超尺寸; 否则 => 备用材料耗尽
				fitsNew := (item.Width <= req.Width && item.Height <= req.Height) ||
					(item.Height <= req.Width && item.Width <= req.Height)
				if !fitsNew {
					oversized = append(oversized, item)
				} else {
					exhausted = append(exhausted, item)
				}
				continue
			}
			materials = rest

			newBin := NewGuillotineBin(selected.Width, selected.Height)
			placement := newBin.Insert(item)
			if placement != nil {
				bins = append(bins, newBin)
				br := model.BinResult{
					BinID:          binID,
					MaterialType:   selected.Name,
					MaterialWidth:  selected.Width,
					MaterialHeight: selected.Height,
					Pieces:         []model.Piece{createPieceFromPlacement(item, *placement)},
				}
				binID++
				results = append(results, br)
			}
		}
	}

	// 计算利用率
	for i := range results {
		calculateUtilization(&results[i])
	}

	unplaced := append(mergeUnplaced(oversized, "oversized"), mergeUnplaced(exhausted, "exhausted")...)
	return &model.PlaneCutResponse{Results: results, Unplaced: unplaced}, nil
}

func createPieceFromPlacement(item model.Item, p Placement) model.Piece {
	return model.Piece{
		Label:   item.Label,
		X:       round2(p.X),
		Y:       round2(p.Y),
		W:       round2(p.W),
		H:       round2(p.H),
		Rotated: p.Rotated,
	}
}

func calculateUtilization(br *model.BinResult) {
	usedArea := 0.0
	for _, p := range br.Pieces {
		usedArea += p.W * p.H
	}
	totalArea := br.MaterialWidth * br.MaterialHeight
	utilization := 0.0
	if totalArea > 0 {
		utilization = (usedArea / totalArea) * 100
	}
	br.Utilization = round2(utilization)
}

// ===== MaxRects 算法实现 =====

// MaxRect 矩形
type MaxRect struct {
	X, Y, Width, Height float64
	Rotated             bool
}

// MaxRectsBin MaxRects切割板材
type MaxRectsBin struct {
	Width, Height  float64
	FreeRectangles []MaxRect
}

func NewMaxRectsBin(width, height float64) *MaxRectsBin {
	return &MaxRectsBin{
		Width:  width,
		Height: height,
		FreeRectangles: []MaxRect{
			{X: 0, Y: 0, Width: width, Height: height, Rotated: false},
		},
	}
}

func (b *MaxRectsBin) Insert(w, h float64, allowRotate bool) *MaxRect {
	var bestRect *MaxRect
	bestShortSideFit := math.MaxFloat64
	bestLongSideFit := math.MaxFloat64

	// 按 Y 然后 X 排序
	sort.Slice(b.FreeRectangles, func(i, j int) bool {
		if b.FreeRectangles[i].Y != b.FreeRectangles[j].Y {
			return b.FreeRectangles[i].Y < b.FreeRectangles[j].Y
		}
		return b.FreeRectangles[i].X < b.FreeRectangles[j].X
	})

	for _, free := range b.FreeRectangles {
		// 尝试不旋转
		if w <= free.Width && h <= free.Height {
			leftoverHoriz := math.Abs(free.Width - w)
			leftoverVert := math.Abs(free.Height - h)
			shortSideFit := math.Min(leftoverHoriz, leftoverVert)
			longSideFit := math.Max(leftoverHoriz, leftoverVert)

			if shortSideFit < bestShortSideFit || (shortSideFit == bestShortSideFit && longSideFit < bestLongSideFit) {
				rect := MaxRect{X: free.X, Y: free.Y, Width: w, Height: h, Rotated: false}
				bestRect = &rect
				bestShortSideFit = shortSideFit
				bestLongSideFit = longSideFit
			}
		}

		// 尝试旋转
		if allowRotate && h <= free.Width && w <= free.Height {
			leftoverHoriz := math.Abs(free.Width - h)
			leftoverVert := math.Abs(free.Height - w)
			shortSideFit := math.Min(leftoverHoriz, leftoverVert)
			longSideFit := math.Max(leftoverHoriz, leftoverVert)

			if shortSideFit < bestShortSideFit || (shortSideFit == bestShortSideFit && longSideFit < bestLongSideFit) {
				rect := MaxRect{X: free.X, Y: free.Y, Width: h, Height: w, Rotated: true}
				bestRect = &rect
				bestShortSideFit = shortSideFit
				bestLongSideFit = longSideFit
			}
		}
	}

	if bestRect != nil {
		b.placeRect(*bestRect)
	}

	return bestRect
}

func (b *MaxRectsBin) placeRect(rect MaxRect) {
	newFree := []MaxRect{}
	for _, free := range b.FreeRectangles {
		if !b.intersect(free, rect) {
			newFree = append(newFree, free)
		} else {
			b.splitFreeRectangle(free, rect, &newFree)
		}
	}
	b.FreeRectangles = newFree
	b.pruneFreeList()
}

func (b *MaxRectsBin) splitFreeRectangle(free, placed MaxRect, newFree *[]MaxRect) {
	if placed.X < free.X+free.Width && placed.X+placed.Width > free.X {
		if placed.Y > free.Y {
			height := placed.Y - free.Y
			if height > 0 {
				*newFree = append(*newFree, MaxRect{X: free.X, Y: free.Y, Width: free.Width, Height: height})
			}
		}
		if placed.Y+placed.Height < free.Y+free.Height {
			height := free.Y + free.Height - (placed.Y + placed.Height)
			if height > 0 {
				*newFree = append(*newFree, MaxRect{X: free.X, Y: placed.Y + placed.Height, Width: free.Width, Height: height})
			}
		}
	}

	if placed.Y < free.Y+free.Height && placed.Y+placed.Height > free.Y {
		if placed.X > free.X {
			width := placed.X - free.X
			if width > 0 {
				*newFree = append(*newFree, MaxRect{X: free.X, Y: free.Y, Width: width, Height: free.Height})
			}
		}
		if placed.X+placed.Width < free.X+free.Width {
			width := free.X + free.Width - (placed.X + placed.Width)
			if width > 0 {
				*newFree = append(*newFree, MaxRect{X: placed.X + placed.Width, Y: free.Y, Width: width, Height: free.Height})
			}
		}
	}
}

func (b *MaxRectsBin) pruneFreeList() {
	for i := 0; i < len(b.FreeRectangles); i++ {
		a := b.FreeRectangles[i]
		removed := false
		for j := 0; j < len(b.FreeRectangles); j++ {
			if i == j {
				continue
			}
			bRect := b.FreeRectangles[j]
			if b.isContainedIn(a, bRect) {
				b.FreeRectangles = append(b.FreeRectangles[:i], b.FreeRectangles[i+1:]...)
				i--
				removed = true
				break
			}
		}
		if !removed {
			for j := i + 1; j < len(b.FreeRectangles); j++ {
				bRect := b.FreeRectangles[j]
				if b.isContainedIn(bRect, a) {
					b.FreeRectangles = append(b.FreeRectangles[:j], b.FreeRectangles[j+1:]...)
					j--
				}
			}
		}
	}
}

func (b *MaxRectsBin) intersect(a, bRect MaxRect) bool {
	return !(bRect.X >= a.X+a.Width ||
		bRect.X+bRect.Width <= a.X ||
		bRect.Y >= a.Y+a.Height ||
		bRect.Y+bRect.Height <= a.Y)
}

func (b *MaxRectsBin) isContainedIn(a, bRect MaxRect) bool {
	return a.X >= bRect.X && a.Y >= bRect.Y &&
		a.X+a.Width <= bRect.X+bRect.Width &&
		a.Y+a.Height <= bRect.Y+bRect.Height
}

// maxRectsCut 最大空闲矩形法切割
func (s *CutService) maxRectsCut(req model.BinRequest) (*model.PlaneCutResponse, error) {
	var results []model.BinResult
	binID := 0

	// 展开所有项目
	allItems := s.expandItems(req.Items)

	// 旧料优先, 用尽后开备用新板材
	materials := buildPlaneMaterials(req)

	// 按面积从大到小排序物品
	sort.Slice(allItems, func(i, j int) bool {
		areaI := allItems[i].Width * allItems[i].Height
		areaJ := allItems[j].Width * allItems[j].Height
		return areaI > areaJ
	})

	oversized := []model.Item{}
	exhausted := []model.Item{}
	bins := []*MaxRectsBin{}

	for _, item := range allItems {
		placed := false

		// 尝试放入已有 bin
		for i, bin := range bins {
			rect := bin.Insert(item.Width, item.Height, true)
			if rect != nil {
				results[i].Pieces = append(results[i].Pieces, createMaxRectPiece(item, *rect))
				placed = true
				break
			}
		}

		// 放不下则选择新的材料开 bin
		if !placed {
			selected, rest := takePlaneMaterial(materials, item.Width, item.Height)
			if selected == nil {
				// 所有材料(含备用新板材)都放不下或已耗尽
				fitsNew := (item.Width <= req.Width && item.Height <= req.Height) ||
					(item.Height <= req.Width && item.Width <= req.Height)
				if !fitsNew {
					oversized = append(oversized, item)
				} else {
					exhausted = append(exhausted, item)
				}
				continue
			}
			materials = rest

			newBin := NewMaxRectsBin(selected.Width, selected.Height)
			rect := newBin.Insert(item.Width, item.Height, true)
			if rect != nil {
				bins = append(bins, newBin)

				br := model.BinResult{
					BinID:          binID,
					MaterialType:   selected.Name,
					MaterialWidth:  selected.Width,
					MaterialHeight: selected.Height,
					Pieces:         []model.Piece{createMaxRectPiece(item, *rect)},
				}
				binID++
				results = append(results, br)
			}
		}
	}

	// 更新利用率
	for i := range results {
		calculateUtilization(&results[i])
	}

	unplaced := append(mergeUnplaced(oversized, "oversized"), mergeUnplaced(exhausted, "exhausted")...)
	return &model.PlaneCutResponse{Results: results, Unplaced: unplaced}, nil
}

func createMaxRectPiece(item model.Item, rect MaxRect) model.Piece {
	return model.Piece{
		Label:   item.Label,
		X:       round2(rect.X),
		Y:       round2(rect.Y),
		W:       round2(rect.Width),
		H:       round2(rect.Height),
		Rotated: rect.Rotated,
	}
}

// expandItems 展开项目（根据数量）
func (s *CutService) expandItems(items []model.Item) []model.Item {
	var expanded []model.Item
	for _, item := range items {
		count := item.Quantity
		if count < 1 {
			count = 1
		}
		for i := 0; i < count; i++ {
			label := item.Label
			if count > 1 {
				label = item.Label + "_" + strconv.Itoa(i+1)
			}
			expanded = append(expanded, model.Item{
				Label:  label,
				Width:  item.Width,
				Height: item.Height,
			})
		}
	}
	return expanded
}

// SaveCutRecord 保存切割记录; 随单扣减从库存带入且被消耗的旧料 (扣减失败则整体失败, 不产生记录)
func (s *CutService) SaveCutRecord(userID uint, req model.RecordRequest) (*model.CutRecord, error) {
	code := s.generateCode(req.Type)
	record := model.CutRecord{
		ID:         uuid.New().String(),
		Type:       req.Type,
		Request:    req.Request,
		Response:   req.Response,
		CreateTime: time.Now(),
		UserID:     userID,
		Code:       code,
		Name:       req.Name,
	}

	// 扣减与建记录必须同事务: 只扣减不落记录 = 库存凭空消失
	err := DB.Transaction(func(tx *gorm.DB) error {
		if len(req.DeductScraps) > 0 {
			if err := deductScrapsTx(tx, userID, req.DeductScraps); err != nil {
				return err
			}
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// generateCode 生成编号: B/P + 日期 + 随机数
func (s *CutService) generateCode(typeStr string) string {
	prefix := "P"
	if typeStr == "1" {
		prefix = "B"
	}
	date := time.Now().Format("20060102")
	random := fmt.Sprintf("%08d", time.Now().UnixNano()%100000000)
	return prefix + date + random
}

// ListCutRecords 查询切割记录列表
func (s *CutService) ListCutRecords(userID uint, params model.CutRecordSearchParams) (*model.CutRecordListResponse, error) {
	query := DB.Model(&model.CutRecord{}).Where("user_id = ?", userID)

	if params.Name != "" {
		query = query.Where("name LIKE ?", "%"+params.Name+"%")
	}
	if params.Type != "" {
		query = query.Where("type = ?", params.Type)
	}
	if params.StartTime != nil {
		query = query.Where("create_time >= ?", time.Unix(*params.StartTime/1000, 0))
	}
	if params.EndTime != nil {
		query = query.Where("create_time <= ?", time.Unix(*params.EndTime/1000, 0))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var records []model.CutRecord
	offset := (params.Page - 1) * params.PageSize
	if err := query.Order("create_time DESC").Offset(offset).Limit(params.PageSize).Find(&records).Error; err != nil {
		return nil, err
	}

	return &model.CutRecordListResponse{
		Total:   total,
		Records: records,
	}, nil
}

// DeleteCutRecord 删除切割记录
func (s *CutService) DeleteCutRecord(userID uint, id string) error {
	result := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.CutRecord{})
	if result.RowsAffected == 0 {
		return errors.New("记录不存在")
	}
	return result.Error
}

// ===== 余料库存 =====

// ListScraps 用户余料库存列表
func (s *CutService) ListScraps(userID uint, scrapType int) ([]model.CutScrap, error) {
	query := DB.Where("user_id = ?", userID)
	if scrapType > 0 {
		query = query.Where("scrap_type = ?", scrapType)
	}
	var list []model.CutScrap
	err := query.Order("created_at DESC").Limit(500).Find(&list).Error
	return list, err
}

// AddScraps 登记余料 (支持批量, 结果页一键入库)
func (s *CutService) AddScraps(userID uint, reqs []model.AddCutScrapRequest) ([]model.CutScrap, error) {
	if len(reqs) == 0 {
		return nil, errors.New("无可入库的余料")
	}
	if len(reqs) > 100 {
		return nil, errors.New("单次最多入库 100 条")
	}
	rows := make([]model.CutScrap, 0, len(reqs))
	for _, req := range reqs {
		if req.Quantity < 1 {
			req.Quantity = 1
		}
		rows = append(rows, model.CutScrap{
			UserID:      userID,
			ScrapType:   req.ScrapType,
			Label:       req.Label,
			LengthValue: req.LengthValue,
			WidthValue:  req.WidthValue,
			HeightValue: req.HeightValue,
			Quantity:    req.Quantity,
			Note:        req.Note,
		})
	}
	if err := DB.Create(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteScrap 删除余料 (仅本人)
func (s *CutService) DeleteScrap(userID, id uint) error {
	result := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.CutScrap{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("余料不存在")
	}
	return nil
}

// DeductScraps 批量扣减库存余料 (一维, quantity -= count, 归零自动删除); 库存不足报错整体回滚
func (s *CutService) DeductScraps(userID uint, items []model.DeductScrapItem) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return deductScrapsTx(tx, userID, items)
	})
}

func deductScrapsTx(tx *gorm.DB, userID uint, items []model.DeductScrapItem) error {
	for _, item := range items {
		if item.ID == 0 || item.Count < 1 {
			return errors.New("扣减项非法")
		}
		result := tx.Model(&model.CutScrap{}).
			Where("id = ? AND user_id = ? AND scrap_type = ? AND quantity >= ?", item.ID, userID, 1, item.Count).
			Update("quantity", gorm.Expr("quantity - ?", item.Count))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("余料库存不足, 请检查库存数量")
		}
	}
	// 扣至归零的条目自动清理
	return tx.Where("user_id = ? AND quantity <= 0", userID).Delete(&model.CutScrap{}).Error
}

// UpdateScrap 修改库存余料 (仅本人)
func (s *CutService) UpdateScrap(userID, id uint, req model.UpdateScrapRequest) error {
	result := DB.Model(&model.CutScrap{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"label": req.Label, "quantity": req.Quantity, "note": req.Note})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("余料不存在")
	}
	return nil
}

func itoa(i int) string {
	// 请求内临时 map key 的数字后缀, 三处调用点共用, 必须同一实现 (旧 rune 写法 i>=10 会变标点)
	return strconv.Itoa(i)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
