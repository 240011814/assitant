package service

import (
	"backend/model"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

type CutService struct{}

func NewCutService() *CutService {
	return &CutService{}
}

// ===== 一维切割算法（列生成 + 贪心分配）=====

// aggItem 聚合后的项目类型
type aggItem struct {
	length float64 // 长度
	demand int     // 需求数量
	indices []int  // 原始索引列表
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

// BarCut 一维切割优化算法
func (s *CutService) BarCut(req model.BarRequest) ([]model.BarResult, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("切割项目不能为空")
	}
	if req.NewMaterialLength <= 0 {
		return nil, errors.New("新材料长度必须大于0")
	}

	L := float64(req.NewMaterialLength)
	kerf := math.Max(0, req.Loss)

	// 入口校验: 每件零件长度必须在 (0, L] 内, 否则该件永远装不进新材料,
	// 求解阶段会死循环 ( OOM/CPU 100%)
	for _, item := range req.Items {
		if item <= 0 {
			return nil, fmt.Errorf("切割项目长度必须大于0, 存在非法项: %d", item)
		}
		if float64(item) > L {
			return nil, fmt.Errorf("切割项目长度 %d 超过新材料长度 %d, 无法切割", item, req.NewMaterialLength)
		}
	}

	// 1. 聚合项目
	aggItems := s.aggregateItems(req.Items)

	// 2. 旧料直配预分配
	fixedResults, remainingScraps, remainingDemand := s.preAssignExactScraps(aggItems, req.Materials, kerf)

	// 3. 生成初始切割模式
	patterns := s.generateInitialPatterns(aggItems, remainingDemand, L, remainingScraps, kerf)

	// 4. 贪心分配求解
	results := s.solveGreedy(patterns, aggItems, remainingDemand, L, remainingScraps, kerf)

	// 合并结果
	fixedResults = append(fixedResults, results...)
	return fixedResults, nil
}

// aggregateItems 聚合相同长度的项目
func (s *CutService) aggregateItems(items []int) []aggItem {
	typeMap := make(map[int]*aggItem)
	order := []int{}

	for i, item := range items {
		if _, exists := typeMap[item]; !exists {
			typeMap[item] = &aggItem{
				length:  float64(item),
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
func (s *CutService) preAssignExactScraps(items []aggItem, scraps []int, kerf float64) ([]model.BarResult, []int, []int) {
	var results []model.BarResult
	remainingScraps := make([]int, len(scraps))
	copy(remainingScraps, scraps)
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
			if item.length == float64(scrap) && remainingDemand[t] > 0 {
				used[i] = true
				remainingDemand[t]--

				results = append(results, model.BarResult{
					Index:       i + 1,
					TotalLength: scrap,
					Cuts:        []int{int(item.length)},
					Used:        round2(item.length),
					Remaining:   0,
				})

				// 移除已使用的索引
				if len(items[t].indices) > 0 {
					items[t].indices = items[t].indices[1:]
				}
				break
			}
		}
	}

	// 过滤已使用的旧料
	var restScraps []int
	for i, scrap := range scraps {
		if !used[i] {
			restScraps = append(restScraps, scrap)
		}
	}

	return results, restScraps, remainingDemand
}

// generateInitialPatterns 生成初始切割模式
func (s *CutService) generateInitialPatterns(items []aggItem, demand []int, L float64, scraps []int, kerf float64) []pattern {
	var patterns []pattern
	seen := make(map[string]bool)

	types := len(items)

	// 1. 单一类型模式
	for t := 0; t < types; t++ {
		maxPieces := int(L / items[t].length)
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

	// 4. 高利用率模式枚举
	s.enumerateEfficientPatterns(items, demand, L, kerf, &patterns, &seen)

	// 5. 旧料模式
	for idx, scrap := range scraps {
		if scrap <= 0 {
			continue
		}
		qty := s.greedyPack(items, float64(scrap), demand, kerf)
		cuts := 0
		for _, q := range qty {
			cuts += q
		}
		if cuts > 0 {
			used := s.dot(qty, items) + kerf*float64(max(0, cuts-1))
			if used <= float64(scrap)+1e-6 {
				key := s.patternKey(qty) + "_scrap_" + itoa(idx)
				if !seen[key] {
					seen[key] = true
					patterns = append(patterns, pattern{
						qty:      qty,
						used:     used,
						capacity: float64(scrap),
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

// enumerateEfficientPatterns 枚举高利用率模式
func (s *CutService) enumerateEfficientPatterns(items []aggItem, demand []int, L float64, kerf float64, patterns *[]pattern, seen *map[string]bool) {
	types := len(items)
	maxPieces := make([]int, types)
	for t := 0; t < types; t++ {
		maxPieces[t] = min(demand[t], int(L/items[t].length))
	}

	// 枚举空间上界 Π(maxPieces+1): 无剪枝的全组合枚举在类型多/需求大时组合爆炸,
	// 会卡死 HTTP 请求, 超过上限直接放弃枚举 (generateInitialPatterns 的贪心/混合模式仍然可用)
	const maxEnumSpace = 1_000_000
	space := 1
	for t := 0; t < types; t++ {
		space *= maxPieces[t] + 1
		if space > maxEnumSpace {
			return
		}
	}

	current := make([]int, types)
	s.dfsEnumerate(items, L, maxPieces, demand, current, 0, patterns, seen, kerf)
}

// dfsEnumerate 深度优先枚举
func (s *CutService) dfsEnumerate(items []aggItem, L float64, maxPieces []int, demand []int, current []int, typeIdx int, patterns *[]pattern, seen *map[string]bool, kerf float64) {
	if typeIdx == len(items) {
		cuts := 0
		for _, q := range current {
			cuts += q
		}
		if cuts == 0 {
			return
		}

		used := 0.0
		for t, q := range current {
			used += float64(q) * items[t].length
		}
		used += kerf * float64(max(0, cuts-1))

		if used <= L+1e-9 {
			utilization := used / L
			isHighUtilization := utilization > 0.90
			isSmallButUseful := cuts >= 2 && used > 0.1*L

			if isHighUtilization || isSmallButUseful {
				qty := make([]int, len(current))
				copy(qty, current)
				key := s.patternKey(qty)
				if !(*seen)[key] {
					(*seen)[key] = true
					*patterns = append(*patterns, pattern{
						qty:      qty,
						used:     used,
						capacity: L,
						cuts:     cuts,
						isNew:    true,
						scrapIdx: -1,
					})
				}
			}
		}
		return
	}

	for n := 0; n <= maxPieces[typeIdx]; n++ {
		current[typeIdx] = n
		s.dfsEnumerate(items, L, maxPieces, demand, current, typeIdx+1, patterns, seen, kerf)
	}
}

// greedyPack 贪心装箱
func (s *CutService) greedyPack(items []aggItem, capLen float64, maxCount []int, kerf float64) []int {
	types := len(items)
	take := make([]int, types)
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
			if take[id] >= maxCount[id] {
				continue
			}
			next := used + items[id].length
			if cuts > 0 {
				next += kerf
			}
			if next <= capLen+1e-9 {
				take[id]++
				cuts++
				used = next
				updated = true
			}
		}
	}
	return take
}

// solveGreedy 贪心求解
func (s *CutService) solveGreedy(patterns []pattern, items []aggItem, demand []int, L float64, scraps []int, kerf float64) []model.BarResult {
	types := len(items)
	remaining := make([]int, len(demand))
	copy(remaining, demand)

	var results []model.BarResult
	newIdx := 1

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

		if canUse {
			// 使用此模式
			cuts := []int{}
			for t := 0; t < types; t++ {
				for i := 0; i < p.qty[t]; i++ {
					cuts = append(cuts, int(items[t].length))
				}
			}

			totalLength := int(L)
			if !p.isNew && p.scrapIdx >= 0 {
				totalLength = scraps[p.scrapIdx]
				scrapUsed[p.scrapIdx] = true
			}

			results = append(results, model.BarResult{
				Index:       newIdx,
				TotalLength: totalLength,
				Cuts:        cuts,
				Used:        round2(p.used),
				Remaining:   round2(p.capacity - p.used),
			})
			newIdx++

			// 更新需求
			for t := 0; t < types; t++ {
				remaining[t] -= p.qty[t]
			}
		}
	}

	// 处理剩余需求
	for t := 0; t < types; t++ {
		for remaining[t] > 0 {
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

			cutLengths := []int{}
			for t2, q := range qty {
				for i := 0; i < q; i++ {
					cutLengths = append(cutLengths, int(items[t2].length))
				}
			}

			// 兜底守卫: 本轮一件都没放下 (入口已校验理论上不应发生), 跳过该类型防止死循环
			if cuts == 0 {
				break
			}

			results = append(results, model.BarResult{
				Index:       newIdx,
				TotalLength: int(L),
				Cuts:        cutLengths,
				Used:        round2(used),
				Remaining:   round2(L - used),
			})
			newIdx++
		}
	}

	return results
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
func (s *CutService) PlaneCut(req model.BinRequest) ([]model.BinResult, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("切割项目不能为空")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return nil, errors.New("材料尺寸必须大于0")
	}

	switch req.Strategy {
	case "Guillotine":
		return s.guillotineCut(req)
	case "MaxRects":
		return s.maxRectsCut(req)
	default:
		return nil, errors.New("不支持的切割策略: " + req.Strategy)
	}
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
func (s *CutService) guillotineCut(req model.BinRequest) ([]model.BinResult, error) {
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

		// 若无板材可放，新开板材
		if !placed {
			newBin := NewGuillotineBin(req.Width, req.Height)
			placement := newBin.Insert(item)
			if placement != nil {
				bins = append(bins, newBin)
				br := model.BinResult{
					BinID:          binID,
					MaterialType:   "新板材",
					MaterialWidth:  req.Width,
					MaterialHeight: req.Height,
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

	return results, nil
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
	Width, Height   float64
	FreeRectangles  []MaxRect
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
func (s *CutService) maxRectsCut(req model.BinRequest) ([]model.BinResult, error) {
	var results []model.BinResult
	binID := 0

	// 展开所有项目
	allItems := s.expandItems(req.Items)

	// 构建可用材料实例列表
	type MaterialInstance struct {
		Name     string
		Width    float64
		Height   float64
		Priority int
	}

	materials := []MaterialInstance{}
	for _, m := range req.Materials {
		count := m.Quantity
		if count < 1 {
			count = 1
		}
		for i := 0; i < count; i++ {
			materials = append(materials, MaterialInstance{Name: m.Label, Width: m.Width, Height: m.Height, Priority: 10})
		}
	}

	// 添加备用材料
	for i := 0; i < 100; i++ {
		materials = append(materials, MaterialInstance{Name: "新板材", Width: req.Width, Height: req.Height, Priority: 0})
	}

	// 按优先级+面积排序
	sort.Slice(materials, func(i, j int) bool {
		if materials[i].Priority != materials[j].Priority {
			return materials[i].Priority > materials[j].Priority
		}
		areaI := materials[i].Width * materials[i].Height
		areaJ := materials[j].Width * materials[j].Height
		return areaI > areaJ
	})

	// 按面积从大到小排序物品
	sort.Slice(allItems, func(i, j int) bool {
		areaI := allItems[i].Width * allItems[i].Height
		areaJ := allItems[j].Width * allItems[j].Height
		return areaI > areaJ
	})

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
			var selectedMaterial *MaterialInstance
			for i, m := range materials {
				if item.Width <= m.Width && item.Height <= m.Height {
					selectedMaterial = &materials[i]
					break
				}
			}
			if selectedMaterial == nil {
				continue
			}

			newBin := NewMaxRectsBin(selectedMaterial.Width, selectedMaterial.Height)
			rect := newBin.Insert(item.Width, item.Height, true)
			if rect != nil {
				bins = append(bins, newBin)

				br := model.BinResult{
					BinID:          binID,
					MaterialType:   selectedMaterial.Name,
					MaterialWidth:  selectedMaterial.Width,
					MaterialHeight: selectedMaterial.Height,
					Pieces:         []model.Piece{createMaxRectPiece(item, *rect)},
				}
				binID++
				results = append(results, br)

				// 移除已使用的材料
				for i, m := range materials {
					if m.Name == selectedMaterial.Name && m.Width == selectedMaterial.Width && m.Height == selectedMaterial.Height {
						materials = append(materials[:i], materials[i+1:]...)
						break
					}
				}
			}
		}
	}

	// 更新利用率
	for i := range results {
		calculateUtilization(&results[i])
	}

	return results, nil
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
				label = item.Label + "_" + itoa(i+1)
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

// SaveCutRecord 保存切割记录
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

	if err := DB.Create(&record).Error; err != nil {
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

func itoa(i int) string {
	return string(rune('0'+i)) + ""
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
