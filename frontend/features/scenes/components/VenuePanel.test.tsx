import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import type { VenueShow, VenueWithShowCount } from '@/features/venues/types'

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}))

// FollowButton pulls AuthContext + usePathname (neither available here) —
// mock at the module boundary, the same idiom AtlasGlobe.test uses.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ entityId }: { entityId: number | string }) => (
    <button data-testid="follow-button">Follow venue {String(entityId)}</button>
  ),
}))

const mockUseVenueShows = vi.fn<(args: unknown) => Record<string, unknown>>(
  () => ({
    data: { shows: [], venue_id: 1, total: 0 },
    isLoading: false,
    isError: false,
  }),
)
vi.mock('@/features/venues/hooks', () => ({
  useVenueShows: (args: unknown) => mockUseVenueShows(args),
}))

import { VenuePanel } from './VenuePanel'

function venue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 7,
    slug: 'hotel-vegas-austin-tx',
    name: 'Hotel Vegas',
    address: '1502 E 6th St',
    capacity: 250,
    city: 'Austin',
    state: 'TX',
    timezone: 'America/Chicago',
    verified: true,
    upcoming_show_count: 11,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-07-23T00:00:00Z',
    ...overrides,
  } as VenueWithShowCount
}

function artist(id: number, name: string): VenueShow['artists'][number] {
  return {
    id,
    name,
    slug: name.toLowerCase().replace(/[^a-z]+/g, '-'),
  } as VenueShow['artists'][number]
}

function show(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 101,
    slug: 'levitation-pre-party',
    title: 'Levitation pre-party',
    // 9pm on Jul 31 in Austin.
    event_date: '2026-08-01T02:00:00Z',
    city: 'Austin',
    state: 'TX',
    price: 18,
    age_requirement: null,
    artists: [],
    ...overrides,
  } as VenueShow
}

function renderPanel(
  props: Partial<React.ComponentProps<typeof VenuePanel>> = {},
) {
  const defaults: React.ComponentProps<typeof VenuePanel> = {
    venue: venue(),
    onClose: vi.fn(),
  }
  const merged = { ...defaults, ...props }
  return { ...render(<VenuePanel {...merged} />), props: merged }
}

beforeEach(() => {
  mockUseVenueShows.mockReturnValue({
    data: { shows: [], venue_id: 7, total: 0 },
    isLoading: false,
    isError: false,
  })
})

describe('VenuePanel', () => {
  it('names itself for assistive tech and titles the venue', () => {
    renderPanel()
    expect(
      screen.getByRole('region', { name: 'Hotel Vegas — upcoming shows' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Hotel Vegas' }),
    ).toBeInTheDocument()
  })

  it('renders the identity line from real venue fields only', () => {
    renderPanel()
    expect(screen.getByTestId('venue-panel-identity')).toHaveTextContent(
      '1502 E 6th St · cap ~250 · Austin, TX',
    )
  })

  it('does not leak an unverified venue’s address', () => {
    // PSY-1536's privacy gate: the API withholds `address` for unverified
    // venues exactly as it withholds their street coordinates. The panel must
    // publish neither, or it defeats the gate the map already respects.
    renderPanel({ venue: venue({ verified: false, address: null }) })
    const identity = screen.getByTestId('venue-panel-identity')
    expect(identity).toHaveTextContent('Austin, TX')
    expect(identity).not.toHaveTextContent('1502 E 6th St')
  })

  it('stamps a real updated time and claims no edit or contributor counts', () => {
    // PSY-1542 owns the counts. PSY-1539 set the precedent of shipping the
    // honest half rather than inventing the rest; hold that line here.
    const provenance = renderPanel().container.querySelector(
      '[data-testid="venue-panel-provenance"]',
    )
    expect(provenance?.textContent).toMatch(/^UPDATED /)
    expect(provenance?.textContent).not.toMatch(/edits|contributors/i)
  })

  it('renders the Confirm action inert until PSY-1542 wires it', () => {
    renderPanel()
    const confirm = screen.getByRole('button', {
      name: 'Confirm info — not available yet',
    })
    expect(confirm).toHaveAttribute('aria-disabled', 'true')
    // Reachable on purpose: a natively `disabled` button leaves the tab order,
    // so a keyboard user could never discover the control OR the reason it
    // does nothing. The reason rides in the accessible name instead.
    expect(confirm).not.toBeDisabled()
  })

  it('shows a loading state distinct from an empty calendar', () => {
    mockUseVenueShows.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })
    renderPanel()
    expect(screen.getByText('Loading shows…')).toBeInTheDocument()
    // The count must not be asserted while it is unknown — "Upcoming — 0
    // shows" during a pending fetch reads as "this venue has nothing".
    expect(screen.queryByText(/Upcoming — /)).not.toBeInTheDocument()
    expect(
      screen.queryByText('Nothing on the calendar yet.'),
    ).not.toBeInTheDocument()
  })

  it('lists upcoming shows with venue-local dates and times', () => {
    mockUseVenueShows.mockReturnValue({
      data: { shows: [show()], venue_id: 7, total: 1 },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    // 2026-08-01T02:00:00Z is 9pm Jul 31 in Austin, whatever zone the test
    // machine is in.
    expect(screen.getByText('FRI 7/31')).toBeInTheDocument()
    expect(screen.getByText('Levitation pre-party')).toBeInTheDocument()
    expect(screen.getByText('9:00 PM · $18.00')).toBeInTheDocument()
  })

  it('falls back to the bill when a show has no title', () => {
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [
          show({
            title: '',
            artists: [artist(1, 'Die Spitz'), artist(2, "Farmer's Wife")],
          }),
        ],
        venue_id: 7,
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    expect(screen.getByText("Die Spitz, Farmer's Wife")).toBeInTheDocument()
  })

  it('caps the list and links the remainder to the venue page', () => {
    // Distinct dates: `dedupVenueShows` keys on headliner + event_date, so
    // nine same-night no-bill rows would legitimately collapse into one.
    const shows = Array.from({ length: 9 }, (_, i) =>
      show({
        id: 200 + i,
        title: `Show ${i}`,
        event_date: `2026-08-0${i + 1}T02:00:00Z`,
      }),
    )
    mockUseVenueShows.mockReturnValue({
      data: { shows, venue_id: 7, total: 9 },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    expect(screen.getByText('Show 0')).toBeInTheDocument()
    expect(screen.getByText('Show 4')).toBeInTheDocument()
    expect(screen.queryByText('Show 5')).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'view all 9 →' }),
    ).toHaveAttribute('href', '/venues/hotel-vegas-austin-tx')
  })

  it('counts what it lists, after de-duplicating the same-night rows', () => {
    // Two ingest rows for one Friday night. The venue page de-dupes them and
    // so must the panel — and the header must then claim 1, not the API's 2,
    // or it contradicts the single row underneath it.
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [show({ id: 1 }), show({ id: 2 })],
        venue_id: 7,
        total: 2,
      },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    expect(screen.getByRole('heading', { name: /Upcoming — 1 show$/ })).toBeInTheDocument()
    expect(screen.getAllByText('FRI 7/31')).toHaveLength(1)
  })

  // Adversarial review: /atlas has no route-level error boundary, so a
  // TypeError in a show row takes down the whole app shell, not just the
  // panel. The live endpoint always sends an array today — these pin the
  // degradation so a future `omitempty` on the wire can't turn into an outage.
  it('degrades rather than throwing when a show carries no bill at all', () => {
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [
          { ...show({ title: '' }), artists: undefined } as unknown as VenueShow,
        ],
        venue_id: 7,
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    expect(() => renderPanel()).not.toThrow()
    expect(screen.getByText('Untitled Show')).toBeInTheDocument()
  })

  it('renders nothing rather than "Invalid Date" for a malformed event date', () => {
    // formatShowTime has no NaN guard of its own, so an unparseable date would
    // otherwise print an empty date gutter beside "Invalid Date · $18.00".
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [show({ event_date: 'not-a-date' })],
        venue_id: 7,
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    expect(screen.getByText('Levitation pre-party')).toBeInTheDocument()
    expect(screen.queryByText(/Invalid Date/)).not.toBeInTheDocument()
    // The price still stands on its own — a bad date shouldn't erase the row.
    expect(screen.getByText('$18.00')).toBeInTheDocument()
  })

  it('says so plainly when nothing is booked', () => {
    renderPanel()
    expect(screen.getByText('Nothing on the calendar yet.')).toBeInTheDocument()
    expect(screen.queryByText(/view all/)).not.toBeInTheDocument()
  })

  it('surfaces a failed shows fetch rather than reading as an empty calendar', () => {
    mockUseVenueShows.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    renderPanel()
    expect(
      screen.getByText('Couldn’t load this venue’s shows.'),
    ).toBeInTheDocument()
  })

  it('closes on the ✕ button', () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    fireEvent.click(
      screen.getByRole('button', { name: 'Close Hotel Vegas panel' }),
    )
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('moves focus to the close control on open', () => {
    // Opening from a rail row must not leave a keyboard user standing on that
    // row — they would have to tab through every remaining venue in the city
    // to reach the panel their keystroke just opened.
    renderPanel()
    expect(screen.getByRole('button', { name: 'Close Hotel Vegas panel' })).toHaveFocus()
  })

  it('hands focus back to whatever opened it', () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const { unmount } = renderPanel()
    expect(opener).not.toHaveFocus()

    unmount()

    expect(opener).toHaveFocus()
    opener.remove()
  })

  it('does not yank focus back from somewhere the user tabbed to', () => {
    // The panel is non-modal with no focus trap, so the user may well have
    // moved on. Restoring focus over their head is worse than not restoring.
    const opener = document.createElement('button')
    const elsewhere = document.createElement('button')
    document.body.append(opener, elsewhere)
    opener.focus()
    const { unmount } = renderPanel()
    elsewhere.focus()

    unmount()

    expect(elsewhere).toHaveFocus()
    opener.remove()
    elsewhere.remove()
  })

  it('closes on Escape', () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('renders show rows inert until PSY-1541 supplies the drill-in', () => {
    mockUseVenueShows.mockReturnValue({
      data: { shows: [show()], venue_id: 7, total: 1 },
      isLoading: false,
      isError: false,
    })
    renderPanel()
    expect(
      screen.queryByRole('button', { name: /Levitation pre-party/ }),
    ).not.toBeInTheDocument()
  })

  it('makes show rows actionable once the drill-in seam is supplied', () => {
    const onShowSelect = vi.fn()
    mockUseVenueShows.mockReturnValue({
      data: { shows: [show()], venue_id: 7, total: 1 },
      isLoading: false,
      isError: false,
    })
    renderPanel({ onShowSelect })
    fireEvent.click(
      screen.getByRole('button', { name: /Levitation pre-party/ }),
    )
    expect(onShowSelect).toHaveBeenCalledWith(
      expect.objectContaining({ id: 101 }),
    )
  })

  it('requests the same page the venue page does, so the two share one cache entry', () => {
    // venueQueryKeys.shows() keys only on venue id + time filter — not on
    // limit or timezone — so a differently-parameterized request here would
    // collide with VenueShowsList's.
    renderPanel()
    expect(mockUseVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({
        venueId: 7,
        timeFilter: 'upcoming',
        limit: 50,
      }),
    )
  })

  it('links out to the venue page', () => {
    renderPanel()
    expect(
      screen.getByRole('link', { name: 'Open venue page →' }),
    ).toHaveAttribute('href', '/venues/hotel-vegas-austin-tx')
  })
})
