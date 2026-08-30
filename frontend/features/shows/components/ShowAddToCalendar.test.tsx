import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ShowResponse } from '../types'

const mockAuth = { isAuthenticated: false, user: undefined as { id: number } | undefined }
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuth,
}))

const saveMutate = vi.fn()
const mockSaveCount = { value: undefined as { save_count: number; is_saved: boolean } | undefined }
vi.mock('../hooks/useSavedShows', () => ({
  useSaveShow: () => ({ mutate: saveMutate, isPending: false }),
  useShowSaveCount: () => ({ data: mockSaveCount.value }),
}))

import {
  ShowAddToCalendar,
  showCalendarIcsUrl,
  showGoogleCalendarUrl,
} from './ShowAddToCalendar'

// vitest.config.mts pins NEXT_PUBLIC_API_URL to http://localhost:8080, so
// API_BASE_URL is deterministic here.
const ICS_URL = 'http://localhost:8080/shows/desert-doom-night/calendar.ics'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 42,
    slug: 'desert-doom-night',
    title: 'Desert Doom Night',
    event_date: '2026-08-15T03:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    price: 12,
    age_requirement: '21+',
    description: null,
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 7,
        slug: 'the-rebel-lounge',
        name: 'The Rebel Lounge',
        address: '2303 E Indian School Rd',
        city: 'Phoenix',
        state: 'AZ',
        timezone: 'America/Phoenix',
        verified: true,
      },
    ],
    artists: [
      { id: 1, slug: 'ajj', name: 'AJJ' },
      { id: 2, slug: 'calexico', name: 'Calexico' },
    ],
    created_at: '2026-06-01T12:00:00Z',
    updated_at: '2026-06-01T12:00:00Z',
    ...overrides,
  } as unknown as ShowResponse
}

describe('showCalendarIcsUrl', () => {
  it('builds an absolute URL against the API origin', () => {
    expect(showCalendarIcsUrl('desert-doom-night')).toBe(ICS_URL)
  })

  it('encodes hostile slugs so they cannot escape the path', () => {
    expect(showCalendarIcsUrl('a/../../evil?x=1')).toBe(
      'http://localhost:8080/shows/a%2F..%2F..%2Fevil%3Fx%3D1/calendar.ics'
    )
  })
})

describe('showGoogleCalendarUrl', () => {
  it('builds the template URL with UTC times and the shared 3h duration', () => {
    const url = new URL(showGoogleCalendarUrl(makeShow()))
    expect(url.origin + url.pathname).toBe('https://calendar.google.com/calendar/render')
    expect(url.searchParams.get('action')).toBe('TEMPLATE')
    expect(url.searchParams.get('text')).toBe('Desert Doom Night')
    // 03:00Z start, +3h end — absolute instants; Google renders them in the
    // viewer's calendar timezone.
    expect(url.searchParams.get('dates')).toBe('20260815T030000Z/20260815T060000Z')
    expect(url.searchParams.get('location')).toBe(
      'The Rebel Lounge, 2303 E Indian School Rd, Phoenix, AZ'
    )
    expect(url.searchParams.get('details')).toContain('/shows/desert-doom-night')
  })

  it('falls back to the bill when the show has no title', () => {
    const url = new URL(showGoogleCalendarUrl(makeShow({ title: '' })))
    expect(url.searchParams.get('text')).toBe('AJJ, Calexico')
  })

  it('carries cancellation into the title and details — Google has no STATUS field', () => {
    const url = new URL(showGoogleCalendarUrl(makeShow({ is_cancelled: true })))
    expect(url.searchParams.get('text')).toBe('CANCELLED: Desert Doom Night')
    expect(url.searchParams.get('details')).toContain('This show has been cancelled.')
  })

  it('marks sold-out shows in the title, matching the .ics summary', () => {
    const url = new URL(showGoogleCalendarUrl(makeShow({ is_sold_out: true })))
    expect(url.searchParams.get('text')).toBe('Desert Doom Night [SOLD OUT]')
  })

  it('links the canonical site origin in details, never the runtime origin', () => {
    const url = new URL(showGoogleCalendarUrl(makeShow()))
    expect(url.searchParams.get('details')).toBe(
      'https://psychichomily.com/shows/desert-doom-night'
    )
  })

  it('omits a redacted (null) address from the location', () => {
    const show = makeShow()
    show.venues[0].address = null as unknown as string
    const url = new URL(showGoogleCalendarUrl(show))
    expect(url.searchParams.get('location')).toBe('The Rebel Lounge, Phoenix, AZ')
  })
})

describe('ShowAddToCalendar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuth.isAuthenticated = false
    mockAuth.user = undefined
    mockSaveCount.value = undefined
  })

  async function openPopover(show = makeShow()) {
    const user = userEvent.setup()
    render(<ShowAddToCalendar show={show} lifecycle="upcoming" />)
    await user.click(screen.getByRole('button', { name: /add to calendar/i }))
    return user
  }

  // The refusal is OWNED here rather than in the caller's markup,
  // so it is asserted here: an appointment for a date that has passed argues
  // with the page's PAST SHOW stripe from inside the reader's calendar app.
  it('renders nothing at all for a past show', () => {
    const { container } = render(
      <ShowAddToCalendar show={makeShow()} lifecycle="past" />
    )

    expect(
      screen.queryByRole('button', { name: /add to calendar/i })
    ).not.toBeInTheDocument()
    expect(container).toBeEmptyDOMElement()
  })

  // Only PAST withdraws it. A show tonight is still a show a reader can plan
  // to be at, and the lifecycle's `today` spans the whole venue-local day.
  it.each(['upcoming', 'today'] as const)(
    'still offers the calendar on a %s show',
    lifecycle => {
      render(<ShowAddToCalendar show={makeShow()} lifecycle={lifecycle} />)

      expect(
        screen.getByRole('button', { name: /add to calendar/i })
      ).toBeInTheDocument()
    }
  )

  it('renders both calendar actions with correct targets once opened', async () => {
    await openPopover()

    expect(
      screen.getByRole('link', { name: /apple \/ outlook/i })
    ).toHaveAttribute('href', ICS_URL)
    const google = screen.getByRole('link', { name: /google calendar/i })
    expect(google.getAttribute('href')).toContain(
      'https://calendar.google.com/calendar/render?action=TEMPLATE'
    )
    expect(google).toHaveAttribute('target', '_blank')
  })

  // The whole point: the calendar path is never auth-gated.
  it('anonymous viewers get a sign-in hint and no checkbox', async () => {
    await openPopover()

    expect(screen.getByText(/sign in to save shows/i)).toBeInTheDocument()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('signed-in viewers get the save checkbox CHECKED by default', async () => {
    mockAuth.isAuthenticated = true
    mockAuth.user = { id: 9 }
    mockSaveCount.value = { save_count: 3, is_saved: false }

    await openPopover()

    expect(screen.getByRole('checkbox', { name: /also save this show/i })).toBeChecked()
    expect(screen.queryByText(/sign in to save/i)).not.toBeInTheDocument()
  })

  it('clicking a calendar action saves the show when the box is checked', async () => {
    mockAuth.isAuthenticated = true
    mockAuth.user = { id: 9 }
    mockSaveCount.value = { save_count: 3, is_saved: false }

    const user = await openPopover()
    const google = screen.getByRole('link', { name: /google calendar/i })
    google.addEventListener('click', e => e.preventDefault())
    await user.click(google)

    expect(saveMutate).toHaveBeenCalledWith(42)
  })

  it('does not save when the box is unchecked', async () => {
    mockAuth.isAuthenticated = true
    mockAuth.user = { id: 9 }
    mockSaveCount.value = { save_count: 3, is_saved: false }

    const user = await openPopover()
    await user.click(screen.getByRole('checkbox', { name: /also save this show/i }))
    const ics = screen.getByRole('link', { name: /apple \/ outlook/i })
    ics.addEventListener('click', e => e.preventDefault())
    await user.click(ics)

    expect(saveMutate).not.toHaveBeenCalled()
  })

  it('hides the checkbox and never re-saves an already-saved show', async () => {
    mockAuth.isAuthenticated = true
    mockAuth.user = { id: 9 }
    mockSaveCount.value = { save_count: 3, is_saved: true }

    const user = await openPopover()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()

    const google = screen.getByRole('link', { name: /google calendar/i })
    google.addEventListener('click', e => e.preventDefault())
    await user.click(google)
    expect(saveMutate).not.toHaveBeenCalled()
  })
})
