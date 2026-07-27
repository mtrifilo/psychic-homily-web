import { describe, expect, it } from 'vitest'
import type { ArtistGraphCard } from '@/features/artists/types'
import type { VenueShow } from '@/features/venues/types'
import {
  artistConnectionsLine,
  artistIdentityLine,
  artistStepAnnouncement,
  artistStepKicker,
  buildArtistSteps,
  clampStepIndex,
  firstStepIndexForShow,
} from './artistDrillIn'

function artist(id: number, name: string) {
  return {
    id,
    name,
    slug: name.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
  } as VenueShow['artists'][number]
}

function show(id: number, artists: VenueShow['artists']): VenueShow {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: '2026-08-01T02:00:00Z',
    city: 'Austin',
    state: 'TX',
    price: null,
    age_requirement: null,
    artists,
  } as VenueShow
}

function card(overrides: Partial<ArtistGraphCard> = {}): ArtistGraphCard {
  return {
    id: 1,
    name: 'Die Spitz',
    slug: 'die-spitz',
    city: 'Austin',
    state: 'TX',
    bandcamp_embed_url: null,
    spotify: null,
    next_show: null,
    labels: [],
    radio: null,
    connections: {
      bills: 0,
      similar: 0,
      members: 0,
      radio: 0,
      shared_labels: 0,
    },
    ...overrides,
  } as ArtistGraphCard
}

describe('buildArtistSteps', () => {
  it('flattens the list in show order, then bill order', () => {
    const steps = buildArtistSteps([
      show(1, [artist(10, 'Die Spitz'), artist(11, 'Farmer’s Wife')]),
      show(2, [artist(12, 'Gouge Away')]),
    ])
    expect(steps.map((s) => s.artistName)).toEqual([
      'Die Spitz',
      'Farmer’s Wife',
      'Gouge Away',
    ])
    expect(steps.map((s) => s.showId)).toEqual([1, 1, 2])
  })

  // A band playing the venue twice this week is ONE entry, at its soonest
  // date — otherwise the stepper revisits it mid-walk and "of N" double-counts.
  it('de-duplicates by artist id, keeping the first occurrence', () => {
    const steps = buildArtistSteps([
      show(1, [artist(10, 'Die Spitz')]),
      show(2, [artist(11, 'Gouge Away'), artist(10, 'Die Spitz')]),
    ])
    expect(steps).toHaveLength(2)
    expect(steps[0]).toMatchObject({ artistId: 10, showId: 1 })
    expect(steps[1]).toMatchObject({ artistId: 11, showId: 2 })
  })

  // `/atlas` has no route-level error boundary, so a TypeError here takes down
  // the whole app shell rather than just the panel.
  it('survives absent shows, absent bills and null entries', () => {
    expect(buildArtistSteps(null)).toEqual([])
    expect(buildArtistSteps(undefined)).toEqual([])
    expect(
      buildArtistSteps([
        { ...show(1, []), artists: undefined } as unknown as VenueShow,
        show(2, [null as unknown as VenueShow['artists'][number]]),
      ]),
    ).toEqual([])
  })

  // Every fetch in the panel keys on the artist id, so an id-less entry would
  // be a step onto a permanently loading card.
  it('drops artists without a usable id', () => {
    const steps = buildArtistSteps([
      show(1, [
        { id: 0, name: 'No Id', slug: '' } as VenueShow['artists'][number],
        artist(10, 'Die Spitz'),
      ]),
    ])
    expect(steps.map((s) => s.artistId)).toEqual([10])
  })

  it('tolerates a missing slug without dropping the artist', () => {
    const steps = buildArtistSteps([
      show(1, [
        { id: 10, name: 'Die Spitz' } as VenueShow['artists'][number],
      ]),
    ])
    expect(steps[0]).toMatchObject({ artistId: 10, artistSlug: '' })
  })
})

describe('firstStepIndexForShow', () => {
  const showOne = show(1, [artist(10, 'Die Spitz'), artist(11, 'Farmer’s Wife')])
  const showTwo = show(2, [artist(12, 'Gouge Away')])
  const steps = buildArtistSteps([showOne, showTwo])

  it('starts on the clicked show’s first artist', () => {
    expect(firstStepIndexForShow(steps, showOne)).toBe(0)
    expect(firstStepIndexForShow(steps, showTwo)).toBe(2)
  })

  // The case a showId match gets WRONG: de-duplication attributes a repeat
  // band to its first date, so matching on showId would skip past it on the
  // second date's row and land the user below the bill they just read.
  it('lands on the top of a repeat band’s second-night bill', () => {
    const nightOne = show(1, [artist(10, 'Die Spitz'), artist(11, 'Meat Wave')])
    const nightTwo = show(2, [artist(11, 'Meat Wave'), artist(12, 'Gouge Away')])
    const both = buildArtistSteps([nightOne, nightTwo])
    expect(both.map((s) => s.artistName)).toEqual([
      'Die Spitz',
      'Meat Wave',
      'Gouge Away',
    ])
    expect(firstStepIndexForShow(both, nightTwo)).toBe(1)
  })

  // -1, never 0: opening on a DIFFERENT show's headliner because the clicked
  // one had no steppable bill is worse than not opening at all.
  it('reports -1 when the show contributed nothing steppable', () => {
    expect(firstStepIndexForShow(steps, show(999, []))).toBe(-1)
    expect(firstStepIndexForShow(steps, null)).toBe(-1)
    expect(
      firstStepIndexForShow(steps, {
        artists: undefined,
      } as unknown as VenueShow),
    ).toBe(-1)
  })
})

describe('clampStepIndex', () => {
  const steps = buildArtistSteps([
    show(1, [artist(10, 'A'), artist(11, 'B'), artist(12, 'C')]),
  ])

  it('clamps out-of-range and non-finite indices into the list', () => {
    expect(clampStepIndex(steps, -5)).toBe(0)
    expect(clampStepIndex(steps, 99)).toBe(2)
    expect(clampStepIndex(steps, Number.NaN)).toBe(0)
    expect(clampStepIndex(steps, 1)).toBe(1)
    expect(clampStepIndex([], 3)).toBe(0)
  })
})

describe('artistStepKicker', () => {
  it('states the position and the scope it belongs to', () => {
    expect(
      artistStepKicker({ index: 1, total: 5, scopeLabel: 'upcoming at Hotel Vegas' }),
    ).toBe('ARTIST · 2 OF 5 UPCOMING AT HOTEL VEGAS')
  })

  // The scope is whatever list you drilled in from — a citywide surface must
  // be able to say so without the panel knowing what a venue is.
  it('carries a non-venue scope verbatim', () => {
    expect(
      artistStepKicker({ index: 0, total: 12, scopeLabel: 'in Austin' }),
    ).toBe('ARTIST · 1 OF 12 IN AUSTIN')
  })

  it('drops the position for a single-entry list', () => {
    expect(
      artistStepKicker({ index: 0, total: 1, scopeLabel: 'upcoming at Mohawk' }),
    ).toBe('ARTIST')
  })

  it('omits the scope segment when there is no label', () => {
    expect(artistStepKicker({ index: 0, total: 3, scopeLabel: '  ' })).toBe(
      'ARTIST · 1 OF 3',
    )
  })
})

describe('artistStepAnnouncement', () => {
  // The visible `‹ ›` glyphs imply the position visually and stepping does not
  // move focus, so this sentence is the ONLY thing that tells a screen-reader
  // user what changed and who they landed on.
  it('names the artist and the position as a sentence', () => {
    expect(
      artistStepAnnouncement({
        index: 1,
        total: 5,
        artistName: 'Die Spitz',
        scopeLabel: 'upcoming at Hotel Vegas',
      }),
    ).toBe('Die Spitz, artist 2 of 5 upcoming at Hotel Vegas')
  })

  it('announces just the name when there is nothing to step through', () => {
    expect(
      artistStepAnnouncement({
        index: 0,
        total: 1,
        artistName: 'Die Spitz',
        scopeLabel: 'upcoming at Hotel Vegas',
      }),
    ).toBe('Die Spitz')
  })
})

describe('artistConnectionsLine', () => {
  it('builds the mock’s line from bills, similar, labels and radio', () => {
    expect(
      artistConnectionsLine(
        card({
          connections: {
            bills: 14,
            similar: 6,
            members: 0,
            radio: 3,
            shared_labels: 1,
          },
          labels: [{ name: 'Sub Pop', slug: 'sub-pop' }],
          radio: { stations: ['WFMU'], play_count: 3 },
        }),
      ),
    ).toBe('14 bills · 6 similar artists · Sub Pop · plays on WFMU')
  })

  it('singularizes counts of one', () => {
    expect(
      artistConnectionsLine(
        card({
          connections: {
            bills: 1,
            similar: 1,
            members: 1,
            radio: 0,
            shared_labels: 0,
          },
        }),
      ),
    ).toBe('1 bill · 1 similar artist · 1 member')
  })

  it('caps labels and stations so the line stays one row', () => {
    expect(
      artistConnectionsLine(
        card({
          labels: [
            { name: 'A', slug: 'a' },
            { name: 'B', slug: 'b' },
            { name: 'C', slug: 'c' },
          ],
          radio: { stations: ['WFMU', 'KEXP', 'NTS'], play_count: 9 },
        }),
      ),
    ).toBe('A · B · plays on WFMU & KEXP')
  })

  // Empty means "omit the heading entirely", not "render an empty row".
  it('returns empty when the artist has no connections at all', () => {
    expect(artistConnectionsLine(card())).toBe('')
  })

  it('survives a card served without labels, radio or connections', () => {
    const bare = {
      ...card(),
      labels: undefined,
      radio: undefined,
      connections: undefined,
    } as unknown as ArtistGraphCard
    expect(artistConnectionsLine(bare)).toBe('')
  })
})

describe('artistIdentityLine', () => {
  it('joins city and state', () => {
    expect(artistIdentityLine({ city: 'Austin', state: 'TX' })).toBe('Austin, TX')
  })

  it('drops the missing half rather than leaving a dangling comma', () => {
    expect(artistIdentityLine({ city: null, state: 'TX' })).toBe('TX')
    expect(artistIdentityLine({ city: 'Austin', state: null })).toBe('Austin')
    expect(artistIdentityLine({ city: null, state: null })).toBe('')
  })
})
