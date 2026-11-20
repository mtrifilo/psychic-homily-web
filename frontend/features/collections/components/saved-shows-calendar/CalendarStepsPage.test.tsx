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
import { GOOGLE_CALENDAR_ADD_BY_URL_PAGE } from '@/lib/utils/calendarFeedUrls'
import { CalendarStepsPage } from './CalendarStepsPage'
import { STEPS_PAGE_COPY } from './calendarStepsPageCopy'
import { PANEL_COPY } from './savedShowsCalendarCopy'

// The page against stubbed endpoints: the real guard, hooks and link
// preparation, with the network, the viewer, the device and the clipboard
// faked.

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
  android:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36',
} as const

interface Server {
  status: CalendarFeedStatusResponse | Error
  /** Answers GET /calendar/token; an Error rejects, a promise holds the answer. */
  token: CalendarTokenStatusResponse | Error | Promise<CalendarTokenStatusResponse>
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

const LINK: CalendarTokenStatusResponse = {
  has_token: true,
  url_recoverable: true,
  feed_url: FEED_URL,
}

function withLink(reads: CalendarFeedRead[] = []) {
  server.status = { has_token: true, reads }
  server.token = LINK
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
      server.token = LINK
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

function renderPage() {
  return render(<CalendarStepsPage />, {
    wrapper: createWrapperWithClient(createTestQueryClient()),
  })
}

/** The computer page once the link is ready: Copy link is enabled. */
async function renderReadyComputerPage() {
  renderPage()
  const copy = await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
  await waitFor(() => expect(copy).toBeEnabled())
  return copy
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
  it('shows the heading and the three steps in order', async () => {
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
  })

  it("step 2 opens Google's Add from URL page in a new tab", async () => {
    withLink()
    await renderReadyComputerPage()

    const open = screen.getByRole('link', { name: STEPS_PAGE_COPY.openButton })
    expect(open).toHaveAttribute('href', GOOGLE_CALENDAR_ADD_BY_URL_PAGE)
    expect(open).toHaveAttribute('target', '_blank')
    expect(open).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('Copy link waits for the link, then copies it in the click and reads Copied', async () => {
    let finishLoading: (value: CalendarTokenStatusResponse) => void = () => {}
    withLink()
    server.token = new Promise(resolve => {
      finishLoading = resolve
    })
    const user = setupUser()
    renderPage()

    const copy = await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    expect(copy).toBeDisabled()
    await act(async () => finishLoading(LINK))
    await waitFor(() => expect(copy).toBeEnabled())

    await user.click(copy)

    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(
      await screen.findByRole('button', { name: STEPS_PAGE_COPY.copiedButton })
    ).toBeInTheDocument()
  })

  it('creates the link once when the account has none', async () => {
    renderPage()
    await screen.findByRole('link', { name: PANEL_COPY.rowOutlookCom })
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
  })

  it('never creates a link when one exists', async () => {
    withLink()
    await renderReadyComputerPage()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('never shows the address, and marks everything that carries it', async () => {
    withLink()
    const user = setupUser()
    const copy = await renderReadyComputerPage()
    await user.click(copy)
    await screen.findByRole('button', { name: STEPS_PAGE_COPY.copiedButton })

    expect(document.body.textContent).not.toContain('secret-token-abc')
    expect(screen.getByRole('button', { name: STEPS_PAGE_COPY.copiedButton })).toHaveClass(
      'ph-no-capture',
      'sentry-block'
    )
    const carriers = Array.from(document.querySelectorAll('a')).filter(
      anchor => anchor.href.includes('secret-token-abc')
    )
    expect(carriers.length).toBeGreaterThan(0)
    for (const anchor of carriers) {
      expect(anchor).toHaveClass('ph-no-capture', 'sentry-block')
    }
  })

  it('offers the other calendars under the steps: Apple on a Mac, and both Outlooks', async () => {
    withLink()
    await renderReadyComputerPage()

    expect(screen.getByText(STEPS_PAGE_COPY.otherCalendarsLead, { exact: false })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.rowApple })).toHaveAttribute(
      'href',
      WEBCAL_URL
    )
    const outlookCom = screen.getByRole('link', { name: PANEL_COPY.rowOutlookCom })
    expect(outlookCom.getAttribute('href')).toMatch(
      /^https:\/\/outlook\.live\.com\/calendar\/0\/addfromweb\/\?url=webcal/
    )
    expect(outlookCom).toHaveAttribute('target', '_blank')
    const outlookWork = screen.getByRole('link', { name: PANEL_COPY.rowOutlookWork })
    expect(outlookWork.getAttribute('href')).toMatch(
      /^https:\/\/outlook\.office\.com\/calendar\/0\/addfromweb\/\?url=webcal/
    )
  })

  it('offers no Apple Calendar on a computer that has none', async () => {
    useDevice(UA.windows)
    withLink()
    await renderReadyComputerPage()

    expect(screen.queryByRole('link', { name: PANEL_COPY.rowApple })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: PANEL_COPY.rowOutlookCom })).toBeInTheDocument()
  })

  it("waits for Google's first check, and turns to the connected block once it is seen", async () => {
    withLink()
    await renderReadyComputerPage()
    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
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

    expect(await screen.findByText('Checked 2h ago.')).toBeInTheDocument()
    expect(screen.getByText(STEPS_PAGE_COPY.done)).toBeInTheDocument()
  })

  it('keeps waiting for Google while only another calendar app checks the link', async () => {
    withLink([readOf('apple_mac', HOUR)])
    await renderReadyComputerPage()

    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.done)).not.toBeInTheDocument()
  })

  it('a stale Google check from before the page opened is read again before it is shown', async () => {
    withLink()
    const client = createTestQueryClient()
    client.setQueryData(['calendar', 'feedStatus'], {
      has_token: true,
      reads: [readOf('google', 0)],
    })
    render(<CalendarStepsPage />, { wrapper: createWrapperWithClient(client) })

    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.done)).not.toBeInTheDocument()
  })
})

describe('CalendarStepsPage: failures, with the panel lines', () => {
  it.each([
    ['loading', PANEL_COPY.loadFailed],
    ['creating', PANEL_COPY.createFailed],
  ] as const)(
    'a failure %s the link replaces Copy link with its line, and "Try again." retries',
    async (step, line) => {
      if (step === 'loading') {
        server.token = new Error('502')
      } else {
        server.mint = async () => {
          throw new Error('502')
        }
      }
      const user = setupUser()
      renderPage()

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent(line)
      expect(
        screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })
      ).not.toBeInTheDocument()

      withLink()
      await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
      expect(
        await screen.findByRole('button', { name: STEPS_PAGE_COPY.copyButton })
      ).toBeEnabled()
    }
  )

  it('a refused copy says so and points to Settings', async () => {
    withLink()
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const user = setupUser()
    const copy = await renderReadyComputerPage()

    await user.click(copy)

    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.copyFailed)
    expect(
      screen.getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).toBeInTheDocument()
  })

  it('a failed status read says the calendar could not be loaded, with its retry', async () => {
    withLink()
    server.status = new Error('502')
    const user = setupUser()
    await renderReadyComputerPage()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.loadFailed)

    server.status = { has_token: true, reads: [] }
    await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
    expect(await screen.findByText(STEPS_PAGE_COPY.waiting)).toBeInTheDocument()
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
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('an old link with a calendar connected warns that a new link disconnects it', async () => {
    server.token = { has_token: true, url_recoverable: false }
    server.status = { has_token: true, reads: [readOf('google', HOUR)] }
    renderPage()

    expect(await screen.findByText(PANEL_COPY.unrecoverable)).toBeInTheDocument()
  })
})

describe('CalendarStepsPage: on a phone', () => {
  it('iPhone: needs a computer, and offers Apple Calendar with the Google app hint', async () => {
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
    expect(
      screen.queryByRole('link', { name: STEPS_PAGE_COPY.openButton })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: STEPS_PAGE_COPY.copyButton })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(STEPS_PAGE_COPY.waiting)).not.toBeInTheDocument()
  })

  it('Android: needs a computer, and points to adding one show at a time; no link is made', async () => {
    useDevice(UA.android)
    renderPage()

    expect(
      await screen.findByText(STEPS_PAGE_COPY.phoneNeedsComputer)
    ).toBeInTheDocument()
    expect(screen.getByText(PANEL_COPY.androidPointer)).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(calls('/calendar/token', 'GET')).toHaveLength(0)
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
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
