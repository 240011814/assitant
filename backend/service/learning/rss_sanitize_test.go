package learning

import (
	"strings"
	"testing"
)

// TestSanitizeArticleHTMLSafety 安全属性: 脚本/事件/危险协议必须被剥干净
func TestSanitizeArticleHTMLSafety(t *testing.T) {
	body := `<div><p onclick="alert(1)">hello <script>alert(2)</script>world</p>
	<a href="javascript:alert(3)">bad link</a>
	<a href="https://ok.com/x">good link</a>
	<img src="x" onerror="alert(4)">
	<iframe src="https://evil.com"></iframe>
	<style>.x{}</style></div>`
	out := sanitizeArticleHTML("https://example.com/a", body)
	for _, bad := range []string{"<script", "onclick", "javascript:", "onerror", "<iframe", "<style"} {
		if strings.Contains(out, bad) {
			t.Fatalf("净化后仍含危险内容 %q: %s", bad, out)
		}
	}
	for _, good := range []string{"hello", "world", `href="https://ok.com/x"`, `rel="noopener noreferrer nofollow"`} {
		if !strings.Contains(out, good) {
			t.Fatalf("正常内容丢失 %q: %s", good, out)
		}
	}
	// javascript: 链接的 <a> 应被剥成纯文本
	if strings.Contains(out, `href="javascript`) {
		t.Fatalf("危险协议链接未剔除: %s", out)
	}
}

// TestSanitizeArticleHTMLResolve 相对地址必须基于原文页解析成绝对地址
func TestSanitizeArticleHTMLResolve(t *testing.T) {
	body := `<article><p>text <img src="/img/pic.jpg" alt="图"> <a href="/story/x">link</a></p></article>`
	out := sanitizeArticleHTML("https://example.com/a/1", body)
	if !strings.Contains(out, `src="https://example.com/img/pic.jpg"`) {
		t.Fatalf("img 相对地址未解析: %s", out)
	}
	if !strings.Contains(out, `href="https://example.com/story/x"`) {
		t.Fatalf("a 相对地址未解析: %s", out)
	}
	if strings.Contains(out, "<article") {
		t.Fatalf("结构标签应被剥壳: %s", out)
	}
}

// TestSanitizeArticleHTMLTextEscaped 文本节点必须转义 (注入 <img> 字面量的防绕过)
func TestSanitizeArticleHTMLTextEscaped(t *testing.T) {
	out := sanitizeArticleHTML("", `<p>a &lt;b&gt; c &amp; d</p>`)
	if !strings.Contains(out, "&lt;b&gt;") || !strings.Contains(out, "&amp;") {
		t.Fatalf("文本转义异常: %s", out)
	}
	if strings.Contains(out, "<b>") {
		t.Fatalf("字面量标签不应复活: %s", out)
	}
}

// TestFigcaptionDropped BBC 图片说明 (读屏标注) 应从正文提取与净化 HTML 中剔除,
// 图片本体保留。样例结构复刻 BBC 原文: figure > picture/img + figcaption(span 前缀 + div>p 内容)
func TestFigcaptionDropped(t *testing.T) {
	page := `<html><head><title>t</title></head><body><article>` +
		`<h1>Headline here</h1>` +
		`<figure><div><span><picture><source srcset="a.webp" type="image/webp"/>` +
		`<img alt="photo" src="https://ichef.bbci.co.uk/x.jpg"/></picture></span></div>` +
		`<figcaption><span>Image caption, </span><div spacing="6"><p>Travellers are subject to checks</p></div></figcaption>` +
		`<figcaption><span>Image source, </span><div spacing="6"><p>Getty Images</p></div></figcaption>` +
		`</figure>` +
		`<p>Body text about the news story continues here.</p>` +
		`</article></body></html>`

	text, _, err := htmlToText(page)
	if err != nil {
		t.Fatalf("htmlToText 失败: %v", err)
	}
	if strings.Contains(text, "Image source") || strings.Contains(text, "Getty Images") || strings.Contains(text, "Image caption") {
		t.Errorf("纯文本仍含读屏标注: %q", text)
	}
	if !strings.Contains(text, "Body text about the news story") || !strings.Contains(text, "Headline here") {
		t.Errorf("正文/标题丢失: %q", text)
	}

	out := sanitizeArticleHTML("https://www.bbc.co.uk/news/articles/x", page)
	if strings.Contains(out, "Image source") || strings.Contains(out, "Getty Images") || strings.Contains(out, "Image caption") {
		t.Errorf("净化 HTML 仍含读屏标注: %q", out)
	}
	if !strings.Contains(out, `<p>Body text about the news story continues here.</p>`) {
		t.Errorf("净化 HTML 段落丢失: %q", out)
	}
	if !strings.Contains(out, `src="https://ichef.bbci.co.uk/x.jpg"`) {
		t.Errorf("图片 src 丢失: %q", out)
	}
}

// TestCleanImageCreditLines 行级剔除: 各种中文/英文图源标注行删掉, 正文行保留
func TestCleanImageCreditLines(t *testing.T) {
	in := strings.Join([]string{
		"Body first paragraph about the news.",
		"Image source, Getty Images",
		"IMAGE SOURCE: Reuters",                        // 全大写 + 冒号
		"Image caption, People queue at a border post", // 图注
		"Image copyright Reuters",                      // 旧版 BBC 无标点格式
		"图片来源，Getty Images",                            // 中文站
		"图片版权：BBC News",
		"Getty Images", // 图源孤儿行
		"AP Photo",     // 图源孤儿行
		"Body second paragraph mentioning the weather.",
	}, "\n")
	out := cleanImageCreditLines(in)
	for _, noise := range []string{"Image source", "Image caption", "Image copyright", "图片来源", "图片版权", "AP Photo"} {
		if strings.Contains(out, noise) {
			t.Errorf("噪音行残留: %q", out)
		}
	}
	for _, keep := range []string{"Body first paragraph", "Body second paragraph mentioning the weather."} {
		if !strings.Contains(out, keep) {
			t.Errorf("正文行被误删: %q", out)
		}
	}
	// 正文行里恰好包含字样但不在行首, 不能误删
	if out2 := cleanImageCreditLines("The images source of truth is the dataset.\nThe caption says it all.\nThis story was updated."); len(out2) != len("The images source of truth is the dataset.\nThe caption says it all.\nThis story was updated.") {
		t.Errorf("中段字样误删: %q", out2)
	}
	// "Image captioning" 不是读屏标注, 保留
	if out3 := cleanImageCreditLines("Image captioning robots are the new news readers."); !strings.Contains(out3, "Image captioning") {
		t.Errorf("Image captioning 被误删: %q", out3)
	}
}

// TestSanitizeCreditOnlyBlockDropped 读屏标注不在 figcaption 里 (裸 p / div 包 span):
// 整块纯文本的标注段从净化 HTML 丢弃; 图片同块共存时保留; 行内 span 里的字样不动
func TestSanitizeCreditOnlyBlockDropped(t *testing.T) {
	page := `<html><body><article>` +
		`<p>He described the scene as quiet and eerie.</p>` +
		`<p>Image source, Getty Images</p>` +
		`<div><span>Image source, </span><span>Getty Images</span></div>` +
		`<p><img src="https://ichef.bbci.co.uk/y.jpg"/>Image source, Getty Images</p>` + // 图文同块, 保留
		`<p>More coverage of the story winding down. <span>Getty Images</span> is credited.</p>` + // 行内字样, 保留
		`<p>Image captioning robots are the new news readers.</p>` +
		`</article></body></html>`

	out := sanitizeArticleHTML("https://www.bbc.co.uk/news/articles/z", page)
	// 图文同块保留是设计行为 (丢块会连图片一起丢), 先摘除该段再断言无残留
	outWithoutMediaBlock := strings.ReplaceAll(out, `<p><img src="https://ichef.bbci.co.uk/y.jpg">Image source, Getty Images</p>`, "")
	if strings.Contains(outWithoutMediaBlock, "Image source") {
		t.Errorf("块级标注残留: %q", out)
	}
	if !strings.Contains(out, "quiet and eerie") {
		t.Errorf("正文段被误删: %q", out)
	}
	if !strings.Contains(out, `src="https://ichef.bbci.co.uk/y.jpg"`) {
		t.Errorf("图文同块的整段被误删: %q", out)
	}
	if !strings.Contains(out, "More coverage of the story winding down.") {
		t.Errorf("行内字样所在正文段被误删: %q", out)
	}
	if !strings.Contains(out, "Image captioning") {
		t.Errorf("正文与标注前缀撞样的段落被误删: %q", out)
	}
}

// TestHtmlToTextCreditLineOutsideFigcaption 标注块不在 figcaption (裸 div/p) 时,
// 纯文本提取按行兜住
func TestHtmlToTextCreditLineOutsideFigcaption(t *testing.T) {
	page := `<html><body><article>` +
		`<h1>Headline</h1>` +
		`<div class="media-caption">Image source, Getty Images</div>` +
		`<p>Body text continues here.</p>` +
		`<p>Getty Images</p>` +
		`</article></body></html>`
	text, _, err := htmlToText(page)
	if err != nil {
		t.Fatalf("htmlToText 失败: %v", err)
	}
	if strings.Contains(text, "Image source") || strings.Contains(text, "Getty Images") {
		t.Errorf("纯文本仍含图源标注: %q", text)
	}
	if !strings.Contains(text, "Body text continues here") || !strings.Contains(text, "Headline") {
		t.Errorf("正文/标题被误删: %q", text)
	}
}
