import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { focusManager } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'
import type {
  CalendarFeedRead,
  CalendarFeedStatusResponse,
  CalendarTokenStatusResponse,
} from '@/features/shows/types'
import { CALENDAR_STATUS_POLL_MS } from '@/features/auth/hooks/useCalendarFeed'
import { GOOGLE_CALENDAR_ADD_BY_URL_PAGE } from '@/lib/utils/calendarFeedUrls'
import { CalendarStepsPage } from './CalendarStepsPage'
import { STEPS_PAGE_COPY } from './calendarStepsPageCopy'
import { PANEL_COPY, RESET_SUB } from './savedShowsCalendarCopy'
import { GOOGLE_CHECK_POLL_LIMIT_MS } from './useCalendarStepsPage'

// The page against stubbed endpoints: the real guard, hooks and reads, with
// the network, the viewer, the device and the clipboard faked.

const auth = vi.hoisted(() => ({
  status: 'authenticated' as 'authenticated' | 'anonymous' | 'pending',
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: auth.status,
    isAuthenticated: auth.status === 'authenticated',
    user:
      auth.status === 'authenticated'
        ? { id: '7', email: 'fan@example.com' }
        : null,
    isLoading: auth.status === 'pending',
  }),
}))

const redirect = vi.hoisted(() => vi.fn())
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => '/library/calendar',
  redirect: (href: string) => redirect(href),
}))

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

const FEED_URL = 'https://api.example.test/calendar/feed/secret-token-abc.ics'
const WEBCAL_URL = 'webcal://api.example.test/calendar/feed/secret-token-abc.ics'
const NEW_FEED_URL = 'https://api.example.test/calendar/feed/secret-token-new.ics'

const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const ago = (ms: number) => new Date(Date.now() - ms).toISOString()

const UA = {
  mac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
  windows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
  iphoneSafari:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
  iphoneChrome:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/131.0.6778.73 Mobile/15E148 Safari/604.1',
  android:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36',
} as const

interface Server {
  /** GET .../feed-status; an Error rejects, a promise holds the answer. */
  status: CalendarFeedStatusResponse | Error | Promise<CalendarFeedStatusResponse>
  /** GET /calendar/token; an Error rejects, a promise holds the answer. */
  token: CalendarTokenStatusResponse | Error | Promise<CalendarTokenStatusResponse>
  /** POST /calendar/token. */
  mint: () => Promise<unknown>
}

let server: Server

function method(options: unknown): string {
  return (options as { method?: string } | undefined)?.method ?? 'GET'
}

function calls(fragment: string, verb: string) {
  return apiRequest.mock.calls.filter(
    ([endpoint, options]) =>
      String(endpoint).endsWith(fragment) && method(options) === verb
  )
}

const statusReads = () => calls('/calendar/token/feed-status', 'GET').length

const linkOf = (feedUrl: string): CalendarTokenStatusResponse => ({
  has_token: true,
  url_recoverable: true,
  feed_url: feedUrl,
})

function withLink(reads: CalendarFeedRead[] = []) {
  server.status = { has_token: true, reads }
  server.token = linkOf(FEED_URL)
}

const readOf = (
  client: CalendarFeedRead['client'],
  lastAgo: number
): CalendarFeedRead => ({
  client,
  first_read_at: ago(lastAgo + DAY),
  last_read_at: ago(lastAgo),
})

const writeText = vi.fn()

beforeEach(() => {
  auth.status = 'authenticated'
  redirect.mockReset()
  apiRequest.mockReset()
  server = {
    status: { has_token: false },
    token: { has_token: false },
    mint: async () => {
      server.token = linkOf(FEED_URL)
      server.status = { has_token: true, reads: [] }
      return {
        token: 'secret-token-abc',
        feed_url: FEED_URL,
        follows_feed_url: 'https://api.example.test/follows.atom',
        created_at: ago(0),
      }
    },
  }
  apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
    const url = String(endpoint)
    const verb = method(options)
    if (url.endsWith('/calendar/token/feed-status')) {
      if (server.status instanceof Error) throw server.status
      return server.status
    }
    if (url.endsWith('/calendar/token') && verb === 'GET') {
      if (server.token instanceof Error) throw server.token
      return server.token
    }
    if (url.endsWith('/calendar/token') && verb === 'POST') return server.mint()
    throw new Error(`unexpected request ${verb} ${url}`)
  })
  writeText.mockReset().mockResolvedValue(undefined)
  useDevice(UA.mac)
})

afterEach(() => {
  focusManager.setFocused(undefined)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

function useDevice(agent: string) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(agent)
}

/** userEvent installs its own clipboard, so the recording stub goes in after. */
function setupUser() {
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  return user
}

function renderPage(client = createTestQueryClient()) {
  return render(<CalendarStepsPage />, {
    wrapper: createWrapperWithClient(client),
  })
}

/** The computer page once its reads are in and the link is ready. */
async function renderReadyComputerPage() {
  renderPage()
  return screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
}

/** The window regains focus, as when the reader comes back from Google's tab. */
function refocusWindow() {
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
}

describe('CalendarStepsPage: signed out', () => {
  it('sends the reader to sign in, returning to this page', () => {
    auth.status = 'anonymous'
    renderPage()
    expect(redirect).toHaveBeenCalledWith('/auth?returnTo=%2Flibrary%2Fcalendar')
    expect(apiRequest).not.toHaveBeenCalled()
  })

  it('waits, with no redirect, while sign-in is unsettled', () => {
    auth.status = 'pending'
    renderPage()
    expect(redirect).not.toHaveBeenCalled()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })
})

describe('CalendarStepsPage: on a computer', () => {
  it('shows the heading and the three numbered steps in order', async () => {
    withLink()
    await renderReadyComputerPage()

    expect(
      screen.getByRole('heading', { level: 1, name: STEPS_PAGE_COPY.heading })
    ).toBeInTheDocument()
    const steps = within(screen.getByRole('list')).getAllByRole('listitem')
    expect(steps.map(step => step.textContent)).toEqual([
      `1.${STEPS_PAGE_COPY.stepCopy}${STEPS_PAGE_COPY.copyButton}`,
      `2.${STEPS_PAGE_COPY.stepOpen}${STEPS_PAGE_COPY.openButton}`,
      `3.${STEPS_PAGE_COPY.stepPaste}`,
    ])
    // The numbers are text a screen reader reads.
    expect(within(steps[0]).getByText('1.')).not.toHaveAttribute('aria-hidden')
  })

  it("step 2 opens Google's Add from URL page in a new tab", async () => {
    withLink()
    await renderReadyComputerPage()

    const open = screen.getByRole('link', { name: STEPS_PAGE_COPY.openButton })
    expect(open).toHaveAttribute('href', GOOGLE_CALENDAR_ADD_BY_URL_PAGE)
    expect(open).toHaveAttribute('target', '_blank')
    expect(open).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('Copy link copies the address in the click and reads Copied', async () => {
    withLink()
    const user = setupUser()
    const copy = await renderReadyComputerPage()

    await user.click(copy)

    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copiedButton })
    ).toBeInTheDocument()
  })

  it('never shows the address, and marks everything that carries it', async () => {
    withLink()
    const user = setupUser()
    await user.click(await renderReadyComputerPage())
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.copiedButton })

    expect(document.body.textContent).not.toContain('secret-token-abc')
    expect(
      screen.getByRole('button', { name: STEPS_PAGE_COPY.copiedButton })
    ).toHaveClass('ph-no-capture', 'sentry-block')
    const carriers = Array.from(document.querySelectorAll('a')).filter(anchor =>
      anchor.href.includes('secret-token-abc')
    )
    expect(carriers.length).toBeGreaterThan(0)
    for (const anchor of carriers) {
      expect(anchor).toHaveClass('ph-no-capture', 'sentry-block')
    }
  })

  it('offers the other calendars under the steps: Apple on a Mac, and both Outlooks', async () => {
    withLink()
    await renderReadyComputerPage()

    expect(
      screen.getByText(STEPS_PAGE_COPY.otherCalendarsLead, { exact: false })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.rowApple })).toHaveAttribute(
      'href',
      WEBCAL_URL
    )
    const outlookCom = screen.getByRole('link', { name: PANEL_COPY.rowOutlookCom })
    expect(outlookCom.getAttribute('href')).toMatch(
      /^https:\/\/outlook\.live\.com\/calendar\/0\/addfromweb\/\?url=webcal/
    )
    expect(outlookCom).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('link', { name: PANEL_COPY.rowOutlookWork }).getAttribute('href')
    ).toMatch(/^https:\/\/outlook\.office\.com\/calendar\/0\/addfromweb\/\?url=webcal/)
  })

  it('offers no Apple Calendar on a computer that has none', async () => {
    useDevice(UA.windows)
    withLink()
    await renderReadyComputerPage()

    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.rowOutlookCom })).toBeInTheDocument()
  })

  it('offers no Apple Calendar link when Apple Calendar already checks the link', async () => {
    withLink([readOf('apple_mac', HOUR)])
    await renderReadyComputerPage()

    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
  })

  it("waits for Google's first check, and turns to the connected block once it is seen", async () => {
    withLink()
    await renderReadyComputerPage()
    expect(screen.getByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.done)).not.toBeInTheDocument()

    server.status = { has_token: true, reads: [readOf('google', 0)] }
    refocusWindow()

    expect(await screen.findByText(STEPS_PAGE_COPY.done)).toBeInTheDocument()
    expect(screen.getByText('Google Calendar', { exact: true })).toBeInTheDocument()
    expect(screen.getByText('Checked just now.')).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
  })

  it('shows the connected block on arrival when Google already checks the link', async () => {
    withLink([readOf('google', 2 * HOUR)])
    await renderReadyComputerPage()

    expect(screen.getByText('Checked 2h ago.')).toBeInTheDocument()
    expect(screen.getByText(STEPS_PAGE_COPY.done)).toBeInTheDocument()
  })

  it('a Google calendar that stopped shows its stopped line, not the waiting line', async () => {
    withLink([readOf('google', 12 * DAY)])
    await renderReadyComputerPage()

    expect(
      screen.getByText(
        'Google Calendar last checked 12 days ago. It may have been removed there.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
    // Plain text: no link or action of its own.
    expect(screen.queryByRole('button', { name: PANEL_COPY.removedIt })).not.toBeInTheDocument()

    server.status = { has_token: true, reads: [readOf('google', 0)] }
    refocusWindow()
    expect(await screen.findByText(STEPS_PAGE_COPY.done)).toBeInTheDocument()
  })

  it('a status cached before the page opened is never shown, even while its new read is out', async () => {
    withLink()
    let finishRead: (status: CalendarFeedStatusResponse) => void = () => {}
    server.status = new Promise(resolve => {
      finishRead = resolve
    })
    const client = createTestQueryClient()
    // Read by another surface a minute before the page opened.
    client.setQueryData(
      ['calendar', 'feedStatus'],
      { has_token: true, reads: [readOf('google', 0)] },
      { updatedAt: Date.now() - MINUTE }
    )
    renderPage(client)

    await screen.findByRole('heading', { level: 1 })
    await waitFor(() => expect(statusReads()).toBe(1))
    expect(screen.queryByText(STEPS_PAGE_COPY.done)).not.toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    // The panel's line for the same wait.
    expect(screen.getByText(PANEL_COPY.loading)).toBeInTheDocument()

    await act(async () => finishRead({ has_token: true, reads: [] }))
    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.done)).not.toBeInTheDocument()
  })
})

describe('CalendarStepsPage: with no link', () => {
  it('arriving makes no link: one Add to calendar button, and no steps', async () => {
    renderPage()

    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    ).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.otherCalendarsLead, { exact: false })).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('a link cached before the page opened is never offered, even while its new read is out', async () => {
    let finishRead: (token: CalendarTokenStatusResponse) => void = () => {}
    server.token = new Promise(resolve => {
      finishRead = resolve
    })
    const client = createTestQueryClient()
    // Read by another surface a minute before the page opened.
    client.setQueryData(['calendar', 'tokenStatus'], linkOf(FEED_URL), {
      updatedAt: Date.now() - MINUTE,
    })
    renderPage(client)

    await screen.findByRole('heading', { level: 1 })
    await waitFor(() => expect(calls('/calendar/token', 'GET')).toHaveLength(1))
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })).not.toBeInTheDocument()

    await act(async () => finishRead({ has_token: false }))
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })).not.toBeInTheDocument()
  })

  it('the click reads the link again first, and makes none when one exists by then', async () => {
    const user = setupUser()
    renderPage()
    const button = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })

    // Made elsewhere (another device or tab) after the page read "no link".
    withLink()
    await user.click(button)

    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('the click makes exactly one link, and the steps act on it', async () => {
    const user = setupUser()
    renderPage()

    await user.click(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    )

    const copy = await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })).not.toBeInTheDocument()
    await user.click(copy)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
  })

  it('the button waits while the link is made', async () => {
    let finishMint: () => void = () => {}
    const mint = server.mint
    server.mint = () =>
      new Promise(resolve => {
        finishMint = () => resolve(mint())
      })
    const user = setupUser()
    renderPage()

    const button = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    await user.click(button)
    expect(button).toBeDisabled()

    await act(async () => finishMint())
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).toBeInTheDocument()
  })

  it('a failed create says so under the button, and "Try again." makes it', async () => {
    server.mint = async () => {
      throw new Error('502')
    }
    const user = setupUser()
    renderPage()

    await user.click(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    )
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.createFailed)

    server.mint = async () => {
      server.token = linkOf(FEED_URL)
      server.status = { has_token: true, reads: [] }
      return { token: 't', feed_url: FEED_URL, follows_feed_url: 'f', created_at: ago(0) }
    }
    await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(2)
  })

  it.each([
    ['sign_out_everywhere'],
    ['password_reset'],
  ] as const)(
    'a link reset by %s says why above the button, and still does after the click',
    async reason => {
      server.status = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: reason,
      }
      server.mint = async () => {
        server.token = linkOf(FEED_URL)
        // Making the link clears the marker on the server.
        server.status = { has_token: true, reads: [] }
        return { token: 't', feed_url: FEED_URL, follows_feed_url: 'f', created_at: ago(0) }
      }
      const user = setupUser()
      renderPage()

      const button = await screen.findByRole('button', {
        name: STEPS_PAGE_COPY.addToCalendar,
      })
      expect(
        screen.getByRole('heading', { level: 2, name: PANEL_COPY.headingReset })
      ).toBeInTheDocument()
      expect(screen.getByText(RESET_SUB[reason])).toBeInTheDocument()

      await user.click(button)
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
      await waitFor(() =>
        expect(calls('/calendar/token/feed-status', 'GET').length).toBeGreaterThan(1)
      )
      expect(screen.getByText(RESET_SUB[reason])).toBeInTheDocument()
    }
  )

  it('a link turned off while the page is open falls back to the one button', async () => {
    withLink()
    await renderReadyComputerPage()

    server.token = { has_token: false }
    server.status = { has_token: false }
    refocusWindow()

    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('a link replaced after a copy no longer says Copied, and the button keeps focus', async () => {
    withLink()
    const user = setupUser()
    const button = await renderReadyComputerPage()
    await user.click(button)
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.copiedButton })
    expect(button).toHaveFocus()

    server.token = linkOf(NEW_FEED_URL)
    refocusWindow()

    await waitFor(() => expect(button).toHaveTextContent(STEPS_PAGE_COPY.copyButton))
    expect(screen.getByRole('button', { name: STEPS_PAGE_COPY.copyButton })).toBe(button)
    expect(button).toHaveFocus()
  })

  it('a link replaced while the page is open is the one Copy link copies', async () => {
    withLink()
    const user = setupUser()
    await renderReadyComputerPage()

    server.token = linkOf(NEW_FEED_URL)
    refocusWindow()
    await waitFor(() => expect(calls('/calendar/token', 'GET').length).toBeGreaterThan(1))

    await user.click(screen.getByRole('button', { name: STEPS_PAGE_COPY.copyButton }))
    await waitFor(() => expect(writeText).toHaveBeenLastCalledWith(NEW_FEED_URL))
  })
})

describe('CalendarStepsPage: reading the status on an interval', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  async function advance(ms: number) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms)
    })
  }

  it('reads on an interval while waiting, and stops once Google is connected', async () => {
    withLink()
    await renderReadyComputerPage()
    const first = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS + 1_000)
    await waitFor(() => expect(statusReads()).toBe(first + 1))

    server.status = { has_token: true, reads: [readOf('google', 0)] }
    await advance(CALENDAR_STATUS_POLL_MS + 1_000)
    expect(await screen.findByText(STEPS_PAGE_COPY.done)).toBeInTheDocument()
    const connected = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(connected)
  })

  it('does not read on an interval while the account has no link', async () => {
    renderPage()
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    const first = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(first)
  })

  it('stops reading on an interval after its time limit, and keeps the waiting line', async () => {
    withLink()
    await renderReadyComputerPage()

    await advance(GOOGLE_CHECK_POLL_LIMIT_MS + CALENDAR_STATUS_POLL_MS * 2)
    const atLimit = statusReads()
    expect(atLimit).toBeGreaterThan(2)

    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(atLimit)
    expect(screen.getByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
  })

  it('a link made long after arrival is still waited on at the interval', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    renderPage()
    const button = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    await advance(GOOGLE_CHECK_POLL_LIMIT_MS * 2)

    await user.click(button)
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    await waitFor(() => expect(calls('/calendar/token', 'POST')).toHaveLength(1))
    const afterCreate = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS * 2 + 2_000)
    await waitFor(() => expect(statusReads()).toBeGreaterThan(afterCreate + 1))
  })

  it('a link turned off elsewhere is noticed at the next interval read, with the page in view', async () => {
    withLink()
    await renderReadyComputerPage()

    server.token = { has_token: false }
    server.status = { has_token: false }
    await advance(CALENDAR_STATUS_POLL_MS + 1_000)

    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })).not.toBeInTheDocument()

    // Nothing to wait on: the interval stops.
    const afterTurnOff = statusReads()
    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(afterTurnOff)
  })

  it('a link replaced elsewhere is noticed at the next interval read, and the click makes none', async () => {
    const madeBefore = ago(DAY)
    const madeSince = ago(0)
    server.token = { ...linkOf(FEED_URL), created_at: madeBefore }
    server.status = { has_token: true, created_at: madeBefore, reads: [] }
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await renderReadyComputerPage()

    server.token = { ...linkOf(NEW_FEED_URL), created_at: madeSince }
    server.status = { has_token: true, created_at: madeSince, reads: [] }
    await advance(CALENDAR_STATUS_POLL_MS + 1_000)

    const add = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    await user.click(add)
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('after its time limit, coming back to the page reads on the interval again', async () => {
    withLink()
    await renderReadyComputerPage()
    await advance(GOOGLE_CHECK_POLL_LIMIT_MS + CALENDAR_STATUS_POLL_MS * 2)
    const atLimit = statusReads()
    await advance(CALENDAR_STATUS_POLL_MS * 2)
    expect(statusReads()).toBe(atLimit)

    refocusWindow()
    await waitFor(() => expect(statusReads()).toBe(atLimit + 1))
    await advance(CALENDAR_STATUS_POLL_MS + 1_000)
    await waitFor(() => expect(statusReads()).toBe(atLimit + 2))
  })

  it('an iPhone never reads on an interval', async () => {
    useDevice(UA.iphoneSafari)
    withLink()
    renderPage()
    await screen.findByRole('link', { name: PANEL_COPY.rowApple })
    const first = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(first)
  })

  it('stops reading when the page unmounts', async () => {
    withLink()
    const { unmount } = renderPage()
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    unmount()
    const atUnmount = statusReads()

    await advance(CALENDAR_STATUS_POLL_MS * 4)
    expect(statusReads()).toBe(atUnmount)
  })
})

describe('CalendarStepsPage: failures, with the panel lines', () => {
  it.each([
    ['the link', () => (server.token = new Error('502'))],
    ['the status', () => (server.status = new Error('502'))],
    [
      'both',
      () => {
        server.token = new Error('502')
        server.status = new Error('502')
      },
    ],
  ] as const)(
    'a failed read of %s shows one load-failure line and nothing else',
    async (_what, fail) => {
      withLink()
      fail()
      const user = setupUser()
      renderPage()

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent(PANEL_COPY.loadFailed)
      expect(screen.getAllByRole('alert')).toHaveLength(1)
      expect(screen.queryByRole('list')).not.toBeInTheDocument()
      expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)

      withLink()
      await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
      expect(
        await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
      ).toBeInTheDocument()
    }
  )

  it('a refused copy says so and points to Settings', async () => {
    withLink()
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const user = setupUser()
    await user.click(await renderReadyComputerPage())

    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.copyFailed)
    expect(
      screen.getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
  })

  it('an old link that cannot be shown points to Settings, with no steps', async () => {
    server.token = { has_token: true, url_recoverable: false }
    server.status = { has_token: true, reads: [] }
    renderPage()

    expect(await screen.findByText(PANEL_COPY.unrecoverableNoApp)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('an old link with a calendar connected names it, and warns that a new link disconnects it', async () => {
    server.token = { has_token: true, url_recoverable: false }
    server.status = {
      has_token: true,
      reads: [readOf('google', HOUR), readOf('apple_mac', 12 * DAY)],
    }
    renderPage()

    expect(await screen.findByText(PANEL_COPY.unrecoverable)).toBeInTheDocument()
    expect(screen.getByText('Google Calendar', { exact: true })).toBeInTheDocument()
    expect(screen.getByText('Checked 1h ago.')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Apple Calendar (Mac) last checked 12 days ago. It may have been removed there.'
      )
    ).toBeInTheDocument()
  })
})

describe('CalendarStepsPage: on an iPhone', () => {
  it('Safari: needs a computer, and offers Apple Calendar with the Google app hint', async () => {
    useDevice(UA.iphoneSafari)
    withLink()
    renderPage()

    const apple = await screen.findByRole('link', { name: PANEL_COPY.rowApple })
    expect(apple).toHaveAttribute('href', WEBCAL_URL)
    expect(apple).toHaveClass('ph-no-capture', 'sentry-block', 'min-h-11')
    expect(screen.getByText(STEPS_PAGE_COPY.phoneNeedsComputer)).toBeInTheDocument()
    expect(screen.getByText(STEPS_PAGE_COPY.iphoneAppleInstead)).toBeInTheDocument()
    expect(screen.getByText(PANEL_COPY.iphoneGoogleAppHint)).toBeInTheDocument()
    // No Google add control on a phone.
    expect(screen.queryByRole('link', { name: STEPS_PAGE_COPY.openButton })).not.toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('with no link: the one button where the Apple Calendar link goes, and the click makes it', async () => {
    useDevice(UA.iphoneSafari)
    const user = setupUser()
    renderPage()

    const button = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    expect(screen.getByText(STEPS_PAGE_COPY.iphoneAppleInstead)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)

    await user.click(button)
    expect(await screen.findByRole('link', { name: PANEL_COPY.rowApple })).toHaveAttribute(
      'href',
      WEBCAL_URL
    )
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
  })

  it('outside Safari: the line to open Safari and Copy link, in place of the hint', async () => {
    useDevice(UA.iphoneChrome)
    withLink()
    const user = setupUser()
    renderPage()

    expect(await screen.findByText(PANEL_COPY.iphoneAppleNotSafari)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
    expect(screen.queryByText(PANEL_COPY.iphoneGoogleAppHint)).not.toBeInTheDocument()

    const copy = screen.getByRole('button', { name: PANEL_COPY.rowCopy })
    expect(copy).toHaveClass('ph-no-capture', 'sentry-block')
    await user.click(copy)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(await screen.findByText(PANEL_COPY.copied)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('secret-token-abc')
  })

  it('outside Safari, a refused copy says so and points to Settings', async () => {
    useDevice(UA.iphoneChrome)
    withLink()
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const user = setupUser()
    renderPage()

    await user.click(await screen.findByRole('button', { name: PANEL_COPY.rowCopy }))
    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.copyFailed)
    expect(screen.getByRole('link', { name: PANEL_COPY.manageInSettings })).toBeInTheDocument()
  })

  it('Apple Calendar already connected: its status block and the hint, with no add link', async () => {
    useDevice(UA.iphoneSafari)
    withLink([readOf('apple_ios', 20 * MINUTE)])
    renderPage()

    expect(await screen.findByText('Apple Calendar (iPhone or iPad)')).toBeInTheDocument()
    expect(screen.getByText('Checked 20m ago.')).toBeInTheDocument()
    expect(screen.getByText(PANEL_COPY.iphoneGoogleAppHint)).toBeInTheDocument()
    expect(screen.getByText(STEPS_PAGE_COPY.phoneNeedsComputer)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.iphoneAppleInstead)).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
  })

  it('a failed read: the needs-computer line and one load-failure line', async () => {
    useDevice(UA.iphoneSafari)
    withLink()
    server.status = new Error('502')
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.loadFailed)
    expect(screen.getAllByRole('alert')).toHaveLength(1)
    expect(screen.getByText(STEPS_PAGE_COPY.phoneNeedsComputer)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.iphoneAppleInstead)).not.toBeInTheDocument()
  })

  it('an old link that cannot be shown points to Settings in place of the offer', async () => {
    useDevice(UA.iphoneSafari)
    server.token = { has_token: true, url_recoverable: false }
    server.status = { has_token: true, reads: [] }
    renderPage()

    expect(await screen.findByText(PANEL_COPY.unrecoverableNoApp)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.manageInSettings })).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.iphoneAppleInstead)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })).not.toBeInTheDocument()
  })

  it('a reset link says why above the button', async () => {
    useDevice(UA.iphoneSafari)
    server.status = {
      has_token: false,
      link_reset_at: ago(HOUR),
      link_reset_reason: 'password_reset',
    }
    renderPage()

    const button = await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar })
    const reason = screen.getByText(RESET_SUB.password_reset)
    expect(
      reason.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen.getByRole('heading', { level: 2, name: PANEL_COPY.headingReset })
    ).toBeInTheDocument()
  })

  it('a stopped Apple calendar shows its line above the Apple Calendar offer', async () => {
    useDevice(UA.iphoneSafari)
    withLink([readOf('apple_ios', 12 * DAY)])
    renderPage()

    expect(
      await screen.findByText(
        'Apple Calendar (iPhone or iPad) last checked 12 days ago. It may have been removed there.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.rowApple })).toBeInTheDocument()
  })
})

describe('CalendarStepsPage: on Android', () => {
  it('needs a computer, and points to adding one show at a time; nothing is read or made', async () => {
    useDevice(UA.android)
    renderPage()

    expect(
      await screen.findByText(STEPS_PAGE_COPY.phoneNeedsComputer)
    ).toBeInTheDocument()
    expect(screen.getByText(PANEL_COPY.androidPointer)).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(apiRequest).not.toHaveBeenCalled()
  })
})

describe('CalendarStepsPage: copy rules', () => {
  const strings = Object.values(STEPS_PAGE_COPY)

  it('has no em or en dashes', () => {
    for (const s of strings) expect(s).not.toMatch(/[–—]/)
  })

  it('keeps to the vocabulary: no sync, feed, subscribe, key or read', () => {
    for (const s of strings) {
      expect(s).not.toMatch(/\b(synced|sync|feed|subscribe|key|read)\b/i)
    }
  })
})
