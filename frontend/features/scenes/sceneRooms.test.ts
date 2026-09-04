import { describe, it, expect } from 'vitest'
import {
  bookedRoomCount,
  defaultRoomOrder,
  orderRooms,
  roomLocationLabel,
  roomWebsite,
} from './sceneRooms'
import type { SceneVenue } from './types'

function room(overrides: Partial<SceneVenue> = {}): SceneVenue {
  return {
    id: 1,
    name: 'Crescent Ballroom',
    slug: 'crescent-ballroom',
    website: 'https://crescentphx.com',
    city: 'Phoenix',
    state: 'AZ',
    upcoming_show_count: 0,
    ...overrides,
  }
}

describe('defaultRoomOrder', () => {
  // The two locked frames, derived from the data alone.
  it('ranks a scene whose counts can order it (the Phoenix frame)', () => {
    const rooms = [
      room({ id: 1, name: 'Crescent Ballroom', upcoming_show_count: 57 }),
      room({ id: 2, name: 'The Van Buren', upcoming_show_count: 41 }),
      room({ id: 3, name: 'Trunk Space', upcoming_show_count: 17 }),
    ]
    expect(defaultRoomOrder(rooms)).toBe('ranked')
  })

  // Portland: one upcoming show in the whole scene. A leaderboard of 1, 0, 0
  // orders nothing and implies the two zeroes lost a contest.
  it('falls to alphabetical when only one room has anything booked', () => {
    const rooms = [
      room({ id: 1, name: 'Mississippi Studios', upcoming_show_count: 1 }),
      room({ id: 2, name: 'Doug Fir Lounge', upcoming_show_count: 0 }),
      room({ id: 3, name: 'Turn! Turn! Turn!', upcoming_show_count: 0 }),
    ]
    expect(defaultRoomOrder(rooms)).toBe('alphabetical')
    expect(bookedRoomCount(rooms)).toBe(1)
  })

  it('falls to alphabetical when nothing is booked anywhere', () => {
    expect(defaultRoomOrder([room({ id: 1 }), room({ id: 2 })])).toBe('alphabetical')
  })

  // A busy scene concentrated in ONE room still reads alphabetically: the
  // question the count answers is "which room first", and a single non-zero
  // room answers it without a ranking.
  it('ignores VOLUME and reads the spread across rooms', () => {
    const rooms = [
      room({ id: 1, name: 'Big Room', upcoming_show_count: 300 }),
      room({ id: 2, name: 'Quiet Room', upcoming_show_count: 0 }),
    ]
    expect(defaultRoomOrder(rooms)).toBe('alphabetical')
  })

  it('treats an empty scene as alphabetical rather than throwing', () => {
    expect(defaultRoomOrder([])).toBe('alphabetical')
  })
})

describe('orderRooms', () => {
  it('ranks by count, then name, then id', () => {
    const rooms = [
      room({ id: 9, name: 'Beta', upcoming_show_count: 3 }),
      room({ id: 4, name: 'Alpha', upcoming_show_count: 3 }),
      room({ id: 7, name: 'Gamma', upcoming_show_count: 12 }),
    ]
    expect(orderRooms(rooms, 'ranked').map(r => r.name)).toEqual([
      'Gamma',
      'Alpha',
      'Beta',
    ])
  })

  // Venue names are unique only WITHIN a city and a metro spans several, so a
  // name tie is a real state and the order still has to be total.
  it('breaks a same-name tie on id so the order is stable', () => {
    const rooms = [
      room({ id: 8, name: 'The Annex', upcoming_show_count: 2 }),
      room({ id: 3, name: 'The Annex', upcoming_show_count: 2 }),
    ]
    expect(orderRooms(rooms, 'ranked').map(r => r.id)).toEqual([3, 8])
    expect(orderRooms(rooms, 'alphabetical').map(r => r.id)).toEqual([3, 8])
  })

  it('sorts alphabetically regardless of count', () => {
    const rooms = [
      room({ id: 1, name: 'Zebra Lounge', upcoming_show_count: 99 }),
      room({ id: 2, name: 'Apple Bar', upcoming_show_count: 0 }),
    ]
    expect(orderRooms(rooms, 'alphabetical').map(r => r.name)).toEqual([
      'Apple Bar',
      'Zebra Lounge',
    ])
  })

  it('does not mutate the caller array', () => {
    const rooms = [
      room({ id: 1, name: 'Zebra Lounge' }),
      room({ id: 2, name: 'Apple Bar' }),
    ]
    orderRooms(rooms, 'alphabetical')
    expect(rooms.map(r => r.name)).toEqual(['Zebra Lounge', 'Apple Bar'])
  })
})

describe('roomLocationLabel', () => {
  it('names the room own city, not the scene principal city', () => {
    expect(roomLocationLabel(room({ city: 'Tempe', state: 'AZ' }), 'AZ')).toBe('Tempe')
  })

  // A Philadelphia metro reaches into Camden NJ; a bare "(Camden)" there reads
  // as a Pennsylvania neighbourhood.
  it('adds the state when the room is in a different one', () => {
    expect(roomLocationLabel(room({ city: 'Camden', state: 'NJ' }), 'PA')).toBe(
      'Camden, NJ'
    )
  })

  // venues.city is NOT NULL but '' is still allowed, so a blank must not
  // render as an empty pair of parens.
  it('renders nothing for a blank city', () => {
    expect(roomLocationLabel(room({ city: '   ' }), 'AZ')).toBeNull()
  })
})

// The nullable-slug href guard lives in `EntityNameLink` (sceneChrome.tsx) and
// is covered by sceneChrome.test.tsx plus each section's own suite.

describe('roomWebsite', () => {
  it('accepts http and https', () => {
    expect(roomWebsite(room({ website: 'https://valleybarphx.com' }))).toBe(
      'https://valleybarphx.com'
    )
    expect(roomWebsite(room({ website: 'http://valleybarphx.com' }))).toBe(
      'http://valleybarphx.com'
    )
  })

  // The column is operator-entered, and this is the only place on the page
  // that turns a stored string into a navigable target.
  it('rejects a scheme that would execute in the reader session', () => {
    expect(roomWebsite(room({ website: 'javascript:alert(1)' }))).toBeNull()
    expect(roomWebsite(room({ website: 'data:text/html,<script>' }))).toBeNull()
  })

  it('returns null for absent, blank or unparseable values', () => {
    expect(roomWebsite(room({ website: undefined }))).toBeNull()
    expect(roomWebsite(room({ website: '  ' }))).toBeNull()
    expect(roomWebsite(room({ website: 'not a url' }))).toBeNull()
  })

  // The same column the venue page reads, resolved the same way, so one
  // stored value cannot link on one surface and not the other.
  it('resolves a legacy scheme-less value, as the venue page does', () => {
    expect(roomWebsite(room({ website: 'valleybarphx.com' }))).toBe(
      'https://valleybarphx.com'
    )
  })
})
