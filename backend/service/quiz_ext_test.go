package service

import (
	"testing"

	"backend/model"
)

// TestSpellTolerantEqual 拼写容错: 精确/屈折变体/近似
func TestSpellTolerantEqual(t *testing.T) {
	cases := []struct {
		answer, truth string
		want          bool
	}{
		{"remarkable", "remarkable", true},
		{"Remarkable.", "remarkable", true}, // 大小写+标点
		{"colours", "colour", true},         // 复数变体
		{"adapted", "adapt", true},          // 过去式变体
		{"remarkble", "remarkable", true},   // 漏一字母 (相似度 >= 0.85)
		{"different", "remarkable", false},  // 完全不同的词
	}
	for _, c := range cases {
		if got := spellTolerantEqual(c.answer, c.truth); got != c.want {
			t.Errorf("spellTolerantEqual(%q, %q) = %v, want %v", c.answer, c.truth, got, c.want)
		}
	}
}

// TestGradeTranslateV2 翻译判分: 词级重叠换序不罚, 漏词少量不罚
func TestGradeTranslateV2(t *testing.T) {
	truth := "The government announced a new strategy for international cooperation."
	if !gradeTranslateV2("The government announced a new strategy for international cooperation.", truth) {
		t.Fatal("完全一致应判对")
	}
	if !gradeTranslateV2("A new strategy for international cooperation was announced by the government.", truth) {
		t.Fatal("换序+被动式应判对 (词重叠率高)")
	}
	if !gradeTranslateV2("The government announced a new strategy for cooperation.", truth) {
		t.Fatal("漏 1 个词应判对")
	}
	if gradeTranslateV2("I like apples and oranges very much every day.", truth) {
		t.Fatal("完全无关句子应判错")
	}
}

// TestSortDistractorsBySimilarity 干扰项相近优先: 首字母相同/长度接近排前
func TestSortDistractorsBySimilarity(t *testing.T) {
	items := []model.Vocabulary{
		{ID: 1, Word: "apple"},
		{ID: 2, Word: "strange"},
		{ID: 3, Word: "structural"},
		{ID: 4, Word: "banana"},
	}
	sorted := sortDistractorsBySimilarity("strategy", items)
	if sorted[0].Word != "strange" && sorted[0].Word != "structural" {
		t.Fatalf("首字母相同的干扰项应排最前, got %s", sorted[0].Word)
	}
}

// TestPickDocSentencesFilter 文档抽句过滤: 英文为主、词数区间
func TestPickDocSentencesFilter(t *testing.T) {
	if !isMostlyEnglish("The quick brown fox jumps over the lazy dog.") {
		t.Fatal("英文句应通过")
	}
	if isMostlyEnglish("这是一段中文句子，应该被过滤掉。") {
		t.Fatal("中文句应被过滤")
	}
}
