import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ALERTS_AREA_HREF } from '@/components/shared/followAlertChoices'
import { LibraryUpdates } from './LibraryUpdates'

const NOW = new Date('2026-11-03T18:00:00Z')

const mockAuthStatus = vi.fn<() => 'pending' | 'authenticated' | 'anonymous'>(
  () => 'authenticated'
)
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => {
    const authStatus = mockAuthStatus()
    return {
      authStatus,
      isAuthenticated: authStatus === 'authenticated',
      user: authStatus === 'authenticated' ? { id: '42' } : null,
    }
  },
}))

// One stand-in for the network, answering by path, so the real hooks (the
// `types` read, its select, mark-read, the follow counts and the home area)
// run as they do in the app.
let notifications: unknown[] | null = []
let notificationsFail = false
let artistFollows = 0
let homeMetro: string | null = '38060'
const apiRequest = vi.fn(async (url: string, init?: RequestInit) => {
  if (url.includes('/me/notifications/mark-read')) {
    return { updated_count: 1, unread_count: 0 }
  }
  if (url.includes('/me/notifications?')) {
    if (notificationsFail) throw new Error('boom')
    return { notifications, unread_count: 7 }
  }
  if (url.endsWith('/me/library/following/counts')) {
    return { artists: artistFollows, venues: 0, scenes: 0, labels: 0, festivals: 0, tags: 0 }
  }
  if (url.endsWith('/auth/preferences/alerts')) {
    return {
      success: true,
      home_metro: homeMetro,
      alert_defaults: {
        shows: { in_app: true, email: false },
        releases: { in_app: true, email: false },
      },
    }
  }
  throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`)
})
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (url: string, init?: RequestInit) => apiRequest(url, init),
}))

function row(id: number, overrides: Record<string, unknown>) {
  return {
    id,
    entity_id: id,
    channel: 'in_app',
    sent_at: '2026-11-01T18:00:00Z',
    read_at: '2026-11-02T00:00:00Z',
    ...overrides,
  }
}

const ARTIST_UNREAD = row(1, {
  entity_type: 'artist_show_alert',
  read_at: undefined,
  alert_artist_name: 'Dan Deacon',
  alert_show_venue_name: 'Walter Studios',
  alert_show_event_date: '2026-11-21T03:00:00Z',
  alert_show_timezone: 'America/Phoenix',
  alert_show_url: '/shows/dan-deacon',
})
const VENUE_READ = row(2, {
  entity_type: 'venue_show_alert',
  sent_at: '2026-10-30T18:00:00Z',
  alert_venue_name: 'Walter Studios',
  alert_venue_url: '/venues/walter-studios',
  alert_show_count: 3,
  alert_show_first_date: '2026-11-15T03:00:00Z',
  alert_show_last_date: '2026-11-23T03:00:00Z',
  alert_show_timezone: 'America/Phoenix',
})
const DIGEST_UNREAD = row(4, {
  entity_type: 'artist_release_digest',
  // The reader's own user id.
  entity_id: 42,
  sent_at: '2026-11-02T18:00:00Z',
  read_at: undefined,
  alert_release_count: 3,
  alert_release_artist_count: 3,
  alert_release_artists: 'Dan Deacon, Primitive Man, Russian Circles',
})
const COMMENT = row(3, { entity_type: 'comment_reply' })

function markReadCalls() {
  return apiRequest.mock.calls.filter(([url]) => url.includes('/mark-read'))
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(NOW)
  apiRequest.mockClear()
  mockAuthStatus.mockReturnValue('authenticated')
  notifications = []
  notificationsFail = false
  artistFollows = 0
  homeMetro = '38060'
})

afterEach(() => {
  vi.useRealTimers()
})

describe('LibraryUpdates', () => {
  it('asks the inbox for the update kinds only', async () => {
    notifications = [ARTIST_UNREAD]
    renderWithProviders(<LibraryUpdates />)

    await screen.findByRole('heading', { name: 'From your library' })
    const [url] = apiRequest.mock.calls[0]
    expect(url).toContain('/me/notifications?limit=5&offset=0&types=artist_show_alert%2Cvenue_show_alert%2Cartist_release_digest')
  })

  it('renders each update as one row, newest first, with a dot on unread rows only', async () => {
    notifications = [ARTIST_UNREAD, COMMENT, VENUE_READ]
    renderWithProviders(<LibraryUpdates />)

    const rows = await screen.findAllByRole('listitem')
    // The comment row is a kind this module does not know, so it is skipped.
    expect(rows).toHaveLength(2)

    const artist = within(rows[0]).getByRole('link')
    expect(artist).toHaveAttribute('href', '/shows/dan-deacon')
    expect(artist).toHaveTextContent('New show')
    expect(artist).toHaveTextContent('Dan Deacon · Walter Studios')
    expect(artist).toHaveTextContent('Fri Nov 20 · announced 2d ago')
    expect(within(artist).getByText('Unread')).toBeInTheDocument()

    const venue = within(rows[1]).getByRole('link')
    expect(venue).toHaveAttribute('href', '/venues/walter-studios')
    expect(venue).toHaveTextContent('3 shows')
    expect(venue).toHaveTextContent('Walter Studios · new shows')
    expect(venue).toHaveTextContent('Nov 14 to Nov 22 · 4d ago')
    expect(within(venue).queryByText('Unread')).not.toBeInTheDocument()
  })

  it('renders the weekly release roundup from its merged fields', async () => {
    notifications = [DIGEST_UNREAD]
    renderWithProviders(<LibraryUpdates />)

    const digest = within(await screen.findByRole('listitem')).getByRole('link')
    expect(digest).toHaveAttribute('href', '/library')
    expect(digest).toHaveTextContent('3 releases')
    expect(digest).toHaveTextContent('Dan Deacon, Primitive Man, Russian Circles')
    expect(digest).toHaveTextContent('added releases last week · 1d ago')
    expect(within(digest).getByText('Unread')).toBeInTheDocument()
  })

  it('caps the list at five rows', async () => {
    notifications = Array.from({ length: 7 }, (_, index) =>
      row(index + 10, { ...ARTIST_UNREAD, id: index + 10 })
    )
    renderWithProviders(<LibraryUpdates />)

    expect(await screen.findAllByRole('listitem')).toHaveLength(5)
  })

  it('links every update to the inbox in the link colour', async () => {
    notifications = [VENUE_READ]
    renderWithProviders(<LibraryUpdates />)

    const all = await screen.findByRole('link', { name: 'All updates →' })
    expect(all).toHaveAttribute('href', '/notifications')
    expect(all.className).toContain('text-link')
  })

  it('marks THAT notification read when an unread row is tapped', async () => {
    const user = userEvent.setup()
    notifications = [ARTIST_UNREAD, VENUE_READ]
    renderWithProviders(<LibraryUpdates />)

    const rows = await screen.findAllByRole('listitem')
    await user.click(within(rows[0]).getByRole('link'))

    await waitFor(() => expect(markReadCalls()).toHaveLength(1))
    const [, init] = markReadCalls()[0]
    expect(init).toMatchObject({ method: 'POST', body: JSON.stringify({ ids: [1] }) })
    // The module's own entry sits under the notification-log prefix, so the
    // mark-read refreshes it and the dot can clear.
    await waitFor(() =>
      expect(
        apiRequest.mock.calls.filter(([url]) => url.includes('/me/notifications?'))
      ).toHaveLength(2)
    )
  })

  it('sends no mark-read for a row that is already read', async () => {
    const user = userEvent.setup()
    notifications = [VENUE_READ]
    renderWithProviders(<LibraryUpdates />)

    await user.click(within(await screen.findByRole('listitem')).getByRole('link'))

    expect(markReadCalls()).toHaveLength(0)
  })

  describe('with no updates', () => {
    it('renders nothing, and asks nothing about the home area, for a viewer who follows no artist', async () => {
      const { container } = renderWithProviders(<LibraryUpdates />)

      await waitFor(() =>
        expect(apiRequest.mock.calls.some(([url]) => url.endsWith('/counts'))).toBe(true)
      )
      expect(container).toBeEmptyDOMElement()
      expect(
        apiRequest.mock.calls.some(([url]) => url.endsWith('/auth/preferences/alerts'))
      ).toBe(false)
    })

    it('renders nothing for a viewer who follows artists and has a home area', async () => {
      artistFollows = 3
      const { container } = renderWithProviders(<LibraryUpdates />)

      await waitFor(() =>
        expect(
          apiRequest.mock.calls.some(([url]) => url.endsWith('/auth/preferences/alerts'))
        ).toBe(true)
      )
      expect(container).toBeEmptyDOMElement()
    })

    it('says how to get alerts when the viewer follows artists but has no home area', async () => {
      artistFollows = 3
      homeMetro = null
      renderWithProviders(<LibraryUpdates />)

      const prompt = await screen.findByRole('link', {
        name: 'Set your area to see new shows from artists you follow →',
      })
      expect(prompt).toHaveAttribute('href', ALERTS_AREA_HREF)
      expect(screen.getByRole('heading', { name: 'From your library' })).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'All updates →' })).not.toBeInTheDocument()
    })

    it('treats rows of unknown kinds as no rows', async () => {
      notifications = [COMMENT]
      artistFollows = 1
      homeMetro = null
      renderWithProviders(<LibraryUpdates />)

      expect(
        await screen.findByRole('link', {
          name: 'Set your area to see new shows from artists you follow →',
        })
      ).toBeInTheDocument()
      expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
    })
  })

  it('renders nothing when the read fails', async () => {
    notificationsFail = true
    const { container } = renderWithProviders(<LibraryUpdates />)

    await waitFor(() => expect(apiRequest).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing for a signed-out viewer', () => {
    mockAuthStatus.mockReturnValue('anonymous')
    const { container } = renderWithProviders(<LibraryUpdates />)

    expect(container).toBeEmptyDOMElement()
    expect(apiRequest).not.toHaveBeenCalled()
  })
})
