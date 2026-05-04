import { describe, it, expect } from 'vitest'
import { dedupArtistShows, dedupVenueShows } from './utils'

// =============================================================================
// PSY-559: dedup helpers
// =============================================================================

describe('dedupArtistShows', () => {
  it('collapses two shows sharing (venue.id, event_date) to lowest id', () => {
    const eventDate = '2026-09-16T02:30:00Z'
    const shows = [
      {
        id: 64,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 10, is_headliner: true }],
      },
      {
        id: 786,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 10, is_headliner: true }],
      },
    ]
    const result = dedupArtistShows(shows)
    expect(result.map(s => s.id)).toEqual([64])
  })

  it('preserves matinee + evening at the same venue on the same day', () => {
    const matinee = '2026-05-17T20:00:00Z' // 1pm AZ
    const evening = '2026-05-18T03:00:00Z' // 8pm AZ
    const shows = [
      {
        id: 100,
        event_date: matinee,
        venue: { id: 7 },
        artists: [{ id: 22, is_headliner: true }],
      },
      {
        id: 101,
        event_date: evening,
        venue: { id: 7 },
        artists: [{ id: 22, is_headliner: true }],
      },
    ]
    const result = dedupArtistShows(shows)
    expect(result.map(s => s.id)).toEqual([100, 101])
  })

  it('does not collapse same artist on the same date at different venues', () => {
    const eventDate = '2026-04-11T02:30:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 99, is_headliner: true }],
      },
      {
        id: 2,
        event_date: eventDate,
        venue: { id: 2 },
        artists: [{ id: 99, is_headliner: true }],
      },
    ]
    expect(dedupArtistShows(shows)).toHaveLength(2)
  })

  it('preserves API ordering when no duplicates', () => {
    const shows = [
      {
        id: 5,
        event_date: '2026-01-01T00:00:00Z',
        venue: { id: 1 },
        artists: [{ id: 1, is_headliner: true }],
      },
      {
        id: 3,
        event_date: '2026-02-01T00:00:00Z',
        venue: { id: 1 },
        artists: [{ id: 1, is_headliner: true }],
      },
    ]
    expect(dedupArtistShows(shows).map(s => s.id)).toEqual([5, 3])
  })

  it('treats missing venue as venue id 0 (still dedupes by event_date)', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      { id: 1, event_date: eventDate, artists: [{ id: 1, is_headliner: true }] },
      { id: 2, event_date: eventDate, artists: [{ id: 1, is_headliner: true }] },
    ]
    expect(dedupArtistShows(shows).map(s => s.id)).toEqual([1])
  })
})

describe('dedupVenueShows', () => {
  it('collapses two shows sharing (headliner_artist_id, event_date) to lowest id', () => {
    const eventDate = '2026-09-16T02:30:00Z'
    const shows = [
      {
        id: 64,
        event_date: eventDate,
        artists: [
          { id: 10, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
      {
        id: 786,
        event_date: eventDate,
        artists: [
          { id: 10, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
    ]
    expect(dedupVenueShows(shows).map(s => s.id)).toEqual([64])
  })

  it('preserves matinee + evening when artist is the same', () => {
    const matinee = '2026-05-17T20:00:00Z'
    const evening = '2026-05-18T03:00:00Z'
    const artists = [
      { id: 22, is_headliner: true, position: 0, set_type: 'headliner' },
    ]
    const shows = [
      { id: 100, event_date: matinee, artists },
      { id: 101, event_date: evening, artists },
    ]
    expect(dedupVenueShows(shows)).toHaveLength(2)
  })

  it('does not collapse different headliners on the same date', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        artists: [
          { id: 1, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
      {
        id: 2,
        event_date: eventDate,
        artists: [
          { id: 2, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
    ]
    expect(dedupVenueShows(shows)).toHaveLength(2)
  })

  it('falls back to position 0 when set_type is unset', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        artists: [{ id: 5, position: 0, set_type: 'performer' }],
      },
      {
        id: 2,
        event_date: eventDate,
        artists: [{ id: 5, position: 0, set_type: 'performer' }],
      },
    ]
    expect(dedupVenueShows(shows).map(s => s.id)).toEqual([1])
  })
})
