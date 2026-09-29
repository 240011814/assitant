package service

import (
	"strings"
	"testing"
	"time"

	"backend/model"
)

// 一维切割边界回归测试: 超长/零长零件必须立即报错 (旧实现会死循环 OOM),
// 大组合输入必须快速返回 (旧实现 DFS 全枚举会卡死请求)

func TestBarCutOversizedItem(t *testing.T) {
	s := NewCutService()
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = s.BarCut(model.BarRequest{
			Items:             []int{7000},
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
	s := NewCutService()
	_, err := s.BarCut(model.BarRequest{Items: []int{0}, NewMaterialLength: 6000})
	if err == nil {
		t.Fatal("零长度零件应报错")
	}
	t.Logf("OK: 零长度零件报错: %v", err)
}

func TestBarCutNormal(t *testing.T) {
	s := NewCutService()
	start := time.Now()
	results, err := s.BarCut(model.BarRequest{
		Items:             []int{2000, 1500, 1500, 1000, 3000, 2500},
		NewMaterialLength: 6000,
		Loss:              5,
	})
	if err != nil {
		t.Fatalf("正常输入不应报错: %v", err)
	}
	totalCut := 0
	for _, r := range results {
		totalCut += len(r.Cuts)
	}
	if totalCut != 6 {
		t.Fatalf("期望切出 6 件, 实际 %d", totalCut)
	}
	t.Logf("OK: 正常切割 %d 根材料, 耗时 %v", len(results), time.Since(start))
}

func TestBarCutEnumExplosion(t *testing.T) {
	// 6 种长度 × 每种 50 件: 旧实现 DFS 组合数 51^6 ≈ 1.8e10, 必然卡死; 新实现应跳过枚举快速返回
	s := NewCutService()
	items := []int{111, 233, 457, 789, 1024, 1899}
	for i := 0; i < 5; i++ {
		items = append(items, items...)
	}
	done := make(chan struct{})
	start := time.Now()
	var results []model.BarResult
	var err error
	go func() {
		defer close(done)
		results, err = s.BarCut(model.BarRequest{Items: items, NewMaterialLength: 6000})
	}()
	select {
	case <-done:
		if err != nil {
			t.Fatalf("大组合输入不应报错: %v", err)
		}
		t.Logf("OK: %d 件大组合输入 %v 内完成, 切出 %d 件/%d 根", len(items), time.Since(start), func() int {
			n := 0
			for _, r := range results {
				n += len(r.Cuts)
			}
			return n
		}(), len(results))
	case <-time.After(20 * time.Second):
		t.Fatal("DFS 枚举上限未生效: 20 秒未返回")
	}
}
