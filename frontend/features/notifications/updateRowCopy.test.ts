import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { NOTIFICATION_ENTITY_ARTIST_RELEASE_DIGEST } from './types'
import { LIBRARY_UPDATE_ENTITY_TYPES } from './libraryUpdates'
import {
  describeLibraryUpdate as describeAt,
  type UpdateRowEntry,
} from './updateRowCopy'

const NOW = new Date('2026-11-03T18:00:00Z')

/** Measured from the read instant, as the hook does. */
const describeLibraryUpdate = (row: UpdateRowEntry) => describeAt(row, NOW)

function entry(overrides: Partial<UpdateRowEntry>): UpdateRowEntry {
  return {
    id: 1,
    entity_type: 'artist_show_alert',
    entity_id: 10,
    channel: 'in_app',
    sent_at: '2026-11-01T18:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(NOW)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('describeLibraryUpdate: artist show alert', () => {
  it('names the artist and the venue, then the show day and the announcement', () => {
    const copy = describeLibraryUpdate(
      entry({
        alert_artist_name: 'Dan Deacon',
        alert_show_venue_name: 'Walter Studios',
        // 20:00 in Phoenix on Fri Nov 20 is 03:00 UTC on Sat Nov 21: the day
        // must be read on the venue's clock.
        alert_show_event_date: '2026-11-21T03:00:00Z',
        alert_show_timezone: 'America/Phoenix',
        alert_show_url: '/shows/dan-deacon-walter-studios',
      })
    )

    expect(copy).toEqual({
      label: 'New show',
      title: 'Dan Deacon · Walter Studios',
      detail: 'Fri Nov 20 · announced 2d ago',
      href: '/shows/dan-deacon-walter-studios',
    })
  })

  it('degrades like the inbox when enrichment came back empty', () => {
    const copy = describeLibraryUpdate(entry({}))

    expect(copy).toEqual({
      label: 'New show',
      title: 'An artist you follow',
      detail: 'announced 2d ago',
      href: '/shows',
    })
  })

  it('falls back to the show title when the show has no venue', () => {
    const copy = describeLibraryUpdate(
      entry({ alert_artist_name: 'Oneida', alert_show_title: 'Oneida (secret set)' })
    )

    expect(copy?.title).toBe('Oneida · Oneida (secret set)')
  })
})

describe('describeLibraryUpdate: venue show alert', () => {
  const venue = (overrides: Partial<UpdateRowEntry>) =>
    entry({
      entity_type: 'venue_show_alert',
      sent_at: '2026-10-30T18:00:00Z',
      alert_venue_name: 'Walter Studios',
      alert_venue_url: '/venues/walter-studios',
      alert_show_timezone: 'America/Phoenix',
      ...overrides,
    })

  it('counts the batch and spans its dates on the venue clock', () => {
    const copy = describeLibraryUpdate(
      venue({
        alert_show_count: 3,
        alert_show_first_date: '2026-11-15T03:00:00Z',
        alert_show_last_date: '2026-11-23T03:00:00Z',
      })
    )

    expect(copy).toEqual({
      label: '3 shows',
      title: 'Walter Studios · new shows',
      detail: 'Nov 14 to Nov 22 · 4d ago',
      href: '/venues/walter-studios',
    })
  })

  it('names one day, and one show, when the batch holds one', () => {
    const copy = describeLibraryUpdate(
      venue({
        alert_show_count: 1,
        alert_show_first_date: '2026-11-15T03:00:00Z',
        alert_show_last_date: '2026-11-15T03:00:00Z',
      })
    )

    expect(copy?.label).toBe('1 show')
    expect(copy?.title).toBe('Walter Studios · new show')
    expect(copy?.detail).toBe('Nov 14 · 4d ago')
  })

  it('states no count when every member has been pulled', () => {
    const copy = describeLibraryUpdate(venue({ alert_venue_name: undefined, alert_venue_url: undefined }))

    expect(copy).toEqual({
      label: 'New shows',
      title: 'A venue you follow · new shows',
      detail: '4d ago',
      href: '/venues',
    })
  })
})

describe('describeLibraryUpdate: weekly release digest', () => {
  const digest = (overrides: Partial<UpdateRowEntry>) =>
    entry({
      entity_type: NOTIFICATION_ENTITY_ARTIST_RELEASE_DIGEST,
      // The reader's own user id: never an artist or release id.
      entity_id: 42,
      sent_at: '2026-11-02T18:00:00Z',
      ...overrides,
    })

  it('counts the week and names its artists', () => {
    const copy = describeLibraryUpdate(
      digest({
        alert_release_count: 3,
        alert_release_artist_count: 3,
        alert_release_artists: 'Dan Deacon, Primitive Man, Russian Circles',
      })
    )

    expect(copy).toEqual({
      label: '3 releases',
      title: 'Dan Deacon, Primitive Man, Russian Circles',
      detail: 'added releases last week · 1d ago',
      href: '/library',
    })
  })

  it('links a one-release week to that release, without guessing its format', () => {
    const copy = describeLibraryUpdate(
      digest({
        alert_release_count: 1,
        alert_release_artist_count: 1,
        alert_release_artists: 'Russian Circles',
        alert_release_url: '/releases/gnosis',
      })
    )

    expect(copy).toEqual({
      label: 'Release',
      title: 'Russian Circles',
      detail: 'added a release last week · 1d ago',
      href: '/releases/gnosis',
    })
  })
})

// The server render has no window, so a same-origin check would leave the
// absolute URL on the server and strip it on the client: a hydration mismatch.
describe('describeLibraryUpdate: destinations', () => {
  it.each([
    'https://www.psychichomily.com/shows/dan-deacon?ref=alert#top',
    'https://psychichomily.com/shows/dan-deacon?ref=alert#top',
    'http://localhost:3000/shows/dan-deacon?ref=alert#top',
  ])('links %s by its app path, whatever origin is serving', url => {
    expect(describeLibraryUpdate(entry({ alert_show_url: url }))?.href).toBe(
      '/shows/dan-deacon?ref=alert#top'
    )
  })
})

// The server render and the hydrating client run in different zones and at
// different instants; given the same read instant they must print the same text.
describe('describeLibraryUpdate: hydration-stable', () => {
  it('measures recency from the read instant, not the clock', () => {
    vi.setSystemTime(new Date('2026-11-03T18:59:59Z'))
    const copy = describeAt(entry({ sent_at: '2026-11-03T17:59:30Z' }), NOW)
    expect(copy?.detail).toBe('announced just now')
  })

  it("prints an old row's date on the row's zone, not the runtime's", () => {
    // 03:00 UTC on Sep 20 is still Sep 19 in Phoenix.
    const copy = describeAt(
      entry({ sent_at: '2026-09-20T03:00:00Z', alert_show_timezone: 'America/Phoenix' }),
      NOW
    )
    expect(copy?.detail).toBe('announced Sep 19')
  })
})

describe('describeLibraryUpdate: the kinds the module asks for', () => {
  it.each(LIBRARY_UPDATE_ENTITY_TYPES)('renders every requested kind (%s)', entityType => {
    expect(describeLibraryUpdate(entry({ entity_type: entityType }))).not.toBeNull()
  })
})

describe('describeLibraryUpdate: other kinds', () => {
  it.each(['comment_reply', 'show', 'request_fulfillment_proposed', 'a_kind_from_the_future'])(
    'skips %s',
    entityType => {
      expect(describeLibraryUpdate(entry({ entity_type: entityType }))).toBeNull()
    }
  )
})
