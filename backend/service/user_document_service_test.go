package service

import (
	"strings"
	"testing"
)

// TestDocChunk read_document 工具的 rune 级分页: 默认值/封顶/越界/多字节边界
func TestDocChunk(t *testing.T) {
	text := "hello世界hello" // 11 rune: h e l l o 世 界 h e l l o -> 12? 逐字数: 5+2+5=12
	// 明确用 rune 数断言而不是硬编码
	total := len([]rune(text))

	cases := []struct {
		name       string
		offset     int
		length     int
		wantChunk  string
		wantOffset int // 断言 chunk 在原文中的 rune 起点 (-1 表示应为空串)
	}{
		{"length=0 取默认 6000", 0, 0, text, 0},
		{"读头部 5 rune", 0, 5, text[:5], 0},
		{"负 offset 归零", -3, 4, text[:4], 0},
		{"中段不切多字节字符", 5, 2, "世界", 5},
		{"超出总量返回空", total + 10, 5, "", -1},
		{"length 封顶 docMaxChunk", 0, docMaxChunk + 100, text, 0},
		{"负 length 取默认值", 0, -1, func() string {
			r := []rune(text)
			if len(r) > docDefaultChunk {
				return string(r[:docDefaultChunk])
			}
			return text
		}(), 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, gotTotal := docChunk(text, tc.offset, tc.length)
			if gotTotal != total {
				t.Fatalf("total = %d, want %d", gotTotal, total)
			}
			if chunk != tc.wantChunk {
				t.Fatalf("chunk = %q, want %q", chunk, tc.wantChunk)
			}
			if tc.wantOffset >= 0 {
				runes := []rune(text)
				if !strings.HasPrefix(string(runes[tc.wantOffset:]), chunk) {
					t.Fatalf("chunk %q 不是 offset=%d 起的前缀", chunk, tc.wantOffset)
				}
			}
		})
	}
}

// TestDocChunkMultibyteBoundary 关键约束: 分页绝不切断多字节字符产生乱码
func TestDocChunkMultibyteBoundary(t *testing.T) {
	text := strings.Repeat("世", 10) // 10 rune, 每个 3 字节
	chunk, total := docChunk(text, 3, 4)
	if chunk != strings.Repeat("世", 4) {
		t.Fatalf("chunk 应为 4 个完整汉字, got %q", chunk)
	}
	if total != 10 {
		t.Fatalf("total = %d, want 10", total)
	}
}

// TestTruncateRunes 解析错误信息截断
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abc", 5); got != "abc" {
		t.Fatalf("未超限应原样返回, got %q", got)
	}
	long := strings.Repeat("世", 10)
	got := truncateRunes(long, 4)
	if got != strings.Repeat("世", 4) {
		t.Fatalf("应按 rune 截断不产生乱码, got %q", got)
	}
}
