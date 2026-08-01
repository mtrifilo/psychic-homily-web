import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  VenueAddToCalendar,
  venueCalendarFeedUrl,
} from './VenueAddToCalendar'

// vitest.config.mts pins NEXT_PUBLIC_API_URL to http://localhost:8080, so
// API_BASE_URL is deterministic here.
const FEED_URL = 'http://localhost:8080/venues/the-rebel-lounge/calendar.ics'

describe('venueCalendarFeedUrl', () => {
  it('builds an absolute URL against the API origin', () => {
    expect(venueCalendarFeedUrl('the-rebel-lounge')).toBe(FEED_URL)
  })

  it('is absolute, because webcal:// and Google Calendar cannot use a relative path', () => {
    expect(venueCalendarFeedUrl('the-rebel-lounge')).toMatch(/^https?:\/\//)
  })

  it('encodes slugs so a hostile slug cannot escape the path', () => {
    expect(venueCalendarFeedUrl('a/../../evil?x=1')).toBe(
      'http://localhost:8080/venues/a%2F..%2F..%2Fevil%3Fx%3D1/calendar.ics'
    )
  })
})

function mockClipboard(writeText: ReturnType<typeof vi.fn>) {
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
    writable: true,
  })
}

function renderAddToCalendar() {
  return render(
    <VenueAddToCalendar
      venueSlug="the-rebel-lounge"
      venueName="The Rebel Lounge"
    />
  )
}

async function openPopover() {
  const user = userEvent.setup()
  renderAddToCalendar()
  await user.click(screen.getByRole('button', { name: /add to calendar/i }))
  return user
}

describe('VenueAddToCalendar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    Reflect.deleteProperty(navigator, 'clipboard')
    vi.restoreAllMocks()
  })

  it('renders an add-to-calendar trigger', () => {
    renderAddToCalendar()
    expect(screen.getByRole('button', { name: /add to calendar/i })).toBeInTheDocument()
  })

  // The whole point of the feed is that a venue can link to it and anyone can
  // subscribe, so nothing here may be gated on being signed in.
  it('shows the feed URL and calendar links once opened', async () => {
    await openPopover()

    const input = await screen.findByLabelText(
      'Calendar feed URL for The Rebel Lounge'
    )
    expect(input).toHaveValue(FEED_URL)

    expect(screen.getByRole('link', { name: /google calendar/i })).toHaveAttribute(
      'href',
      `https://calendar.google.com/calendar/r?cid=${encodeURIComponent(
        'webcal://localhost:8080/venues/the-rebel-lounge/calendar.ics'
      )}`
    )
    expect(screen.getByRole('link', { name: /apple calendar/i })).toHaveAttribute(
      'href',
      'webcal://localhost:8080/venues/the-rebel-lounge/calendar.ics'
    )
  })

  it('copies the feed URL to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    const user = await openPopover()
    mockClipboard(writeText)
    await user.click(await screen.findByRole('button', { name: /copy feed url/i }))

    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(await screen.findByRole('button', { name: /copied/i })).toBeInTheDocument()
  })

  it('survives a rejected clipboard write', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    const user = await openPopover()
    mockClipboard(writeText)
    await user.click(await screen.findByRole('button', { name: /copy feed url/i }))

    // Still shows the un-copied state, and the URL is still readable.
    expect(
      await screen.findByLabelText('Calendar feed URL for The Rebel Lounge')
    ).toHaveValue(FEED_URL)
  })

  it('renders nothing without a slug, since there is no feed to point at', () => {
    const { container } = render(
      <VenueAddToCalendar venueSlug="" venueName="Unnamed" />
    )
    expect(container).toBeEmptyDOMElement()
  })
})
