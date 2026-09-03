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
// The confirm mutation (PSY-1542). Mocked at the module boundary like the
// shows hook so the panel's behaviour can be driven without a QueryClient.
const mockConfirmMutate = vi.fn()
const mockUseVenueConfirm = vi.fn<() => Record<string, unknown>>(() => ({
  mutate: mockConfirmMutate,
  isPending: false,
  data: undefined,
  error: null,
}))
vi.mock('@/features/venues/hooks', () => ({
  useVenueShows: (args: unknown) => mockUseVenueShows(args),
  useVenueConfirm: () => mockUseVenueConfirm(),
  formatVenueConfirmError: (error: unknown) =>
    error ? (error as { rendered?: string }).rendered ?? 'Confirm failed' : null,
}))

// The field-note rollup (PSY-1590). Mocked at the module boundary like the
// shows hook — it is a useQuery underneath, and there is no QueryClient here.
const mockUseVenueFieldNotes = vi.fn<
  (venueId: number, options?: unknown) => Record<string, unknown>
>(() => ({ data: undefined }))
vi.mock('@/features/comments/hooks', () => ({
  useVenueFieldNotes: (venueId: number, options?: unknown) =>
    mockUseVenueFieldNotes(venueId, options),
}))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/atlas',
}))

// `authStatus` is the setting, and `isAuthenticated` is derived from it, so no
// case here can describe a viewer whose two auth signals disagree.
let mockAuthStatus: 'pending' | 'anonymous' | 'authenticated' = 'authenticated'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: mockAuthStatus,
    isAuthenticated: mockAuthStatus === 'authenticated',
  }),
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
  mockConfirmMutate.mockReset()
  mockPush.mockReset()
  // Default: a venue nobody has written a note about, which is the common case
  // and the one the panel must render as NO section rather than an empty box.
  mockUseVenueFieldNotes.mockReturnValue({ data: undefined })
  mockAuthStatus = 'authenticated'
  mockUseVenueConfirm.mockReturnValue({
    mutate: mockConfirmMutate,
    isPending: false,
    data: undefined,
    error: null,
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

  it('stamps the updated time and every non-zero provenance count', () => {
    const provenance = renderPanel({
      venue: venue({
        provenance: {
          updated_at: '2026-07-23T00:00:00Z',
          edit_count: 4,
          contributor_count: 2,
          confirmation_count: 7,
          sources: ['ingest', 'community'],
        },
      }),
    }).container.querySelector('[data-testid="venue-panel-provenance"]')
    expect(provenance?.textContent).toMatch(/^UPDATED /)
    expect(provenance?.textContent).toContain('4 edits by 2 contributors')
    expect(provenance?.textContent).toContain('7 confirmations')
    expect(provenance?.textContent).toContain('ingest + community')
  })

  it('omits zero counts rather than stamping "0 edits"', () => {
    // A stamp that lists what it does NOT have reads as broken. Absence of a
    // segment already says the same thing, more quietly.
    const provenance = renderPanel({
      venue: venue({
        provenance: {
          updated_at: '2026-07-23T00:00:00Z',
          edit_count: 0,
          contributor_count: 0,
          confirmation_count: 0,
          sources: [],
        },
      }),
    }).container.querySelector('[data-testid="venue-panel-provenance"]')
    expect(provenance?.textContent).toMatch(/^UPDATED /)
    expect(provenance?.textContent).not.toMatch(/edit|contributor|confirmation/i)
  })

  it('singularises a lone edit, contributor and confirmation', () => {
    const provenance = renderPanel({
      venue: venue({
        provenance: {
          updated_at: '2026-07-23T00:00:00Z',
          edit_count: 1,
          contributor_count: 1,
          confirmation_count: 1,
          sources: [],
        },
      }),
    }).container.querySelector('[data-testid="venue-panel-provenance"]')
    expect(provenance?.textContent).toContain('1 edit by 1 contributor')
    expect(provenance?.textContent).toContain('1 confirmation')
  })

  it('confirms the venue for a signed-in user of any trust tier', () => {
    renderPanel()
    fireEvent.click(screen.getByTestId('venue-panel-confirm'))
    expect(mockConfirmMutate).toHaveBeenCalledWith(7)
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('sends a signed-out user to auth instead of writing', () => {
    mockAuthStatus = 'anonymous'
    renderPanel()
    fireEvent.click(screen.getByTestId('venue-panel-confirm'))
    expect(mockConfirmMutate).not.toHaveBeenCalled()
    expect(mockPush).toHaveBeenCalledWith(
      expect.stringContaining('/auth?returnTo='),
    )
  })

  it('neither writes nor redirects while auth is unsettled', () => {
    // The redirect cannot tell "no session" from "profile in flight", so a tap
    // in this window would either write as a viewer we cannot identify or send
    // a signed-in one to the sign-in form.
    mockAuthStatus = 'pending'
    renderPanel()
    const confirm = screen.getByTestId('venue-panel-confirm')
    expect(confirm).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(confirm)
    expect(mockConfirmMutate).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('reads as done, and refuses a second write, once confirmed', () => {
    // The write is idempotent server-side, so a repeat tap would be harmless —
    // but a button that still says "Confirm info" after you confirmed reads as
    // a tap that did nothing.
    mockUseVenueConfirm.mockReturnValue({
      mutate: mockConfirmMutate,
      isPending: false,
      data: {
        confirmation_count: 8,
        last_confirmed_at: '2026-07-27T10:00:00Z',
        viewer_has_confirmed: true,
      },
      error: null,
    })
    renderPanel({
      venue: venue({
        provenance: {
          updated_at: '2026-07-23T00:00:00Z',
          edit_count: 0,
          contributor_count: 0,
          confirmation_count: 7,
          sources: [],
        },
      }),
    })
    const button = screen.getByTestId('venue-panel-confirm')
    expect(button).toHaveTextContent('✓ Confirmed')
    // aria-disabled, NOT native disabled: a natively disabled button leaves the
    // tab order, so a keyboard user tabbing back to the panel would never land
    // on it and never hear that their confirmation registered.
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toBeDisabled()
    expect(button).toHaveAccessibleName(
      'You confirmed Hotel Vegas’s info is current',
    )
    // The stamp reflects the count the mutation just returned, not the stale
    // one the rail's list was fetched with.
    expect(
      screen.getByTestId('venue-panel-provenance').textContent,
    ).toContain('8 confirmations')

    fireEvent.click(button)
    expect(mockConfirmMutate).not.toHaveBeenCalled()
  })

  it('says it is working, and refuses a second write, while in flight', () => {
    mockUseVenueConfirm.mockReturnValue({
      mutate: mockConfirmMutate,
      isPending: true,
      data: undefined,
      error: null,
    })
    renderPanel()
    const button = screen.getByTestId('venue-panel-confirm')
    expect(button).toHaveTextContent('Confirming…')
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toBeDisabled()

    fireEvent.click(button)
    expect(mockConfirmMutate).not.toHaveBeenCalled()
  })

  it('stamps a confirmation on a venue that had no provenance at all', () => {
    // A venue nobody has touched arrives with no stamp object; the first
    // confirmation must still produce a readable one rather than nothing.
    mockUseVenueConfirm.mockReturnValue({
      mutate: mockConfirmMutate,
      isPending: false,
      data: {
        confirmation_count: 1,
        last_confirmed_at: '2026-07-27T10:00:00Z',
        viewer_has_confirmed: true,
      },
      error: null,
    })
    renderPanel({ venue: venue({ provenance: undefined }) })
    expect(
      screen.getByTestId('venue-panel-provenance').textContent,
    ).toContain('1 confirmation')
    expect(
      screen.getByTestId('venue-panel-provenance').textContent,
    ).toContain('community')
  })

  it('offers a way back in when the session expired mid-tap', () => {
    // The pre-tap auth check reads a client-side flag; a long-lived Atlas tab
    // can still hold `isAuthenticated` after the cookie expires, and a bare
    // "sign in" sentence with no link is a dead end.
    mockUseVenueConfirm.mockReturnValue({
      mutate: mockConfirmMutate,
      isPending: false,
      data: undefined,
      error: { status: 401, rendered: 'Your session expired.' },
    })
    renderPanel()
    const alert = screen.getByTestId('venue-panel-confirm-error')
    expect(alert).toHaveTextContent('Your session expired.')
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/auth?returnTo=%2Fatlas',
    )
  })

  it('surfaces a rate-limited confirm inline instead of failing silently', () => {
    mockUseVenueConfirm.mockReturnValue({
      mutate: mockConfirmMutate,
      isPending: false,
      data: undefined,
      error: { rendered: 'Too many confirmations — try again in 47s.' },
    })
    renderPanel()
    const alert = screen.getByTestId('venue-panel-confirm-error')
    expect(alert).toHaveAttribute('role', 'alert')
    expect(alert).toHaveTextContent('Too many confirmations — try again in 47s.')
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
    expect(screen.getByText('9:00 PM · $18')).toBeInTheDocument()
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
    // otherwise print an empty date gutter beside "Invalid Date · $18".
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
    expect(screen.getByText('$18')).toBeInTheDocument()
  })

  // The panel's meta line joins time and price with a middot, so a withheld
  // clock has to take its separator with it rather than leaving "· $18".
  it('names no hour, and no orphaned separator, when the venue zone is a guess', () => {
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [show({ state: '' })],
        venue_id: 7,
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    renderPanel({ venue: venue({ state: '', timezone: null }) })
    expect(screen.queryByText(/9:00\s?PM/)).not.toBeInTheDocument()
    expect(screen.getByText('$18')).toBeInTheDocument()
    // The date gutter is untouched: only the hour was a guess.
    expect(screen.getByText('FRI 7/31')).toBeInTheDocument()
  })

  it('names the hour once the venue carries its own zone', () => {
    mockUseVenueShows.mockReturnValue({
      data: {
        shows: [show({ state: '' })],
        venue_id: 7,
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    renderPanel({ venue: venue({ state: '', timezone: 'America/Chicago' }) })
    expect(screen.getByText(/9:00\s?PM · \$18/)).toBeInTheDocument()
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

  it('renders show rows inert when no drill-in seam is supplied', () => {
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
      [expect.objectContaining({ id: 101 })],
    )
  })

  // The drill-in stepper walks THE LIST YOU DRILLED IN FROM, so the second
  // argument must be exactly the rows the panel is drawing — deduped and
  // truncated to VENUE_PANEL_SHOW_ROWS. A caller re-deriving it from its own
  // fetch is how "2 of 5" starts disagreeing with the rows above it.
  it('hands the drill-in the rows it is actually drawing, not the raw page', () => {
    const onShowSelect = vi.fn()
    // Distinct dates as well as ids — `dedupVenueShows` collapses same-day
    // same-bill rows, which is exactly what it is there for.
    const many = Array.from({ length: 8 }, (_, i) =>
      show({
        id: 200 + i,
        title: `Show ${i}`,
        slug: `show-${i}`,
        event_date: `2026-08-0${i + 1}T02:00:00Z`,
      }),
    )
    mockUseVenueShows.mockReturnValue({
      data: { shows: many, venue_id: 7, total: 8 },
      isLoading: false,
      isError: false,
    })
    renderPanel({ onShowSelect })
    fireEvent.click(screen.getByRole('button', { name: /Show 0/ }))
    const listed = onShowSelect.mock.calls[0][1] as VenueShow[]
    expect(listed).toHaveLength(5)
    expect(listed.map((s) => s.id)).toEqual([200, 201, 202, 203, 204])
  })

  it('requests the same page the venue page does, so the two share one cache entry', () => {
    // venueQueryKeys.showsPage() keys on the limit, so requesting the venue
    // page's exact page is what puts this panel on the same cache entry instead
    // of a redundant second one.
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

// ── Field notes teaser (PSY-1590) ─────────────────────────────────────────
describe('VenuePanel field notes teaser', () => {
  function note(overrides: Record<string, unknown> = {}) {
    return {
      id: 900,
      entity_type: 'show',
      entity_id: 101,
      kind: 'field_note',
      user_id: 5,
      author_name: 'Renata',
      author_username: 'renata',
      body: 'Loudest set I have heard in that room all year.',
      body_html: '<p>Loudest set I have heard in that room all year.</p>',
      parent_id: null,
      root_id: null,
      depth: 0,
      ups: 9,
      downs: 0,
      score: 0.91,
      visibility: 'visible',
      reply_permission: 'anyone',
      edit_count: 0,
      is_edited: false,
      created_at: '2024-06-15T04:00:00Z',
      updated_at: '2024-06-15T04:00:00Z',
      show_title: 'Doom Night',
      show_artists: ['Neckbeard', 'Gel'],
      show_slug: 'doom-night',
      // 11pm Jun 14 in Austin, so the venue-local month is June — a reader in
      // UTC would otherwise see the 15th.
      show_date: '2024-06-15T04:00:00Z',
      ...overrides,
    }
  }

  function withNotes(notes: unknown[], total = notes.length) {
    mockUseVenueFieldNotes.mockReturnValue({
      data: { notes, total, has_more: total > notes.length },
    })
  }

  it('quotes the note and attributes it to the show it came from', () => {
    withNotes([note()], 3)
    renderPanel()

    const section = screen.getByTestId('venue-panel-field-notes')
    expect(section).toHaveTextContent(
      'Loudest set I have heard in that room all year.',
    )
    // The whole point of the rollup: the reader can see WHICH night this was
    // about, so the quote never reads as a verdict on the venue.
    expect(
      screen.getByTestId('venue-panel-field-note-attribution'),
    ).toHaveTextContent('Doom Night, Jun 2024')
    expect(
      screen.getByTestId('venue-panel-field-note-author'),
    ).toHaveTextContent('Renata')
  })

  it('states the venue-wide note count as plain text, never a link', () => {
    withNotes([note()], 3)
    renderPanel()

    const section = screen.getByTestId('venue-panel-field-notes')
    // The count spans the venue, not the single note fetched for the quote.
    expect(section).toHaveTextContent('3 notes')
    // Locked decision: no "N notes →" affordance until a real destination
    // exists. The only link in the panel is the footer's venue link.
    expect(section.querySelector('a')).toBeNull()
  })

  it('singularizes a lone note', () => {
    withNotes([note()], 1)
    renderPanel()
    expect(screen.getByTestId('venue-panel-field-notes')).toHaveTextContent(
      '1 note',
    )
  })

  it('renders no section at all for a venue with no notes', () => {
    withNotes([], 0)
    renderPanel()
    expect(screen.queryByTestId('venue-panel-field-notes')).toBeNull()
  })

  it('renders no section while the rollup is unresolved or failed', () => {
    // A teaser is supplementary to the show list; a spinner or error line
    // beside content that loaded fine would be noise.
    mockUseVenueFieldNotes.mockReturnValue({ data: undefined })
    renderPanel()
    expect(screen.queryByTestId('venue-panel-field-notes')).toBeNull()
  })

  it('tolerates a null notes array on the wire', () => {
    mockUseVenueFieldNotes.mockReturnValue({
      data: { notes: null, total: 0, has_more: false },
    })
    renderPanel()
    expect(screen.queryByTestId('venue-panel-field-notes')).toBeNull()
  })

  // Most shows carry no title of their own, so this is the COMMON case, not an
  // edge one: naming it from the bill is what keeps the teaser visible on the
  // majority of real venues.
  it('names an untitled show from its bill rather than dropping the note', () => {
    withNotes([note({ show_title: '', show_artists: ['Neckbeard', 'Gel'] })], 4)
    renderPanel()
    expect(
      screen.getByTestId('venue-panel-field-note-attribution'),
    ).toHaveTextContent('Neckbeard, Gel, Jun 2024')
  })

  it('still names a show with neither title nor bill', () => {
    withNotes([note({ show_title: '', show_artists: [] })], 1)
    renderPanel()
    expect(
      screen.getByTestId('venue-panel-field-note-attribution'),
    ).toHaveTextContent('Untitled Show, Jun 2024')
  })

  // Setlist spoilers: FieldNoteCard gates these behind click-to-reveal, and
  // the teaser has nowhere to put that gate. Because the rollup sorts by
  // score, an upvoted spoiler is exactly the note that would surface.
  it('never quotes a setlist-spoiler note, even ranked first', () => {
    withNotes(
      [
        note({
          id: 1,
          body: 'they closed with the unreleased one',
          structured_data: { setlist_spoiler: true },
        }),
        note({ id: 2, body: 'no spoilers in this one' }),
      ],
      2,
    )
    renderPanel()

    const section = screen.getByTestId('venue-panel-field-notes')
    expect(section).toHaveTextContent('no spoilers in this one')
    expect(section).not.toHaveTextContent('they closed with the unreleased one')
  })

  it('renders no section when every note is a spoiler', () => {
    withNotes([note({ structured_data: { setlist_spoiler: true } })], 1)
    renderPanel()
    expect(screen.queryByTestId('venue-panel-field-notes')).toBeNull()
  })

  it('quotes the note as prose, not as raw Markdown source', () => {
    // `body` is Markdown SOURCE; the teaser must not show its asterisks.
    withNotes([note({ body: '**Loudest** set of the *year*' })], 1)
    renderPanel()
    expect(screen.getByTestId('venue-panel-field-notes')).toHaveTextContent(
      'Loudest set of the year',
    )
  })

  it('still attributes a note whose show date is unreadable', () => {
    // The title alone attributes the note to a night; only the age hint is
    // lost, so this degrades rather than dropping the note.
    withNotes([note({ show_date: 'not-a-date' })], 1)
    renderPanel()
    expect(
      screen.getByTestId('venue-panel-field-note-attribution'),
    ).toHaveTextContent('Doom Night')
  })

  it('asks the rollup for this venue', () => {
    withNotes([note()], 1)
    renderPanel()
    expect(mockUseVenueFieldNotes).toHaveBeenCalledWith(7, undefined)
  })
})
