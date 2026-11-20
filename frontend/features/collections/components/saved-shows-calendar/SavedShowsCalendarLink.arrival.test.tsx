import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { renderWithProviders, screen, waitFor } from '@/test/utils'

// The link with the real panel behind it and the network stubbed, to pin the
// rule that a calendar link is only ever created by the person's own click:
// arriving at the old fragment must neither create a link nor read one.

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: 'authenticated',
    isAuthenticated: true,
    user: { id: '7', email: 'fan@example.com' },
    isLoading: false,
  }),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/library',
}))

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

import { SavedShowsCalendarLink } from './SavedShowsCalendarLink'

function method(options: unknown): string {
  return (options as { method?: string } | undefined)?.method ?? 'GET'
}

/** Requests that read or create the link itself, the address included. */
function linkRequests() {
  return apiRequest.mock.calls
    .filter(([endpoint]) => String(endpoint).endsWith('/calendar/token'))
    .map(([, options]) => method(options))
}

beforeEach(() => {
  apiRequest.mockReset()
  apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
    const url = String(endpoint)
    const verb = method(options)
    // The status, which carries no address, is what the label reads.
    if (url.endsWith('/calendar/token/feed-status')) {
      return { has_token: false, reads: [] }
    }
    if (url.endsWith('/calendar/token') && verb === 'GET') {
      return { has_token: false }
    }
    if (url.endsWith('/calendar/token') && verb === 'POST') {
      return {
        token: 'secret-token-abc',
        feed_url: 'https://api.example.test/calendar/feed/secret-token-abc.ics',
        follows_feed_url: 'https://api.example.test/follows.atom',
        created_at: new Date().toISOString(),
      }
    }
    if (url.includes('/saved-shows?')) {
      return { shows: [], total: 0, limit: 1, offset: 0 }
    }
    throw new Error(`unexpected request ${verb} ${url}`)
  })
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36'
  )
  vi.mocked(window.matchMedia).mockImplementation(
    (query: string) =>
      ({
        matches: true,
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }) as unknown as MediaQueryList
  )
})

afterEach(() => {
  window.history.replaceState(null, '', '/')
})

describe('SavedShowsCalendarLink: arriving at the old fragment', () => {
  it('focuses the link and neither reads nor creates a calendar link until it is clicked', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(Element.prototype, 'scrollIntoView')
    window.history.replaceState(null, '', '/library#calendar-feed')

    renderWithProviders(
      <SavedShowsCalendarLink placement="section-header" focusesFromHash />
    )
    const link = screen.getByRole('button', { name: 'Add to calendar' })

    expect(link).toHaveFocus()
    expect(link).toHaveAttribute('aria-expanded', 'false')
    // The status read has run, and settled, by the time this holds.
    await waitFor(() =>
      expect(
        apiRequest.mock.calls.some(([endpoint]) =>
          String(endpoint).endsWith('/calendar/token/feed-status')
        )
      ).toBe(true)
    )
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(linkRequests()).toEqual([])
    expect(screen.queryByTestId('saved-shows-calendar-panel')).toBeNull()

    // The person's click is what prepares the link: read, then created
    // because this account has none.
    await user.click(link)

    // The panel's code is imported on first open; under Vitest that first
    // import is transformed on demand and can take over a second.
    expect(
      await screen.findByTestId(
        'saved-shows-calendar-panel',
        {},
        { timeout: 10_000 }
      )
    ).toBeInTheDocument()
    await waitFor(() => expect(linkRequests()).toEqual(['GET', 'POST']))
    scrollIntoView.mockRestore()
  })
})
