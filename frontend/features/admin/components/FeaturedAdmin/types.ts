/**
 * Wire-shape types for the admin featured-slot endpoints (PSY-835).
 * Mirrors `backend/internal/api/handlers/admin/featured_slot.go`.
 *
 * `slot_type` is the only discriminator — `entity_id` resolves to a
 * show ID (`bill`) or collection ID (`collection`). The backend does
 * not validate the referent exists; the admin tool is responsible for
 * picking a real entity via the entity-search primitive.
 */

export type FeaturedSlotType = 'bill' | 'collection'

export const FEATURED_SLOT_TYPES: readonly FeaturedSlotType[] = [
  'bill',
  'collection',
] as const

/**
 * Backend response shape for a single featured-slot row.
 * `curator_note` is the raw markdown source so the admin can re-edit a
 * pick without round-tripping through the rendered HTML.
 */
export interface FeaturedSlotResponse {
  id: number
  slot_type: FeaturedSlotType
  entity_id: number
  curator_note?: string | null
  curator_note_html?: string
  active_from: string
  active_until?: string | null
  created_by: number
  created_at: string
  updated_at: string
}

/**
 * One slot type's bundle: the currently-active pick (or null) plus
 * recent retired history. Backend returns one entry per slot type.
 */
export interface FeaturedSlotsPerType {
  slot_type: FeaturedSlotType
  active?: FeaturedSlotResponse | null
  history: FeaturedSlotResponse[]
}

export interface ListFeaturedSlotsResponse {
  slots: FeaturedSlotsPerType[]
}

export interface SetFeaturedSlotInput {
  slot_type: FeaturedSlotType
  entity_id: number
  curator_note?: string | null
}

/**
 * Backend POST /admin/featured-slots returns the new active row
 * directly — Huma serializes the handler's `Body` field VALUE as the
 * JSON response, so the wire shape is a flat `FeaturedSlotResponse`,
 * NOT `{ body: FeaturedSlotResponse }` (PSY-854 followup to PSY-838).
 * Alias kept so importers don't need to switch to `FeaturedSlotResponse`
 * at call sites.
 */
export type SetFeaturedSlotResponse = FeaturedSlotResponse

export interface RetireFeaturedSlotResponse {
  slot_type: FeaturedSlotType
  message: string
}

/**
 * Mirrors the backend `MaxFeaturedSlotCuratorNoteLength` constant
 * (10,000) so the editor's character counter and the server-side cap
 * stay in sync. Also matches the comment / field-note body limits the
 * same renderer is shared with.
 */
export const MAX_CURATOR_NOTE_LENGTH = 10_000

export const FEATURED_SLOT_LABEL: Record<FeaturedSlotType, string> = {
  bill: 'Featured Bill',
  collection: 'Featured Collection',
}

// ──────────────────────────────────────────────
// Hydrated referent shapes (from /explore/featured)
// ──────────────────────────────────────────────
//
// The admin endpoint returns only `entity_id` for the active row.
// `/explore/featured` (PSY-835 public read endpoint) returns the same
// active picks with referent details hydrated — name, thumbnail,
// curator-note-rendered-HTML. The admin page uses the explore
// endpoint as the read source for the "current active" cards so the
// curator can see WHAT they picked without an extra backend round-trip.

export interface ExploreFeaturedBill {
  id: number
  slug: string
  title: string
  event_date: string
  headliner_name: string
  venue_name: string
  venue_city: string
  venue_state: string
  image_url?: string | null
  curator_note?: string | null
  curator_note_html?: string
}

export interface ExploreFeaturedCollection {
  id: number
  slug: string
  title: string
  description?: string
  description_html?: string
  cover_image_url?: string | null
  curator_note?: string | null
  curator_note_html?: string
}

export interface ExploreFeaturedResponse {
  bill: ExploreFeaturedBill | null
  collection: ExploreFeaturedCollection | null
}
