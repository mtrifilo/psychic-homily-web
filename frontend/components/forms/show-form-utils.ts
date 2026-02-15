import {
  parseISOToDateAndTime,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { ShowResponse, VenueResponse } from '@/lib/types/show'

export interface FormArtist {
  name: string
  is_headliner: boolean
  matched_id?: number
  instagram_handle?: string
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
}

export const defaultFormValues: FormValues = {
  title: '',
  artists: [{ name: '', is_headliner: true, matched_id: undefined, instagram_handle: undefined }],
  venue: { id: undefined, name: '', city: '', state: '', address: '' },
  date: '',
  time: '20:00',
  cost: '',
  ages: '',
  description: '',
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
    artists: show.artists.map(artist => ({
      name: artist.name,
      is_headliner: artist.is_headliner ?? false,
      matched_id: artist.id,
      instagram_handle: undefined,
    })),
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
  }
}

/**
 * Parse a cost string (e.g. "$20", "Free", "$12.50") into a number or undefined.
 */
export function parseCost(cost: string): number | undefined {
  if (!cost) return undefined
  const parsed = parseFloat(cost.replace(/[^0-9.]/g, ''))
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
