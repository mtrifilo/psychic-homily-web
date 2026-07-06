/**
 * graphAnnouncements (PSY-1304) — aria-live copy for graph state changes a
 * keyboard / screen-reader user makes from the accessible connections list.
 *
 * The ego dialog already announces re-centers (buildRecenterAnnouncement, in
 * features/artists/.../graphTraversalHistory). These cover the other reachable
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

/**
 * "Shared bills connections shown/hidden." — after a type-filter toggle.
 * `label` is the human relationship-type label (e.g. "Shared bills").
 */
export function buildFilterAnnouncement(label: string, active: boolean): string {
  return `${label} connections ${active ? 'shown' : 'hidden'}.`
}
