import type { components } from '@/types/api'

// ──────────────────────────────────────────────
// Data Quality / Contribution Opportunities
// ──────────────────────────────────────────────

export interface DataQualityCategory {
  key: string
  label: string
  entity_type: string
  count: number
  description: string
}

/**
 * PSY-1484 "Loose Ends" — the two high-signal, chart-seeded opportunity
 * categories surfaced in a highlighted band atop `/contribute` (spike
 * PSY-1426). Both flow through the existing `DataQualityCategory` /
 * opportunities-summary shape; these keys just tag which categories get
 * pulled out of the global grid and rendered in the band.
 *
 * - `followed_artists_missing_links` — authed-only; artists the viewer
 *   follows that are missing Bandcamp + Spotify links. Backend (PSY-1483)
 *   only includes it in the summary when authed and non-empty.
 * - `charting_artists_missing_links` — everyone; artists moving on the
 *   Broadsheet charts missing Bandcamp + Spotify. Included when non-empty.
 *
 * Defined here (not derived from generated backend types) so the frontend
 * is self-sufficient even before the companion backend ticket merges.
 */
export const LOOSE_ENDS_CATEGORY_KEYS = [
  'followed_artists_missing_links',
  'charting_artists_missing_links',
] as const

export type LooseEndsCategoryKey = (typeof LOOSE_ENDS_CATEGORY_KEYS)[number]

/** The authed-only "artists you follow" loose-ends category key. */
export const FOLLOWED_LOOSE_ENDS_KEY: LooseEndsCategoryKey =
  'followed_artists_missing_links'

/** True when a category key belongs to the Loose Ends band. */
export function isLooseEndsCategory(key: string): key is LooseEndsCategoryKey {
  return (LOOSE_ENDS_CATEGORY_KEYS as readonly string[]).includes(key)
}

export interface DataQualitySummary {
  categories: DataQualityCategory[]
  total_items: number
}

export interface DataQualityItem {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  reason: string
  show_count: number
}

// ──────────────────────────────────────────────
// Pending Edits
// ──────────────────────────────────────────────

export type PendingEditStatus = 'pending' | 'approved' | 'rejected'

/**
 * Entity types whose edit flow runs through {@link EntityEditDrawer}.
 *
 * Most entities (artist, venue, festival, release, label) route through
 * `useSuggestEdit` → /suggest-edit endpoint, which supports both direct
 * application (admin / trusted contributor / owner) and pending-review
 * submission for community contributions. Shows (PSY-461 / PSY-489 /
 * PSY-563) intentionally diverge: show edits are admin/owner-only and
 * always apply directly, so the drawer dispatches show saves to
 * `useShowUpdate` instead of `useSuggestEdit`. The suggest-edit pipeline
 * is *not* available for shows; that exclusion is preserved by design.
 */
export type EditableEntityType = 'artist' | 'venue' | 'festival' | 'release' | 'label' | 'show'

export interface FieldChange {
  field: string
  old_value: unknown
  new_value: unknown
}

export interface PendingEditResponse {
  id: number
  entity_type: string
  entity_id: number
  /** Resolved display name for the affected entity (e.g. "Phantogram"). */
  entity_name?: string
  /**
   * Slug-based URL segment for entity types whose public pages are slug-
   * addressed (artist, venue, festival, release, label). nil for entities
   * without slugs. Use to build /artists/:slug-style links — falling back
   * to entity_id alone produces broken URLs (those routes are slug-only).
   */
  entity_slug?: string | null
  submitted_by: number
  submitter_name: string
  /**
   * Submitter's username when set, null otherwise. Pass to
   * `<UserAttribution username={...} />` to render the byline as a link to
   * /users/:username when non-null. PSY-619.
   */
  submitter_username?: string | null
  field_changes: FieldChange[]
  summary: string
  /**
   * PSY-605: sanitised HTML of `summary` rendered server-side via the shared
   * MarkdownRenderer (goldmark + bluemonday, comment-system allowlist).
   * Render via `dangerouslySetInnerHTML` — the sanitiser is the source of
   * truth for XSS safety. Empty/undefined for legacy rows; the raw `summary`
   * is still available alongside as a fallback.
   */
  summary_html?: string
  status: PendingEditStatus
  reviewed_by?: number
  reviewer_name?: string
  reviewer_username?: string | null
  reviewed_at?: string
  rejection_reason?: string
  /**
   * PSY-605: sanitised HTML of `rejection_reason`. Same renderer + allowlist
   * as `summary_html`. Empty when no rejection reason has been written.
   */
  rejection_reason_html?: string
  created_at: string
  updated_at: string
}

export interface SuggestEditResponse {
  pending_edit?: PendingEditResponse
  applied: boolean
  message: string
}

/**
 * Result payload passed to {@link EntityEditDrawer}'s `onSuccess` callback.
 * `applied: true` means the change was committed directly (admin / trusted
 * contributor / owner); `applied: false` means a pending edit was filed for
 * review. Page-level success affordances (e.g. the "Changes saved" banner)
 * key off `applied`.
 */
export interface EntityEditSuccess {
  applied: boolean
}

export interface SuggestEditRequest {
  changes: FieldChange[]
  summary: string
}

/** Field configuration for the edit drawer. */
export interface EditableField {
  key: string
  label: string
  /**
   * `number` fields are submitted as a JSON number (or `null` when cleared),
   * not as the string the input holds. Every other type submits its string
   * verbatim. See `fieldChangeValue`.
   */
  type: 'text' | 'textarea' | 'url' | 'number'
  placeholder?: string
  group?: 'info' | 'social' | 'details'
  /**
   * Character cap for fields whose backing column is length-bounded. Rendered
   * as the input's `maxLength` so the browser stops the user before a round
   * trip that the server would 422 anyway. The server check is still the real
   * gate: this is UX, not validation.
   */
  maxLength?: number
  /** Inclusive bounds for `type: 'number'` fields. Mirrors the server range. */
  min?: number
  max?: number
}

/**
 * PSY-599: client-side URL pre-validator for the suggest-edit drawer's
 * `type: 'url'` fields (Instagram, Bandcamp, Twitter, Image URL, etc.).
 *
 * Returns null for valid input, or a short user-facing error string. Empty
 * input is treated as valid because empty means "clear the field" — the
 * server preserves that semantic.
 *
 * Mirrors the backend rule in `backend/internal/utils/url.go`:
 * - must parse via the WHATWG `URL` constructor
 * - protocol must be `http:` or `https:`
 * - empty or whitespace-only string is valid (clear-the-field intent)
 *
 * Server-side validation remains the source of truth; this is purely UX so
 * curators see the problem before they hit Submit and avoid a 422
 * roundtrip. Same shape as `validateCoverImageUrl` in
 * `features/collections/types.ts` so the two surfaces stay congruent.
 */
export function validateUrlField(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed.length === 0) return null

  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return 'Enter a valid URL starting with http:// or https://.'
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return 'URL must start with http:// or https://.'
  }

  return null
}

/**
 * Inclusive bounds for `venues.capacity`, mirroring `contracts.MinVenueCapacity`
 * and `contracts.MaxVenueCapacity` on the Go side.
 *
 * Declared once and spread into the field definition so the drawer, its tests,
 * and any future capacity control read the same pair. The coupling to the Go
 * constants is MANUAL: nothing enforces it across the language boundary, so a
 * change there has to be repeated here AND in `cli/src/commands/submit-venue.ts`,
 * which carries a third copy. The Go side pins only its own two copies (the
 * huma schema tags) with `TestVenueCapacitySchemaTagsMatchContract`.
 */
export const VENUE_CAPACITY_BOUNDS = { min: 1, max: 200000 } as const

/**
 * Floor for the year-valued catalog columns a contributor can edit
 * (`labels.founded_year`, `releases.release_year`). Mirrors
 * `contracts.MinCatalogYear` on the Go side, with the same MANUAL coupling as
 * `VENUE_CAPACITY_BOUNDS`: nothing enforces it across the language boundary.
 *
 * A four-digit sanity rail, not a claim about when records began. The rationale
 * lives with the Go constant.
 */
export const MIN_CATALOG_YEAR = 1000

/**
 * Inclusive ceiling for those same columns: next year, because a release can be
 * announced before it exists.
 *
 * A function, mirroring `contracts.MaxCatalogYear()`. Freezing this at module
 * load would let a tab left open across New Year's Eve reject a value the server
 * accepts, which is the one direction a client-side pre-validator must never
 * fail in: the server is the source of truth, so the client may only ever be
 * the more permissive of the two. UTC on both sides so they agree.
 */
export function maxCatalogYear(): number {
  return new Date().getUTCFullYear() + 1
}

/**
 * Bounds object for a year field. `max` is a GETTER so `field.max` resolves at
 * read time; spreading this into a field definition would freeze the ceiling and
 * defeat the point, hence the explicit getter at each use site below.
 */
export const CATALOG_YEAR_BOUNDS: { readonly min: number; readonly max: number } = {
  min: MIN_CATALOG_YEAR,
  get max() {
    return maxCatalogYear()
  },
}

/**
 * Optional-sign-then-digits grammar for `type: 'number'` fields: nothing else.
 *
 * Deliberately stricter than `Number()`, which accepts forms nobody types into
 * a capacity box and which would then be stored as a number the user never
 * wrote: `Number('0x10')` is 16 and `Number('1e3')` is 1000. `parseInt` is
 * worse still: `parseInt('3600abc')` returns 3600. Signs are matched so `-40`
 * and `+40` read as out-of-range values ("must be between 1 and 200,000")
 * rather than as gibberish, which is the more useful message.
 */
const WHOLE_NUMBER_PATTERN = /^[+-]?\d+$/

/**
 * Parses a drawer input as a whole number, or null when it is not one.
 *
 * `Number.isSafeInteger` rather than `Number.isInteger`: a 20-digit entry
 * parses to a float that no longer represents the digits the user typed, and
 * comparing that against a bound would be theatre. Every caller treats a
 * digits-only string that lands here as out of range, which it always is.
 */
function parseWholeNumber(value: string): number | null {
  const trimmed = value.trim()
  if (!WHOLE_NUMBER_PATTERN.test(trimmed)) return null
  const parsed = Number(trimmed)
  return Number.isSafeInteger(parsed) ? parsed : null
}

/**
 * Client-side pre-validator for the drawer's `type: 'number'` fields.
 *
 * Returns null for valid input, or a short user-facing error string. Empty
 * input is valid because empty means "clear the field": the server stores
 * NULL rather than a zero.
 *
 * Mirrors the backend rule in `validateBoundedInt`
 * (`backend/internal/api/handlers/shared/url_validation.go`): whole numbers
 * only, within the field's inclusive range.
 *
 * Server-side validation remains the source of truth; this is purely UX, the
 * same way `validateUrlField` is.
 */
export function validateNumberField(
  value: string,
  bounds: { min?: number; max?: number } = {}
): string | null {
  const trimmed = value.trim()
  if (trimmed.length === 0) return null

  const { min, max } = bounds
  const outOfRange =
    min !== undefined && max !== undefined
      ? `Enter a number between ${min.toLocaleString()} and ${max.toLocaleString()}.`
      : min !== undefined
        ? `Enter a number of at least ${min.toLocaleString()}.`
        : max !== undefined
          ? `Enter a number of at most ${max.toLocaleString()}.`
          : null

  const parsed = parseWholeNumber(trimmed)
  if (parsed === null) {
    // Digits that failed to parse are too large to represent, which is an
    // out-of-range value rather than a malformed one. Saying "that is not a
    // whole number" about 99999999999999999999 would be false and unhelpful.
    if (WHOLE_NUMBER_PATTERN.test(trimmed) && outOfRange !== null) return outOfRange
    return 'Enter a whole number.'
  }

  if (min !== undefined && parsed < min) return outOfRange
  if (max !== undefined && parsed > max) return outOfRange

  return null
}

/**
 * Validation dispatch for one drawer field. Keeps the drawer from having to
 * know which validator belongs to which field type, so adding a validated type
 * is a change here rather than a new branch in the component.
 */
export function validateFieldValue(field: EditableField, value: string): string | null {
  switch (field.type) {
    case 'url':
      return validateUrlField(value)
    case 'number':
      return validateNumberField(value, { min: field.min, max: field.max })
    default:
      return null
  }
}

/**
 * Converts a drawer input's string into the value that goes on the wire for
 * that field.
 *
 * Empty is `null` for every type: the drawer's clear gesture has to reach the
 * column as SQL NULL, not as `''` or `0`. `type: 'number'` fields send an
 * actual JSON number, because their column is an integer and the backend
 * rejects a numeric string on purpose rather than parsing it (one encoding for
 * one edit, in both `pending_entity_edits` and `revisions.field_changes`).
 *
 * A value that fails `validateFieldValue` is passed through unconverted. That
 * case cannot reach the server (Submit is disabled while any field has an
 * error), so coercing it would only invent a number the user did not type.
 *
 * The drawer also uses this to decide WHETHER a field changed, so the function
 * has to be stable: two inputs that mean the same edit must convert to the same
 * value, or a no-op lands in the review queue.
 */
export function fieldChangeValue(field: EditableField, value: string): string | number | null {
  // Non-numeric fields keep their long-standing behavior verbatim: the raw
  // string, or null when it is empty.
  if (field.type !== 'number') return value || null
  if (value.trim().length === 0) return null
  return parseWholeNumber(value) ?? value
}

export type ReportableEntityType = 'artist' | 'venue' | 'festival' | 'show' | 'comment' | 'collection' | 'release' | 'label'

/**
 * A row in `entity_reports` — the ONE table every entity report round-trips
 * through, whatever the entity type.
 *
 * Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600):
 * the reporter's read-back and the moderation queue must not be able to drift
 * apart, and generating both from the spec enforces that structurally rather
 * than by discipline. Regenerate with `bun run api:types`.
 *
 * PSY-1633 folded the last per-entity report shape (`ArtistReportResponse`,
 * keyed on `artist_id`) into this one, after the artist route turned out to be
 * answering with this shape all along.
 */
export type EntityReportResponse =
  components['schemas']['EntityReportResponse']

export interface ReportTypeOption {
  value: string
  label: string
  description: string
}

/** Report type options per entity type — matches backend entity_reports report_type values. */
export const REPORT_TYPES: Record<ReportableEntityType, ReportTypeOption[]> = {
  artist: [
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Name, bio, social links, or other info is wrong' },
    { value: 'duplicate', label: 'Duplicate Artist', description: 'This artist already exists under a different name' },
    { value: 'wrong_image', label: 'Wrong Image', description: 'The artist image is incorrect' },
    { value: 'removal_request', label: 'Removal Request', description: 'This artist page should be removed' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
  venue: [
    { value: 'closed_permanently', label: 'Permanently Closed', description: 'This venue has permanently closed' },
    { value: 'wrong_address', label: 'Wrong Address', description: 'The address or location is incorrect' },
    { value: 'duplicate', label: 'Duplicate Venue', description: 'This venue already exists under a different name' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Name, details, or other info is wrong' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
  festival: [
    { value: 'cancelled', label: 'Cancelled', description: 'This festival has been cancelled' },
    { value: 'wrong_dates', label: 'Wrong Dates', description: 'The festival dates are incorrect' },
    { value: 'duplicate', label: 'Duplicate Festival', description: 'This festival already exists' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Information is wrong or outdated' },
  ],
  show: [
    { value: 'cancelled', label: 'Cancelled', description: 'This show has been cancelled' },
    { value: 'sold_out', label: 'Sold Out', description: 'This show is sold out' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Date, time, venue, or other info is wrong' },
    { value: 'wrong_venue', label: 'Wrong Venue', description: 'This show is listed at the wrong venue' },
    { value: 'wrong_date', label: 'Wrong Date', description: 'The show date or time is incorrect' },
  ],
  comment: [
    { value: 'spam', label: 'Spam', description: 'This comment is spam or advertising' },
    { value: 'harassment', label: 'Harassment', description: 'This comment is abusive or harassing' },
    { value: 'off_topic', label: 'Off Topic', description: 'This comment is irrelevant to the discussion' },
    { value: 'inaccurate', label: 'Inaccurate', description: 'This comment contains incorrect information' },
    { value: 'other', label: 'Other', description: 'Another issue not listed above' },
  ],
  // PSY-578: collection-specific taxonomy (diverges from comment vocab —
  // "Harassment" rarely fits a curated list, "Off Topic" is a category
  // complaint not a moderation issue, and "Inaccurate" doesn't capture the
  // common cases). Aligned with the backend allow list in
  // backend/internal/models/community/entity_report.go.
  collection: [
    { value: 'spam', label: 'Spam', description: 'This collection is spam or advertising' },
    { value: 'inappropriate', label: 'Inappropriate', description: 'NSFW cover, hateful theme, or abusive content' },
    { value: 'misleading', label: 'Misleading', description: 'False claims in the description or item notes' },
    { value: 'other', label: 'Other', description: 'Another issue not listed above' },
  ],
  // PSY-661: release-tailored taxonomy (diverges from the generic
  // artist/venue vocab to name field-specific corrections common on a
  // release record). Aligned with the backend allow list in
  // backend/internal/models/community/entity_report.go — `value` strings
  // must match byte-for-byte.
  release: [
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Title, year, tracklist, or other info is wrong' },
    { value: 'duplicate', label: 'Duplicate Release', description: 'This release already exists under a different entry' },
    { value: 'wrong_cover_art', label: 'Wrong Cover Art', description: 'The cover image is incorrect' },
    { value: 'wrong_release_date', label: 'Wrong Release Date', description: 'The release date or year is incorrect' },
    { value: 'wrong_artist_attribution', label: 'Wrong Artist', description: 'This release is attributed to the wrong artist' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
  // PSY-666: label-tailored taxonomy (mirrors the PSY-578 collection +
  // PSY-661 release precedent of tailoring per entity). Aligned with the
  // backend allow list in
  // backend/internal/models/community/entity_report.go — `value` strings
  // must match byte-for-byte. "Defunct" is deliberately omitted: label
  // lifecycle is a `status` field edit, not a moderation report.
  label: [
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Name, bio, or other info is wrong' },
    { value: 'duplicate', label: 'Duplicate Label', description: 'This label already exists under a different entry' },
    { value: 'wrong_image', label: 'Wrong Image', description: 'The label image is incorrect' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
}

/** Editable fields per entity type — matches backend allowedEditFields. */
export const EDITABLE_FIELDS: Record<EditableEntityType, EditableField[]> = {
  artist: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
  venue: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'address', label: 'Address', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'zipcode', label: 'Zipcode', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    // PSY-1682: the venue's HOUSE DEFAULT age rule. A show's own age
    // requirement is the per-event override and is edited on the show, not
    // here. Free text so the room's real wording survives.
    { key: 'age_policy', label: 'Age Policy', type: 'text', placeholder: 'All Ages, 17+, 21+', group: 'details', maxLength: 100 },
    // Room capacity. Submitted as a JSON number, like the two year fields
    // below; every other field in this map rides as a string. The server is the
    // real gate; these bounds only stop the round trip.
    { key: 'capacity', label: 'Capacity', type: 'number', placeholder: 'e.g. 550', group: 'details', ...VENUE_CAPACITY_BOUNDS },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
  festival: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'location_name', label: 'Location Name', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'ticket_url', label: 'Ticket URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'flyer_url', label: 'Flyer URL', type: 'url', placeholder: 'https://...', group: 'info' },
  ],
  release: [
    { key: 'title', label: 'Title', type: 'text', group: 'info' },
    { key: 'release_type', label: 'Release Type', type: 'text', placeholder: 'lp, ep, single, compilation, live, remix, demo', group: 'info' },
    // PSY-1703: an integer column, so it must submit a JSON number. `max` is a
    // getter (see CATALOG_YEAR_BOUNDS); spreading would freeze the ceiling.
    { key: 'release_year', label: 'Release Year', type: 'number', placeholder: '1991', group: 'info', min: CATALOG_YEAR_BOUNDS.min, get max() { return CATALOG_YEAR_BOUNDS.max } },
    // release_date stays text: it is a separate, free-text column, not gated by
    // the numeric registry.
    { key: 'release_date', label: 'Release Date', type: 'text', placeholder: 'YYYY-MM-DD', group: 'info' },
    { key: 'cover_art_url', label: 'Cover Art URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
  ],
  label: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    // PSY-1703: an integer column, so it must submit a JSON number. `max` is a
    // getter (see CATALOG_YEAR_BOUNDS); spreading would freeze the ceiling.
    { key: 'founded_year', label: 'Founded Year', type: 'number', placeholder: '1985', group: 'info', min: CATALOG_YEAR_BOUNDS.min, get max() { return CATALOG_YEAR_BOUNDS.max } },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
  // PSY-563: shows expose only the scalar fields the backend
  // UpdateShowRequest accepts via the direct-save pathway. Venue and
  // artist association edits stay in the dedicated ShowForm — they need
  // entity-search UI that the field-by-field drawer doesn't model.
  show: [
    { key: 'title', label: 'Title', type: 'text', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'age_requirement', label: 'Age Requirement', type: 'text', placeholder: '21+, All Ages', group: 'details' },
    { key: 'ticket_url', label: 'Ticket URL', type: 'url', placeholder: 'https://...', group: 'details' },
    { key: 'image_url', label: 'Flyer Image URL', type: 'url', placeholder: 'https://...', group: 'details' },
  ],
}
