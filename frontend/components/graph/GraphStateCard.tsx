import Link from 'next/link'

/**
 * GraphStateCard — the shared visible card for a graph surface's non-canvas
 * terminal states (settled fetch error, sub-640px mobile teaser).
 *
 * Complements GraphSkeleton (the loading placeholder): same bordered/muted
 * box language, but with visible, announced content instead of a pulse.
 * Standardized across SceneGraph, StationGraph, CollectionGraph, and
 * VenueBillNetwork so the states can't drift apart per surface again.
 *
 * - Error states pass `role="alert"` so the settled failure is announced.
 * - Mobile teasers pass a `linkHref`/`linkLabel` pair to point the visitor at
 *   a browse-able alternative (PSY-1472): the four Section detail-page teasers
 *   (scene / station / venue / collection) now scroll to that page's own list
 *   (`#scene-artists`, `#recent-playlists`, `#venue-shows`, `#items`), and the
 *   homepage teaser links to the scene page. Error-state cards omit the link.
 * - Sizing is the caller's via `className` (same contract as GraphSkeleton)
 *   so each surface can match its own canvas/skeleton height budget.
 */
export function GraphStateCard({
  message,
  role,
  linkHref,
  linkLabel,
  className = '',
}: {
  message: string
  role?: 'alert'
  linkHref?: string
  linkLabel?: string
  className?: string
}) {
  return (
    <div
      role={role}
      className={`w-full rounded-lg border border-border/50 bg-muted/10 flex flex-col items-center justify-center text-center p-6 gap-3 ${className}`}
    >
      <p className="text-sm text-muted-foreground max-w-xs">{message}</p>
      {linkHref && linkLabel && (
        <Link
          href={linkHref}
          className="text-sm text-primary hover:underline underline-offset-4"
        >
          {linkLabel}
        </Link>
      )}
    </div>
  )
}

/**
 * Shared height contract for the full-size graph surfaces' loading skeleton
 * and error card: 240px below the 640px canvas gate (teaser-height band),
 * then tracks ForceGraphView's default canvas height (400px under 768px,
 * 560px above — the md values MUST stay in sync with `graphHeight` in
 * ForceGraphView). Viewport-keyed where the canvas is container-keyed; the
 * mismatch only survives in narrow padded-column bands (same accepted
 * trade-off as HomeSceneGraph's PLACEHOLDER_HEIGHT_CLASS).
 */
export const GRAPH_BOX_HEIGHT_CLASS = 'h-[240px] sm:h-[400px] md:h-[560px]'

/**
 * min-height mirror of GRAPH_BOX_HEIGHT_CLASS for content-driven state cards
 * (e.g. the Observatory's no-connections empty state with its escape-hatch
 * pill rows): reserves the same contract heights but grows with the content
 * on narrow viewports instead of forcing an inner scroll region. Keep the
 * values in lockstep with GRAPH_BOX_HEIGHT_CLASS above.
 */
export const GRAPH_BOX_MIN_HEIGHT_CLASS = 'min-h-[240px] sm:min-h-[400px] md:min-h-[560px]'

/** Height for the sub-640px mobile teaser card (HomeSceneGraph precedent). */
export const GRAPH_TEASER_HEIGHT_CLASS = 'h-[240px]'
