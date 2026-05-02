package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/services/contracts"
)

// These tests cover the markdown-rendering plumbing on CollectionService
// without requiring a database. They mirror the unit tests in
// engagement.TestMarkdownRendering so collection notes/description go through
// the same goldmark + bluemonday pipeline as comments and field notes.
//
// PSY-349.

func newRenderer(t *testing.T) *CollectionService {
	t.Helper()
	// db=nil is fine: renderMarkdown/renderNotes don't touch the DB.
	return NewCollectionService(nil)
}

func TestCollectionService_RenderMarkdown_Bold(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("**bold text**")
	assert.Contains(t, html, "<strong>bold text</strong>")
}

func TestCollectionService_RenderMarkdown_Italic(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("*italic text*")
	assert.Contains(t, html, "<em>italic text</em>")
}

func TestCollectionService_RenderMarkdown_Link(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("[click here](https://example.com)")
	assert.Contains(t, html, `href="https://example.com"`)
	assert.Contains(t, html, "click here")
}

func TestCollectionService_RenderMarkdown_Blockquote(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("> quoted text")
	assert.Contains(t, html, "<blockquote>")
}

func TestCollectionService_RenderMarkdown_List(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("- one\n- two")
	assert.Contains(t, html, "<ul>")
	assert.Contains(t, html, "<li>")
}

func TestCollectionService_RenderMarkdown_StripsScript(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("<script>alert('xss')</script>")
	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "alert(")
}

func TestCollectionService_RenderMarkdown_StripsImage(t *testing.T) {
	svc := newRenderer(t)
	html := svc.renderMarkdown("![alt](https://example.com/img.png)")
	// bluemonday must strip <img> per the comment-system policy.
	assert.NotContains(t, html, "<img")
}

func TestCollectionService_RenderMarkdown_PreservesPlainText(t *testing.T) {
	svc := newRenderer(t)
	// A plain-text note (the legacy format) is valid markdown — it must
	// still render safely so existing rows display correctly.
	html := svc.renderMarkdown("just a plain note about this artist")
	assert.Contains(t, html, "just a plain note about this artist")
	assert.NotContains(t, html, "<script>")
}

func TestCollectionService_RenderMarkdown_EmptyReturnsEmpty(t *testing.T) {
	svc := newRenderer(t)
	assert.Equal(t, "", svc.renderMarkdown(""))
}

func TestCollectionService_RenderNotes_NilReturnsEmpty(t *testing.T) {
	svc := newRenderer(t)
	assert.Equal(t, "", svc.renderNotes(nil))
}

func TestCollectionService_RenderNotes_RendersValue(t *testing.T) {
	svc := newRenderer(t)
	v := "**bold notes**"
	html := svc.renderNotes(&v)
	assert.Contains(t, html, "<strong>bold notes</strong>")
}

func TestCollectionService_MaxLengthConstantsMirrorComments(t *testing.T) {
	// PSY-349: collection limits are aliases of engagementm.MaxCommentBodyLength
	// (10,000). Guard the wiring so a future change to the comment limit
	// flows through here automatically.
	assert.Equal(t, 10000, contracts.MaxCollectionDescriptionLength)
	assert.Equal(t, 10000, contracts.MaxCollectionItemNotesLength)
}

// Sanity check: a maximally long body is rendered without panicking.
func TestCollectionService_RenderMarkdown_LongBody(t *testing.T) {
	svc := newRenderer(t)
	body := strings.Repeat("a ", contracts.MaxCollectionDescriptionLength/2)
	html := svc.renderMarkdown(body)
	assert.NotEmpty(t, html)
}
