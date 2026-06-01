import {
  parseISOToDateAndTime,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { ShowResponse, VenueResponse } from '@/features/shows'
import type { ExtractedShowData } from '@/lib/types/extraction'

export interface FormArtist {
  /**
   * Stable per-entry identifier used as the React key in the artists list.
   * Transient (UI-only) — never sent to the backend. Required so add/remove/reorder
   * does not let React reuse the wrong DOM/component state across siblings.
   */
  _clientId: string
  name: string
  is_headliner: boolean
  matched_id?: number
  instagram_handle?: string
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
      is_headliner: true,
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
        is_headliner: artist.is_headliner ?? false,
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
 */
export function removeArtistAtIndex(
  artists: FormArtist[],
  index: number
): FormArtist[] | null {
  if (artists.length <= 1) return null

  const wasHeadliner = artists[index]?.is_headliner
  const remaining = artists.filter((_, i) => i !== index)

  if (wasHeadliner && remaining.length > 0) {
    remaining[0] = { ...remaining[0], is_headliner: true }
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
        name: a.matched_name || a.name,
        is_headliner: a.is_headliner,
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
