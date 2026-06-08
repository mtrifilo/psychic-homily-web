import { cn } from '@/lib/utils'

interface EntityDetailContainerProps {
  children: React.ReactNode
  className?: string
}

/**
 * Canonical horizontal container for detail-page content that renders as a
 * sibling of {@link EntityDetailLayout} rather than inside it — e.g. the
 * History (RevisionHistory) and Discussion (CommentThread) sections that sit
 * below the main layout on entity detail pages.
 *
 * `EntityDetailLayout` already wraps its own content in
 * `container max-w-6xl mx-auto px-4` (plus `py-6`). Sections rendered AFTER the
 * layout — at the page-root fragment — have no container, so they render flush
 * against the nav and full-bleed on desktop. This wrapper gives them the SAME
 * left gutter and max-width as the rest of the page, without the page-level
 * vertical padding (those sections carry their own top margin). Keep this
 * string in sync with `EntityDetailLayout`'s container so all detail-page
 * surfaces share one gutter. (PSY-1026)
 */
export function EntityDetailContainer({
  children,
  className,
}: EntityDetailContainerProps) {
  return (
    <div className={cn('container max-w-6xl mx-auto px-4', className)}>
      {children}
    </div>
  )
}
