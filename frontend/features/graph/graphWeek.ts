/**
 * "This week in the graph" — the snapshot-aligned week the share card reports.
 *
 * Pure and framework-free (no `next/og`, no React), because this is the one
 * place that decides WHICH arrivals count as "this week" and every surface has
 * to agree: the share card, the share page, and the affordance on the map that
 * offers them. A second copy of the boundary rule is a second answer to
 * "how many artists arrived", on two surfaces a reader sees minutes apart.
 *
 * THE WINDOW IS SNAPSHOT-ALIGNED, NOT WALL-CLOCK (locked decision, 2026-08-02).
 * It ends at the snapshot's own `last_mapped` and spans the seven days before
 * it, so the counts can never disagree with the map that is on screen, and the
 * card is deterministic per snapshot: re-render it a hundred times and it says
 * the same thing until the next nightly build. A rolling window computed at
 * render time was rejected for exactly the disagreement it invites.
 *
 * EVERYTHING HERE IS UTC. The window is a fact about a global map with no
 * venue, no scene and no reader timezone attached to it, and the card is
 * rendered server-side and then cached in front of every reader in the world —
 * so a boundary that moved with the viewer's clock would make the same cached
 * PNG's counts wrong for most of them. The site's venue-local date rule does
 * not apply and must not be reached for here.
 *
 * That is also why `formatLastMapped` sits at the bottom of this module rather
 * than beside the footer that renders it: the snapshot's own build date is not
 * part of the week, but it is derived from the same `last_mapped` instant, and
 * one timezone rule for that instant is the whole point.
 */

import type { SceneMap } from './sceneMap'

/** The share surface both the affordance and the card's metadata point at. */
export const GRAPH_WEEK_PATH = '/graph/this-week'

/** Days the highlight window spans, counting its last day. */
export const GRAPH_WEEK_DAYS = 7

const SECONDS_PER_DAY = 86_400
const MS_PER_DAY = SECONDS_PER_DAY * 1000

/** One snapshot's worth of "what arrived this week". */
export interface GraphWeek {
  /**
   * First instant the window includes — UTC midnight, `GRAPH_WEEK_DAYS - 1`
   * days before the snapshot's own day.
   */
  start: Date
  /** Last instant the window includes: the snapshot's `last_mapped`. */
  end: Date
  /** Artists that arrived in the window. Label hubs are not counted. */
  newArtistCount: number
  /** Edges that arrived in the window. */
  newConnectionCount: number
  /**
   * Ids of the nodes that arrived in the window — what the card paints orange.
   * Includes label hubs: a hub that appeared this week is a genuinely new part
   * of the map even though it is not an "artist" in the count above.
   */
  newNodeIds: Set<number>
  /**
   * The window as `appear` values see it: epoch-relative seconds, INCLUSIVE on
   * both ends.
   *
   * Exposed so the boundary rule has exactly ONE definition — `isInGraphWeek`
   * below — shared by the counts here and by the card's motif, which has to
   * decide the same question about edges. Two copies of an inclusive/exclusive
   * choice is how a card comes to draw a line it did not count.
   */
  appearRange: { start: number; end: number }
}

/**
 * Whether this week is worth INVITING someone to share.
 *
 * A function rather than a field on `GraphWeek`, so the policy has one
 * definition and cannot be set inconsistently with the counts it is about.
 *
 * A week with nothing new renders a truthful but dull `+0 ARTISTS · +0
 * CONNECTIONS`. That is fine for someone who already has the link, and wrong as
 * something to offer — so the map's affordance is gated on this while the page
 * and the card are not. Splitting the two is deliberate: a share URL that
 * disappeared on quiet weeks would break links that were live yesterday.
 */
export function isGraphWeekShareworthy(week: GraphWeek): boolean {
  return week.newArtistCount > 0 || week.newConnectionCount > 0
}

/**
 * Resolve the week a snapshot describes, or null when it cannot describe one.
 *
 * Null for three reasons, all of which mean the same thing to a visitor —
 * there is nothing to say about this week — so the callers collapse them:
 *
 *  1. `last_mapped` or `epoch` did not parse. Both go straight into date maths.
 *  2. The snapshot carries no usable `appear` column, which `buildSceneMap`
 *     surfaces as every node reading 0. Indistinguishable from "the whole
 *     catalog predates the epoch", and equally unusable: dating this week's
 *     arrivals is the entire feature.
 *  3. The window would start before the epoch every `appear` counts from, so
 *     no arrival can be placed inside or outside it.
 */
export function resolveGraphWeek(map: SceneMap): GraphWeek | null {
  const endMs = map.lastMapped.getTime()
  const epochMs = map.epoch.getTime()
  if (Number.isNaN(endMs) || Number.isNaN(epochMs)) return null

  // A snapshot whose arrivals are all clamped to the epoch cannot be dated.
  // `some` rather than a count: one dated node is enough for the window to
  // mean something, and this walks a few thousand entries on a hot path.
  if (!map.nodes.some(node => node.appear > 0)) return null

  const end = new Date(endMs)
  const start = new Date(utcMidnight(endMs) - (GRAPH_WEEK_DAYS - 1) * MS_PER_DAY)

  // Whole seconds, because `appear` is whole seconds. Truncating the START
  // DOWN and the END DOWN both widen nothing: `start` is midnight so its
  // seconds value is exact, and flooring the end can only exclude a node that
  // arrived in the final fractional second of the snapshot's own build.
  const startSeconds = Math.floor((start.getTime() - epochMs) / 1000)
  const endSeconds = Math.floor((endMs - epochMs) / 1000)
  if (startSeconds < 0 || endSeconds < startSeconds) return null

  // INCLUSIVE ON BOTH ENDS, and the ends cannot collide: `start` is a UTC
  // midnight and `end` is the snapshot's build time, so consecutive snapshots'
  // windows overlap by design (they are both "the last seven days") rather than
  // tiling. Nothing double-counts, because only ONE window is ever reported.
  const appearRange = { start: startSeconds, end: endSeconds }

  const newNodeIds = new Set<number>()
  let newArtistCount = 0
  for (const node of map.nodes) {
    if (!inRange(appearRange, node.appear)) continue
    newNodeIds.add(node.id)
    if (node.kind === 'artist') newArtistCount += 1
  }

  // An edge's `appear` is the LATER of its two endpoints (the backend resolves
  // it that way; see `SceneMapEdge.appear`), never the relationship row's own
  // timestamp. So a connection counted here is one whose newer end arrived this
  // week — which is what the card's `+{m} CONNECTIONS` means. A new similarity
  // discovered between two long-standing artists is NOT reported, because the
  // snapshot does not carry the date that would place it.
  let newConnectionCount = 0
  for (const edge of map.edges) {
    if (!inRange(appearRange, edge.appear)) continue
    newConnectionCount += 1
  }

  return {
    start,
    end,
    newArtistCount,
    newConnectionCount,
    newNodeIds,
    appearRange,
  }
}

/**
 * Whether an `appear` second falls inside a resolved week.
 *
 * The ONE place the inclusive/exclusive choice is made, for nodes and for edges
 * alike. The card's motif calls it directly so a drawn connector and a counted
 * connection can never be different sets.
 */
export function isInGraphWeek(week: GraphWeek, appear: number): boolean {
  return inRange(week.appearRange, appear)
}

function inRange(range: { start: number; end: number }, appear: number): boolean {
  return appear >= range.start && appear <= range.end
}

/** Midnight UTC on the day `ms` falls in. */
function utcMidnight(ms: number): number {
  const date = new Date(ms)
  return Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate())
}

/**
 * Built once for the module rather than per call — `toLocaleDateString` with
 * options constructs a fresh ICU formatter every time, and the card and the
 * page both format the same range.
 */
const RANGE_PART_FORMAT = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  timeZone: 'UTC',
})

const COUNT_FORMAT = new Intl.NumberFormat('en-US')

/**
 * `JUL 27 - AUG 2 2026` — the card's date line, and the page's.
 *
 * A HYPHEN, not an en or em dash: no em dashes in UI copy (project rule), and
 * the OG font subset is Latin-only anyway, so a dash outside it would be
 * fetched from Google mid-render (see `lib/og/brand`).
 *
 * `en-US` is pinned rather than left to the runtime locale for the same reason
 * the window is UTC: one cached PNG is served to every reader, so the string
 * has to be a property of the snapshot and not of whichever edge region
 * rendered it.
 *
 * The year appears once when both ends share it, and on both ends when they do
 * not — a range that straddles New Year is otherwise ambiguous about which of
 * the two years each half belongs to.
 */
export function formatGraphWeekRange(start: Date, end: Date): string {
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return ''
  const startYear = start.getUTCFullYear()
  const endYear = end.getUTCFullYear()
  const left = RANGE_PART_FORMAT.format(start).toUpperCase()
  const right = RANGE_PART_FORMAT.format(end).toUpperCase()
  if (startYear !== endYear) return `${left} ${startYear} - ${right} ${endYear}`
  return `${left} - ${right} ${endYear}`
}

/**
 * `+12 ARTISTS · +34 CONNECTIONS` — the card's count line.
 *
 * Upper case at the source, unlike the app's own copy helpers: this string is
 * drawn by Satori, which has no `text-transform`, so the casing cannot be a
 * styling decision here the way it is in CSS.
 */
export function formatGraphWeekCounts(week: GraphWeek): string {
  return [
    `+${counted(week.newArtistCount, 'ARTIST', 'ARTISTS')}`,
    `+${counted(week.newConnectionCount, 'CONNECTION', 'CONNECTIONS')}`,
  ].join(' · ')
}

/** `12 artists` / `1 artist` — grouped, and pluralised in exactly one place. */
function counted(value: number, one: string, many: string): string {
  return `${COUNT_FORMAT.format(value)} ${value === 1 ? one : many}`
}

/**
 * The same facts as a sentence, for the page's heading, the OG description and
 * the affordance's accessible name.
 *
 * Sentence case and spelled out, because unlike the card this text is read by
 * an unfurler's snippet and by a screen reader, where `+12 ARTISTS` reads as
 * punctuation.
 */
export function graphWeekSummary(week: GraphWeek): string {
  const artists = counted(week.newArtistCount, 'new artist', 'new artists')
  const connections = counted(week.newConnectionCount, 'new connection', 'new connections')
  return `${artists} and ${connections} joined the map, ${formatGraphWeekRange(
    week.start,
    week.end
  )}.`
}

/**
 * A stable key for the week this snapshot describes: `2026-08-02`, the UTC date
 * the window ends on.
 *
 * It exists to make the card's URL vary. Third-party unfurl caches (Facebook,
 * Discord, Slack) key on the image URL and hold it far longer than any
 * `Cache-Control` we set, and a file-convention OG route's URL carries a hash
 * of the ROUTE SOURCE — a constant. So a share route whose content changes
 * weekly would keep serving whichever week the scraper first saw. The scene
 * cards solved this by advertising an archived permalink that carries the week;
 * this route has no archive, so the week rides in a query parameter instead.
 * See `pattern_og_card_family`.
 *
 * Day precision, which matches the nightly cadence exactly. If the job were ever
 * run TWICE in one UTC day, the second build's card would inherit the first's
 * key and any unfurl cache already holding it would keep serving the earlier
 * image until the next day. Acceptable for a job that runs once a night;
 * worth revisiting before anything rebuilds the snapshot on demand.
 */
export function graphWeekKey(week: GraphWeek): string {
  // `toISOString` is already UTC, which is the basis this whole module works in
  // — so the date half of it IS the key, with no hand-rolled month arithmetic
  // to get off by one.
  return week.end.toISOString().slice(0, 10)
}

/**
 * Built once for the module, for the same reason as the range formatter above.
 *
 * The LOCALE is deliberately the reader's, unlike the card's: this string is
 * formatted per visitor in the browser rather than baked into the one cached
 * PNG every reader is served, so `2 Aug 2026` for a reader whose device says so
 * is right. The TIMEZONE is not similarly negotiable — see `formatLastMapped`.
 */
const LAST_MAPPED_FORMAT = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  timeZone: 'UTC',
})

/**
 * `Aug 2, 2026` — the calendar day a snapshot was built on, for the map footer.
 *
 * UTC, so that this and the week offered beside it in the same footer row name
 * the same day. The nightly job lands in the small hours UTC, the PREVIOUS
 * calendar day everywhere in the Americas: a viewer-local footer told a reader
 * in Phoenix the map was "last mapped Aug 1" while the share affordance beside
 * it offered a week ending Aug 2, for one and the same snapshot. Two dates for
 * one instant on one line of chrome, and neither one wrong on its own terms —
 * which is exactly why the rule has to be chosen once, here, and not per site.
 *
 * Returns `''` for an unparseable instant, matching `formatGraphWeekRange`, so
 * a caller drops the clause instead of printing `Invalid Date`.
 */
export function formatLastMapped(lastMapped: Date): string {
  if (Number.isNaN(lastMapped.getTime())) return ''
  return LAST_MAPPED_FORMAT.format(lastMapped)
}
