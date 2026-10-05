package service

import (
	"strings"
	"testing"
	"time"

	"backend/model"
)

// 一维切割边界回归测试: 超长/零长零件必须立即报错 (旧实现会死循环 OOM),
// 大组合输入必须快速返回 (旧实现 DFS 全枚举会卡死请求);
// 多材料规格/材料类型/旧料消费行为在这里固化

func TestBarCutOversizedItem(t *testing.T) {
	s := NewCutService(nil)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = s.BarCut(1, model.BarRequest{
			Items:             model.IntItems(7000),
			NewMaterialLength: 6000,
		})
	}()
	select {
	case <-done:
		if err == nil {
			t.Fatal("期望返回错误, 实际成功")
		}
		if !strings.Contains(err.Error(), "超过") {
			t.Fatalf("错误信息不符: %v", err)
		}
		t.Logf("OK: 超长零件立即报错: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("死循环未修复: 5 秒未返回")
	}
}

func TestBarCutZeroItem(t *testing.T) {
	s := NewCutService(nil)
	_, err := s.BarCut(1, model.BarRequest{Items: model.IntItems(0), NewMaterialLength: 6000})
	if err == nil {
		t.Fatal("零长度零件应报错")
	}
	t.Logf("OK: 零长度零件报错: %v", err)
}

func TestBarCutNormal(t *testing.T) {
	s := NewCutService(nil)
	start := time.Now()
	resp, err := s.BarCut(1, model.BarRequest{
		Items:             model.IntItems(2000, 1500, 1500, 1000, 3000, 2500),
		NewMaterialLength: 6000,
		Loss:              5,
	})
	if err != nil {
		t.Fatalf("正常输入不应报错: %v", err)
	}
	totalCut := 0
	for _, r := range resp.Results {
		totalCut += len(r.Cuts)
	}
	if totalCut != 6 {
		t.Fatalf("期望切出 6 件, 实际 %d", totalCut)
	}
	// 汇总一致性: 零件总长 = 11500, 材料总长 = Σ TotalLength
	if resp.Summary.TotalCutLength != 11500 {
		t.Fatalf("汇总零件总长期望 11500, 实际 %v", resp.Summary.TotalCutLength)
	}
	t.Logf("OK: 正常切割 %d 根材料, 利用率 %v%%, 余料 %v, 耗时 %v",
		len(resp.Results), resp.Summary.Utilization, resp.Summary.TotalRemaining, time.Since(start))
}

func TestBarCutEnumExplosion(t *testing.T) {
	// 6 种长度 × 每种 50 件: 旧实现 DFS 组合数 51^6 ≈ 1.8e10, 必然卡死; 新实现应跳过枚举快速返回
	s := NewCutService(nil)
	items := model.IntItems(111, 233, 457, 789, 1024, 1899)
	for i := 0; i < 5; i++ {
		items = append(items, items...)
	}
	done := make(chan struct{})
	start := time.Now()
	var resp *model.BarCutResponse
	var err error
	go func() {
		defer close(done)
		resp, err = s.BarCut(1, model.BarRequest{Items: items, NewMaterialLength: 6000})
	}()
	select {
	case <-done:
		if err != nil {
			t.Fatalf("大组合输入不应报错: %v", err)
		}
		t.Logf("OK: %d 件大组合输入 %v 内完成, 切出 %d 件/%d 根", len(items), time.Since(start), func() int {
			n := 0
			for _, r := range resp.Results {
				n += len(r.Cuts)
			}
			return n
		}(), len(resp.Results))
	case <-time.After(20 * time.Second):
		t.Fatal("DFS 枚举上限未生效: 20 秒未返回")
	}
}

func TestPlaneCutUnplacedAndScraps(t *testing.T) {
	// 二维: 零件大于所有材料 => 未排入(oversized); guillotine 应消费旧料
	s := NewCutService(nil)
	resp, err := s.PlaneCut(model.BinRequest{
		Items: []model.Item{
			{Label: "大件", Width: 3000, Height: 2000, Quantity: 1}, // 超出板材
			{Label: "小件", Width: 400, Height: 300, Quantity: 3},
		},
		Materials: []model.Item{{Label: "旧料A", Width: 1000, Height: 800, Quantity: 1}},
		Height:    1200,
		Width:     2400,
		Strategy:  "Guillotine",
	})
	if err != nil {
		t.Fatalf("平面切割不应报错: %v", err)
	}
	foundOversized := false
	usedScrap := false
	for _, u := range resp.Unplaced {
		if u.Label == "大件" && u.Reason == "oversized" {
			foundOversized = true
		}
	}
	for _, r := range resp.Results {
		if r.MaterialType == "旧料A" {
			usedScrap = true
		}
	}
	if !foundOversized {
		t.Fatalf("超大件应进入未排入清单, 实际: %+v", resp.Unplaced)
	}
	if !usedScrap {
		t.Fatalf("guillotine 应消费旧料, 结果板材类型: %+v", func() []string {
			types := []string{}
			for _, r := range resp.Results {
				types = append(types, r.MaterialType)
			}
			return types
		}())
	}
	if resp.Summary.UnplacedCount != 1 {
		t.Fatalf("汇总未排入数应=1, 实际 %d", resp.Summary.UnplacedCount)
	}
	t.Logf("OK: 未排入警示生效, 旧料被 guillotine 消费, 利用率 %v%%, 用料 %d 块",
		resp.Summary.Utilization, resp.Summary.BinCount)
}

func TestBarCutMultiMaterial(t *testing.T) {
	// 多材料规格: 7000 与 4000 两种新材料; 零件 4500 只能进 7000, 小件应优先用 4000 减少浪费
	s := NewCutService(nil)
	resp, err := s.BarCut(1, model.BarRequest{
		Items: model.IntItems(4500, 4500, 3500, 3500, 3500),
		NewMaterials: []model.BarMaterial{
			{Label: "长料", Length: 7000},
			{Label: "短料", Length: 4000},
		},
		Loss: 0,
	})
	if err != nil {
		t.Fatalf("多材料切割不应报错: %v", err)
	}
	// 校验: 每根材料要么是 7000 要么是 4000; 零件总长 19500 应全部切出
	totalCut := 0
	for _, r := range resp.Results {
		if r.TotalLength != 7000 && r.TotalLength != 4000 {
			t.Fatalf("材料长度 %d 不在规格列表内", r.TotalLength)
		}
		for _, c := range r.Cuts {
			totalCut += c
		}
	}
	if totalCut != 19500 {
		t.Fatalf("零件总长期望 19500, 实际 %d", totalCut)
	}
	if resp.Summary.MaterialCount != len(resp.Results) {
		t.Fatalf("汇总材料根数与结果不一致")
	}
	t.Logf("OK: 多规格切割 %d 根 (长料+短料), 利用率 %v%%, 余料 %v", len(resp.Results), resp.Summary.Utilization, resp.Summary.TotalRemaining)
}

func TestBarCutMultiMaterialOversize(t *testing.T) {
	// 零件超过最长材料(6500 > 6000) => 入口报错
	s := NewCutService(nil)
	_, err := s.BarCut(1, model.BarRequest{
		Items: model.IntItems(6500),
		NewMaterials: []model.BarMaterial{
			{Length: 6000},
			{Length: 4000},
		},
	})
	if err == nil {
		t.Fatal("超过所有材料规格的零件应报错")
	}
	// 但 4500 可以放进 6000
	if _, err = s.BarCut(1, model.BarRequest{
		Items: model.IntItems(4500),
		NewMaterials: []model.BarMaterial{
			{Length: 6000},
			{Length: 4000},
		},
	}); err != nil {
		t.Fatalf("4500 应可放入 6000 料: %v", err)
	}
	t.Logf("OK: 超规格报错 / 合规零件正常")
}

func TestBarCutMaterialTypes(t *testing.T) {
	// 旧料对象形态(带类型) + 多规格新材料: 每根结果的 materialType 应正确标注
	s := NewCutService(nil)
	resp, err := s.BarCut(1, model.BarRequest{
		Items: model.IntItems(3000, 3500, 3500),
		Materials: model.BarMaterialList{
			{Label: "旧方管", Length: 3500},
		},
		NewMaterials: []model.BarMaterial{
			{Label: "长料", Length: 7000},
		},
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	sawScrapType, sawNewType := false, false
	for _, r := range resp.Results {
		if r.MaterialType == "旧方管" {
			sawScrapType = true
		}
		if r.MaterialType == "长料" {
			sawNewType = true
		}
	}
	if !sawScrapType || !sawNewType {
		t.Fatalf("材料类型标注缺失: %+v", func() []string {
			ts := []string{}
			for _, r := range resp.Results {
				ts = append(ts, r.MaterialType)
			}
			return ts
		}())
	}
	// 旧格式 []int 兼容: 不报错且正常出结果
	if _, err := s.BarCut(1, model.BarRequest{
		Items:             model.IntItems(2000),
		NewMaterialLength: 6000,
	}); err != nil {
		t.Fatalf("旧格式 materials/newMaterialLength 应兼容: %v", err)
	}
	t.Logf("OK: 旧料/新料类型标注正确, 旧 JSON 格式兼容")
}
