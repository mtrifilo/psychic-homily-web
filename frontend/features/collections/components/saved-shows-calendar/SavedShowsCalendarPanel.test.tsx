import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState } from 'react'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'
import { AuthError } from '@/lib/errors'
import type {
  CalendarFeedRead,
  CalendarFeedStatusResponse,
  CalendarTokenStatusResponse,
} from '@/features/shows/types'
import { SavedShowsCalendarPanel } from './SavedShowsCalendarPanel'
import { endsWithRetry } from './CalendarPanelParts'
import {
  PANEL_COPY,
  RESET_SUB,
  addAgainLabel,
  downloadLabel,
  emailSentLine,
} from './savedShowsCalendarCopy'
import {
  CALENDAR_CLIENT_NAMES,
  calendarCheckedLine,
  calendarStoppedLine,
} from '@/features/collections/lib/calendarFeedStatus'

// The panel against stubbed endpoints: the real hooks, model and rows, with
// the network, the viewer, the device and the browser's side effects faked.

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
  usePathname: () => '/shows/1',
}))

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

const FEED_URL = 'https://api.example.test/calendar/feed/secret-token-abc.ics'
const WEBCAL_URL = 'webcal://api.example.test/calendar/feed/secret-token-abc.ics'
const TOKEN = 'secret-token-abc'

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

/** What the stubbed server answers. */
interface Server {
  status: CalendarFeedStatusResponse
  /** Answers GET /calendar/token; an Error rejects. */
  token: CalendarTokenStatusResponse | Error
  /** Answers POST /calendar/token. */
  mint: () => Promise<unknown>
  newestSavedAt: string | null
  upcoming: number
  forget: () => Promise<unknown>
  emailSteps: () => Promise<unknown>
  verifySend: () => Promise<unknown>
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

/** The account has a link, with these observed checks. */
function withLink(reads: CalendarFeedRead[] = []) {
  server.status = { has_token: true, reads }
  server.token = LINK
}

beforeEach(() => {
  server = {
    status: { has_token: false },
    token: { has_token: false },
    mint: async () => {
      // The server now holds the link it just made.
      server.token = LINK
      server.status = { has_token: true, reads: [] }
      return {
        token: TOKEN,
        feed_url: FEED_URL,
        follows_feed_url: 'https://api.example.test/follows.atom',
        created_at: ago(0),
      }
    },
    newestSavedAt: ago(5 * HOUR),
    upcoming: 8,
    forget: async () => ({ success: true, message: 'ok' }),
    emailSteps: async () => undefined,
    verifySend: async () => ({ success: true }),
  }
  apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
    const url = String(endpoint)
    const verb = method(options)
    if (url.endsWith('/calendar/token/feed-status')) return server.status
    if (url.includes('/calendar/token/reads?client=') && verb === 'DELETE') {
      return server.forget()
    }
    if (url.endsWith('/calendar/token/email-steps') && verb === 'POST') {
      return server.emailSteps()
    }
    if (url.endsWith('/calendar/token') && verb === 'GET') {
      if (server.token instanceof Error) throw server.token
      return server.token
    }
    if (url.endsWith('/calendar/token') && verb === 'POST') return server.mint()
    if (url.includes('/saved-shows?') && url.includes('time_filter=upcoming')) {
      return { shows: [], total: server.upcoming, limit: 1, offset: 0 }
    }
    if (url.includes('/saved-shows?')) {
      const shows = server.newestSavedAt
        ? [{ id: 1, saved_at: server.newestSavedAt }]
        : []
      return { shows, total: shows.length, limit: 1, offset: 0 }
    }
    if (url.endsWith('/auth/verify-email/send')) return server.verifySend()
    throw new Error(`unexpected request ${verb} ${url}`)
  })
})

/** Every tab the panel opens, and the order of browser side effects. */
interface FakeTab {
  opener: unknown
  closed: boolean
  location: { href: string }
  close: () => void
}
let tabs: FakeTab[]
let effects: string[]
const writeText = vi.fn()

let viewportIsWide = true
const mediaListeners = new Set<() => void>()

function useDevice(agent: string, wide: boolean) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(agent)
  viewportIsWide = wide
  vi.mocked(window.matchMedia).mockImplementation(
    (query: string) =>
      ({
        get matches() {
          return viewportIsWide
        },
        media: query,
        addEventListener: (_type: string, listener: () => void) =>
          mediaListeners.add(listener),
        removeEventListener: (_type: string, listener: () => void) =>
          mediaListeners.delete(listener),
      }) as unknown as MediaQueryList
  )
}

/** The viewport crosses the popover breakpoint, as a rotated phone does. */
function crossBreakpoint(wide: boolean) {
  viewportIsWide = wide
  act(() => {
    for (const listener of mediaListeners) listener()
  })
}

/** Keeps jsdom from following a link (it cannot); React handlers still run. */
function stopNavigation(event: MouseEvent) {
  if ((event.target as Element).closest('a')) event.preventDefault()
}

beforeEach(() => {
  tabs = []
  effects = []
  vi.spyOn(window, 'open').mockImplementation(() => {
    effects.push('open')
    const tab: FakeTab = {
      opener: window,
      closed: false,
      location: { href: '' },
      close() {
        tab.closed = true
      },
    }
    tabs.push(tab)
    return tab as unknown as Window
  })
  writeText.mockReset().mockImplementation(async () => {
    effects.push('copy')
  })
  useDevice(UA.mac, true)
  document.addEventListener('click', stopNavigation, true)
})

afterEach(() => {
  document.removeEventListener('click', stopNavigation, true)
  vi.restoreAllMocks()
})

/**
 * userEvent installs its own clipboard on setup, so the stub that records
 * what the panel copies goes in after it.
 */
function setupUser() {
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  return user
}

function Harness() {
  const anchorRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(true)
  return (
    <>
      <div ref={anchorRef}>
        <button type="button">Saved</button>
      </div>
      <SavedShowsCalendarPanel
        open={open}
        onOpenChange={setOpen}
        anchorRef={anchorRef}
      />
    </>
  )
}

async function renderPanel(client = createTestQueryClient()) {
  render(<Harness />, { wrapper: createWrapperWithClient(client) })
  const panel = await screen.findByTestId('saved-shows-calendar-panel')
  // Ready once the loading line is gone.
  await waitFor(() =>
    expect(within(panel).queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
  )
  return panel
}

const NOT_ROWS = new Set(['Close', PANEL_COPY.manageInSettings])

/** The panel's rows and links in order: buttons and links, minus the chrome. */
function rowNames(panel: HTMLElement): string[] {
  return Array.from(panel.querySelectorAll('a, button'))
    .map(element => element.textContent ?? '')
    .filter(name => !NOT_ROWS.has(name) && !name.startsWith('Download'))
}

const readOf = (
  client: CalendarFeedRead['client'],
  lastAgo: number,
  firstAgo = lastAgo + DAY
): CalendarFeedRead => ({
  client,
  first_read_at: ago(firstAgo),
  last_read_at: ago(lastAgo),
})
const google = (lastAgo: number) => readOf('google', lastAgo)

describe('SavedShowsCalendarPanel: preparing the link as it opens', () => {
  it('F: shows one muted line and no rows until the link and status are ready', async () => {
    let finish!: () => void
    server.token = LINK
    const linkRead = new Promise<void>(resolve => {
      finish = resolve
    })
    const base = apiRequest.getMockImplementation()!
    apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
      if (String(endpoint).endsWith('/calendar/token') && method(options) === 'GET') {
        await linkRead
      }
      return base(endpoint, options)
    })
    render(<Harness />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.loading)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])

    finish()
    await waitFor(() => expect(rowNames(panel)).toContain('Copy link'))
  })

  it('creates the link once, on open, when the account has none', async () => {
    const panel = await renderPanel()
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
    expect(rowNames(panel)).toContain('Copy link')
  })

  it('never creates a link when one exists', async () => {
    withLink()
    await renderPanel()
    expect(calls('/calendar/token', 'GET')).toHaveLength(1)
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('reads the link fresh: a cached "no link" from another page never creates one', async () => {
    const user = setupUser()
    const client = createTestQueryClient()
    client.setQueryData(['calendar', 'feedStatus'], { has_token: false })
    client.setQueryData(['calendar', 'tokenStatus'], { has_token: false })
    withLink()
    const panel = await renderPanel(client)

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    await within(panel).findByText(PANEL_COPY.copied)
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
  })

  it('reads the link fresh: a cached replaced address is never handed out', async () => {
    const user = setupUser()
    const client = createTestQueryClient()
    client.setQueryData(['calendar', 'tokenStatus'], {
      has_token: true,
      url_recoverable: true,
      feed_url: 'https://api.example.test/calendar/feed/replaced-token.ics',
    })
    withLink()
    const panel = await renderPanel(client)

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    await within(panel).findByText(PANEL_COPY.copied)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
  })

  it.each([
    ['creating', 'create', PANEL_COPY.createFailed],
    ['loading', 'load', PANEL_COPY.loadFailed],
  ] as const)(
    'a failure %s the link shows its line in place of the rows, and "Try again." retries',
    async (_label, which, line) => {
      const user = setupUser()
      if (which === 'create') {
        const mint = server.mint
        let failed = false
        server.mint = async () => {
          if (!failed) {
            failed = true
            throw new Error('500')
          }
          return mint()
        }
      } else {
        server.token = new Error('500')
      }
      render(<Harness />, {
        wrapper: createWrapperWithClient(createTestQueryClient()),
      })
      const panel = await screen.findByTestId('saved-shows-calendar-panel')

      const alert = await within(panel).findByRole('alert')
      expect(alert).toHaveTextContent(line)
      expect(
        within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
      ).toBeInTheDocument()
      expect(rowNames(panel)).toEqual(['Try again.'])

      if (which === 'load') server.token = LINK
      await user.click(within(alert).getByRole('button', { name: 'Try again.' }))

      await waitFor(() => expect(rowNames(panel)).toContain('Copy link'))
      expect(within(panel).queryByRole('alert')).not.toBeInTheDocument()
    }
  )

  it('H with no app listed keeps the status heading, never the add heading', async () => {
    server.status = { has_token: true, reads: [] }
    server.token = { has_token: true, url_recoverable: false }
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    // With nothing connected the sentence about a connected calendar is
    // dropped.
    expect(within(panel).getByText(PANEL_COPY.unrecoverableNoApp)).toBeInTheDocument()
    expect(within(panel).queryByText(PANEL_COPY.unrecoverable)).not.toBeInTheDocument()
    expect(within(panel).queryByText(PANEL_COPY.subAdd)).not.toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])
  })

  it('an expired session while creating keeps its AuthError and shows the create line', async () => {
    server.mint = async () => {
      throw new AuthError('expired', 'UNAUTHORIZED' as never, { status: 401 })
    }
    const client = createTestQueryClient()
    render(<Harness />, { wrapper: createWrapperWithClient(client) })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.createFailed
    )
    // The query cache sees the AuthError itself, so session recovery runs.
    expect(
      client.getQueryState(['calendarPanelLink', '7'])?.error
    ).toBeInstanceOf(AuthError)
  })

  it('H: a link whose address cannot be shown is known on open, with no rows', async () => {
    server.status = { has_token: true, reads: [google(2 * HOUR)] }
    server.token = { has_token: true, url_recoverable: false }
    const panel = await renderPanel()

    expect(within(panel).getByText(PANEL_COPY.unrecoverable)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])
    expect(
      within(panel).getByText('Checked 2h ago, since your last save.')
    ).toBeInTheDocument()
    expect(
      within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    expect(within(panel).queryByText(/Download/)).not.toBeInTheDocument()
  })
})

describe('SavedShowsCalendarPanel: states from the status payload', () => {
  it('A: nothing connected on a Mac', async () => {
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingAdd })
    ).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.subAdd)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([
      'Google Calendar',
      'Apple Calendar',
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
    const download = within(panel).getByRole('link', {
      name: 'Download these 8 shows (.ics)',
    })
    expect(download.getAttribute('href')).toMatch(/\/saved-shows\/calendar\.ics$/)
    expect(within(panel).getByText(PANEL_COPY.downloadCaption)).toBeInTheDocument()
    expect(
      within(panel).queryByRole('link', { name: PANEL_COPY.manageInSettings })
    ).not.toBeInTheDocument()
  })

  it('A2: nothing connected on Windows has no Apple row', async () => {
    useDevice(UA.windows, true)
    const panel = await renderPanel()
    expect(rowNames(panel)).toEqual([
      'Google Calendar',
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
  })

  it('names one upcoming show in the singular', async () => {
    server.upcoming = 1
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('link', { name: 'Download this show (.ics)' })
    ).toBeInTheDocument()
  })

  it('hides the download row and its caption with no upcoming saved shows', async () => {
    server.upcoming = 0
    const panel = await renderPanel()
    expect(within(panel).queryByText(/Download/)).not.toBeInTheDocument()
    expect(
      within(panel).queryByText(PANEL_COPY.downloadCaption)
    ).not.toBeInTheDocument()
  })

  it('B: a connected app is a status block, never an add row', async () => {
    withLink([google(2 * HOUR)])
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    expect(
      within(panel).getByText('Checked 2h ago, since your last save.')
    ).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.addAnother)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([
      'Apple Calendar',
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
    expect(
      within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toHaveAttribute('href', '/profile?tab=settings#calendar')
  })

  it('B2: a check before the newest save says it appears at the next check', async () => {
    withLink([google(9 * HOUR)])
    server.newestSavedAt = ago(HOUR)
    const panel = await renderPanel()
    expect(
      within(panel).getByText(
        'Checked 9h ago, before your last save. It appears at its next check.'
      )
    ).toBeInTheDocument()
  })

  it('drops the comparison when the account has no saved shows', async () => {
    withLink([google(2 * HOUR)])
    server.newestSavedAt = null
    const panel = await renderPanel()
    expect(within(panel).getByText('Checked 2h ago.')).toBeInTheDocument()
  })

  it('C: several connected apps, each with its own line', async () => {
    withLink([
      google(2 * HOUR),
      readOf('apple_ios', DAY + HOUR),
      readOf('other', 3 * HOUR, 5 * HOUR),
    ])
    const panel = await renderPanel()
    const blocks = within(panel).getAllByRole('listitem')
    expect(blocks.map(b => b.textContent)).toEqual([
      'Google CalendarChecked 2h ago, since your last save.',
      'Another calendar appChecked 3h ago, since your last save.',
      'Apple Calendar (iPhone or iPad)Checked 1d ago, before your last save. It appears at its next check.',
    ])
    expect(rowNames(panel)).toEqual([
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
  })

  it('counts an unnamed app only once its checks span an hour', async () => {
    withLink([readOf('other', 10 * MINUTE, 69 * MINUTE)])
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingAdd })
    ).toBeInTheDocument()
    expect(within(panel).queryByText('Another calendar app')).not.toBeInTheDocument()
  })

  it.each([
    ['Google, just inside 3 days', google(3 * DAY - MINUTE), 'connected'],
    ['Google, just past 3 days', google(3 * DAY + MINUTE), 'stopped'],
    ['Apple, just inside 8 days', readOf('apple_mac', 8 * DAY - MINUTE), 'connected'],
    ['Apple, just past 8 days', readOf('apple_mac', 8 * DAY + MINUTE), 'stopped'],
  ] as const)('%s reads as %s', async (_label, r, want) => {
    withLink([r])
    const panel = await renderPanel()
    if (want === 'connected') {
      expect(within(panel).getByText(/^Checked /)).toBeInTheDocument()
    } else {
      expect(
        within(panel).getByText(/last checked .* ago\. It may have been removed there\.$/)
      ).toBeInTheDocument()
    }
  })

  it('D: a stopped app offers the add again and I removed it', async () => {
    withLink([google(12 * DAY)])
    const panel = await renderPanel()
    expect(
      within(panel).getByText(
        'Google Calendar last checked 12 days ago. It may have been removed there.'
      )
    ).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([
      'Add to Google Calendar again',
      'I removed it',
      'Apple Calendar',
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
    expect(within(panel).queryByText(PANEL_COPY.addAnother)).not.toBeInTheDocument()
    expect(
      within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
  })

  it('D2: connected lines first, then the stopped line and its links', async () => {
    withLink([readOf('apple_ios', DAY), google(12 * DAY)])
    server.newestSavedAt = ago(2 * DAY)
    const panel = await renderPanel()
    const text = panel.textContent ?? ''
    expect(text.indexOf('Apple Calendar (iPhone or iPad)')).toBeLessThan(
      text.indexOf('Google Calendar last checked')
    )
    expect(rowNames(panel)).toEqual([
      'Add to Google Calendar again',
      'I removed it',
      'Outlook.com',
      'Outlook (work or school)',
      'Copy link',
    ])
  })

  it('offers one Apple "again" link when both Apple families stopped', async () => {
    withLink([readOf('apple_ios', 10 * DAY), readOf('apple_mac', 12 * DAY)])
    const panel = await renderPanel()
    expect(
      within(panel).getAllByRole('link', { name: 'Add to Apple Calendar again' })
    ).toHaveLength(1)
    expect(within(panel).getAllByRole('button', { name: 'I removed it' })).toHaveLength(2)
  })

  it('offers no Apple "again" link while the other Apple family is connected', async () => {
    withLink([readOf('apple_mac', HOUR), readOf('apple_ios', 12 * DAY)])
    const panel = await renderPanel()
    expect(rowNames(panel)).not.toContain('Add to Apple Calendar again')
    expect(rowNames(panel)).not.toContain('Apple Calendar')
  })

  it('"I removed it" forgets the app, and the panel offers its row as new', async () => {
    const user = setupUser()
    withLink([google(12 * DAY)])
    const panel = await renderPanel()
    server.status = { has_token: true, reads: [] }

    await user.click(within(panel).getByRole('button', { name: 'I removed it' }))

    expect(calls('/calendar/token/reads?client=google', 'DELETE')).toHaveLength(1)
    await waitFor(() =>
      expect(rowNames(panel)).toEqual([
        'Google Calendar',
        'Apple Calendar',
        'Outlook.com',
        'Outlook (work or school)',
        'Copy link',
      ])
    )
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingAdd })
    ).toBeInTheDocument()
  })

  it('a failed forget says so under its line, and "Try again." retries it', async () => {
    const user = setupUser()
    withLink([google(12 * DAY)])
    server.forget = async () => {
      throw new Error('500')
    }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'I removed it' }))

    const alert = await within(panel).findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.forgetFailed)
    await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
    await waitFor(() =>
      expect(calls('/calendar/token/reads?client=google', 'DELETE')).toHaveLength(2)
    )
  })

  it.each([
    ['an unnamed app', UA.mac, readOf('other', 12 * DAY, 13 * DAY)],
    ['Apple seen from Windows', UA.windows, readOf('apple_mac', 12 * DAY)],
  ])(
    'a stopped app this device cannot add (%s) gets only I removed it',
    async (_label, agent, r) => {
      useDevice(agent, true)
      withLink([r])
      const panel = await renderPanel()
      expect(within(panel).queryByText(/^Add to .* again$/)).not.toBeInTheDocument()
      expect(rowNames(panel)).toContain('I removed it')
      expect(rowNames(panel)).toContain('Copy link')
    }
  )

  it.each([
    ['sign_out_everywhere', RESET_SUB.sign_out_everywhere],
    ['password_reset', RESET_SUB.password_reset],
  ] as const)(
    'E: the reset state for %s survives the link this open created',
    async (reason, sub) => {
      server.status = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: reason,
      }
      server.token = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: reason,
      }
      const panel = await renderPanel()
      // Creating the link cleared the marker on the server.
      await waitFor(() =>
        expect(calls('/calendar/token/feed-status', 'GET').length).toBeGreaterThan(1)
      )
      expect(
        within(panel).getByRole('heading', { name: PANEL_COPY.headingReset })
      ).toBeInTheDocument()
      expect(within(panel).getByText(sub)).toBeInTheDocument()
      expect(rowNames(panel)).toEqual([
        'Google Calendar',
        'Apple Calendar',
        'Outlook.com',
        'Outlook (work or school)',
        'Copy link',
      ])
    }
  )
})

describe('SavedShowsCalendarPanel: rows act inside the click', () => {
  it('Copy link copies in the handler and never shows the address', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    const copy = within(panel).getByRole('button', { name: 'Copy link' })
    expect(copy).toHaveClass('ph-no-capture', 'sentry-block')

    await user.click(copy)

    await within(panel).findByText(PANEL_COPY.copied)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(panel.textContent).not.toContain(TOKEN)
  })

  it('every element whose href carries the address is kept out of autocapture', async () => {
    withLink([google(12 * DAY)])
    useDevice(UA.mac, true)
    const panel = await renderPanel()
    const carriers = Array.from(panel.querySelectorAll('[href]')).filter(
      element => element.getAttribute('href')?.includes(TOKEN)
    )
    expect(carriers.length).toBeGreaterThan(0)
    for (const element of carriers) {
      // PostHog skips the first, Sentry Session Replay blanks the second.
      expect(element).toHaveClass('ph-no-capture', 'sentry-block')
    }
  })

  it("Google: copies, then opens Google's page, both in the click", async () => {
    const user = setupUser()
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Google Calendar' }))

    expect(effects).toEqual(['copy', 'open'])
    expect(tabs[0].opener).toBeNull()
    expect(tabs[0].location.href).toBe(
      'https://calendar.google.com/calendar/r/settings/addbyurl'
    )
    for (const line of PANEL_COPY.googlePending) {
      expect(await within(panel).findByText(line)).toBeInTheDocument()
    }
  })

  it('Google: a blocked tab is never reported as open', async () => {
    const user = setupUser()
    vi.mocked(window.open).mockReturnValue(null)
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Google Calendar' }))

    expect(await within(panel).findByText(PANEL_COPY.copied)).toBeInTheDocument()
    expect(within(panel).queryByText(PANEL_COPY.googlePending[0])).not.toBeInTheDocument()
  })

  it("Google: a refused copy closes Google's tab and points to Settings", async () => {
    const user = setupUser()
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Google Calendar' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.copyFailed
    )
    expect(tabs[0].closed).toBe(true)
  })

  it('Apple: a webcal link to the address', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    const apple = within(panel).getByRole('link', { name: 'Apple Calendar' })
    expect(apple).toHaveAttribute('href', WEBCAL_URL)
    expect(apple).not.toHaveAttribute('target')

    await user.click(apple)

    expect(await within(panel).findByText(PANEL_COPY.appleOpened)).toBeInTheDocument()
  })

  it('Outlook: a new-tab link to Microsoft with the webcal address', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    const outlook = within(panel).getByRole('link', {
      name: 'Outlook (work or school)',
    })
    expect(outlook).toHaveAttribute('target', '_blank')
    expect(outlook).toHaveAttribute('rel', 'noopener noreferrer')
    const sent = new URL(outlook.getAttribute('href')!)
    expect(sent.origin + sent.pathname).toBe(
      'https://outlook.office.com/calendar/0/addfromweb/'
    )
    expect(sent.searchParams.get('url')).toBe(WEBCAL_URL)

    await user.click(outlook)

    expect(await within(panel).findByText(PANEL_COPY.outlookOpened)).toBeInTheDocument()
  })

  it('a refused copy points to Settings', async () => {
    const user = setupUser()
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.copyFailed
    )
    expect(
      within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
  })
})

describe('SavedShowsCalendarPanel: keeps this open together', () => {
  it('a failed saved-shows read blocks nothing: no comparison, no download, until it succeeds', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      withLink([google(2 * HOUR)])
      let saves: 'fail' | 'hang' | 'ok' = 'fail'
      const hung: Array<() => void> = []
      const base = apiRequest.getMockImplementation()!
      apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
        if (String(endpoint).includes('/saved-shows?')) {
          if (saves === 'fail') throw new Error('502')
          if (saves === 'hang') {
            await new Promise<void>(resolve => {
              hung.push(resolve)
            })
            throw new Error('502')
          }
        }
        return base(endpoint, options)
      })
      const panel = await renderPanel()

      expect(within(panel).getByText('Checked 2h ago.')).toBeInTheDocument()
      expect(within(panel).queryByText(/Download/)).not.toBeInTheDocument()
      expect(rowNames(panel)).toContain('Copy link')

      // While a retry is in flight, the panel stays as it is.
      saves = 'hang'
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000)
      })
      expect(within(panel).queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
      expect(rowNames(panel)).toContain('Copy link')
      await act(async () => {
        for (const release of hung) release()
      })

      saves = 'ok'
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000)
      })

      await waitFor(() =>
        expect(
          within(panel).getByText('Checked 2h ago, since your last save.')
        ).toBeInTheDocument()
      )
      expect(
        within(panel).getByRole('link', { name: 'Download these 8 shows (.ics)' })
      ).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('a failed first status read shows the load line, and its retry recovers', async () => {
    const user = setupUser()
    let failStatus = true
    const base = apiRequest.getMockImplementation()!
    apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
      if (String(endpoint).endsWith('/calendar/token/feed-status') && failStatus) {
        throw new Error('502')
      }
      return base(endpoint, options)
    })
    render(<Harness />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')

    const alert = await within(panel).findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.loadFailed)
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    expect(rowNames(panel)).toEqual(['Try again.'])

    failStatus = false
    await user.click(within(alert).getByRole('button', { name: 'Try again.' }))

    await waitFor(() => expect(rowNames(panel)).toContain('Copy link'))
  })

  it('a status refetch that fails keeps the panel and its notes on screen', async () => {
    const user = setupUser()
    const client = createTestQueryClient()
    const panel = await renderPanel(client)
    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))
    await within(panel).findByText(PANEL_COPY.copied)

    const base = apiRequest.getMockImplementation()!
    apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
      if (String(endpoint).endsWith('/calendar/token/feed-status')) {
        throw new Error('502')
      }
      return base(endpoint, options)
    })
    await act(async () => {
      await client.refetchQueries({ queryKey: ['calendar', 'feedStatus'] })
    })

    await waitFor(() =>
      expect(client.getQueryState(['calendar', 'feedStatus'])?.status).toBe('error')
    )
    expect(within(panel).getByText(PANEL_COPY.copied)).toBeInTheDocument()
    expect(within(panel).queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
  })

  it('crossing the breakpoint while open keeps the same shell and its notes', async () => {
    const user = setupUser()
    useDevice(UA.iphoneSafari, false)
    const panel = await renderPanel()
    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))
    await within(panel).findByText(PANEL_COPY.copied)
    const linkReads = calls('/calendar/token', 'GET').length

    crossBreakpoint(true)

    const after = screen.getByTestId('saved-shows-calendar-panel')
    expect(after).toBe(panel)
    expect(within(after).getByText(PANEL_COPY.copied)).toBeInTheDocument()
    expect(calls('/calendar/token', 'GET')).toHaveLength(linkReads)
  })
})

describe('SavedShowsCalendarPanel: focus', () => {
  it('keeps focus in the panel when "I removed it" takes its own block away', async () => {
    const user = setupUser()
    withLink([google(12 * DAY)])
    const panel = await renderPanel()
    server.status = { has_token: true, reads: [] }

    await user.click(within(panel).getByRole('button', { name: 'I removed it' }))

    await waitFor(() => expect(rowNames(panel)).not.toContain('I removed it'))
    expect(panel.contains(document.activeElement)).toBe(true)
  })

  it('Tab while the panel is still loading stays in the panel', async () => {
    const user = setupUser()
    let finish!: () => void
    const linkRead = new Promise<void>(resolve => {
      finish = resolve
    })
    const base = apiRequest.getMockImplementation()!
    apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
      if (String(endpoint).endsWith('/calendar/token') && method(options) === 'GET') {
        await linkRead
      }
      return base(endpoint, options)
    })
    render(<Harness />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    panel.focus()

    await user.tab()

    expect(screen.getByTestId('saved-shows-calendar-panel')).toBe(panel)
    finish()
  })

  it('Shift+Tab from the panel itself closes it and returns focus to Save', async () => {
    const user = setupUser()
    // A page control after Save, as a real page has: the portaled panel sits
    // after it, so a plain Shift+Tab would land there.
    render(
      <>
        <Harness />
        <a href="#footer">footer</a>
      </>,
      { wrapper: createWrapperWithClient(createTestQueryClient()) }
    )
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    await waitFor(() =>
      expect(within(panel).queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
    )
    panel.focus()

    await user.tab({ shift: true })

    await waitFor(() =>
      expect(screen.queryByTestId('saved-shows-calendar-panel')).not.toBeInTheDocument()
    )
    expect(screen.getByRole('button', { name: 'Saved' })).toHaveFocus()
  })

  it('Tab past the last control closes the popover and returns focus to Save', async () => {
    const user = setupUser()
    withLink([google(2 * HOUR)])
    const panel = await renderPanel()
    within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings }).focus()

    await user.tab()

    await waitFor(() =>
      expect(screen.queryByTestId('saved-shows-calendar-panel')).not.toBeInTheDocument()
    )
    expect(screen.getByRole('button', { name: 'Saved' })).toHaveFocus()
  })

  it('returns focus to the Save control on Escape', async () => {
    const user = setupUser()
    await renderPanel()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByTestId('saved-shows-calendar-panel')).not.toBeInTheDocument()
    )
    expect(screen.getByRole('button', { name: 'Saved' })).toHaveFocus()
  })

  it('leaves focus where the person clicked when they dismiss it outside', async () => {
    const user = setupUser()
    render(
      <>
        <Harness />
        <input aria-label="search" />
      </>,
      { wrapper: createWrapperWithClient(createTestQueryClient()) }
    )
    await screen.findByTestId('saved-shows-calendar-panel')

    await user.click(screen.getByRole('textbox', { name: 'search' }))

    await waitFor(() =>
      expect(screen.queryByTestId('saved-shows-calendar-panel')).not.toBeInTheDocument()
    )
    expect(screen.getByRole('textbox', { name: 'search' })).toHaveFocus()
  })

  // A control that toggles the panel names itself as the trigger. Its press
  // must not dismiss the popover as an outside press, or the click that
  // follows would open it again.
  function ToggleHarness({ withTrigger }: { withTrigger: boolean }) {
    const anchorRef = useRef<HTMLDivElement>(null)
    const [open, setOpen] = useState(true)
    return (
      <>
        <div ref={anchorRef}>
          <button type="button" onClick={() => setOpen(!open)}>
            Calendar
          </button>
        </div>
        <SavedShowsCalendarPanel
          open={open}
          onOpenChange={setOpen}
          anchorRef={anchorRef}
          triggerRef={withTrigger ? anchorRef : undefined}
        />
      </>
    )
  }

  // A browser runs the press and the click as separate tasks, rendering in
  // between, so the click sees whatever the press left; each is flushed here
  // on its own for the same reason.
  async function pressThenClick(control: HTMLElement) {
    // Radix starts listening for outside presses a tick after the popover
    // mounts.
    await act(() => new Promise(resolve => setTimeout(resolve, 0)))
    await act(async () => {
      fireEvent.pointerDown(control)
    })
    const openAfterPress = !!screen.queryByTestId('saved-shows-calendar-panel')
    await act(async () => {
      fireEvent.click(control)
    })
    return { openAfterPress }
  }

  it('lets a press on its trigger close it, rather than dismiss and reopen', async () => {
    render(<ToggleHarness withTrigger />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    await screen.findByTestId('saved-shows-calendar-panel')

    const { openAfterPress } = await pressThenClick(
      screen.getByRole('button', { name: 'Calendar' })
    )

    // The press left it open for the control's own click, which closed it.
    expect(openAfterPress).toBe(true)
    await waitFor(() =>
      expect(screen.queryByTestId('saved-shows-calendar-panel')).not.toBeInTheDocument()
    )
  })

  it('treats a press on its anchor as outside when no trigger is named', async () => {
    render(<ToggleHarness withTrigger={false} />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    await screen.findByTestId('saved-shows-calendar-panel')

    const { openAfterPress } = await pressThenClick(
      screen.getByRole('button', { name: 'Calendar' })
    )

    // The press dismissed it before the control's click could act: what a
    // Save control anchoring the panel relies on.
    expect(openAfterPress).toBe(false)
  })
})

describe('SavedShowsCalendarPanel: phones', () => {
  it('iPhone in Safari: Apple leads, then the Google block, in a sheet', async () => {
    useDevice(UA.iphoneSafari, false)
    const panel = await renderPanel()
    expect(panel).toHaveAttribute('role', 'dialog')
    expect(within(panel).getByText(PANEL_COPY.iphoneGoogleAppHint)).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.iphoneGoogleSub)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([
      'Apple Calendar',
      'Email me the steps',
      'Copy link',
    ])
    expect(
      within(panel).getByRole('link', { name: 'Download these 8 shows (.ics)' })
    ).toBeInTheDocument()
  })

  it('iPhone outside Safari: the Apple row is the fallback text', async () => {
    useDevice(UA.iphoneChrome, false)
    const panel = await renderPanel()
    expect(within(panel).getByText(PANEL_COPY.iphoneAppleNotSafari)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual(['Email me the steps', 'Copy link'])
  })

  it('Android: the Google block, Copy link and the per-show pointer; no Apple, no download', async () => {
    useDevice(UA.android, false)
    const panel = await renderPanel()
    expect(within(panel).getByText(PANEL_COPY.androidGoogleSub)).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.androidPointer)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual(['Email me the steps', 'Copy link'])
    expect(within(panel).queryByText(/Download/)).not.toBeInTheDocument()
  })

  it('a stopped Google calendar on a phone keeps the Google block as the way back', async () => {
    useDevice(UA.iphoneSafari, false)
    withLink([google(12 * DAY)])
    const panel = await renderPanel()
    expect(within(panel).queryByText(/^Add to .* again$/)).not.toBeInTheDocument()
    expect(rowNames(panel)).toEqual([
      'I removed it',
      'Apple Calendar',
      'Email me the steps',
      'Copy link',
    ])
  })

  it('iPhone, connected: the Apple row gives way to its status block', async () => {
    useDevice(UA.iphoneSafari, false)
    withLink([readOf('apple_ios', 20 * MINUTE)])
    server.newestSavedAt = ago(HOUR)
    const panel = await renderPanel()
    expect(
      within(panel).getByText('Checked 20m ago, since your last save.')
    ).toBeInTheDocument()
    expect(rowNames(panel)).toEqual(['Email me the steps', 'Copy link'])
  })
})

describe('SavedShowsCalendarPanel: Email me the steps', () => {
  beforeEach(() => useDevice(UA.android, false))

  it('confirms where it went and offers Send again', async () => {
    const user = setupUser()
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Email me the steps' }))

    expect(
      await within(panel).findByText('Sent to fan@example.com. Open it on a computer.')
    ).toBeInTheDocument()
    expect(within(panel).getByRole('button', { name: 'Send again' })).toBeInTheDocument()
    expect(calls('/calendar/token/email-steps', 'POST')).toHaveLength(1)
  })

  it('an unverified address gets the verification resend in place', async () => {
    const user = setupUser()
    server.emailSteps = async () => {
      throw new AuthError('x', 'EMAIL_VERIFICATION_REQUIRED' as never, {
        status: 403,
      })
    }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Email me the steps' }))

    expect(await within(panel).findByText(PANEL_COPY.emailUnverified)).toBeInTheDocument()
    await user.click(within(panel).getByRole('button', { name: 'Verify email' }))
    await waitFor(() =>
      expect(calls('/auth/verify-email/send', 'POST')).toHaveLength(1)
    )
  })

  it('too many asks gets the hour line', async () => {
    const user = setupUser()
    server.emailSteps = async () => {
      throw Object.assign(new Error('x'), {
        status: 429,
        errorCode: 'CALENDAR_STEPS_RATE_LIMITED_HOURLY',
      })
    }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Email me the steps' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.emailRateLimited
    )
  })

  it('a 503 gets the generic line with its retry, and a later failure takes the sent line down', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    await user.click(within(panel).getByRole('button', { name: 'Email me the steps' }))
    await within(panel).findByText(/^Sent to /)

    server.emailSteps = async () => {
      throw Object.assign(new Error('x'), { status: 503 })
    }
    await user.click(within(panel).getByRole('button', { name: 'Send again' }))

    const alert = await within(panel).findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.emailFailed)
    expect(within(panel).queryByText(/^Sent to /)).not.toBeInTheDocument()

    await user.click(within(alert).getByRole('button', { name: 'Try again.' }))
    await waitFor(() =>
      expect(calls('/calendar/token/email-steps', 'POST')).toHaveLength(3)
    )
  })
})

describe('SavedShowsCalendarPanel: copy rules', () => {
  const now = new Date('2026-11-19T12:00:00Z')
  const strings = [
    ...Object.values(PANEL_COPY).flat(),
    ...Object.values(RESET_SUB),
    ...Object.values(CALENDAR_CLIENT_NAMES),
    downloadLabel(1),
    downloadLabel(8),
    addAgainLabel('google'),
    addAgainLabel('apple'),
    emailSentLine('fan@example.com'),
    calendarCheckedLine('2026-11-19T10:00:00Z', null, now),
    calendarCheckedLine('2026-11-19T10:00:00Z', '2026-11-19T09:00:00Z', now),
    calendarCheckedLine('2026-11-19T03:00:00Z', '2026-11-19T09:00:00Z', now),
    calendarStoppedLine(
      {
        client: 'other',
        name: CALENDAR_CLIENT_NAMES.other,
        lastReadAt: '2026-11-07T12:00:00Z',
      },
      now
    ),
  ]

  it('ends every line that retries with the words that are its retry', () => {
    for (const line of [
      PANEL_COPY.createFailed,
      PANEL_COPY.loadFailed,
      PANEL_COPY.forgetFailed,
      PANEL_COPY.emailFailed,
    ]) {
      expect(endsWithRetry(line)).toBe(true)
    }
  })

  it('has no em or en dashes', () => {
    for (const s of strings) expect(s).not.toMatch(/[–—]/)
  })

  it('keeps to the vocabulary: no sync, feed, subscribe, key or read', () => {
    for (const s of strings) {
      expect(s).not.toMatch(/\b(synced|sync|feed|subscribe|key|read)\b/i)
    }
  })
})
