package settings

import (
	stdhtml "html"
	"net/url"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

type notificationMessage struct {
	Content string
	Format  NotificationTemplateFormat
}

func normalizeNotificationTemplateFormat(format NotificationTemplateFormat) NotificationTemplateFormat {
	switch format {
	case NotificationTemplateFormatMarkdown, NotificationTemplateFormatHTML:
		return format
	default:
		return NotificationTemplateFormatText
	}
}

// markdownForChannel 将 HTML 模板转换成机器人普遍支持的 Markdown 子集。转换过程使用
// HTML 解析树而不是正则剥离标签，既保留常见排版，又确保 script/style 等不可见内容不会
// 混入通知。Markdown 源文本则原样交给目标平台解析。
func markdownForChannel(message notificationMessage) string {
	if normalizeNotificationTemplateFormat(message.Format) != NotificationTemplateFormatHTML {
		return message.Content
	}
	converted, err := htmlToMarkdown(message.Content)
	if err != nil {
		return message.Content
	}
	return converted
}

func htmlToMarkdown(source string) (string, error) {
	document, err := xhtml.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	renderMarkdownNode(&builder, document)
	return normalizeBlockWhitespace(builder.String()), nil
}

func renderMarkdownNode(builder *strings.Builder, node *xhtml.Node) {
	if node.Type == xhtml.TextNode {
		builder.WriteString(node.Data)
		return
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		return
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "script", "style", "noscript", "template", "head":
		return
	case "br":
		builder.WriteByte('\n')
		return
	case "hr":
		ensureLineBreaks(builder, 2)
		builder.WriteString("---")
		ensureLineBreaks(builder, 2)
		return
	case "img":
		alt := strings.TrimSpace(nodeAttribute(node, "alt"))
		sourceURL := safeLink(nodeAttribute(node, "src"))
		if sourceURL != "" {
			builder.WriteString("![")
			builder.WriteString(escapeMarkdownLabel(alt))
			builder.WriteString("](")
			builder.WriteString(sourceURL)
			builder.WriteByte(')')
		} else if alt != "" {
			builder.WriteString(alt)
		}
		return
	case "pre":
		ensureLineBreaks(builder, 2)
		builder.WriteString("```\n")
		builder.WriteString(strings.TrimSpace(htmlTextContent(node)))
		builder.WriteString("\n```")
		ensureLineBreaks(builder, 2)
		return
	case "blockquote":
		var nested strings.Builder
		renderMarkdownChildren(&nested, node)
		content := strings.TrimSpace(normalizeBlockWhitespace(nested.String()))
		if content != "" {
			ensureLineBreaks(builder, 2)
			for index, line := range strings.Split(content, "\n") {
				if index > 0 {
					builder.WriteByte('\n')
				}
				builder.WriteString("> ")
				builder.WriteString(line)
			}
			ensureLineBreaks(builder, 2)
		}
		return
	case "li":
		ensureLineBreaks(builder, 1)
		if node.Parent != nil && strings.EqualFold(node.Parent.Data, "ol") {
			builder.WriteString(strconv.Itoa(listItemIndex(node)))
			builder.WriteString(". ")
		} else {
			builder.WriteString("- ")
		}
		renderMarkdownChildren(builder, node)
		ensureLineBreaks(builder, 1)
		return
	case "a":
		var label strings.Builder
		renderMarkdownChildren(&label, node)
		text := strings.TrimSpace(label.String())
		href := safeLink(nodeAttribute(node, "href"))
		if text != "" && href != "" {
			builder.WriteByte('[')
			builder.WriteString(escapeMarkdownLabel(text))
			builder.WriteString("](")
			builder.WriteString(href)
			builder.WriteByte(')')
		} else {
			builder.WriteString(text)
		}
		return
	}

	prefix, suffix := "", ""
	switch tag {
	case "strong", "b":
		prefix, suffix = "**", "**"
	case "em", "i":
		prefix, suffix = "_", "_"
	case "del", "s", "strike":
		prefix, suffix = "~~", "~~"
	case "code":
		prefix, suffix = "`", "`"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		ensureLineBreaks(builder, 2)
		level, _ := strconv.Atoi(strings.TrimPrefix(tag, "h"))
		prefix = strings.Repeat("#", level) + " "
		suffix = "\n\n"
	case "p", "div", "section", "article", "header", "footer", "ul", "ol", "table", "tr":
		ensureLineBreaks(builder, 2)
		suffix = "\n\n"
	case "th", "td":
		if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
			prefix = " | "
		}
	}

	builder.WriteString(prefix)
	renderMarkdownChildren(builder, node)
	builder.WriteString(suffix)
}

func renderMarkdownChildren(builder *strings.Builder, node *xhtml.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderMarkdownNode(builder, child)
	}
}

// telegramHTMLForChannel preserves the documented Telegram HTML subset and discards unsafe
// elements and attributes. This keeps arbitrary admin-authored HTML from turning into an invalid
// Bot API request while still retaining headings, emphasis, links, quotes, lists, and code.
func telegramHTMLForChannel(source string) string {
	document, err := xhtml.Parse(strings.NewReader(source))
	if err != nil {
		return stdhtml.EscapeString(source)
	}
	var builder strings.Builder
	renderTelegramHTMLNode(&builder, document)
	return strings.TrimSpace(normalizeBlockWhitespace(builder.String()))
}

func renderTelegramHTMLNode(builder *strings.Builder, node *xhtml.Node) {
	if node.Type == xhtml.TextNode {
		builder.WriteString(stdhtml.EscapeString(node.Data))
		return
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		return
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "script", "style", "noscript", "template", "head", "iframe", "object":
		return
	case "br":
		builder.WriteByte('\n')
		return
	case "img":
		builder.WriteString(stdhtml.EscapeString(strings.TrimSpace(nodeAttribute(node, "alt"))))
		return
	case "li":
		ensureLineBreaks(builder, 1)
		builder.WriteString("- ")
		renderTelegramHTMLChildren(builder, node)
		ensureLineBreaks(builder, 1)
		return
	case "a":
		href := safeLink(nodeAttribute(node, "href"))
		if href != "" {
			builder.WriteString(`<a href="`)
			builder.WriteString(stdhtml.EscapeString(href))
			builder.WriteString(`">`)
			renderTelegramHTMLChildren(builder, node)
			builder.WriteString("</a>")
		} else {
			renderTelegramHTMLChildren(builder, node)
		}
		return
	}

	openTag, closeTag := "", ""
	switch tag {
	case "strong", "b", "h1", "h2", "h3", "h4", "h5", "h6":
		openTag, closeTag = "<b>", "</b>"
	case "em", "i":
		openTag, closeTag = "<i>", "</i>"
	case "u", "ins":
		openTag, closeTag = "<u>", "</u>"
	case "del", "s", "strike":
		openTag, closeTag = "<s>", "</s>"
	case "code":
		openTag, closeTag = "<code>", "</code>"
	case "pre":
		openTag, closeTag = "<pre>", "</pre>"
	case "blockquote":
		openTag, closeTag = "<blockquote>", "</blockquote>"
	case "p", "div", "section", "article", "header", "footer", "ul", "ol":
		ensureLineBreaks(builder, 2)
		closeTag = "\n\n"
	}

	builder.WriteString(openTag)
	renderTelegramHTMLChildren(builder, node)
	builder.WriteString(closeTag)
}

func renderTelegramHTMLChildren(builder *strings.Builder, node *xhtml.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderTelegramHTMLNode(builder, child)
	}
}

func htmlTextContent(node *xhtml.Node) string {
	var builder strings.Builder
	var visit func(*xhtml.Node)
	visit = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func nodeAttribute(node *xhtml.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func safeLink(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "tg":
		return parsed.String()
	default:
		return ""
	}
}

func listItemIndex(node *xhtml.Node) int {
	index := 1
	for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
		if sibling.Type == xhtml.ElementNode && strings.EqualFold(sibling.Data, "li") {
			index++
		}
	}
	return index
}

func escapeMarkdownLabel(value string) string {
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]").Replace(value)
}

func ensureLineBreaks(builder *strings.Builder, count int) {
	value := builder.String()
	existing := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\n'; index-- {
		existing++
	}
	if existing < count {
		builder.WriteString(strings.Repeat("\n", count-existing))
	}
}

func normalizeBlockWhitespace(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}
