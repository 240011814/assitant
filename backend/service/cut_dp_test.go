package service

import (
	"testing"
)

// 贪心降序会失败的经典案例: L=100, 零件 {65, 60, 41, 39} 各 1 件
// 贪心降序只能放下 65 (利用率 0.65); DP 精确解 60+39=99 (利用率 0.99)
func TestDPPatternBeatsGreedy(t *testing.T) {
	s := NewCutService(nil)
	items := []aggItem{{length: 65, demand: 1}, {length: 60, demand: 1}, {length: 41, demand: 1}, {length: 39, demand: 1}}
	demand := []int{1, 1, 1, 1}
	patterns := s.generateInitialPatterns(items, demand, 100, nil, 0)

	var bestUsed float64
	for _, p := range patterns {
		if p.used > bestUsed {
			bestUsed = p.used
		}
	}
	if bestUsed < 99-1e-9 {
		t.Fatalf("DP 应找到 60+39=99 的模式, 实际最优 used=%v", bestUsed)
	}
}

// kerf 语义: used = n×len + kerf×(n−1)。L=10, kerf=2, len=4 → 最多 2 件 (8+2=10 恰好)
func TestDPPatternKerfSemantics(t *testing.T) {
	s := NewCutService(nil)
	items := []aggItem{{length: 4, demand: 5}}
	demand := []int{5}
	patterns := s.generateInitialPatterns(items, demand, 10, nil, 2)

	var bestUsed float64
	var bestCuts int
	for _, p := range patterns {
		if p.used > bestUsed {
			bestUsed = p.used
			bestCuts = p.cuts
		}
	}
	if bestCuts != 2 || bestUsed != 10 {
		t.Fatalf("考虑 kerf 应最多 2 件且 used=10, 实际 cuts=%d used=%v", bestCuts, bestUsed)
	}
}

// 爆炸规模: 40 种类型 (旧 DFS 枚举空间远超 1e6 会直接放弃), DP 仍产出高利用率模式
func TestDPPatternLargeScale(t *testing.T) {
	s := NewCutService(nil)
	types := 40
	items := make([]aggItem, types)
	demand := make([]int, types)
	for i := 0; i < types; i++ {
		items[i] = aggItem{length: float64(1000 - i*13), demand: 20}
		demand[i] = 20
	}
	patterns := s.generateInitialPatterns(items, demand, 5000, nil, 1)
	if len(patterns) == 0 {
		t.Fatal("应产出模式")
	}
	var bestUtil float64
	var greedyUtil float64
	for _, p := range patterns {
		u := p.used / p.capacity
		if u > bestUtil {
			bestUtil = u
		}
		if p.cuts <= len(items) { // 贪心/单类型模式粗略上界: 每类至多一件或纯小件, 不精确, 仅对比用
			_ = u
		}
	}
	_ = greedyUtil
	if bestUtil < 0.99 {
		t.Fatalf("大规模下 DP 最优模式利用率应 ≥0.99, 实际 %v", bestUtil)
	}
}

// 必含长料变体: 全局最优是 51+49=100 (不含 30), 贪心降序同样产出 51+49;
// 只有"必含一件 30"的变体能产出 30+49=79 —— 保证 30 长度件有机会被认领
func TestDPPatternForcedLongPiece(t *testing.T) {
	s := NewCutService(nil)
	items := []aggItem{
		{length: 51, demand: 1},
		{length: 49, demand: 1},
		{length: 30, demand: 1},
	}
	demand := []int{1, 1, 1}
	patterns := s.generateInitialPatterns(items, demand, 100, nil, 0)

	found := false
	for _, p := range patterns {
		if p.qty[0] == 0 && p.qty[1] == 1 && p.qty[2] == 1 && p.used == 79 {
			found = true
		}
	}
	if !found {
		t.Fatal("应存在必含 30 的变体模式 (30+49=79)")
	}
}

func TestDPBestPatternUnit(t *testing.T) {
	// 完美装满
	qty, ok := dpBestPattern([]int{60, 40}, []int{1, 1}, 100)
	if !ok || qty[0] != 1 || qty[1] != 1 {
		t.Fatalf("60+40 应精确装满 100: %v ok=%v", qty, ok)
	}
	// 数量上限约束
	qty, ok = dpBestPattern([]int{30}, []int{2}, 100)
	if !ok || qty[0] != 2 {
		t.Fatalf("上限 2 应只取 2 件: %v ok=%v", qty, ok)
	}
	// 单件超容量
	qty, ok = dpBestPattern([]int{150}, []int{1}, 100)
	if ok || qty != nil {
		t.Fatalf("无可行解应返回 false: %v ok=%v", qty, ok)
	}
	// 内存守卫: 巨大容量跳过
	bigWeights := make([]int, 10)
	bigCaps := make([]int, 10)
	for i := range bigWeights {
		bigWeights[i] = 1
		bigCaps[i] = 1
	}
	_, ok = dpBestPattern(bigWeights, bigCaps, 100_000_000)
	if ok {
		t.Fatal("超大容量应触发内存守卫返回 false")
	}
}
