import {
  parseISOToDateAndTime,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { SetType, ShowResponse, VenueResponse } from '../types'
import type { ExtractedShowData } from '@/lib/types/extraction'

/**
 * The set_type choices offered in the show form, in the order they appear in
 * the selector: top of bill first, then descending specificity, with the
 * neutral default last.
 *
 * Order and membership mirror contracts.SetTypeVocabulary() on the backend.
 * The labels are UI copy for the FORM only -- how a role is annotated on a
 * show page is a separate decision and deliberately not defined here.
 */
export const SET_TYPE_OPTIONS: ReadonlyArray<{
  value: SetType
  label: string
}> = [
  { value: 'headliner', label: 'Headliner' },
  { value: 'direct_support', label: 'Direct support' },
  { value: 'opener', label: 'Opener' },
  { value: 'special_guest', label: 'Special guest' },
  { value: 'dj', label: 'DJ' },
  { value: 'performer', label: 'Performer (slot unknown)' },
]

/**
 * The vocabulary as a non-empty tuple, for schema validators that need one.
 * Derived from SET_TYPE_OPTIONS so the selector and the validator can never
 * disagree about what is accepted.
 */
export const SET_TYPE_VALUES = SET_TYPE_OPTIONS.map(
  option => option.value
) as [SetType, ...SetType[]]

/** The value written when nobody has curated an act's slot. */
export const DEFAULT_SET_TYPE: SetType = 'performer'

/**
 * Coerce a server-supplied set_type into the vocabulary the form can render.
 *
 * Falls back to the neutral default rather than dropping the row or guessing
 * a role: an unrecognized value means the server knows a slot this build of
 * the form does not, and showing "slot unknown" is the only honest answer a
 * stale client can give.
 */
export function toSetType(value: string | null | undefined): SetType {
  const match = SET_TYPE_OPTIONS.find(option => option.value === value)
  return match ? match.value : DEFAULT_SET_TYPE
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
  const venueTz = venue?.state ? getTimezoneForState(venue.state) : undefined
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
      state: venue?.state || show.state || '',
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
        // leaves it empty when the flyer did not state a slot. Prefer it, and
        // fall back to the headliner flag rather than inventing a support
        // role for everything below the top line.
        name: a.matched_name || a.name,
        set_type: a.set_type
          ? toSetType(a.set_type)
          : a.is_headliner
            ? 'headliner'
            : DEFAULT_SET_TYPE,
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
