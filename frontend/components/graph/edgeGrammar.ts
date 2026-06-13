/**
 * Typed-edge visual grammar for the shared graph layer (PSY-1083).
 *
 * Extracted from `features/artists/components/ArtistGraph.tsx` (PSY-362 /
 * PSY-363) so every graph surface — artist, scene, venue, collection,
 * /explore — speaks the same edge language: one color + dash pattern +
 * stroke-weight rule + tooltip format per relationship type.
 *
 * Colors live as per-theme CSS tokens (`--edge-*` in `app/globals.css`,
 * light + dark blocks). Canvas paint callbacks can't consume bare `var()`
 * strings, so canvas consumers resolve the tokens to hex via
 * `useGraphPalette()` in `./graphPalette.ts`; DOM surfaces (legend
 * swatches) use `edgeColorCSS()`, which returns a `var()` expression and
 * stays theme-reactive with no JS re-resolution.
 *
 * Colorblind audit (method: Brettel/Vienot CVD matrices for Protanopia,
 * Deuteranopia, and Tritanopia; pairwise RGB Euclidean distance; 30-unit
 * threshold; full method + results in docs/research/graph-colorblind-audit.md):
 *   - DARK palette (audited 2026-04-24, PSY-362 + PSY-363): the original
 *     audit — all 21 pairs pass under all 3 CVD types. Closest pair is
 *     shared_bills ↔ radio_cooccurrence at d=35.3 (protanopia), which is
 *     also dash-differentiated (solid vs dashed-8-3).
 *   - LIGHT palette (audited 2026-06-11, PSY-1083): the dark Tailwind-400s
 *     are near-invisible on the newsprint bg `#f4f1ea` (member_of 1.48:1),
 *     so the light set drops each hue to its Tailwind-600 weight, with
 *     amber and green nudged to 700 to clear the WCAG 3:1
 *     graphical-object bar. Re-ran the same pairwise audit on the light
 *     set: all 21 pairs pass under all 3 CVD types. Closest pair is
 *     member_of ↔ festival_cobill at d=31.9 (tritanopia), which is also
 *     dash-differentiated (dotted-2-4 vs dashed-10-4). Contrast vs
 *     #f4f1ea ranges 3.32:1 (radio_cooccurrence) to 4.77:1 (shared_label).
 *
 * WCAG 2.2 §1.4.1 ("Use of Color"): we never rely on color alone — every
 * edge type has a dash pattern (solid / dashed / dotted) and the magnitude
 * types also have weight scaling, so information is conveyed through at
 * least two channels.
 */

/** Canonical edge types, in legend display order. */
export const EDGE_TYPES = [
  'similar',
  'shared_bills',
  'shared_label',
  'side_project',
  'member_of',
  'radio_cooccurrence',
  'festival_cobill',
] as const

const EDGE_LABELS: Record<string, string> = {
  similar: 'Similar',
  shared_bills: 'Shared Bills',
  shared_label: 'Shared Label',
  side_project: 'Side Project',
  member_of: 'Member Of',
  radio_cooccurrence: 'Radio Co-occurrence',
  festival_cobill: 'Festival co-lineup',
}

/** CSS custom-property name per edge type (defined in app/globals.css). */
export const EDGE_CSS_VARS: Record<string, string> = {
  similar: '--edge-similar',
  shared_bills: '--edge-shared-bills',
  shared_label: '--edge-shared-label',
  side_project: '--edge-side-project',
  member_of: '--edge-member-of',
  radio_cooccurrence: '--edge-radio-cooccurrence',
  festival_cobill: '--edge-festival-cobill',
}

export const UNKNOWN_EDGE_CSS_VAR = '--edge-unknown'

/**
 * Dark-theme hex per edge type — MUST stay in sync with the `.dark` block
 * in app/globals.css. These are the exact pre-PSY-1083 ArtistGraph values,
 * used as the canvas fallback wherever the tokens can't resolve (jsdom
 * tests, pre-mount SSR pass), so the dark artist graph renders identically
 * to the pre-extraction component.
 */
export const FALLBACK_EDGE_COLORS: Record<string, string> = {
  similar: '#a1a1aa', //              zinc-400 (neutral)
  shared_bills: '#60a5fa', //         blue-400
  shared_label: '#c084fc', //         purple-400
  side_project: '#4ade80', //         green-400
  member_of: '#fbbf24', //            amber-400
  radio_cooccurrence: '#2dd4bf', //   teal-400
  festival_cobill: '#D55E00', //      vermillion (Okabe-Ito)
}

/**
 * Neutral fallback color for edge types outside the grammar — e.g. the
 * collection graph's derived types (played_at, discography, signed_to,
 * lineup, show_lineup, show_venue — PSY-555). Unknown types render solid,
 * thin, and grey: visible but unprivileged, never hidden, never a crash.
 * Matches the pre-PSY-1083 `EDGE_COLORS[type] || '#71717a'` fallback.
 */
export const FALLBACK_UNKNOWN_EDGE_COLOR = '#71717a'

/**
 * `var()` expression for an edge type's color — for DOM surfaces (legend
 * swatches). Theme-reactive for free; the embedded fallback keeps jsdom
 * and token-less environments on the dark palette.
 */
export function edgeColorCSS(type: string): string {
  const cssVar = EDGE_CSS_VARS[type] ?? UNKNOWN_EDGE_CSS_VAR
  const fallback = FALLBACK_EDGE_COLORS[type] ?? FALLBACK_UNKNOWN_EDGE_COLOR
  return `var(${cssVar}, ${fallback})`
}

/**
 * Canvas dash pattern per edge type, fed to force-graph's native
 * `linkLineDash` prop (no custom canvas renderer needed — PSY-1079 spike).
 * Solid (empty array) for similar / shared_bills and any unknown type.
 */
export function edgeLineDash(type: string): number[] {
  switch (type) {
    case 'shared_label':
      return [5, 5]
    case 'side_project':
    case 'member_of':
      return [2, 4]
    case 'radio_cooccurrence':
      return [8, 3]
    // PSY-363: long-dash pattern for festival_cobill. Color (vermillion)
    // is sufficiently distinct under all 3 CVD types per the audit, but
    // the dash provides redundant encoding (WCAG 2.2 §1.4.1).
    case 'festival_cobill':
      return [10, 4]
    default:
      return []
  }
}

/**
 * PSY-362 + PSY-363: Stroke weight encoding per edge type.
 *
 *   similar              — magnitude (Wilson similarity score). Scaled.
 *   shared_bills         — magnitude (recency-weighted shared-show count). Scaled.
 *   radio_cooccurrence   — magnitude (cross-station-weighted co-occurrence). Scaled.
 *   shared_label         — magnitude (count of shared labels, normalized to [0,1] in
 *                          the deriver, capped at 5+ shared labels = 1.0). Scaled.
 *   festival_cobill      — magnitude (recency-weighted shared-festival count, capped at
 *                          3+ shared festivals = 1.0 in the deriver). Scaled.
 *   side_project         — BINARY fact ("X is a side project of Y"). Intentionally uniform —
 *                          a side project either exists or does not, there is no magnitude.
 *   member_of            — BINARY fact ("X is a member of Y"). Intentionally uniform — same
 *                          rationale as side_project.
 *   (unknown types)      — uniform thin stroke (neutral fallback).
 */
export function edgeWidth(type: string, score?: number): number {
  switch (type) {
    case 'similar':
    case 'shared_bills':
    case 'shared_label':
    case 'radio_cooccurrence':
    case 'festival_cobill':
      return Math.max(1, (score ?? 0) * 3)
    case 'side_project':
    case 'member_of':
      // Binary relationship — uniform stroke is intentional.
      return 1
    default:
      return 1
  }
}

/**
 * Human-readable label for an edge type. Canonical types use the PSY-362
 * copy; unknown types (collection-derived edges etc.) are humanized from
 * the snake_case identifier ("played_at" → "Played at") so raw enum
 * strings never reach the legend or tooltips.
 */
export function edgeTypeLabel(type: string): string {
  const label = EDGE_LABELS[type]
  if (label) return label
  const words = type.replace(/_/g, ' ').trim()
  if (!words) return type
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/**
 * Order edge types for legend display: canonical grammar order first,
 * then any unknown types alphabetically (stable for tests + scanning).
 */
export function orderEdgeTypes(types: ReadonlyArray<string>): string[] {
  const canonicalRank = new Map<string, number>(EDGE_TYPES.map((t, i) => [t, i]))
  return [...types].sort((a, b) => {
    const ra = canonicalRank.get(a)
    const rb = canonicalRank.get(b)
    if (ra !== undefined && rb !== undefined) return ra - rb
    if (ra !== undefined) return -1
    if (rb !== undefined) return 1
    return a.localeCompare(b)
  })
}

/** Per-type edge counts for the legend, in one pass over the payload. */
export function countLinkTypes(links: ReadonlyArray<{ type: string }>): Map<string, number> {
  const counts = new Map<string, number>()
  for (const link of links) {
    if (!link.type) continue
    counts.set(link.type, (counts.get(link.type) ?? 0) + 1)
  }
  return counts
}

// ──────────────────────────────────────────────
// Edge hover tooltip (PSY-362)
// ──────────────────────────────────────────────

// Helper: pull a number out of the loosely-typed `detail` JSONB blob.
// Returns undefined when the field is missing or not coercible to a number.
function detailNumber(detail: Record<string, unknown> | undefined, key: string): number | undefined {
  if (!detail) return undefined
  const v = detail[key]
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : undefined
  }
  return undefined
}

function detailString(detail: Record<string, unknown> | undefined, key: string): string | undefined {
  if (!detail) return undefined
  const v = detail[key]
  return typeof v === 'string' && v.length > 0 ? v : undefined
}

/**
 * Minimal link shape `buildLinkLabel` needs. Votes and score are optional —
 * the artist endpoint carries votes, the scene/venue/collection endpoints
 * don't, and the tooltip degrades gracefully either way.
 */
export interface EdgeTooltipLink {
  type: string
  score?: number
  votes_up?: number
  votes_down?: number
  detail?: Record<string, unknown>
}

// Build the hover tooltip string for an edge. The text is edge-type aware and surfaces
// the underlying raw signal (count, score, label name) sourced from the link's `detail`
// JSONB or `score` field. If the data shape doesn't carry the field we'd ideally show,
// we fall back to a description that uses what's available — never fabricate a number.
//
// Exported for unit testing the format of each edge type's tooltip string.
export function buildLinkLabel(link: EdgeTooltipLink): string {
  const detail = link.detail
  switch (link.type) {
    case 'similar': {
      const pct = Math.round((link.score ?? 0) * 100)
      const votesUp = link.votes_up ?? 0
      const votesDown = link.votes_down ?? 0
      if (votesUp + votesDown > 0) {
        return `Similar: ${pct}% (${votesUp} up / ${votesDown} down)`
      }
      return `Similar: ${pct}%`
    }
    case 'shared_bills': {
      const count = detailNumber(detail, 'shared_count')
      const lastShared = detailString(detail, 'last_shared')
      if (count !== undefined) {
        const noun = count === 1 ? 'show' : 'shows'
        return lastShared
          ? `${count} shared ${noun} (last: ${lastShared})`
          : `${count} shared ${noun}`
      }
      return 'Shared bills'
    }
    case 'shared_label': {
      const count = detailNumber(detail, 'shared_count')
      const labelNames = detailString(detail, 'label_names')
      if (labelNames) {
        return count !== undefined && count > 1
          ? `${count} shared labels: ${labelNames}`
          : `Both on ${labelNames}`
      }
      if (count !== undefined) {
        const noun = count === 1 ? 'label' : 'labels'
        return `${count} shared ${noun}`
      }
      return 'Shared label'
    }
    case 'radio_cooccurrence': {
      const coCount = detailNumber(detail, 'co_occurrence_count')
      const stationCount = detailNumber(detail, 'station_count')
      if (coCount !== undefined) {
        const stationPart =
          stationCount !== undefined && stationCount > 1 ? ` across ${stationCount} stations` : ''
        const noun = coCount === 1 ? 'show' : 'shows'
        return `Played together on ${coCount} radio ${noun}${stationPart}`
      }
      return 'Radio co-occurrence'
    }
    case 'side_project':
      return 'Side project'
    case 'member_of':
      return 'Member of'
    case 'festival_cobill': {
      const count = detailNumber(detail, 'count')
      const names = detailString(detail, 'festival_names')
      const year = detailNumber(detail, 'most_recent_year')
      if (count === undefined) {
        return EDGE_LABELS.festival_cobill
      }
      const noun = count === 1 ? 'festival' : 'festivals'
      const headline = names ? `${count} shared ${noun}: ${names}` : `${count} shared ${noun}`
      return year !== undefined ? `${headline} (last: ${year})` : headline
    }
    default:
      // Unknown types (collection-derived edges etc.) get the humanized
      // type label — same copy the legend row shows.
      return edgeTypeLabel(link.type)
  }
}
