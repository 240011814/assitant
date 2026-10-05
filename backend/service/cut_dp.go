package service

import (
	"math"
	"sort"
)

// ===== DP 背包精确模式生成 (替代原 DFS 全组合枚举) =====
// kerf 语义: used = Σ(qty×len) + kerf×(cuts−1) ≤ L。等价变换: 每件有效重 w = len+kerf,
// 容量 C = L+kerf, 则整根浪费 = C − Σ(qty×w) —— "单根利用率最高"就是标准有界背包
// (maximize Σ qty×w)。复杂度 O(类型数×容量), 无组合爆炸, 大订单不再触发枚举放弃。
// 除了全局最优模式, 还为最长的 N 种类型生成"必含一件"变体 (防最优模式全由小件组成、
// 长料无人认领), 该变体同样精确: 一件 t + 剩余容量的最优填充。

const (
	dpMaxCapacity  = 400_000 // 离散化容量上限 (0.01m 粒度下 4km, 防御异常输入)
	dpMaxTypes     = 200
	dpForcedTypes  = 6        // 生成"必含一件"模式的最长类型数
	dpMaxMemory    = 64 << 20 // DP 表内存上限, 超限跳过 DP (回退贪心路径)
)

// dpScale 离散化倍率: 使 kerf×scale 为整数 (kerf 精度最高 0.01)
func dpScale(kerf float64) int {
	for _, s := range []int{1, 10, 100} {
		if v := kerf * float64(s); v == math.Trunc(v) {
			return s
		}
	}
	return 100
}

type dpBundle = int32 // 二进制拆分后的捆重 (同类型内 0/1 递推)

// dpBestPattern 有界背包: 第 i 类单件离散重量 weights[i]、数量上限 caps[i]、容量 C,
// 价值=重量。返回最优选取数量与是否可行 (dp 表超内存/无可行解返回 false)。
// 类型内部按二进制拆分成 0/1 捆降序递推; 回溯按"该类取 k 件"扫描重建 (二进制拆分可
// 表示 0..cap 内任意数量, 等式必可命中)。
func dpBestPattern(weights, caps []int, C int) ([]int, bool) {
	types := len(weights)
	if types == 0 || C < 0 {
		return nil, false
	}
	if (types+1)*(C+1)*4 > dpMaxMemory {
		return nil, false
	}

	dp := make([][]int32, types+1)
	dp[0] = make([]int32, C+1)
	bundlesByType := make([][]dpBundle, types)
	for t := 0; t < types; t++ {
		w, cap := weights[t], caps[t]
		if w <= 0 || cap <= 0 {
			dp[t+1] = dp[t] // 该类不可用: 行保持不变 (只读共享)
			continue
		}
		// 二进制拆分: 1,2,4,...,余数
		k := 1
		for rem := cap; rem > 0; {
			b := min(k, rem)
			bundlesByType[t] = append(bundlesByType[t], int32(b*w))
			rem -= b
			k *= 2
		}
		row := make([]int32, C+1)
		copy(row, dp[t])
		for _, bw := range bundlesByType[t] {
			for c := C; c >= int(bw); c-- {
				if v := row[c-int(bw)] + bw; v > row[c] {
					row[c] = v
				}
			}
		}
		dp[t+1] = row
	}
	if dp[types][C] == 0 {
		return nil, false
	}

	// 回溯: 从最后一个类型往前, 找使 dp[t+1][c] = dp[t][c−k·w] + k·w 的 k
	qty := make([]int, types)
	c := C
	for t := types - 1; t >= 0; t-- {
		if len(bundlesByType[t]) == 0 {
			continue
		}
		w := weights[t]
		target := dp[t+1][c]
		maxK := c / w
		if maxK > caps[t] {
			maxK = caps[t]
		}
		found := false
		for k := 0; k <= maxK; k++ {
			if dp[t][c-k*w]+int32(k*w) == target {
				qty[t] = k
				c -= k * w
				found = true
				break
			}
		}
		if !found {
			return nil, false // 不应发生, 防御
		}
	}
	return qty, true
}

// dpPatternsForCapacity 对一个容量 (新料规格或某根旧料) 生成 DP 精确模式:
// 全局最优 + (仅新料) 最长 dpForcedTypes 类的必含一件变体。经 seen 去重后返回。
// L 为该容量, isNew/scrapIdx 决定模式归属; DP 不可行 (超限) 时返回空, 调用方原有路径兜底。
func (s *CutService) dpPatternsForCapacity(items []aggItem, demand []int, L float64, kerf float64, isNew bool, scrapIdx int, seen map[string]bool) []pattern {
	types := len(items)
	if types == 0 || types > dpMaxTypes || L <= 0 {
		return nil
	}
	scale := dpScale(kerf)
	C := int(math.Round((L + kerf) * float64(scale)))
	if C <= 0 || C > dpMaxCapacity {
		return nil
	}

	weights := make([]int, types)
	caps := make([]int, types)
	order := make([]int, types) // 按长度降序 (必含变体用)
	for t := 0; t < types; t++ {
		weights[t] = int(math.Round((items[t].length + kerf) * float64(scale)))
		maxPieces := int(L / items[t].length)
		caps[t] = min(maxPieces, demand[t])
		if weights[t] <= 0 || weights[t] > C {
			caps[t] = 0
		}
		order[t] = t
	}
	sort.Slice(order, func(i, j int) bool { return items[order[i]].length > items[order[j]].length })

	buildPattern := func(qty []int) *pattern {
		cuts := 0
		for _, q := range qty {
			cuts += q
		}
		if cuts == 0 {
			return nil
		}
		used := s.dot(qty, items) + kerf*float64(cuts-1)
		if used > L+1e-9 {
			return nil // 离散化舍入防御: 超容量丢弃
		}
		return &pattern{qty: qty, used: used, capacity: L, cuts: cuts, isNew: isNew, scrapIdx: scrapIdx}
	}
	addPattern := func(qty []int) *pattern {
		p := buildPattern(qty)
		if p == nil {
			return nil
		}
		key := s.patternKey(qty)
		if !isNew {
			key += "_scrap_" + itoa(scrapIdx)
		}
		if seen[key] {
			return nil
		}
		seen[key] = true
		return p
	}

	var out []pattern
	if qty, ok := dpBestPattern(weights, caps, C); ok {
		if p := addPattern(qty); p != nil {
			out = append(out, *p)
		}
	}
	// 必含变体: 仅新料 (旧料根数多, 控制耗时)
	if isNew && scrapIdx < 0 {
		for n := 0; n < min(dpForcedTypes, types); n++ {
			t := order[n]
			if caps[t] == 0 {
				continue
			}
			subCaps := make([]int, types)
			copy(subCaps, caps)
			subCaps[t]--
			subC := C - weights[t]
			if subCaps[t] < 0 || subC < 0 {
				continue
			}
			if qty, ok := dpBestPattern(weights, subCaps, subC); ok {
				qty[t]++
				if p := addPattern(qty); p != nil {
					out = append(out, *p)
				}
			}
		}
	}
	return out
}
