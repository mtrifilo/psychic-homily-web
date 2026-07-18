/**
 * Shared "how many is this graph showing" phrase for capped graph surfaces
 * (PSY-1476). Generalizes the scene graph's shipped `sceneArtistCountPhrase`
 * (features/scenes/components/sceneGraphCopy.ts): when a surface caps its node
 * set, the header must say "top N of M" instead of a bare "N", so a visitor
 * knows they're seeing a slice rather than the whole thing. Consumed by the
 * scene, venue bill network, and collection graphs.
 *
 * Trust the truncated flag only when the numbers back it up — a skewed or
 * stale payload (total missing, zero, or ≤ the shown count) degrades to the
 * plain count rather than rendering "top 12 of 0 items". This mirrors the
 * scene graph's original guard exactly.
 *
 * Returns lowercase ("top 12 of 90 artists" / "12 artists"). A caller that
 * leads a sentence with the phrase sentence-cases the first character (a no-op
 * for the digit-leading plain count), the same treatment the scene header
 * applies; the scene canvas aria-label reads it mid-sentence and leaves it
 * lowercase.
 */
export function truncatedCountPhrase({
  shown,
  total,
  truncated,
  singular,
  plural,
}: {
  /** Node count actually returned in the payload (post-cap). */
  shown: number
  /** Full pre-cap count. Optional because older payloads omit it. */
  total: number | undefined
  /** Server's cap-bit flag. Optional for the same reason. */
  truncated: boolean | undefined
  singular: string
  plural: string
}): string {
  if (truncated && total !== undefined && total > shown) {
    return `top ${shown} of ${total} ${plural}`
  }
  return `${shown} ${shown === 1 ? singular : plural}`
}
