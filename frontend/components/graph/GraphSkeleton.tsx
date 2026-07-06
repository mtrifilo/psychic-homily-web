import type { CSSProperties } from 'react'

/**
 * Height-reserving placeholder for a lazy-mounted graph section (CLS budget),
 * shared by InlineGraph (PSY-837) and HomeSceneGraph (PSY-1344) — the pre-mount
 * state, the data-loading state, and the dynamic-import loading fallback all
 * render it, so they can't drift apart (PSY-1347).
 *
 * The base look (bordered, muted, pulsing, full-width, aria-hidden) is shared;
 * each call site passes its own SIZING via `className`/`style` because the two
 * surfaces reserve height differently — InlineGraph uses a 16/9 aspect box with
 * a min-height floor, HomeSceneGraph a responsive fixed-height (240px → 560px).
 */
export function GraphSkeleton({
  className = '',
  style,
}: {
  className?: string
  style?: CSSProperties
}) {
  return (
    <div
      className={`w-full rounded-lg border border-border/50 bg-muted/10 animate-pulse ${className}`}
      style={style}
      aria-hidden="true"
    />
  )
}
