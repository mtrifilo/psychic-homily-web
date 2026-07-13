/**
 * graphAnnouncements (PSY-1304) — aria-live copy for graph state changes a
 * keyboard / screen-reader user makes from the accessible connections list.
 *
 * The ego dialog already announces re-centers (buildRecenterAnnouncement, in
 * components/graph/graphTraversalHistory). These cover the other reachable
 * mutations so EACH state change fires exactly one polite announcement:
 *   - expand / collapse a node's connections (the tree's core action)
 *   - toggle a relationship-type filter
 * (The discovery-bias slider is a native range input and self-voices via
 * aria-valuetext — it is deliberately NOT echoed here, to avoid a double
 * announcement. See docs/features/graph-mobile-a11y.md.)
 *
 * Pure + framework-free so they're unit-testable and shareable across surfaces.
 */

/** "Added N artists connected to X." — after an expand fetch merges. */
export function buildExpandAnnouncement(nodeName: string, addedCount: number): string {
  if (addedCount <= 0) {
    return `No new artists connected to ${nodeName}.`
  }
  const noun = addedCount === 1 ? 'artist' : 'artists'
  return `Added ${addedCount} ${noun} connected to ${nodeName}.`
}

/** "Collapsed the connections under X." — after a collapse. */
export function buildCollapseAnnouncement(nodeName: string): string {
  return `Collapsed the connections under ${nodeName}.`
}

/** "Couldn't load connections for X." — when an expand fetch fails. */
export function buildExpandErrorAnnouncement(nodeName: string): string {
  return `Couldn't load connections for ${nodeName}. Try again.`
}

/** "Collapsed all expansions…" — after the Collapse-all bulk reset. */
export function buildCollapseAllAnnouncement(): string {
  return 'Collapsed all expansions back to the starting graph.'
}

/**
 * "Shared bills connections shown/hidden." — after a type-filter toggle.
 * `label` is the human relationship-type label (e.g. "Shared bills").
 */
export function buildFilterAnnouncement(label: string, active: boolean): string {
  return `${label} connections ${active ? 'shown' : 'hidden'}.`
}

/**
 * Depth control (PSY-1303). Depth 2 auto-expands the top DOI-ranked neighbours,
 * so the copy speaks how many artists that ADDED; depth 1 collapses them back.
 */
export function buildDepthAnnouncement(depth: 1 | 2, addedCount: number): string {
  if (depth === 1) {
    return 'Back to 1 hop — collapsed the second-hop connections.'
  }
  if (addedCount <= 0) {
    return 'Showing 2 hops — no new artists to add.'
  }
  const noun = addedCount === 1 ? 'artist' : 'artists'
  return `Showing 2 hops — added ${addedCount} ${noun} from the top connections.`
}
