'use client'

/**
 * MarkdownEditor — textarea + preview toggle for collection description and
 * per-item notes (PSY-349).
 *
 * Why local preview? The server is the source of truth for markdown rendering
 * (goldmark) and HTML sanitization (bluemonday) — the same stack used by
 * comments and field notes. The author-facing preview here is a lightweight
 * approximation so the user can see their formatting before saving; once the
 * value is persisted, the server-rendered + sanitized `description_html` /
 * `notes_html` is what every other user actually sees. The preview is NOT a
 * second sanitizer — it does NOT replace the server's bluemonday policy.
 *
 * Allowed primitives (per the comment-system policy): bold, italic, links,
 * blockquotes, lists, inline code/pre, h3+. The preview uses `marked` for
 * markdown → HTML conversion and additionally strips obviously-dangerous
 * fragments (script/style/iframe/event-handlers) defense-in-depth, since
 * the rendered preview is shown via `dangerouslySetInnerHTML` to the author.
 */

import { useState, useMemo } from 'react'
import { marked } from 'marked'
import { Eye, Pencil } from 'lucide-react'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * Strips a small set of unsafe constructs from preview HTML. The author is
 * the only viewer — but defense-in-depth against accidentally pasting an
 * unsafe snippet is cheap. The server's bluemonday policy is the actual
 * security boundary; this is a hygiene pass for the preview pane only.
 */
function stripUnsafe(html: string): string {
  return html
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
    .replace(/<style\b[^<]*(?:(?!<\/style>)<[^<]*)*<\/style>/gi, '')
    .replace(/<iframe\b[^>]*>[\s\S]*?<\/iframe>/gi, '')
    .replace(/<iframe\b[^>]*\/?>/gi, '')
    // Strip on* event handlers from any tag.
    .replace(/\s+on[a-z]+\s*=\s*"[^"]*"/gi, '')
    .replace(/\s+on[a-z]+\s*=\s*'[^']*'/gi, '')
    .replace(/\s+on[a-z]+\s*=\s*[^\s>]+/gi, '')
    // Strip javascript: URLs from href/src.
    .replace(/(href|src)\s*=\s*"javascript:[^"]*"/gi, '$1="#"')
    .replace(/(href|src)\s*=\s*'javascript:[^']*'/gi, "$1='#'")
}

export interface MarkdownEditorProps {
  value: string
  onChange: (next: string) => void
  /** Optional id for the textarea (links to a label). */
  id?: string
  /** Placeholder text in edit mode. */
  placeholder?: string
  /** Number of rows shown in edit mode (default 4). */
  rows?: number
  /** Disable edits (e.g. while saving). */
  disabled?: boolean
  /** Maximum length enforced in the UI. Mirrors the server limit. */
  maxLength?: number
  /** Optional className for the outer wrapper. */
  className?: string
  /** Optional aria-label when no visible label is provided. */
  ariaLabel?: string
  /** Optional autoFocus on mount. */
  autoFocus?: boolean
  /** Test id for the textarea. */
  testId?: string
}

export function MarkdownEditor({
  value,
  onChange,
  id,
  placeholder = 'Markdown supported: **bold**, *italic*, [link](url), > quote, - list',
  rows = 4,
  disabled = false,
  maxLength,
  className,
  ariaLabel,
  autoFocus = false,
  testId = 'markdown-editor-textarea',
}: MarkdownEditorProps) {
  const [mode, setMode] = useState<'edit' | 'preview'>('edit')

  const previewHtml = useMemo(() => {
    const trimmed = value.trim()
    if (!trimmed) return ''
    // marked.parse is sync when called without a callback. Cast through
    // unknown to satisfy strict TS (the lib types both sync and async).
    const rendered = marked.parse(value, {
      gfm: true,
      breaks: false,
    }) as unknown as string
    return stripUnsafe(rendered)
  }, [value])

  return (
    <div className={cn('space-y-2', className)} data-testid="markdown-editor">
      {/* Mode toggle */}
      <div className="flex items-center gap-1">
        <Button
          type="button"
          size="sm"
          variant={mode === 'edit' ? 'secondary' : 'ghost'}
          className="h-7 px-2 text-xs"
          onClick={() => setMode('edit')}
          aria-pressed={mode === 'edit'}
          disabled={disabled}
          data-testid="markdown-editor-edit-tab"
        >
          <Pencil className="h-3 w-3 mr-1" />
          Write
        </Button>
        <Button
          type="button"
          size="sm"
          variant={mode === 'preview' ? 'secondary' : 'ghost'}
          className="h-7 px-2 text-xs"
          onClick={() => setMode('preview')}
          aria-pressed={mode === 'preview'}
          disabled={disabled}
          data-testid="markdown-editor-preview-tab"
        >
          <Eye className="h-3 w-3 mr-1" />
          Preview
        </Button>
        {maxLength !== undefined && (
          <span
            className={cn(
              'ml-auto text-[11px] tabular-nums',
              value.length > maxLength
                ? 'text-destructive'
                : 'text-muted-foreground'
            )}
            aria-live="polite"
          >
            {value.length.toLocaleString()} / {maxLength.toLocaleString()}
          </span>
        )}
      </div>

      {mode === 'edit' ? (
        <Textarea
          id={id}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          rows={rows}
          disabled={disabled}
          maxLength={maxLength}
          aria-label={ariaLabel}
          autoFocus={autoFocus}
          data-testid={testId}
        />
      ) : (
        <div
          className={cn(
            'rounded-md border border-border/50 bg-muted/20 px-3 py-2 min-h-[6rem]',
            'prose prose-sm dark:prose-invert max-w-none'
          )}
          data-testid="markdown-editor-preview"
        >
          {previewHtml ? (
            <div dangerouslySetInnerHTML={{ __html: previewHtml }} />
          ) : (
            <p className="text-sm text-muted-foreground italic">
              Nothing to preview yet.
            </p>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * MarkdownContent renders server-sanitized HTML produced by goldmark +
 * bluemonday on the backend. It is a thin wrapper around the
 * `dangerouslySetInnerHTML` pattern used by the comment system, so callers
 * don't reach for the raw escape hatch.
 */
export interface MarkdownContentProps {
  html: string
  className?: string
  testId?: string
}

export function MarkdownContent({ html, className, testId }: MarkdownContentProps) {
  if (!html) return null
  return (
    <div
      className={cn('prose prose-sm dark:prose-invert max-w-none', className)}
      data-testid={testId}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
