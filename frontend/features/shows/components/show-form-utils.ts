import { parseISOToDateAndTime } from '@/lib/utils/timeUtils'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { showTimingInput } from '../utils'
import type { SetType, ShowResponse, VenueResponse } from '../types'
import type { ExtractedShowData } from '@/lib/types/extraction'

/**
 * Human labels for every bill role, for the show FORM.
 *
 * Typed as an EXHAUSTIVE Record on purpose. `SetType` is derived from the
 * generated OpenAPI enum, so a value added on the backend widens the union and
 * this object stops compiling until somebody supplies a label. That compile
 * error is the guard: without it a stale client would silently coerce the new
 * role to the neutral default and overwrite it on the next save.
 *
 * These strings are form copy only. How a role is ANNOTATED on a show page is
 * a separate decision and deliberately not defined here.
 */
const SET_TYPE_LABELS: Record<SetType, string> = {
  headliner: 'Headliner',
  direct_support: 'Direct support',
  opener: 'Opener',
  special_guest: 'Special guest',
  dj: 'DJ',
  performer: 'Performer (slot unknown)',
}

/**
 * The vocabulary in presentation order: top of bill first, then descending
 * specificity, with the neutral default last. Mirrors the order of
 * contracts.SetTypeVocabulary() in
 * backend/internal/services/contracts/set_type.go.
 *
 * Order cannot be derived from a union (TypeScript unions are unordered), so
 * this list is written out. `satisfies` pins every entry to a real SetType, and
 * the exhaustiveness assertion below pins the reverse direction.
 */
export const SET_TYPE_VALUES = [
  'headliner',
  'direct_support',
  'opener',
  'special_guest',
  'dj',
  'performer',
] as const satisfies readonly SetType[]

/**
 * Compile-time proof that SET_TYPE_VALUES covers the whole vocabulary. If the
 * backend adds a role and the ordered list above is not updated, this type
 * resolves to `never` and the assignment below fails to build.
 */
type UnlistedSetType = Exclude<SetType, (typeof SET_TYPE_VALUES)[number]>
const _everySetTypeIsListed: UnlistedSetType extends never ? true : never = true
void _everySetTypeIsListed

/** The set_type choices offered in the show form, in presentation order. */
export const SET_TYPE_OPTIONS: ReadonlyArray<{
  value: SetType
  label: string
}> = SET_TYPE_VALUES.map(value => ({ value, label: SET_TYPE_LABELS[value] }))

/** The value written when nobody has curated an act's slot. */
export const DEFAULT_SET_TYPE: SetType = 'performer'

/**
 * Coerce a server-supplied set_type into the vocabulary the form can render.
 *
 * Falls back to the neutral default rather than dropping the row or guessing a
 * role. This is a DISPLAY coercion, and it is only safe because it should be
 * unreachable: every backend write path validates against the same vocabulary,
 * the PSY-1673 migration normalized the rows that predate it, and SetType is
 * derived from the generated enum so this build cannot fall behind the server
 * without failing to compile. If it ever does fire, the form will send the
 * coerced value back on save.
 */
export function toSetType(value: string | null | undefined): SetType {
  return SET_TYPE_VALUES.find(known => known === value) ?? DEFAULT_SET_TYPE
}

/**
 * Resolve the bill role for an inbound record that may carry either signal.
 *
 * The same precedence the backend applies when writing the row: a curated
 * set_type wins, the legacy is_headliner flag decides only in its absence, and
 * anything else is the neutral default. Named rather than inlined so the one
 * caller that needs it today (the AI-extraction merge) and any future importer
 * share a ladder instead of each carrying a private copy.
 *
 * Note it never infers a headliner from list position: that inference belongs
 * to the write path, which knows the whole bill.
 */
export function resolveFormSetType(source: {
  set_type?: string | null
  is_headliner?: boolean | null
}): SetType {
  if (source.set_type) return toSetType(source.set_type)
  return source.is_headliner ? 'headliner' : DEFAULT_SET_TYPE
}

export interface FormArtist {
  /**
   * Stable per-entry identifier used as the React key in the artists list.
   * Transient (UI-only) — never sent to the backend. Required so add/remove/reorder
   * does not let React reuse the wrong DOM/component state across siblings.
   */
  _clientId: string
  name: string
  /**
   * The act's curated bill role, and the form's SINGLE source of truth for
   * headliner-ness: is_headliner is derived from it at submit time, never
   * tracked alongside it, so the two cannot drift apart in form state.
   */
  set_type: SetType
  matched_id?: number
  instagram_handle?: string
}

/** Artist entry as sent to the show create/update endpoints. */
export interface ArtistPayload {
  id?: number
  name: string
  is_headliner: boolean
  set_type: SetType
  instagram_handle?: string
}

/**
 * Map the form's artist rows onto the API payload.
 *
 * is_headliner is derived here rather than carried: the backend treats
 * set_type as authoritative and derives the flag the same way, and sending
 * both keeps older readers of the response working.
 */
export function toArtistPayloads(artists: FormArtist[]): ArtistPayload[] {
  return artists.map(artist => ({
    id: artist.matched_id,
    name: artist.name,
    is_headliner: artist.set_type === 'headliner',
    set_type: artist.set_type,
    instagram_handle: artist.matched_id
      ? undefined
      : artist.instagram_handle || undefined,
  }))
}

/**
 * Build a FormArtist, minting a fresh _clientId. Always use this when creating
 * a new entry (initial form values, add-artist button, AI extraction) so the
 * artists list always has stable keys.
 */
export function makeFormArtist(
  artist: Omit<FormArtist, '_clientId'>
): FormArtist {
  return { ...artist, _clientId: crypto.randomUUID() }
}

export interface FormValues {
  title: string
  artists: FormArtist[]
  venue: {
    id?: number
    name: string
    city: string
    state: string
    address: string
  }
  date: string
  time: string
  cost: string
  ages: string
  description: string
  image_url: string
}

export const defaultFormValues: FormValues = {
  title: '',
  artists: [
    makeFormArtist({
      name: '',
      set_type: 'headliner',
      matched_id: undefined,
      instagram_handle: undefined,
    }),
  ],
  venue: { id: undefined, name: '', city: '', state: '', address: '' },
  date: '',
  time: '20:00',
  cost: '',
  ages: '',
  description: '',
  image_url: '',
}

/**
 * Convert ShowResponse data to form values for editing
 */
export function showToFormValues(show: ShowResponse): FormValues {
  const venue = show.venues[0]
  // Same resolver AND the same inputs the show PAGE renders with (PSY-1873).
  // Reading the stored instant back through a different zone than it is
  // displayed in would show the editor a date and time nobody else sees, and
  // saving would then rewrite the instant to match that misreading. Before
  // this, a show at a venue outside the US opened in the form at 20:00
  // America/Phoenix while the page rendered 04:00 the next day.
  //
  // Going through showTimingInput rather than re-spelling its two fields is
  // what keeps the form and the page from drifting: it carries the
  // `venue?.state ?? show.state` fallback, which the write path below also
  // applies (the venue.state form field is seeded the same way). Spelling only
  // `venue?.state` here would read a venue-less show in Phoenix and write it
  // back in its own state, shifting event_date on a no-op save.
  const timing = showTimingInput(show)
  const venueTz = resolveShowTimezone(timing.state, timing.timezone)
  const { date, time } = parseISOToDateAndTime(show.event_date, venueTz)

  return {
    title: show.title || '',
    artists: show.artists.map(artist =>
      makeFormArtist({
        name: artist.name,
        set_type: toSetType(artist.set_type),
        matched_id: artist.id,
        instagram_handle: undefined,
      })
    ),
    venue: {
      name: venue?.name || '',
      city: venue?.city || show.city || '',
      // Same expression `showTimingInput` now uses, which is the point: this
      // field is what the SUBMIT path resolves its zone from, so if the two
      // spellings diverge a no-op Save rewrites `event_date` (PSY-1696).
      state: timing.state || '',
      address: venue?.address || '',
    },
    date,
    time,
    cost: show.price != null ? `$${show.price}` : '',
    ages: show.age_requirement || '',
    description: show.description || '',
    image_url: show.image_url || '',
  }
}

/**
 * Parse a cost string (e.g. "$20", "Free", "$12.50", "$12 adv / $18 day of")
 * into a number or undefined.
 *
 * Extracts the first dollar amount from the string. For compound prices like
 * "$12 adv / $18 day of", returns the first price (12). Recognizes "free"
 * (case-insensitive) as 0.
 */
export function parseCost(cost: string): number | undefined {
  if (!cost) return undefined

  // "Free" (case-insensitive) means $0
  if (/^\s*free\s*$/i.test(cost)) return 0

  // Match the first dollar amount: optional "$", then digits with optional decimal
  const match = cost.match(/\$?\s*(\d+(?:\.\d+)?)/)
  if (!match) return undefined

  const parsed = parseFloat(match[1])
  return isNaN(parsed) ? undefined : parsed
}

/**
 * Remove an artist at the given index. If the removed artist was the headliner,
 * promote the first remaining artist to headliner.
 * Returns null if removal would leave zero artists.
 *
 * Promotion only ever assigns the HEADLINER slot. It never rewrites any other
 * role, because losing an act says nothing about what slot the survivors
 * played.
 */
export function removeArtistAtIndex(
  artists: FormArtist[],
  index: number
): FormArtist[] | null {
  if (artists.length <= 1) return null

  const wasHeadliner = artists[index]?.set_type === 'headliner'
  const remaining = artists.filter((_, i) => i !== index)

  if (wasHeadliner && remaining.length > 0) {
    remaining[0] = { ...remaining[0], set_type: 'headliner' }
  }

  return remaining
}

/**
 * Determine whether venue location fields are editable.
 *
 * Editable if:
 * 1. No prefilled venue (locks venue selection), AND
 * 2. User is admin (always), OR no venue selected, OR selected venue is unverified
 */
export function isVenueLocationEditable(
  isAdmin: boolean,
  selectedVenue: VenueResponse | null,
  hasPrefilledVenue: boolean
): boolean {
  return !hasPrefilledVenue && (isAdmin || !selectedVenue || !selectedVenue.verified)
}

/**
 * Fold AI-extracted show data into a base set of form values, producing the
 * `defaultValues` ShowForm hands to TanStack Form at mount.
 *
 * This is the calculate-during-render replacement for the old prop-derived
 * `useEffect` (PSY-795): the parent remounts ShowForm via `key` on each new
 * extraction, so seeding `defaultValues` here is the correct one-shot init.
 * Only fields present in the extraction override the base; everything else
 * keeps its base value (so a sparse extraction won't blank out defaults).
 */
export function mergeExtraction(
  base: FormValues,
  extraction: ExtractedShowData | undefined
): FormValues {
  if (!extraction) return base

  const merged: FormValues = { ...base, venue: { ...base.venue } }

  if (extraction.artists.length > 0) {
    merged.artists = extraction.artists.map(a =>
      makeFormArtist({
        // The extraction endpoint normalizes set_type to the vocabulary and
        // leaves it empty when the flyer did not state a slot, so the headliner
        // flag is the fallback rather than an invented support role.
        name: a.matched_name || a.name,
        set_type: resolveFormSetType(a),
        matched_id: a.matched_id,
        instagram_handle: a.matched_id ? undefined : a.instagram_handle,
      })
    )
  }

  if (extraction.venue) {
    const v = extraction.venue
    merged.venue = {
      id: v.matched_id,
      name: v.matched_name || v.name,
      city: v.city || '',
      state: v.state || '',
      address: '',
    }
  }

  if (extraction.date) merged.date = extraction.date
  if (extraction.time) merged.time = extraction.time
  if (extraction.cost) merged.cost = extraction.cost
  if (extraction.ages) merged.ages = extraction.ages
  if (extraction.description) merged.description = extraction.description

  return merged
}

/**
 * Derive the selected-venue state for a fresh ShowForm mount from an AI
 * extraction. Returns a VenueResponse only when the extraction matched an
 * existing venue (id + name + slug present) — matched venues are assumed
 * verified, which locks the location fields for non-admins exactly as the old
 * effect did. Returns null for an unmatched / absent venue, which surfaces the
 * "New Venue" banner.
 */
export function extractedVenueToSelected(
  extraction: ExtractedShowData | undefined
): VenueResponse | null {
  const v = extraction?.venue
  if (v?.matched_id && v.matched_name && v.matched_slug) {
    return {
      id: v.matched_id,
      slug: v.matched_slug,
      name: v.matched_name,
      address: null,
      city: v.city || '',
      state: v.state || '',
      verified: true, // matched venues are assumed verified (mirrors prior effect)
    }
  }
  return null
}
