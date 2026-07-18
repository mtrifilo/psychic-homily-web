/**
 * Shared "how many is this graph showing" phrase for capped graph surfaces
 * (PSY-1476). Generalizes the scene graph's shipped `sceneArtistCountPhrase`
 * (features/scenes/components/sceneGraphCopy.ts): when a surface caps its node
 * set, the header must say "top N of M" instead of a bare "N", so a visitor
 * knows they're seeing a slice rather than the whole thing. Consumed by the
 * scene, venue bill network, and collection graphs.
 *
 * Trust the truncated flag only when the numbers back it up — a skewed or
 * stale payload (total missing, zero, or ≤ the shown count, or nothing shown
 * at all) degrades to the plain count rather than rendering "top 12 of 0" or
 * "top 0 of 5". The `shown > 0` guard matters for the collection graph, whose
 * backend sets `nodes_truncated` even when every node was dropped (a
 * deleted-entity payload: 0 nodes, positive total).
 *
 * Returns BOTH the phrase and whether the truncation cue was shown, so a caller
 * that also needs to branch on truncation (e.g. the collection graph swaps its
 * per-type breakdown for the cue) reads one source of truth instead of
 * re-deriving the guard — the divergence risk this feature exists to avoid.
 *
 * `phrase` is lowercase ("top 12 of 90 artists" / "12 artists"). A caller that
 * leads a sentence with it wraps it in `sentenceCase` (a no-op for the
 * digit-leading plain count); the canvas aria-label reads it mid-sentence and
 * leaves it lowercase.
 */
export interface CountPhrase {
  phrase: string
  /** True when the "top N of M" cue is shown (i.e. the cap was disclosed). */
  truncated: boolean
}

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
}): CountPhrase {
  if (truncated && total !== undefined && shown > 0 && total > shown) {
    return { phrase: `top ${shown} of ${total} ${plural}`, truncated: true }
  }
  return { phrase: `${shown} ${shown === 1 ? singular : plural}`, truncated: false }
}

/**
 * Sentence-case a phrase to lead a subtitle line (PSY-1476). Matches the scene
 * header's treatment; a digit-leading plain count ("12 artists") is a no-op.
 */
export function sentenceCase(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
