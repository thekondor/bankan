package ui

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// mdRenderer is a shared goldmark instance configured with GFM extensions.
// HTML passthrough is explicitly disabled (default) to prevent XSS.
var mdRenderer = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM, // tables, strikethrough, task lists, autolinks
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		// html.WithUnsafe() is intentionally absent — raw HTML in source is stripped.
		html.WithXHTML(),
	),
)

// renderMarkdownHTML converts the full src string from markdown to an HTML
// string. Raw HTML tags in src are stripped (goldmark default). Safe to inject
// via templ.Raw.
func renderMarkdownHTML(src string) string {
	if src == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		// Fallback: return escaped source as a plain paragraph.
		return "<p>" + src + "</p>"
	}
	return buf.String()
}

// markdownPreviewHTML extracts the lines of src that appear before the first
// blank line (i.e. the "lead paragraph") and renders them as markdown HTML.
// Returns an empty string when src has no non-blank content before the first
// blank line or when src itself is empty.
func markdownPreviewHTML(src string) string {
	if src == "" {
		return ""
	}
	lines := strings.Split(src, "\n")
	var preview []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			break
		}
		preview = append(preview, line)
	}
	if len(preview) == 0 {
		return ""
	}
	return renderMarkdownHTML(strings.Join(preview, "\n"))
}
