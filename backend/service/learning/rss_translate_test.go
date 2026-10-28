package learning

import (
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// parseTestHTML 解析片段并返回 body 节点
func parseTestHTML(t *testing.T, frag string) *xhtml.Node {
	t.Helper()
	root, err := xhtml.Parse(strings.NewReader(frag))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body := findHTMLBody(root)
	if body == nil {
		t.Fatal("no body")
	}
	return body
}

func TestCollectRSSUnits(t *testing.T) {
	body := parseTestHTML(t, `<div><p>First paragraph here.</p>`+
		`<blockquote><p>Quoted inner.</p></blockquote>`+
		`<ul><li>Item one</li><li>Item two</li></ul>`+
		`<p><img src="https://x/a.png" alt="pic"></p>`+
		`<p>a</p>`+
		`<pre>  keep   raw
line two  </pre>`+
		`</div>`)
	units := collectRSSUnits(body)
	texts := make([]string, 0, len(units))
	for _, u := range units {
		texts = append(texts, u.tag+"|"+u.text)
	}
	got := strings.Join(texts, "\n")
	want := strings.Join([]string{
		"p|First paragraph here.",
		// blockquote 是翻译单元: 整块引用 (含内部 p) 作为一个单元, 不重复收集内层块
		"blockquote|Quoted inner.",
		"li|Item one",
		"li|Item two",
		// 纯图片块与超短块 ("a") 不进翻译单元
		"pre|keep   raw\nline two", // pre 保留原始空白
	}, "\n")
	if got != want {
		t.Fatalf("units mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildTranslateBatches(t *testing.T) {
	// 三个 1000-rune 单元, 预算 2400: [0,1] 成批 (2000), 第三个放不下独立成批
	mk := func(lens ...int) []*rssTranslateUnit {
		units := make([]*rssTranslateUnit, 0, len(lens))
		for _, n := range lens {
			units = append(units, &rssTranslateUnit{tag: "p", text: strings.Repeat("x", n)})
		}
		return units
	}
	batches := buildTranslateBatches(mk(1000, 1000, 1000))
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("want [[0 1] [2]], got %v", batches)
	}
	// 单个单元超预算也必须独立成批 (不死循环、不丢弃), 之后的普通单元照常合并
	batches = buildTranslateBatches(mk(3000, 10, 10))
	if len(batches) != 2 || len(batches[0]) != 1 || batches[0][0] != 0 {
		t.Fatalf("oversize unit should be its own batch, got %v", batches)
	}
}

func TestAssembleBilingualHTML(t *testing.T) {
	frag := `<div><p>One.</p><ul><li>Item</li></ul><pre>code</pre><p>skip me</p></div>`
	body := parseTestHTML(t, frag)
	units := collectRSSUnits(body)
	if len(units) != 4 {
		t.Fatalf("want 4 units, got %d", len(units))
	}
	// 第 4 个单元 ("skip me") 用空译文模拟"不翻译", 组装时必须跳过
	zh := []string{"一。", "条目", "代码", ""}
	out := assembleBilingualHTML(body, units, zh)

	// p: 译文作为兄弟块 (原文块带 rss-src 标记); li: 译文在 li 内部 (div 不能直接放 ul 下); pre: 带 rss-zh-pre 类
	for _, want := range []string{
		`<p class="rss-src">One.</p><div class="rss-zh">一。</div>`,
		`<li class="rss-src">Item<div class="rss-zh">条目</div></li>`,
		`<pre class="rss-src">code</pre><div class="rss-zh rss-zh-pre">代码</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// 空译文不插入
	if strings.Contains(out, `>skip me</p><div`) {
		t.Fatalf("empty zh should be skipped:\n%s", out)
	}
	// 译文文本须被转义
	body2 := parseTestHTML(t, `<div><p>xy</p></div>`)
	units2 := collectRSSUnits(body2)
	out2 := assembleBilingualHTML(body2, units2, []string{"<b>注入</b>"})
	if strings.Contains(out2, "<b>注入</b>") {
		t.Fatalf("zh text must be escaped:\n%s", out2)
	}
	if !strings.Contains(out2, "&lt;b&gt;注入&lt;/b&gt;") {
		t.Fatalf("escaped zh missing:\n%s", out2)
	}
}

func TestRSSContentHashStable(t *testing.T) {
	a := rssContentHash("<p>hello</p>")
	b := rssContentHash("<p>hello</p>")
	c := rssContentHash("<p>hello2</p>")
	if a != b || a == c {
		t.Fatalf("hash unstable: %s %s %s", a, b, c)
	}
	if len(a) != 64 {
		t.Fatalf("sha256 hex length want 64, got %d", len(a))
	}
}
