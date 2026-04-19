package utils

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// MarkdownRenderer renders user-submitted markdown to sanitized HTML.
// Uses goldmark for markdown → HTML conversion and bluemonday for XSS sanitization.
// Allowed tags match the comments/field-notes policy: p, br, strong/em, code/pre,
// lists, blockquote, headings h3-h6, and anchor links with href.
type MarkdownRenderer struct {
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
}

// NewMarkdownRenderer constructs a MarkdownRenderer with the standard policy.
// The same policy is applied to comments, field notes, and tag descriptions.
func NewMarkdownRenderer() *MarkdownRenderer {
	policy := bluemonday.NewPolicy()
	policy.AllowStandardURLs()
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowElements("p", "br",
		"strong", "b", "em", "i",
		"code", "pre",
		"ul", "ol", "li",
		"blockquote",
		"h3", "h4", "h5", "h6",
	)
	policy.RequireNoFollowOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	return &MarkdownRenderer{
		md:       goldmark.New(),
		sanitize: policy,
	}
}

// Render converts a markdown string to sanitized HTML. Returns "" for empty input.
// On conversion failure the raw input is escaped and wrapped in a <p>.
func (r *MarkdownRenderer) Render(body string) string {
	if body == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(body), &buf); err != nil {
		return "<p>" + r.sanitize.Sanitize(body) + "</p>"
	}
	return r.sanitize.Sanitize(buf.String())
}
