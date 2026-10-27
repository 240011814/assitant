package learning

import (
	"html"
	"net/url"
	"regexp"
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
			// 整块都是图片来源标注 (div 包 span 的图源/图注) 直接丢弃, 不剥壳留噪音文本
			if nodeIsImageCreditBlock(n) {
				return
			}
			// 结构标签剥壳留内容
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				writeSanitizedNode(sb, c, base)
			}
			return
		}
		if !sanitizeVoid[n.Data] && nodeIsImageCreditBlock(n) {
			// 整块都是图片来源标注 (如 <p>Image source, Getty Images</p>), 连标签丢弃
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

// ===================== 图片来源/图片说明读屏标注清理 =====================
// BBC 等站给读屏器的图片标注 ("Image source, Getty Images"/"Image caption, ..."),
// 中文站对应 "图片来源，XXX"/"图片版权，XXX"。
// 结构层面 <figcaption> 已在净化器/htmlToText 中连内容跳过; 行级清洗兜住
// 旧缓存 (figcaption 过滤上线前抓取的文章) 与不在 figcaption 里的标注块。

// 行首前缀匹配: 整行以 "Image source/caption/copyright" (中/英) 开头的行视为噪音行。
// \b 防止误伤 "Image captioning" 这类正文
var imageCreditPrefixRe = regexp.MustCompile(
	`(?i)^\s*(?:image\s+(?:source|caption|copyright)\b|(?:图|圖)片\s*(?:来源|來源|提供|版权|版權))`,
)

// 整行仅为图源名 (Getty Images/Reuters/...): 图源与前缀被拆进相邻两个块时残留的孤儿行
var imageProviderLineRe = regexp.MustCompile(
	`(?i)^\s*(?:getty\s*images?|associated\s*press|ap\s*photo|afp(?:\s*via\s*getty)?|pa\s*media|press\s*association|alamy|istock(?:photo)?|shutterstock|rex\s*features|reuters|bloomberg|epa)\s*[.,;]?\s*$`,
)

// 压回标准 \n\n 分段 (删行/删块后产生的空段)
var blankLinesRe = regexp.MustCompile(`\n{3,}`)

// cleanImageCreditLines 按行剔除图片来源类读屏噪音行 (纯文本正文路径)
func cleanImageCreditLines(text string) string {
	if text == "" {
		return ""
	}
	out := make([]string, 0, 32)
	for _, ln := range strings.Split(text, "\n") {
		if imageCreditPrefixRe.MatchString(ln) || imageProviderLineRe.MatchString(ln) {
			continue
		}
		out = append(out, ln)
	}
	cleaned := blankLinesRe.ReplaceAllString(strings.Join(out, "\n"), "\n\n")
	return strings.TrimSpace(cleaned)
}

// nodeIsImageCreditBlock 判断一个块级容器子树是否"整块都是图片来源标注"
// (部分站点把读屏标注放在 figcaption 之外的 p/li/div 里)。
// 子树含媒体 (img/figure/table/pre...) 时绝不整块丢弃: 图片与来源常同块共存, 误删伤正文
func nodeIsImageCreditBlock(n *xhtml.Node) bool {
	switch n.Data {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "li", "blockquote", "caption", "div":
	default:
		return false
	}
	var sb strings.Builder
	hasMedia := false
	var walk func(c *xhtml.Node)
	walk = func(c *xhtml.Node) {
		if hasMedia {
			return
		}
		switch c.Type {
		case xhtml.TextNode:
			sb.WriteString(c.Data)
		case xhtml.ElementNode:
			if c.Data == "img" || c.Data == "picture" || c.Data == "video" || c.Data == "audio" ||
				c.Data == "svg" || c.Data == "canvas" || c.Data == "iframe" || c.Data == "object" ||
				c.Data == "table" || c.Data == "pre" || c.Data == "figure" {
				hasMedia = true
				return
			}
		}
		if !hasMedia {
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				walk(cc)
			}
		}
	}
	walk(n)
	if hasMedia {
		return false
	}
	// 标注行必短; 长度上限防止误删"恰好以 Image source 开头"的大正文容器
	txt := strings.TrimSpace(sb.String())
	if txt == "" || len([]rune(txt)) > 300 {
		return false
	}
	return imageCreditPrefixRe.MatchString(txt)
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
