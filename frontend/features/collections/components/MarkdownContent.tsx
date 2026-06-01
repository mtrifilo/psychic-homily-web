/**
 * MarkdownContent — read-only renderer for server-sanitized markdown HTML.
 *
 * Split out of `MarkdownEditor.tsx` (PSY-951) so display-only consumers don't
 * transitively import the write-mode editor's `marked` + `dompurify` deps
 * (~80 KB raw). Those libs ride only in `MarkdownEditor` (the author-facing
 * preview); the read path here renders HTML the backend already produced via
 * goldmark + bluemonday (the same stack as comments and field notes), so it
 * needs nothing beyond `cn`.
 *
 * This module is intentionally NOT `'use client'`: it has no hooks or browser
 * APIs, so it can render on the server (keeping it out of the client bundle
 * boundary) while remaining importable from client components.
 *
 * Security note: the `html` prop is ALREADY sanitized server-side (bluemonday).
 * This component is a thin, intentional `dangerouslySetInnerHTML` wrapper so
 * callers don't reach for the raw escape hatch — it is NOT a sanitizer and must
 * only ever be fed server-sanitized HTML (`*_html` fields), never raw user input.
 */

import { cn } from '@/lib/utils'

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
