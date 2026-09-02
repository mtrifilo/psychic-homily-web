import { describe, it, expect } from 'vitest'
import {
  showToFormValues,
  parseCost,
  removeArtistAtIndex,
  isVenueLocationEditable,
  defaultFormValues,
  makeFormArtist,
  mergeExtraction,
  extractedVenueToSelected,
  toArtistPayloads,
  toSetType,
  resolveFormSetType,
  DEFAULT_SET_TYPE,
  SET_TYPE_OPTIONS,
  SET_TYPE_VALUES,
  type FormArtist,
} from './show-form-utils'
import type { ShowResponse, VenueResponse } from '../types'
import type { ExtractedShowData } from '@/lib/types/extraction'

// --- Helpers ---

function makeShowResponse(overrides?: Partial<ShowResponse>): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-03-15T03:00:00Z', // 8pm MST (America/Phoenix = UTC-7)
    city: 'Phoenix',
    state: 'AZ',
    price: 20,
    age_requirement: '21+',
    description: 'A great show',
    status: 'approved',
    venues: [
      {
        id: 10,
        slug: 'the-venue',
        name: 'The Venue',
        address: '123 Main St',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      {
        id: 100,
        slug: 'artist-one',
        name: 'Artist One',
        is_headliner: true,
        set_type: 'headliner',
        position: 1,
        socials: {},
      },
      {
        id: 101,
        slug: 'artist-two',
        name: 'Artist Two',
        is_headliner: false,
        set_type: 'opener',
        position: 2,
        socials: {},
      },
    ],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

function makeVenueResponse(overrides?: Partial<VenueResponse>): VenueResponse {
  return {
    id: 1,
    slug: 'test-venue',
    name: 'Test Venue',
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    ...overrides,
  }
}

// --- showToFormValues ---

describe('showToFormValues', () => {
  it('maps basic show fields', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.title).toBe('Test Show')
    expect(result.cost).toBe('$20')
    expect(result.ages).toBe('21+')
    expect(result.description).toBe('A great show')
  })

  it('maps venue from first venue in array', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.venue.id).toBe(10)
    expect(result.venue.name).toBe('The Venue')
    expect(result.venue.city).toBe('Phoenix')
    expect(result.venue.state).toBe('AZ')
    expect(result.venue.address).toBe('123 Main St')
  })


  it('maps artists with their stored bill role', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.artists).toHaveLength(2)
    expect(result.artists[0].name).toBe('Artist One')
    expect(result.artists[0].set_type).toBe('headliner')
    expect(result.artists[0].matched_id).toBe(100)
    expect(result.artists[1].name).toBe('Artist Two')
    // A curated 'opener' round-trips into the editor unchanged.
    expect(result.artists[1].set_type).toBe('opener')
  })

  it('loads every vocabulary value into the editor unchanged', () => {
    const show = makeShowResponse({
      artists: SET_TYPE_VALUES.map((value, index) => ({
        id: 200 + index,
        slug: `artist-${value}`,
        name: `Artist ${value}`,
        set_type: value,
        position: index,
        socials: {},
      })),
    })

    expect(showToFormValues(show).artists.map(a => a.set_type)).toEqual([
      ...SET_TYPE_VALUES,
    ])
  })

  it('falls back to the neutral default for a set_type this build cannot render', () => {
    // A newer server can know a slot this client does not. Showing "slot
    // unknown" is the only honest answer; guessing a role is not.
    const show = makeShowResponse({
      artists: [
        {
          id: 300,
          slug: 'artist-future',
          name: 'Future Slot',
          set_type: 'co_headliner' as ShowResponse['artists'][number]['set_type'],
          position: 0,
          socials: {},
        },
      ],
    })

    expect(showToFormValues(show).artists[0].set_type).toBe('performer')
  })

  it('assigns a unique _clientId to every artist in edit mode', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.artists[0]._clientId).toBeTruthy()
    expect(result.artists[1]._clientId).toBeTruthy()
    expect(result.artists[0]._clientId).not.toBe(result.artists[1]._clientId)
  })

  it('parses date and time in venue timezone', () => {
    // 2026-03-15T03:00:00Z = 2026-03-14 at 20:00 MST (UTC-7)
    const show = makeShowResponse({ event_date: '2026-03-15T03:00:00Z' })
    const result = showToFormValues(show)

    expect(result.date).toBe('2026-03-14')
    expect(result.time).toBe('20:00')
  })

  it("reads the instant back in the venue's own timezone, not the state map", () => {
    // PSY-1873: "England" is not in the US state map, so the editor used to
    // open a Leeds show at 20:00 America/Phoenix on Oct 23 while the show page
    // rendered 4:00 AM on Oct 24. Saving from that state rewrote the instant.
    const show = makeShowResponse({
      event_date: '2026-10-24T03:00:00Z',
      city: 'Leeds',
      state: 'England',
      venues: [
        {
          id: 160,
          slug: 'boom-leeds-leeds-england',
          name: 'Boom Leeds',
          city: 'Leeds',
          state: 'England',
          timezone: 'Europe/London',
          verified: true,
        },
      ],
    })
    const result = showToFormValues(show)

    expect(result.date).toBe('2026-10-24')
    expect(result.time).toBe('04:00')
  })

  it('falls back to show.state for a show with no venue row', () => {
    // The read side must apply the same `venue?.state ?? show.state` fallback
    // the write path does, or opening a venue-less New York show and saving it
    // unchanged shifts event_date by the Phoenix/Eastern offset. Reading only
    // `venue?.state` would resolve America/Phoenix here.
    const show = makeShowResponse({
      event_date: '2026-01-16T01:00:00Z', // 20:00 America/New_York on Jan 15
      city: 'Brooklyn',
      state: 'NY',
      venues: [],
    })
    const result = showToFormValues(show)

    expect(result.date).toBe('2026-01-15')
    expect(result.time).toBe('20:00')
    // And the venue.state the form submits back with resolves the same zone.
    expect(result.venue.state).toBe('NY')
  })

  it('falls back to the state map when the venue carries no timezone', () => {
    // Unchanged behaviour for the pre-geocoding rows the fallback exists for.
    const show = makeShowResponse({
      event_date: '2026-03-15T03:00:00Z',
      venues: [
        {
          id: 10,
          slug: 'the-venue',
          name: 'The Venue',
          city: 'Phoenix',
          state: 'AZ',
          timezone: null,
          verified: true,
        },
      ],
    })
    const result = showToFormValues(show)

    expect(result.date).toBe('2026-03-14')
    expect(result.time).toBe('20:00')
  })

  it('round-trips a show whose zone resolves to neither venue nor state', () => {
    // PSY-1696, the last rung of the chain: no `venues.timezone` AND a state
    // the US map does not list, so `resolveShowTimezone` answers
    // FALLBACK_SHOW_TIMEZONE. This is the case that makes the fallback a
    // matched write/read PAIR rather than a display default: the submit path
    // composed this instant as 20:00 in that same zone, so reading it back
    // through the same resolver returns exactly what was typed.
    //
    // The expectations are HARDCODED rather than recomputed from the constant.
    // Composing the fixture with `FALLBACK_SHOW_TIMEZONE` and then asserting
    // against a value derived from it too would move both sides together and
    // pass for any zone at all; pinning the literal is what makes a change to
    // the constant surface here as a failure, which is the entire point of the
    // test. Reading this same instant in UTC would give Aug 16 at 03:00.
    const show = makeShowResponse({
      event_date: '2026-08-16T03:00:00Z', // 20:00 Aug 15, America/Phoenix
      city: 'Berlin',
      state: '',
      venues: [
        {
          id: 11,
          slug: 'hall-ohne-zone',
          name: 'Hall Ohne Zone',
          city: 'Berlin',
          state: '',
          timezone: null,
          verified: true,
        },
      ],
    })
    expect(show.event_date).toBe('2026-08-16T03:00:00Z')

    const result = showToFormValues(show)

    expect(result.date).toBe('2026-08-15')
    expect(result.time).toBe('20:00')
  })

  it('returns empty cost when price is null', () => {
    const show = makeShowResponse({ price: null })
    expect(showToFormValues(show).cost).toBe('')
  })

  it('returns empty cost when price is undefined', () => {
    const show = makeShowResponse({ price: undefined })
    expect(showToFormValues(show).cost).toBe('')
  })

  it('returns "$0" when price is 0', () => {
    const show = makeShowResponse({ price: 0 })
    expect(showToFormValues(show).cost).toBe('$0')
  })

  // PSY-1864: door_cost round-trips independently of cost. An unrecorded door
  // price opens the field EMPTY — echoing the advance price back would let a
  // no-op save invent a door price the source never stated.
  it('round-trips the door price into its own field', () => {
    const show = makeShowResponse({ price: 35, door_price: 40 })
    const result = showToFormValues(show)
    expect(result.cost).toBe('$35')
    expect(result.door_cost).toBe('$40')
  })

  it('returns empty door_cost when no door price is recorded', () => {
    expect(showToFormValues(makeShowResponse({ price: 35 })).door_cost).toBe('')
    expect(
      showToFormValues(makeShowResponse({ price: 35, door_price: null })).door_cost
    ).toBe('')
  })

  it('returns "$0" when the door price is 0', () => {
    const show = makeShowResponse({ price: 35, door_price: 0 })
    expect(showToFormValues(show).door_cost).toBe('$0')
  })

  it('round-trips a door price with no advance price', () => {
    const show = makeShowResponse({ price: null, door_price: 40 })
    const result = showToFormValues(show)
    expect(result.cost).toBe('')
    expect(result.door_cost).toBe('$40')
  })

  it('falls back to the show city when the venue has none, but not to its state', () => {
    const show = makeShowResponse({
      city: 'Tucson',
      state: 'AZ',
      venues: [{ id: 1, slug: 's', name: 'V', city: '', state: '', verified: false }],
    })
    const result = showToFormValues(show)

    // City is a display and payload field only, so the show row still fills it.
    expect(result.venue.city).toBe('Tucson')
    // The state is not, because the submit resolves the timezone from it.
    // `showTimingInput` keeps the venue's blank for this row, and the field
    // has to name the same zone the instant was read in.
    expect(result.venue.state).toBe('')
    // The id is what keeps that blank payload resolvable on the backend.
    expect(result.venue.id).toBe(1)
  })

  it('handles empty venues array gracefully', () => {
    const show = makeShowResponse({ venues: [] as VenueResponse[], city: 'Mesa', state: 'AZ' })
    const result = showToFormValues(show)

    expect(result.venue.name).toBe('')
    expect(result.venue.city).toBe('Mesa')
    expect(result.venue.state).toBe('AZ')
    // Nothing to address by id, so the payload describes the venue by
    // name/city/state and the state field is required again.
    expect(result.venue.id).toBeUndefined()
  })

  it('handles null description, age_requirement, title', () => {
    const show = makeShowResponse({
      title: '',
      description: null,
      age_requirement: null,
    })
    const result = showToFormValues(show)

    expect(result.title).toBe('')
    expect(result.description).toBe('')
    expect(result.ages).toBe('')
  })
})

// --- parseCost ---

describe('parseCost', () => {
  it('parses "$20" to 20', () => {
    expect(parseCost('$20')).toBe(20)
  })

  it('parses "$12.50" to 12.5', () => {
    expect(parseCost('$12.50')).toBe(12.5)
  })

  it('parses "15" to 15', () => {
    expect(parseCost('15')).toBe(15)
  })

  it('returns 0 for "Free"', () => {
    expect(parseCost('Free')).toBe(0)
  })

  it('returns 0 for "free" (case-insensitive)', () => {
    expect(parseCost('free')).toBe(0)
  })

  it('returns 0 for "FREE"', () => {
    expect(parseCost('FREE')).toBe(0)
  })

  it('returns 0 for " Free " (with whitespace)', () => {
    expect(parseCost(' Free ')).toBe(0)
  })

  it('returns undefined for empty string', () => {
    expect(parseCost('')).toBeUndefined()
  })

  it('parses "$0" to 0', () => {
    expect(parseCost('$0')).toBe(0)
  })

  it('parses "$5 suggested donation" to 5', () => {
    expect(parseCost('$5 suggested donation')).toBe(5)
  })

  it('parses "$12 adv / $18 day of" to 12 (first price)', () => {
    expect(parseCost('$12 adv / $18 day of')).toBe(12)
  })

  it('parses "$15/$20" to 15 (first price)', () => {
    expect(parseCost('$15/$20')).toBe(15)
  })

  it('parses "$10 - $15" to 10 (first price)', () => {
    expect(parseCost('$10 - $15')).toBe(10)
  })

  it('parses "$ 25" with space after dollar sign', () => {
    expect(parseCost('$ 25')).toBe(25)
  })

  it('returns undefined for text with no numbers', () => {
    expect(parseCost('donation')).toBeUndefined()
  })
})

// --- removeArtistAtIndex ---

describe('removeArtistAtIndex', () => {
  const headliner: FormArtist = { _clientId: 'cid-1', name: 'Head', set_type: 'headliner', matched_id: 1 }
  const opener: FormArtist = { _clientId: 'cid-2', name: 'Opener', set_type: 'opener', matched_id: 2 }
  const support: FormArtist = { _clientId: 'cid-3', name: 'Support', set_type: 'direct_support', matched_id: 3 }

  it('returns null when only one artist remains', () => {
    expect(removeArtistAtIndex([headliner], 0)).toBeNull()
  })

  it('removes the artist at the given index', () => {
    const result = removeArtistAtIndex([headliner, opener, support], 1)!
    expect(result).toHaveLength(2)
    expect(result.map(a => a.name)).toEqual(['Head', 'Support'])
  })

  it('promotes first remaining artist to headliner when headliner is removed', () => {
    const result = removeArtistAtIndex([headliner, opener, support], 0)!
    expect(result[0].set_type).toBe('headliner')
    expect(result[0].name).toBe('Opener')
  })

  it('leaves the other acts\' curated roles alone when promoting', () => {
    // Losing an act says nothing about what slot the survivors played.
    const result = removeArtistAtIndex([headliner, opener, support], 0)!
    expect(result.map(a => a.set_type)).toEqual(['headliner', 'direct_support'])
  })

  it('does not change headliner status when non-headliner is removed', () => {
    const result = removeArtistAtIndex([headliner, opener], 1)!
    expect(result[0].set_type).toBe('headliner')
    expect(result[0].name).toBe('Head')
  })

  it('does not mutate the original array', () => {
    const artists = [headliner, opener]
    removeArtistAtIndex(artists, 1)
    expect(artists).toHaveLength(2)
  })

  it('preserves _clientId on the remaining artists so React keys stay stable', () => {
    // Removing the middle entry must not shift _clientIds onto the wrong
    // rows — that's the underlying invariant that lets the React key stay
    // tied to the same logical row across renders.
    const result = removeArtistAtIndex([headliner, opener, support], 1)!
    expect(result.map(a => a._clientId)).toEqual(['cid-1', 'cid-3'])
  })
})

// --- isVenueLocationEditable ---

describe('isVenueLocationEditable', () => {
  const verifiedVenue = makeVenueResponse({ verified: true })
  const unverifiedVenue = makeVenueResponse({ verified: false })

  it('returns false when prefilled venue exists (regardless of other factors)', () => {
    expect(isVenueLocationEditable(true, null, true)).toBe(false)
    expect(isVenueLocationEditable(false, null, true)).toBe(false)
  })

  it('returns true for admin (even with verified venue)', () => {
    expect(isVenueLocationEditable(true, verifiedVenue, false)).toBe(true)
  })

  it('returns true when no venue is selected', () => {
    expect(isVenueLocationEditable(false, null, false)).toBe(true)
  })

  it('returns true for unverified venue (non-admin)', () => {
    expect(isVenueLocationEditable(false, unverifiedVenue, false)).toBe(true)
  })

  it('returns false for verified venue (non-admin)', () => {
    expect(isVenueLocationEditable(false, verifiedVenue, false)).toBe(false)
  })
})

// --- defaultFormValues ---

describe('defaultFormValues', () => {
  it('has one artist in the headliner slot', () => {
    expect(defaultFormValues.artists).toHaveLength(1)
    expect(defaultFormValues.artists[0].set_type).toBe('headliner')
    expect(defaultFormValues.artists[0].name).toBe('')
  })

  it('default artist has a _clientId for stable React keys', () => {
    expect(defaultFormValues.artists[0]._clientId).toBeTruthy()
  })

  it('has default time of 20:00', () => {
    expect(defaultFormValues.time).toBe('20:00')
  })
})

// --- makeFormArtist ---

describe('makeFormArtist', () => {
  it('mints a unique _clientId on each call', () => {
    const a = makeFormArtist({ name: 'A', set_type: 'headliner' })
    const b = makeFormArtist({ name: 'B', set_type: 'performer' })
    expect(a._clientId).toBeTruthy()
    expect(b._clientId).toBeTruthy()
    expect(a._clientId).not.toBe(b._clientId)
  })

  it('preserves all supplied fields', () => {
    const artist = makeFormArtist({
      name: 'A',
      set_type: 'headliner',
      matched_id: 42,
      instagram_handle: '@a',
    })
    expect(artist.name).toBe('A')
    expect(artist.set_type).toBe('headliner')
    expect(artist.matched_id).toBe(42)
    expect(artist.instagram_handle).toBe('@a')
  })
})

// --- mergeExtraction (PSY-795) ---

describe('mergeExtraction', () => {
  const fullExtraction: ExtractedShowData = {
    artists: [
      { name: 'Headliner', is_headliner: true },
      { name: 'Opener', is_headliner: false, instagram_handle: '@opener' },
    ],
    venue: { name: 'The Venue', city: 'Tempe', state: 'AZ' },
    date: '2099-09-09',
    time: '21:30',
    cost: '$20',
    ages: 'All Ages',
    description: 'flyer text',
  }

  it('returns the base unchanged when extraction is undefined', () => {
    expect(mergeExtraction(defaultFormValues, undefined)).toBe(defaultFormValues)
  })

  // PSY-1864: a flyer that states both prices seeds both fields. The
  // extractor only emits door_cost when the source spelled a separate door
  // price, so an absent door_cost must leave the field empty rather than
  // echoing the advance price into it.
  it('seeds both cost fields when the flyer stated a door price', () => {
    const result = mergeExtraction(defaultFormValues, {
      ...fullExtraction,
      cost: '$20',
      door_cost: '$25',
    })

    expect(result.cost).toBe('$20')
    expect(result.door_cost).toBe('$25')
  })

  it('leaves door_cost empty when the flyer stated only one price', () => {
    const result = mergeExtraction(defaultFormValues, {
      ...fullExtraction,
      cost: '$20',
    })

    expect(result.cost).toBe('$20')
    expect(result.door_cost).toBe('')
  })

  it('prefers the extraction set_type over the headliner flag', () => {
    const result = mergeExtraction(defaultFormValues, {
      ...fullExtraction,
      artists: [
        { name: 'Top', is_headliner: true, set_type: 'headliner' },
        { name: 'Second', is_headliner: false, set_type: 'direct_support' },
        { name: 'Spinner', is_headliner: false, set_type: 'dj' },
      ],
    })

    expect(result.artists.map(a => a.set_type)).toEqual([
      'headliner',
      'direct_support',
      'dj',
    ])
  })

  it('gives non-headliners the neutral default when the flyer stated no slot', () => {
    // The extraction endpoint leaves set_type empty when the source did not
    // say. Filling that in as 'opener' is the guess this ticket removed.
    const result = mergeExtraction(defaultFormValues, fullExtraction)

    expect(result.artists.map(a => a.set_type)).toEqual([
      'headliner',
      'performer',
    ])
  })

  it('folds every extracted field into the form values', () => {
    const result = mergeExtraction(defaultFormValues, fullExtraction)

    expect(result.artists).toHaveLength(2)
    expect(result.artists[0].name).toBe('Headliner')
    expect(result.artists[0].set_type).toBe('headliner')
    expect(result.artists[1].name).toBe('Opener')
    expect(result.artists[1].instagram_handle).toBe('@opener')
    expect(result.venue.name).toBe('The Venue')
    expect(result.venue.city).toBe('Tempe')
    expect(result.venue.state).toBe('AZ')
    expect(result.date).toBe('2099-09-09')
    expect(result.time).toBe('21:30')
    expect(result.cost).toBe('$20')
    expect(result.ages).toBe('All Ages')
    expect(result.description).toBe('flyer text')
  })

  it('prefers the matched_name over the raw extracted name for artists and venue', () => {
    const result = mergeExtraction(defaultFormValues, {
      artists: [
        { name: 'mountain goats', is_headliner: true, matched_id: 7, matched_name: 'The Mountain Goats' },
      ],
      venue: { name: 'valley bar', matched_id: 3, matched_name: 'Valley Bar', matched_slug: 'valley-bar' },
    })

    expect(result.artists[0].name).toBe('The Mountain Goats')
    expect(result.artists[0].matched_id).toBe(7)
    expect(result.venue.name).toBe('Valley Bar')
    expect(result.venue.id).toBe(3)
  })

  it('drops the instagram handle for a matched artist', () => {
    const result = mergeExtraction(defaultFormValues, {
      artists: [
        { name: 'Matched', is_headliner: true, matched_id: 9, instagram_handle: '@matched' },
      ],
    })
    expect(result.artists[0].instagram_handle).toBeUndefined()
  })

  it('keeps base values for fields the sparse extraction omits', () => {
    const result = mergeExtraction(defaultFormValues, {
      artists: [{ name: 'Only Artist', is_headliner: true }],
    })

    // Only artists were extracted; everything else keeps the defaults.
    expect(result.artists[0].name).toBe('Only Artist')
    expect(result.venue).toEqual(defaultFormValues.venue)
    expect(result.time).toBe('20:00') // default time survives
    expect(result.date).toBe('')
    expect(result.cost).toBe('')
  })

  it('keeps the default single artist when the extraction has no artists', () => {
    const result = mergeExtraction(defaultFormValues, {
      artists: [],
      date: '2099-01-01',
    })
    expect(result.artists).toBe(defaultFormValues.artists)
    expect(result.date).toBe('2099-01-01')
  })

  it('does not mutate the base form values', () => {
    const base = { ...defaultFormValues, venue: { ...defaultFormValues.venue } }
    mergeExtraction(base, fullExtraction)
    expect(base.venue.name).toBe('')
    expect(base.date).toBe('')
    expect(base.artists).toBe(defaultFormValues.artists)
  })
})

// --- extractedVenueToSelected (PSY-795) ---

describe('extractedVenueToSelected', () => {
  it('returns null when extraction is undefined', () => {
    expect(extractedVenueToSelected(undefined)).toBeNull()
  })

  it('returns null when the venue did not match an existing entity', () => {
    expect(
      extractedVenueToSelected({
        artists: [],
        venue: { name: 'Unmatched Spot', city: 'Mesa', state: 'AZ' },
      })
    ).toBeNull()
  })

  it('returns a verified VenueResponse when the venue matched (id + name + slug)', () => {
    const result = extractedVenueToSelected({
      artists: [],
      venue: {
        name: 'valley bar',
        city: 'Phoenix',
        state: 'AZ',
        matched_id: 5,
        matched_name: 'Valley Bar',
        matched_slug: 'valley-bar',
      },
    })

    expect(result).toEqual({
      id: 5,
      slug: 'valley-bar',
      name: 'Valley Bar',
      address: null,
      city: 'Phoenix',
      state: 'AZ',
      verified: true,
    })
  })

  it('returns null when a match id is present but slug is missing', () => {
    expect(
      extractedVenueToSelected({
        artists: [],
        venue: { name: 'Partial', matched_id: 5, matched_name: 'Partial' },
      })
    ).toBeNull()
  })
})


// --- set_type vocabulary (PSY-1673) ---

describe('SET_TYPE_OPTIONS', () => {
  it('matches the backend vocabulary, in order', () => {
    // Mirrors contracts.SetTypeVocabulary() in
    // backend/internal/services/contracts/set_type.go, which the OpenAPI enum
    // on the show create/update body publishes into types/api.d.ts.
    //
    // MEMBERSHIP is already guaranteed at COMPILE time: SetType is derived
    // from that generated enum, SET_TYPE_LABELS is an exhaustive
    // Record<SetType, string>, and an exhaustiveness assertion pins this list
    // against the union. What this test adds is ORDER, which a union cannot
    // express -- it is the presentation order of the selector.
    expect(SET_TYPE_VALUES).toEqual([
      'headliner',
      'direct_support',
      'opener',
      'special_guest',
      'dj',
      'performer',
    ])
  })

  it('labels every value the vocabulary contains', () => {
    // Cheap runtime echo of the compile-time Record<SetType, string> guard,
    // so a reader of the test file sees the invariant stated.
    expect(SET_TYPE_OPTIONS.map(option => option.value)).toEqual([
      ...SET_TYPE_VALUES,
    ])
  })

  it('offers a label for every value', () => {
    for (const option of SET_TYPE_OPTIONS) {
      expect(option.label.trim().length).toBeGreaterThan(0)
    }
  })

  it('defaults to the neutral value, which stays in the vocabulary', () => {
    expect(DEFAULT_SET_TYPE).toBe('performer')
    expect(SET_TYPE_VALUES).toContain(DEFAULT_SET_TYPE)
  })
})

describe('toSetType', () => {
  it('passes every vocabulary value through', () => {
    for (const value of SET_TYPE_VALUES) {
      expect(toSetType(value)).toBe(value)
    }
  })

  it('falls back to the neutral default for anything else', () => {
    expect(toSetType('co_headliner')).toBe(DEFAULT_SET_TYPE)
    expect(toSetType('')).toBe(DEFAULT_SET_TYPE)
    expect(toSetType(undefined)).toBe(DEFAULT_SET_TYPE)
    expect(toSetType(null)).toBe(DEFAULT_SET_TYPE)
    // Strict, matching the server: casing is not coerced.
    expect(toSetType('Headliner')).toBe(DEFAULT_SET_TYPE)
  })
})

// --- toArtistPayloads (PSY-1673) ---

describe('toArtistPayloads', () => {
  it('sends every vocabulary value verbatim', () => {
    const artists: FormArtist[] = SET_TYPE_VALUES.map((value, index) => ({
      _clientId: `cid-${index}`,
      name: `Artist ${value}`,
      set_type: value,
    }))

    expect(toArtistPayloads(artists).map(a => a.set_type)).toEqual([
      ...SET_TYPE_VALUES,
    ])
  })

  it('derives is_headliner from set_type rather than tracking it separately', () => {
    const artists: FormArtist[] = SET_TYPE_VALUES.map((value, index) => ({
      _clientId: `cid-${index}`,
      name: `Artist ${value}`,
      set_type: value,
    }))

    expect(toArtistPayloads(artists).map(a => a.is_headliner)).toEqual([
      true,
      false,
      false,
      false,
      false,
      false,
    ])
  })

  it('carries the matched id and drops instagram for matched artists', () => {
    const payloads = toArtistPayloads([
      {
        _clientId: 'cid-1',
        name: 'Matched',
        set_type: 'headliner',
        matched_id: 7,
        instagram_handle: '@matched',
      },
      {
        _clientId: 'cid-2',
        name: 'New Act',
        set_type: 'direct_support',
        instagram_handle: '@newact',
      },
    ])

    expect(payloads[0]).toEqual({
      id: 7,
      name: 'Matched',
      is_headliner: true,
      set_type: 'headliner',
      instagram_handle: undefined,
    })
    expect(payloads[1]).toEqual({
      id: undefined,
      name: 'New Act',
      is_headliner: false,
      set_type: 'direct_support',
      instagram_handle: '@newact',
    })
  })
})


// --- resolveFormSetType (PSY-1673) ---

describe('resolveFormSetType', () => {
  it('prefers a curated set_type over the legacy flag', () => {
    expect(
      resolveFormSetType({ set_type: 'opener', is_headliner: true })
    ).toBe('opener')
  })

  it('falls back to the headliner flag when no slot was stated', () => {
    expect(resolveFormSetType({ is_headliner: true })).toBe('headliner')
    expect(resolveFormSetType({ set_type: '', is_headliner: true })).toBe(
      'headliner'
    )
  })

  it('resolves to the neutral default with no signal at all', () => {
    // Never 'opener': absence of a stated slot is not evidence of one.
    expect(resolveFormSetType({})).toBe(DEFAULT_SET_TYPE)
    expect(resolveFormSetType({ is_headliner: false })).toBe(DEFAULT_SET_TYPE)
    expect(resolveFormSetType({ set_type: null, is_headliner: null })).toBe(
      DEFAULT_SET_TYPE
    )
  })

  it('coerces an unrecognized set_type rather than passing it through', () => {
    expect(
      resolveFormSetType({ set_type: 'co_headliner', is_headliner: true })
    ).toBe(DEFAULT_SET_TYPE)
  })
})
