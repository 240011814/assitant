package service

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
