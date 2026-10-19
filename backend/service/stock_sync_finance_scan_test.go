package service

import (
	"testing"
	"time"

	"backend/model"
)

// financeScanTestNow 固定测试时间: 2026-10-19 (Q3 年报尚未发布, Q2 已过最晚披露期08/31)
var financeScanTestNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local)

func financeScanQuarter(y, q int) model.StockFinanceScan {
	return model.StockFinanceScan{Ep: "P", EmptyTop: quarterEndDate(y, q), EmptyBottom: quarterEndDate(y, q)}
}

// TestSpanCovering 区域覆盖判断 (含双边界)
func TestSpanCovering(t *testing.T) {
	spans := []model.StockFinanceScan{
		{EmptyTop: quarterEndDate(2026, 3), EmptyBottom: quarterEndDate(2026, 3)},
		{EmptyTop: quarterEndDate(2022, 2), EmptyBottom: quarterEndDate(2017, 1)},
	}
	if spanCovering(spans, quarterEndDate(2026, 3)) == nil {
		t.Fatal("区段顶报告期应被覆盖")
	}
	if spanCovering(spans, quarterEndDate(2017, 1)) == nil {
		t.Fatal("区段底报告期应被覆盖")
	}
	if spanCovering(spans, quarterEndDate(2026, 2)) != nil {
		t.Fatal("区段上方的季度不应被覆盖")
	}
	if spanCovering(spans, quarterEndDate(2016, 4)) != nil {
		t.Fatal("区段下方的季度不应被覆盖")
	}
}

// TestFinanceScanPending_SkipsMarkedQuarters 已标记的报告期跳过, 不仅有直到顶部的空报告期
func TestFinanceScanPending_SkipsMarkedQuarters(t *testing.T) {
	spansByEp := map[string][]model.StockFinanceScan{} // 无缓存
	marked := func([2]int, string) bool { return false }

	// 无标记无区段: 每来源都是 2007Q1..2026Q3 全量回溯 (首建语义与旧逻辑一致)
	got := financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": false}, financeScanTestNow)
	want := financeQuarterIdx(2026, 3) - financeQuarterIdx(2007, 1) + 1
	if len(got["P"]) != want {
		t.Fatalf("无标记时应全量回溯: got %d, want %d", len(got["P"]), want)
	}
	if got["P"][0] != (financeQuarter{y: 2026, q: 3}) {
		t.Fatalf("待扫描列表应从最新季度开始: %v", got["P"][0])
	}
	if got["P"][len(got["P"])-1] != (financeQuarter{y: 2007, q: 1}) {
		t.Fatalf("待扫描列表应以2007Q1结束: %v", got["P"][len(got["P"])-1])
	}
}

// TestFinanceScanPending_TailSpanStopsDescent 已建库股票: 命中自然尽头区段(未命中轮询桶) => 零请求;
// 命中轮询桶 => 只复扫仍在披露保留窗内的区段内报告期 (上市前的老区段已终结, 永不再扫)
func TestFinanceScanPending_TailSpanStopsDescent(t *testing.T) {
	spansByEp := map[string][]model.StockFinanceScan{
		"P": {
			financeScanQuarter(2026, 3), // 新季度未公布区段 (未终结: 09/30+150天 > now)
			{Ep: "P", EmptyTop: quarterEndDate(2022, 2), EmptyBottom: quarterEndDate(2007, 1)}, // 上市前区段 (已终结)
		},
	}
	// P 已标记 2022Q3..2026Q2
	marked := func(pair [2]int, ep string) bool {
		return ep == "P" && pair[0] == 2026 && pair[1] == 2 ||
			ep == "P" && pair[0] == 2026 && pair[1] == 1 ||
			ep == "P" && pair[0] == 2025 && pair[1] >= 1 ||
			ep == "P" && pair[0] >= 2023 && pair[0] <= 2024 && pair[1] >= 1 ||
			ep == "P" && pair[0] == 2022 && pair[1] == 4 ||
			ep == "P" && pair[0] == 2022 && pair[1] == 3
	}

	// 未命中轮询桶: 覆盖新季度(未终结)和上市前(已终结)区段都跳过, 无需任何请求
	got := financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": false}, financeScanTestNow)
	if len(got["P"]) != 0 {
		t.Fatalf("未命中轮询桶时应跳过全部空区段: %v", got["P"])
	}

	// 命中轮询桶: 只复扫未终结区段(2026Q3), 老区段内季度都已过披露保留窗不扫
	got = financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": true}, financeScanTestNow)
	if len(got["P"]) != 1 || got["P"][0] != (financeQuarter{y: 2026, q: 3}) {
		t.Fatalf("命中轮询桶时应只复扫新季度: %v", got["P"])
	}
}

// TestFinanceScanPending_ProbeCoversFresh span 只覆盖未公布的新季度 (已非轮询桶时跳过)
func TestFinanceScanPending_ProbeCoversFresh(t *testing.T) {
	spansByEp := map[string][]model.StockFinanceScan{
		"P": {financeScanQuarter(2026, 3)},
	}
	// 无标记 (2026Q3..2007Q1 除 2026Q3之外的本来源报告期都无标记)
	marked := func([2]int, string) bool { return false }

	got := financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": false}, financeScanTestNow)
	want := financeQuarterIdx(2026, 2) - financeQuarterIdx(2007, 1) + 1
	// 2026Q3 区段未命中轮询桶跳过, 区段内已终结才停止向下回溯... 但 2026Q3 是区段底部且未终结, 继续向下
	if len(got["P"]) != want {
		t.Fatalf("未终结区段之后应继续向下回溯其余报告期: got %d, want %d", len(got["P"]), want)
	}

	got = financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": true}, financeScanTestNow)
	// 命中桶: 区段内新季度(2026Q3)复扫放最前, 之后继续向下回溯全量未标记报告期
	want = financeQuarterIdx(2026, 3) - financeQuarterIdx(2007, 1) + 1
	if len(got["P"]) != want || got["P"][0] != (financeQuarter{y: 2026, q: 3}) || got["P"][1] != (financeQuarter{y: 2026, q: 2}) {
		t.Fatalf("命中桶时应复扫区段内新季度+继续向下: 首项 %v", got["P"][0])
	}
}

// TestFinanceScanPending_UncacheQuartersBetweenSpans 区段之间的标记缺口仍会向下修复
func TestFinanceScanPending_UncacheQuartersBetweenSpans(t *testing.T) {
	spansByEp := map[string][]model.StockFinanceScan{
		"P": {
			financeScanQuarter(2026, 3),
			{Ep: "P", EmptyTop: quarterEndDate(2015, 4), EmptyBottom: quarterEndDate(2007, 1)},
		},
	}
	// 标记: 2026Q2, 以及 2016Q1..2023Q4 之间已被标记的数据段 (2015Q4 区段上方)
	marked := func(pair [2]int, ep string) bool {
		if ep != "P" {
			return false
		}
		if pair[0] == 2026 && pair[1] == 2 {
			return true
		}
		if pair[0] == 2026 && pair[1] == 1 {
			return true
		}
		if pair[0] == 2025 && pair[1] >= 1 {
			return true
		}
		if pair[0] >= 2021 && pair[0] <= 2024 && pair[1] >= 1 {
			return true
		}
		if pair[0] >= 2016 && pair[0] <= 2020 && pair[1] >= 1 {
			return true
		}
		return false
	}
	got := financeScanPending(2026, 3, 2007, spansByEp, marked, map[string]bool{"P": false}, financeScanTestNow)
	// 无命中轮询桶: 2026Q3 区段跳过; 2016..2023 标记段中间的 2019Q1..Q4 等缺口季度待扫?
	// 标记已覆盖 2016Q1..2024Q4 中仅部分. 手工构造: 2016Q1..2020Q4 与 2021Q1..2024Q4 均全标记, 无缺口,
	// 下方区段已终结 -> 待扫列表应该为空.
	if len(got["P"]) != 0 {
		t.Fatalf("标记完整无缺口时应零请求: %v", got["P"])
	}

	// 制造缺口: 2019Q4 未标记 (旧标记覆盖缺失, 需要修复)
	markedWithGap := func(pair [2]int, ep string) bool {
		if ep != "P" {
			return false
		}
		if pair[0] == 2019 && pair[1] == 4 {
			return false
		}
		return marked(pair, ep)
	}
	got = financeScanPending(2026, 3, 2007, spansByEp, markedWithGap, map[string]bool{"P": false}, financeScanTestNow)
	if len(got["P"]) != 1 || got["P"][0] != (financeQuarter{y: 2019, q: 4}) {
		t.Fatalf("区段之间缺口应能继续修复: %v", got["P"])
	}
}

// TestRebuildFinanceScans 打洞/成段/合并/去重与变化检测
func TestRebuildFinanceScans(t *testing.T) {
	const code = "sz.000001"
	const ep = "P"

	t.Run("区段中已补数据的报告期打洞并重新成段", func(t *testing.T) {
		marked := func(pair [2]int) bool { return pair[0] == 2019 && pair[1] == 3 }
		rows, changed := rebuildFinanceScans(code, ep,
			[]model.StockFinanceScan{{Ep: ep, EmptyTop: quarterEndDate(2020, 1), EmptyBottom: quarterEndDate(2019, 1)}},
			nil, marked)
		if !changed || len(rows) != 2 {
			t.Fatalf("应拆成两段且需要写库: rows=%v changed=%v", rows, changed)
		}
		if rows[0].EmptyTop != quarterEndDate(2020, 1) || rows[0].EmptyBottom != quarterEndDate(2019, 4) {
			t.Fatalf("上段错误: %v", rows[0])
		}
		if rows[1].EmptyTop != quarterEndDate(2019, 2) || rows[1].EmptyBottom != quarterEndDate(2019, 1) {
			t.Fatalf("下段错误: %v", rows[1])
		}
	})

	t.Run("相邻区段合并", func(t *testing.T) {
		marked := func([2]int) bool { return false }
		rows, changed := rebuildFinanceScans(code, ep,
			[]model.StockFinanceScan{{Ep: ep, EmptyTop: quarterEndDate(2020, 2), EmptyBottom: quarterEndDate(2020, 2)}},
			[]model.StockFinanceScan{{Ep: ep, EmptyTop: quarterEndDate(2020, 1), EmptyBottom: quarterEndDate(2020, 1)}},
			marked)
		if !changed || len(rows) != 1 || rows[0].EmptyTop != quarterEndDate(2020, 2) || rows[0].EmptyBottom != quarterEndDate(2020, 1) {
			t.Fatalf("相邻应合并为一段: rows=%v changed=%v", rows, changed)
		}
	})

	t.Run("重复扫描去重且无变化不写库", func(t *testing.T) {
		marked := func([2]int) bool { return false }
		spans := []model.StockFinanceScan{{Ep: ep, EmptyTop: quarterEndDate(2026, 3), EmptyBottom: quarterEndDate(2026, 3)}}
		rows, changed := rebuildFinanceScans(code, ep, spans, spans, marked)
		if changed || len(rows) != 1 {
			t.Fatalf("无变化不应写库: rows=%v changed=%v", rows, changed)
		}
	})

	t.Run("自然尽头预设延伸与常规空段按季度末成段", func(t *testing.T) {
		marked := func([2]int) bool { return false }
		// 上轮实际场景: 新季度空段 与 自然尽头断点(预设延伸一段到 2007Q1) 同轮写入, 中间的报告期未被标记
		rows, changed := rebuildFinanceScans(code, ep, nil,
			[]model.StockFinanceScan{
				{Ep: ep, EmptyTop: quarterEndDate(2026, 3), EmptyBottom: quarterEndDate(2026, 3)},
				{Ep: ep, EmptyTop: quarterEndDate(2022, 2), EmptyBottom: quarterEndDate(2007, 1)},
			},
			marked)
		if !changed || len(rows) != 2 {
			t.Fatalf("两段空区段应独立成段: rows=%v changed=%v", rows, changed)
		}
		if rows[0].EmptyTop != quarterEndDate(2026, 3) || rows[0].EmptyBottom != quarterEndDate(2026, 3) {
			t.Fatalf("上段错误: %v", rows[0])
		}
		if rows[1].EmptyTop != quarterEndDate(2022, 2) || rows[1].EmptyBottom != quarterEndDate(2007, 1) {
			t.Fatalf("尽头预设段错误: %v", rows[1])
		}
	})

	t.Run("单纯预设区间为一段", func(t *testing.T) {
		marked := func([2]int) bool { return false }
		rows, changed := rebuildFinanceScans(code, ep, nil,
			[]model.StockFinanceScan{{Ep: ep, EmptyTop: quarterEndDate(2026, 3), EmptyBottom: quarterEndDate(2007, 1)}},
			marked)
		if !changed || len(rows) != 1 || rows[0].EmptyTop != quarterEndDate(2026, 3) || rows[0].EmptyBottom != quarterEndDate(2007, 1) {
			t.Fatalf("连续空区段应为一个区段: rows=%v changed=%v", rows, changed)
		}
	})
}

// TestFinanceProbeBucketHit 确定性
func TestFinanceProbeBucketHit(t *testing.T) {
	a := financeProbeBucketHit("sz.000001", "P", financeScanTestNow)
	b := financeProbeBucketHit("sz.000001", "P", financeScanTestNow)
	if a != b {
		t.Fatal("同一输入应命中一致")
	}
	// 全部来源命中比例应在合理范围 (1/7 左右)
	hits := 0
	for _, ep := range []string{"P", "G", "O", "C", "B"} {
		if financeProbeBucketHit("sz.000001", ep, financeScanTestNow) {
			hits++
		}
	}
	if hits > 2 {
		t.Fatalf("同一天全部来源不该多数命中: %d", hits)
	}
}

// TestQuarterEndDate 季度末日期与 (y,q) 反推的往返一致性
func TestQuarterEndDate(t *testing.T) {
	cases := map[int]int{1: 3, 2: 6, 3: 9, 4: 12}
	for q, month := range cases {
		d := quarterEndDate(2026, q)
		if int(d.Month()) != month {
			t.Fatalf("Q%d 的月份应为 %d: %v", q, month, d)
		}
		ry, rq := quarterPairOfDate(d)
		if ry != 2026 || rq != q {
			t.Fatalf("季度末往返反推失败: %v -> (%d,%d)", d, ry, rq)
		}
	}
	if y, q := prevQuarterPair(2026, 1); y != 2025 || q != 4 {
		t.Fatalf("2026Q1 的上一季度应为 2025Q4: (%d,%d)", y, q)
	}
}
