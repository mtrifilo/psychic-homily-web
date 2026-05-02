import {
  parseISOToDateAndTime,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { ShowResponse, VenueResponse } from '@/features/shows'

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
  image_url: string
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
