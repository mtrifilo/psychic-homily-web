import type { CSSProperties, ReactNode } from 'react'

/**
 * Height-reserving placeholder for a lazy-mounted graph, shared across all three
 * graph surfaces (PSY-1347, PSY-1359):
 *   - InlineGraph (PSY-837) and HomeSceneGraph (PSY-1344) use the DEFAULT form —
 *     a bordered, muted, pulsing, full-width, aria-hidden CLS placeholder — for
 *     their pre-mount state, data-loading state, AND dynamic-import loading
 *     fallback, so those can't drift apart.
 *   - ArtistGraph's ego dialog (PSY-1359) passes `children` — a visible spinner +
 *     "Loading graph…" label — for the modal's chunk-loading state, where an
 *     announced indicator beats an invisible pulse. With children the box does
 *     NOT pulse or hide (the content is the affordance); it centers them instead.
 *
 * Each call site passes its own SIZING via `className` / `style` (16/9 aspect box,
 * responsive fixed height, or a flat height:400 modal box).
 */
export function GraphSkeleton({
  className = '',
  style,
  children,
}: {
  className?: string
  style?: CSSProperties
  children?: ReactNode
}) {
  const hasContent = children != null
  return (
    <div
      className={`w-full rounded-lg border border-border/50 ${
        hasContent
          ? 'bg-muted/20 flex items-center justify-center'
          : 'bg-muted/10 animate-pulse'
      } ${className}`}
      style={style}
      aria-hidden={hasContent ? undefined : true}
    >
      {children}
    </div>
  )
}
