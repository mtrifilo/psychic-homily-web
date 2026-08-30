import { describe, it, expect, vi } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { SceneDetail, SceneVenue } from '../types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

import { SceneRooms } from './SceneRooms'

function venue(overrides: Partial<SceneVenue> = {}): SceneVenue {
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

function buildScene(venues: SceneVenue[], overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
    stats: {
      venue_count: venues.length,
      artist_count: 17,
      upcoming_show_count: 328,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 0,
      shows_prev_month: 0,
      shows_trend: 0,
      new_artists_30d: 0,
      active_venues_this_month: 0,
      shows_by_month: [],
    },
    venues,
    ...overrides,
  }
}

const denseRooms = [
  venue({ id: 1, name: 'Crescent Ballroom', upcoming_show_count: 57 }),
  venue({
    id: 2,
    name: 'Nile Theater',
    slug: 'nile-theater',
    city: 'Mesa',
    upcoming_show_count: 27,
  }),
  venue({
    id: 3,
    name: 'Trunk Space',
    slug: 'trunk-space',
    website: undefined,
    upcoming_show_count: 17,
  }),
]

// Portland: one upcoming show in the whole scene.
const sparseRooms = [
  venue({
    id: 1,
    name: 'Mississippi Studios',
    slug: 'mississippi-studios',
    city: 'Portland',
    state: 'OR',
    upcoming_show_count: 1,
  }),
  venue({
    id: 2,
    name: 'Doug Fir Lounge',
    slug: 'doug-fir-lounge',
    city: 'Portland',
    state: 'OR',
    upcoming_show_count: 0,
  }),
  venue({
    id: 3,
    name: 'Turn! Turn! Turn!',
    slug: undefined,
    website: undefined,
    city: 'Portland',
    state: 'OR',
    upcoming_show_count: 0,
  }),
]

const portlandScene = buildScene(sparseRooms, { city: 'Portland', state: 'OR' })

function roomNames() {
  return screen
    .getAllByRole('listitem')
    .map(li => li.querySelector('a, span')?.textContent)
}

describe('SceneRooms — dense', () => {
  it('names every tracked room, busiest first, with its count', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)

    expect(roomNames()).toEqual(['Crescent Ballroom', 'Nile Theater', 'Trunk Space'])
    expect(screen.getByText('57 shows')).toBeInTheDocument()
    expect(screen.getByText('27 shows')).toBeInTheDocument()
    expect(screen.getByText('17 shows')).toBeInTheDocument()
  })

  it('says what the order is', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)
    expect(
      screen.getByRole('heading', { name: /Rooms \/ 3 tracked · ordered by upcoming shows/i })
    ).toBeInTheDocument()
  })

  it('links each room to its own page and its site separately', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)

    expect(screen.getByRole('link', { name: 'Crescent Ballroom' })).toHaveAttribute(
      'href',
      '/venues/crescent-ballroom'
    )
    // Every room renders an identical bare [site ↗], so the room name is what
    // tells the links apart; assert only that half here and let the bracket
    // primitive's own suite own the new-tab suffix.
    const site = screen.getByRole('link', {
      name: /^Crescent Ballroom website\b/,
    })
    expect(site).toHaveAttribute('href', 'https://crescentphx.com')
    expect(site).toHaveAttribute('target', '_blank')
    expect(site).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('prints the room own city so a metro reads as a region', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)
    expect(screen.getByText('(Mesa)')).toBeInTheDocument()
    expect(screen.getAllByText('(Phoenix)')).toHaveLength(2)
  })

  it('pluralizes a single upcoming show', () => {
    renderWithProviders(
      <SceneRooms
        scene={buildScene([
          venue({ id: 1, name: 'A Room', upcoming_show_count: 1 }),
          venue({ id: 2, name: 'B Room', slug: 'b-room', upcoming_show_count: 2 }),
        ])}
      />
    )
    expect(screen.getByText('1 show')).toBeInTheDocument()
    expect(screen.getByText('2 shows')).toBeInTheDocument()
  })

  it('offers alphabetical as the escape hatch and reorders in place', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)

    await user.click(screen.getByRole('button', { name: 'Alphabetical' }))

    expect(roomNames()).toEqual(['Crescent Ballroom', 'Nile Theater', 'Trunk Space'])
    expect(
      screen.getByRole('heading', { name: /Rooms \/ 3 tracked · alphabetical/i })
    ).toBeInTheDocument()
    // The counts are real per-room figures; the escape hatch changes the ORDER,
    // not whether we are willing to state them.
    expect(screen.getByText('57 shows')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'By upcoming shows' })
    ).toBeInTheDocument()
  })

  it('reorders alphabetically when the ranked and alphabetical orders differ', async () => {
    const user = userEvent.setup()
    const rooms = [
      venue({ id: 1, name: 'Zebra Lounge', slug: 'zebra', upcoming_show_count: 9 }),
      venue({ id: 2, name: 'Apple Bar', slug: 'apple', upcoming_show_count: 4 }),
    ]
    renderWithProviders(<SceneRooms scene={buildScene(rooms)} />)

    expect(roomNames()).toEqual(['Zebra Lounge', 'Apple Bar'])
    await user.click(screen.getByRole('button', { name: 'Alphabetical' }))
    expect(roomNames()).toEqual(['Apple Bar', 'Zebra Lounge'])
  })

  it('asks for the rooms we are missing', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)
    expect(screen.getByText(/Missing a room\?/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Suggest a venue →' })
    ).toHaveAttribute('href', '/contribute')
  })
})

describe('SceneRooms — sparse', () => {
  it('drops the counts and lists the rooms alphabetically', () => {
    renderWithProviders(
      <SceneRooms scene={portlandScene} />
    )

    expect(roomNames()).toEqual([
      'Doug Fir Lounge',
      'Mississippi Studios',
      'Turn! Turn! Turn!',
    ])
    expect(
      screen.getByRole('heading', { name: /Rooms \/ 3 tracked · alphabetical/i })
    ).toBeInTheDocument()
    expect(screen.queryByText(/\d+ shows?$/)).not.toBeInTheDocument()
  })

  // A ranking of 1, 0, 0 orders nothing, so there is no second order to offer.
  it('offers no order toggle when the counts order nothing', () => {
    renderWithProviders(
      <SceneRooms scene={portlandScene} />
    )
    expect(screen.queryByRole('button', { name: 'Alphabetical' })).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'By upcoming shows' })
    ).not.toBeInTheDocument()
  })

  // A room with no page of its own is still named: the list discloses WHICH
  // rooms this page speaks for, and dropping one would misstate the coverage.
  it('names a slugless room without linking it', () => {
    renderWithProviders(
      <SceneRooms scene={portlandScene} />
    )
    expect(screen.getByText('Turn! Turn! Turn!')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Turn! Turn! Turn!' })
    ).not.toBeInTheDocument()
  })

  // A reader's sort choice only survives while the control that made it does.
  // A refetch that drops the scene below the rankable threshold takes the
  // toggle away, and a retained `ranked` would leave the list sorted by counts
  // it no longer prints with no control to get back from.
  it('drops a retained ranked choice when the scene stops being rankable', async () => {
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)

    await user.click(screen.getByRole('button', { name: 'Alphabetical' }))
    await user.click(screen.getByRole('button', { name: 'By upcoming shows' }))
    expect(roomNames()).toEqual(['Crescent Ballroom', 'Nile Theater', 'Trunk Space'])

    // Same rooms, now with only one of them booked.
    rerender(
      <SceneRooms
        scene={buildScene([
          venue({ id: 1, name: 'Crescent Ballroom', upcoming_show_count: 3 }),
          venue({ id: 2, name: 'Nile Theater', slug: 'nile-theater', upcoming_show_count: 0 }),
          venue({ id: 3, name: 'Apple Bar', slug: 'apple-bar', upcoming_show_count: 0 }),
        ])}
      />
    )

    expect(roomNames()).toEqual(['Apple Bar', 'Crescent Ballroom', 'Nile Theater'])
    expect(
      screen.getByRole('heading', { name: /Rooms \/ 3 tracked · alphabetical/i })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Alphabetical|By upcoming/ })).toBeNull()
  })

  it('still asks for the rooms we are missing', () => {
    renderWithProviders(
      <SceneRooms scene={portlandScene} />
    )
    expect(
      screen.getByRole('link', { name: 'Suggest a venue →' })
    ).toBeInTheDocument()
  })
})

describe('SceneRooms — empty', () => {
  // Decision 11: an empty-capable module SUBSTITUTES, never scaffolds. The
  // shape being retired is a titled `0 venues in Phoenix` header over nothing.
  it('substitutes the ask rather than scaffolding an empty leaderboard', () => {
    renderWithProviders(<SceneRooms scene={buildScene([])} />)

    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
    expect(screen.queryByText(/ordered by upcoming shows/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/0 tracked/i)).not.toBeInTheDocument()
    expect(
      screen.getByText(/we track none in Phoenix yet/i)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Suggest a venue →' })
    ).toHaveAttribute('href', '/contribute')
  })

  it('survives a payload whose venues array is absent', () => {
    const scene = buildScene([])
    // The generator types `venues` nullable because every Go slice is.
    renderWithProviders(
      <SceneRooms scene={{ ...scene, venues: undefined as unknown as SceneVenue[] }} />
    )
    expect(screen.getByText(/we track none in Phoenix yet/i)).toBeInTheDocument()
  })
})

/**
 * PSY-1850: the root is an index into the scene, not a directory of it, so the
 * rooms module lists the top eight and offers the rest.
 */
describe('SceneRooms — front-page cap', () => {
  // 12 rooms, descending counts, so the ranked order is unambiguous.
  const manyRooms = Array.from({ length: 12 }, (_, i) =>
    venue({
      id: i + 1,
      name: `Room ${String(i + 1).padStart(2, '0')}`,
      slug: `room-${i + 1}`,
      upcoming_show_count: 100 - i,
    })
  )

  it('lists only the top eight rooms', () => {
    renderWithProviders(<SceneRooms scene={buildScene(manyRooms)} />)

    expect(screen.getAllByRole('listitem')).toHaveLength(8)
    expect(roomNames()).toEqual([
      'Room 01',
      'Room 02',
      'Room 03',
      'Room 04',
      'Room 05',
      'Room 06',
      'Room 07',
      'Room 08',
    ])
  })

  // The heading names the scene's real coverage, not the eight drawn. A page
  // that said "8 tracked" would understate what it speaks for.
  it('still names the full tracked count in the heading', () => {
    renderWithProviders(<SceneRooms scene={buildScene(manyRooms)} />)
    expect(
      screen.getByRole('heading', { name: /Rooms \/ 12 tracked/i })
    ).toBeInTheDocument()
  })

  it('offers the rest, labelled with the total', () => {
    renderWithProviders(<SceneRooms scene={buildScene(manyRooms)} />)
    expect(
      screen.getByRole('button', { name: 'Show all 12 rooms →' })
    ).toBeInTheDocument()
  })

  it('reveals every room on click, without a fetch', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SceneRooms scene={buildScene(manyRooms)} />)

    await user.click(screen.getByRole('button', { name: 'Show all 12 rooms →' }))

    expect(screen.getAllByRole('listitem')).toHaveLength(12)
    expect(
      screen.queryByRole('button', { name: /show all/i })
    ).not.toBeInTheDocument()
  })

  // Sliced AFTER ordering, so the escape hatch re-picks which eight are shown
  // rather than re-sorting whichever eight the ranked order surfaced.
  it('re-picks the eight when the reader flips to alphabetical', async () => {
    const user = userEvent.setup()
    // Reverse-alphabetical names against descending counts, so the two orders
    // cannot agree on which eight belong on the page.
    const rooms = Array.from({ length: 12 }, (_, i) =>
      venue({
        id: i + 1,
        name: `Room ${String(12 - i).padStart(2, '0')}`,
        slug: `room-${i + 1}`,
        upcoming_show_count: 100 - i,
      })
    )
    renderWithProviders(<SceneRooms scene={buildScene(rooms)} />)

    expect(roomNames()[0]).toBe('Room 12')

    await user.click(screen.getByRole('button', { name: /alphabetical/i }))

    expect(roomNames()).toEqual([
      'Room 01',
      'Room 02',
      'Room 03',
      'Room 04',
      'Room 05',
      'Room 06',
      'Room 07',
      'Room 08',
    ])
  })

  it('offers no control when every room already fits', () => {
    renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)
    expect(screen.queryByRole('button', { name: /show all/i })).not.toBeInTheDocument()
  })
})

describe('SceneRooms — copy conventions', () => {
  it('uses no em dashes anywhere', () => {
    const { container } = renderWithProviders(<SceneRooms scene={buildScene(denseRooms)} />)
    expect(container.textContent).not.toContain('—')
  })

  it('keeps one navigational link per room, plus its site', () => {
    renderWithProviders(<SceneRooms scene={buildScene([denseRooms[0]])} />)
    const row = screen.getByRole('listitem')
    expect(within(row).getAllByRole('link')).toHaveLength(2)
  })
})
