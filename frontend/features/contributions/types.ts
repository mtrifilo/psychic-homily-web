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
  type: 'text' | 'textarea' | 'url'
  placeholder?: string
  group?: 'info' | 'social' | 'details'
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

export type ReportableEntityType = 'artist' | 'venue' | 'festival' | 'show' | 'comment' | 'collection' | 'release' | 'label'

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
    { key: 'release_year', label: 'Release Year', type: 'text', placeholder: '1991', group: 'info' },
    { key: 'release_date', label: 'Release Date', type: 'text', placeholder: 'YYYY-MM-DD', group: 'info' },
    { key: 'cover_art_url', label: 'Cover Art URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
  ],
  label: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'founded_year', label: 'Founded Year', type: 'text', placeholder: '1985', group: 'info' },
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
