/**
 * Scene-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/catalog/scene_service.go
 */

import type { ArtistGraphCardShow } from '@/features/artists/types'
import type { components } from '@/types/api'

export interface SceneListItem {
  city: string
  state: string
  slug: string
  venue_count: number
  upcoming_show_count: number
  total_show_count: number
  // The ≤7-day slice of upcoming_show_count (PSY-1309) — drives the globe's
  // "happening in the next 7 days" pulse treatment.
  //
  // The authority for the WORDING rule below: the surfaces that render this
  // field point here instead of restating it. It is NOT the only place the
  // rolling-vs-calendar distinction is written down — `sceneWeek.ts`,
  // `SceneList.tsx` and `ThisWeekByCity.tsx` each carry a calendar-side
  // account with measurements this one does not, and `CommunityPulseResponse`
  // has its own separate field. Do not assume editing here is sufficient.
  //
  // A ROLLING window from now, so it is NOT the number /scenes/{slug}/week
  // prints. Two rules follow, and they are separate:
  //   1. WHICH FIELD: anything rendered beside a link to that page reads
  //      shows_calendar_week, so the number agrees with its destination.
  //   2. WHICH WORDS: anything rendered FROM this field is worded "next 7
  //      days", never "this week" (PSY-1732) — the calendar phrasing is
  //      reserved for shows_calendar_week, whose week really does reset.
  shows_this_week: number
  // The scene's Monday-to-Sunday total, resolved in its OWN venue timezone —
  // exactly what its /scenes/{slug}/week page reports (PSY-1623). The only
  // count safe to show next to a link to that page.
  shows_calendar_week: number
  // Geocoded city centroid for the /atlas map (PSY-1212). Absent (undefined)
  // or null when the geocoder couldn't place the city — such scenes can't be
  // plotted on the globe.
  latitude?: number | null
  longitude?: number | null
  // Dominant genre-family key for the globe dot tint (PSY-1315), present only when
  // one family holds ≥40% of the scene roster's genre mass; absent (undefined) for
  // a mixed / no-data scene, which keeps the default orange dot. Values are the
  // family keys in genreFamilies.ts (mirrors the backend catalog genre-family map).
  dominant_genre?: string
}

export interface SceneListResponse {
  scenes: SceneListItem[]
  count: number
}

export interface SceneStats {
  venue_count: number
  artist_count: number
  upcoming_show_count: number
  festival_count: number
}

export interface ScenePulse {
  shows_this_month: number
  shows_prev_month: number
  shows_trend: number
  new_artists_30d: number
  active_venues_this_month: number
  shows_by_month: number[]
}

/**
 * One room on the scene's leaderboard, ranked by upcoming shows.
 *
 * DERIVED from the generated OpenAPI schema for the same reason
 * SceneShowSummary below is: a hand-written copy drifts silently, because the
 * `api:types:check` drift gate cannot see a type it does not generate. The
 * `shows_trend` field a few lines up is the standing proof — declared here as
 * `number` while the API has long served a string.
 */
export type SceneVenue = components['schemas']['SceneVenueSummary']

export interface SceneDetail {
  city: string
  state: string
  slug: string
  description: string | null
  stats: SceneStats
  pulse: ScenePulse
  // The scene's tracked rooms, busiest first. Always an array on the wire; the
  // generated schema types it nullable only because every Go slice is.
  venues: SceneVenue[]
}

export interface SceneArtist {
  id: number
  slug: string
  name: string
  city: string
  state: string
  show_count: number
  // True when the band has an upcoming approved show or one within the active
  // window (~6mo), played anywhere (PSY-1255 step C). The roster lists every
  // band BASED in the metro; the UI highlights the active ones.
  is_active: boolean
  // The artist's embeddable Bandcamp /album|/track URL, null when the artist has
  // none. (The /atlas preview's player now uses the scene-level
  // `representative_embed` — PSY-1294 — so the preview no longer reads this
  // per-artist field, but it stays on the roster payload.)
  bandcamp_embed_url?: string | null
}

// The single band whose Bandcamp embed represents the scene (PSY-1294), chosen
// server-side over the FULL metro roster (active-first) so the preview's player
// is independent of the fetched window. Populated on the first page only
// (offset 0); null on later pages and when no band based here has one.
export interface SceneRepresentativeEmbed {
  embed_url: string
  artist_name: string
  artist_slug: string
}

export interface SceneArtistsResponse {
  artists: SceneArtist[]
  total: number
  representative_embed?: SceneRepresentativeEmbed | null
}

/**
 * The one show attached to a new-band row.
 *
 * DERIVED from the generated schema, same rule as `SceneVenue` above. Read
 * `is_upcoming` BEFORE wording anything around the date: the backend sends the
 * band's soonest upcoming approved show when it has one and its most recent
 * PAST show otherwise, so a single "next show" phrasing would be wrong half the
 * time.
 */
export type SceneNewArtistShow = components['schemas']['SceneNewArtistShow']

/** One of the scene roster's most recently listed bands (PSY-1781/PSY-1844). */
export type SceneNewArtistRow = components['schemas']['SceneNewArtistRow']

/**
 * There is no `total`. PSY-1781 published one to drive a "+N more" line while
 * the endpoint was a trailing window; PSY-1844 removed the window, so what sits
 * beyond the cap is just the rest of the roster — already on the page, with its
 * own count in `stats.artist_count`.
 */
export interface SceneNewArtistsResponse {
  artists: SceneNewArtistRow[]
}

/**
 * One upcoming show in the scene preview's "Next 7 days" row (PSY-1309).
 *
 * DERIVED from the generated OpenAPI schema, not hand-written. The hand-written
 * copy that used to live here had already fallen two fields behind the API
 * (`is_sold_out`, `is_cancelled` — so the Atlas preview panel could not have
 * told a cancelled show from a live one), and the `api:types:check` drift gate
 * cannot see a type it does not generate. Same rule as `sceneWeek.ts`.
 */
export type SceneShowSummary = components['schemas']['SceneShowSummary']

export interface SceneShowsResponse {
  shows: SceneShowSummary[]
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367) — derived per-scene artist relationship graph
// ──────────────────────────────────────────────

export interface SceneGraphInfo {
  slug: string
  city: string
  state: string
  /** Artists in the graph response (top-N cap applied). */
  artist_count: number
  edge_count: number
  /** Full based-in metro roster before the top-N cap (PSY-1277). */
  metro_roster_total: number
  /** True when metro_roster_total > artist_count. */
  roster_truncated: boolean
  /**
   * Label hub nodes in the response (PSY-1530). Counted apart from
   * `artist_count` so the roster-truncation phrasing keeps describing artists
   * only. Absent on payloads served before hubs shipped.
   */
  label_count?: number
}

export interface SceneGraphCluster {
  /** "v_<venue_id>" for first-class clusters, "other" for the rolled-up tail. */
  id: string
  /** Venue name, or "Other". */
  label: string
  /** Number of artists in this cluster. */
  size: number
  /** 0..7 = `--chart-{n+1}` palette index (PSY-1083). -1 = "other" (neutral grey). */
  color_index: number
}

export interface SceneGraphNode {
  /**
   * Artist ID for artist nodes; label hubs are offset into a reserved range so
   * both kinds share one numeric node-ID space (PSY-1530). Pair with
   * `entity_type` before treating this as a database key.
   */
  id: number
  /**
   * "artist" or "label" (PSY-1530). Optional for payloads served before hubs
   * shipped — absent means artist.
   */
  entity_type?: string
  name: string
  slug: string
  city?: string
  state?: string
  /** Label hubs only: home country, for the canvas caption when city/state are unknown. */
  country?: string
  upcoming_show_count: number
  /** Matches SceneGraphCluster.id. */
  cluster_id: string
  /** True when the artist has zero in-scene edges (post type-filter). */
  is_isolate: boolean
  /** Soonest upcoming approved show; omitted when upcoming_show_count is 0. */
  next_show?: ArtistGraphCardShow
  /** True when selecting this node opens a playable embed (Bandcamp or an
   * embeddable Spotify URL) — drives the canvas playable-marker ring (PSY-1379). */
  has_playable_audio: boolean
}

/**
 * Membership edge from a label hub to one of its in-scene roster artists
 * (PSY-1530). Built at query time — it has no `artist_relationships` row and is
 * never a valid `types` filter value.
 */
export const SCENE_EDGE_TYPE_ON_LABEL = 'on_label'

export interface SceneGraphLink {
  source_id: number
  target_id: number
  type: string
  score: number
  detail?: Record<string, unknown>
  /** True when source.cluster_id !== target.cluster_id. */
  is_cross_cluster: boolean
}

export interface SceneGraphResponse {
  scene: SceneGraphInfo
  clusters: SceneGraphCluster[]
  nodes: SceneGraphNode[]
  links: SceneGraphLink[]
}
