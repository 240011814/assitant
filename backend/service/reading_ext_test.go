package service

import (
	"encoding/json"
	"testing"
)

// TestEstimateUnknownRateFromSet 难度预估: 生词本交集口径 + 长词兜底 + 短文本无意义
func TestEstimateUnknownRateFromSet(t *testing.T) {
	text := "The government announced a remarkable strategy to investigate the phenomenon of international cooperation between different countries and their representative institutions."
	set := map[string]bool{"remarkable": true, "phenomenon": true}

	// 有生词本: 交集口径 (2 命中 / distinct 词数, 略放大)
	r := estimateUnknownRateFromSet(text, set)
	if r <= 0 || r > 1 {
		t.Fatalf("personalized rate out of range: %v", r)
	}
	// 未命中词不计入
	r2 := estimateUnknownRateFromSet("the cat sat on the mat with a hat and the dog saw it too", set)
	if r2 != 0 {
		t.Fatalf("no-hit text should be 0, got %v", r2)
	}

	// 无生词本: 长词启发兜底
	r3 := estimateUnknownRateFromSet(text, map[string]bool{})
	if r3 <= 0 || r3 > 1 {
		t.Fatalf("heuristic rate out of range: %v", r3)
	}

	// 太短: 无统计意义
	if got := estimateUnknownRateFromSet("short text", set); got != 0 {
		t.Fatalf("short text should be 0, got %v", got)
	}
}

// TestSplitAnnotationChunks 分块标注: 段落边界均分 + 少段时不强拆
func TestSplitAnnotationChunks(t *testing.T) {
	paras := make([]string, 10)
	for i := range paras {
		paras[i] = "para"
	}
	joined := ""
	for i, p := range paras {
		if i > 0 {
			joined += "\n\n"
		}
		joined += p
	}
	chunks := splitAnnotationChunks(joined, 3)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		total += countSegments(c)
	}
	if total != 10 {
		t.Fatalf("chunk paragraph count mismatch: %d", total)
	}
	if got := len(splitAnnotationChunks("a\n\nb", 3)); got != 2 {
		t.Fatalf("few paragraphs should return as-is, got %d", got)
	}
}

func countSegments(s string) int {
	n := 1
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\n' && s[i+1] == '\n' {
			n++
			i++
		}
	}
	return n
}

// TestStripVocabTestQuestions 题面剥离: 下发无 answer 字段, 选项顺序不变
func TestStripVocabTestQuestions(t *testing.T) {
	questions := []map[string]interface{}{
		{"id": "a1-1", "band": "A1", "word": "apple", "options": []string{"苹果", "香蕉", "橙子", "葡萄"}, "answer": 0},
	}
	raw, _ := json.Marshal(questions)
	stripped := stripVocabTestQuestions(string(raw))
	if len(stripped) != 1 {
		t.Fatalf("want 1 question, got %d", len(stripped))
	}
	if _, has := stripped[0]["answer"]; has {
		t.Fatal("answer must be stripped before sending to client")
	}
	opts, _ := stripped[0]["options"].([]string)
	if len(opts) != 4 || opts[0] != "苹果" {
		t.Fatalf("options order must be preserved, got %v", opts)
	}
}
