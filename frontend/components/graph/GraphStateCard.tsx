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
 * - Mobile teasers pass a `linkHref`/`linkLabel` pair only when a link-out
 *   target exists (mirrors HomeSceneGraph / InlineGraph's teaser pattern);
 *   surfaces whose non-graph content lives on the same page omit it.
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

/** Height for the sub-640px mobile teaser card (HomeSceneGraph precedent). */
export const GRAPH_TEASER_HEIGHT_CLASS = 'h-[240px]'
