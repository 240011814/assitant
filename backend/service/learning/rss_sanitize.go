package learning

import (
	"html"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

// RSS 文章 HTML 净化: 服务端白名单重写, 供阅读视图 v-html 安全渲染。
// - 允许的标签白名单放行, 纯结构标签 (div/span/section 等) 剥壳留内容,
//   危险标签 (script/iframe/form/svg...) 连内容一起丢
// - 属性仅保留 a[href]/img[src,alt]/td[colspan,rowspan], URL 限 http(s)
//   且相对地址基于原文页解析为绝对地址; <a> 强制加 target/rel
// - 文本一律转义, 事件属性天然不存在

var sanitizeAllowed = map[string]bool{
	"p": true, "br": true, "hr": true,
	"a": true, "img": true,
	"strong": true, "b": true, "em": true, "i": true, "u": true, "s": true, "del": true, "ins": true, "small": true, "mark": true, "sub": true, "sup": true,
	"code": true, "pre": true, "kbd": true,
	"blockquote": true,
	"ul":         true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "caption": true,
	"figure": true, "figcaption": true,
}

// skipElems 危险/无意义标签: 连内容一起丢弃 (区别于剥壳留内容的结构标签)
// figcaption: BBC 等站的 "Image source, Getty Images"/"Image caption, ..." 读屏标注, 纯噪音
var sanitizeSkip = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true, "frame": true, "frameset": true,
	"object": true, "embed": true, "applet": true, "form": true, "input": true, "button": true,
	"select": true, "textarea": true, "option": true, "svg": true, "math": true, "canvas": true,
	"audio": true, "video": true, "source": true, "track": true, "link": true, "meta": true,
	"title": true, "template": true, "dialog": true,
	"figcaption": true,
}

var sanitizeVoid = map[string]bool{"br": true, "img": true, "hr": true}

// sanitizeArticleHTML 把页面/摘要 HTML 净化为可安全渲染的阅读 HTML。
// pageURL 用于把相对的 img src / a href 解析成绝对地址, 可为空。
func sanitizeArticleHTML(pageURL, body string) string {
	root, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	var bodyNode *xhtml.Node
	// body 在 <html> 之下 (根是 DocumentNode), 递归查找
	var findBody func(n *xhtml.Node)
	findBody = func(n *xhtml.Node) {
		if bodyNode != nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			bodyNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(root)
	if bodyNode == nil {
		bodyNode = root
	}
	var base *url.URL
	if pageURL != "" {
		base, _ = url.Parse(pageURL)
	}
	var sb strings.Builder
	writeSanitizedNode(&sb, bodyNode, base)
	return strings.TrimSpace(sb.String())
}

func writeSanitizedNode(sb *strings.Builder, n *xhtml.Node, base *url.URL) {
	switch n.Type {
	case xhtml.TextNode:
		sb.WriteString(html.EscapeString(n.Data))
		return
	case xhtml.ElementNode:
		if sanitizeSkip[n.Data] {
			return
		}
		if !sanitizeAllowed[n.Data] {
			// 结构标签剥壳留内容
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				writeSanitizedNode(sb, c, base)
			}
			return
		}
		sb.WriteString("<" + n.Data)
		for _, a := range n.Attr {
			switch {
			case n.Data == "a" && a.Key == "href":
				if u := sanitizeSafeURL(base, a.Val); u != "" {
					sb.WriteString(` href="` + html.EscapeString(u) + `"`)
				}
			case n.Data == "img" && a.Key == "src":
				if u := sanitizeSafeURL(base, a.Val); u != "" {
					sb.WriteString(` src="` + html.EscapeString(u) + `"`)
				}
			case n.Data == "img" && a.Key == "alt":
				sb.WriteString(` alt="` + html.EscapeString(a.Val) + `"`)
			case (n.Data == "td" || n.Data == "th") && (a.Key == "colspan" || a.Key == "rowspan"):
				sb.WriteString(` ` + a.Key + `="` + html.EscapeString(a.Val) + `"`)
			}
		}
		if n.Data == "a" {
			sb.WriteString(` target="_blank" rel="noopener noreferrer nofollow"`)
		}
		sb.WriteString(">")
		if sanitizeVoid[n.Data] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeSanitizedNode(sb, c, base)
		}
		sb.WriteString("</" + n.Data + ">")
	default:
		// 注释/doctype 等一律丢弃
	}
}

// sanitizeSafeURL 解析相对地址为绝对地址, 仅放行 http(s)
func sanitizeSafeURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if base != nil {
		if u, err := base.Parse(raw); err == nil {
			raw = u.String()
		} else {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return raw
}
