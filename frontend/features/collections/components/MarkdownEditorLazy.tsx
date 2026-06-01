'use client'

/**
 * MarkdownEditorLazy — the single `dynamic(ssr:false)` boundary for the
 * write-mode `MarkdownEditor` (PSY-951).
 *
 * Why this module exists: `MarkdownEditor` statically imports `marked` +
 * `dompurify` (~80 KB raw). Because the editor is imported by several
 * components living on distinct route trees (CollectionDetail, CollectionList,
 * FeaturedAdmin, and the per-item notes editor), a STATIC import keeps it —
 * and those libs — multi-route-reachable, so Turbopack hoists them into the
 * global shared client chunk loaded on every route (incl. /explore, which uses
 * none of it). See the PSY-944 spike: splitting the read-only renderer out is
 * necessary but insufficient; the editor itself must be lazily loaded.
 *
 * `ssr: false` is correct (not a regression) here because the editor's preview
 * pane is client-only by design — `sanitizePreview` returns "" on the server
 * (DOMPurify needs a DOM) — and the editor only mounts behind an interaction
 * (opening an edit/create form). So nothing is lost server-side, and the
 * marked/dompurify chunk is fetched only when an author actually opens an
 * editor.
 *
 * All four consumers MUST import from THIS module (not `./MarkdownEditor`
 * directly), so the lazy boundary stays the only edge into the markdown libs.
 */

import dynamic from 'next/dynamic'

export const MarkdownEditor = dynamic(
  () => import('./MarkdownEditor').then(m => ({ default: m.MarkdownEditor })),
  {
    ssr: false,
    // Reserve space while the chunk downloads so the surrounding edit form
    // doesn't shift when the editor mounts. ~6.5rem ≈ the toggle row + a
    // few-row textarea, matching the editor's default footprint; the mode
    // toggle's `space-y-2` + min-h preview keep the real editor close to this.
    loading: () => (
      <div
        className="min-h-[6.5rem] rounded-md border border-border/50 bg-muted/20"
        aria-hidden="true"
        data-testid="markdown-editor-loading"
      />
    ),
  }
)

export type { MarkdownEditorProps } from './MarkdownEditor'
