import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
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

const openInCalendarApp = vi.fn()
vi.mock('./calendarPanelBrowser', async importOriginal => ({
  ...(await importOriginal<typeof import('./calendarPanelBrowser')>()),
  openInCalendarApp: (url: string) => openInCalendarApp(url),
}))

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
  /** Answers POST /calendar/token; an Error rejects; a promise waits. */
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
      String(endpoint).includes(fragment) && method(options) === verb
  )
}

beforeEach(() => {
  server = {
    status: { has_token: false },
    token: { has_token: true, url_recoverable: true, feed_url: FEED_URL },
    mint: async () => ({
      token: TOKEN,
      feed_url: FEED_URL,
      follows_feed_url: 'https://api.example.test/follows.atom',
      created_at: ago(0),
    }),
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

/** A fake tab the panel opens; records where it was sent. */
interface FakeTab {
  opener: unknown
  closed: boolean
  location: { href: string }
  close: () => void
}
let tabs: FakeTab[]
const writeText = vi.fn()

function useDevice(agent: string, wide: boolean) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(agent)
  vi.mocked(window.matchMedia).mockImplementation(
    (query: string) =>
      ({
        matches: wide,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }) as unknown as MediaQueryList
  )
}

beforeEach(() => {
  tabs = []
  vi.spyOn(window, 'open').mockImplementation(() => {
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
  writeText.mockReset().mockResolvedValue(undefined)
  useDevice(UA.mac, true)
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

afterEach(() => {
  vi.restoreAllMocks()
})

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

async function renderPanel() {
  const client = createTestQueryClient()
  render(<Harness />, { wrapper: createWrapperWithClient(client) })
  const panel = await screen.findByTestId('saved-shows-calendar-panel')
  // Ready once the loading line is gone.
  await waitFor(() =>
    expect(within(panel).queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
  )
  return panel
}

function rowNames(panel: HTMLElement): string[] {
  return within(panel)
    .queryAllByRole('button')
    .map(button => button.textContent ?? '')
    .filter(name => name !== 'Close')
}

const google = (lastAgo: number, firstAgo = lastAgo + DAY): CalendarFeedRead => ({
  client: 'google',
  first_read_at: ago(firstAgo),
  last_read_at: ago(lastAgo),
})
const readOf = (
  client: CalendarFeedRead['client'],
  lastAgo: number,
  firstAgo = lastAgo + DAY
): CalendarFeedRead => ({
  client,
  first_read_at: ago(firstAgo),
  last_read_at: ago(lastAgo),
})

describe('SavedShowsCalendarPanel: states from the status payload', () => {
  it('F: loading shows one muted line and no rows', async () => {
    apiRequest.mockImplementation(() => new Promise(() => {}))
    render(<Harness />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    expect(within(panel).getByText(PANEL_COPY.loading)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])
  })

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

  it('names one upcoming show in the singular, and hides the download at none', async () => {
    server.upcoming = 1
    const one = await renderPanel()
    expect(
      within(one).getByRole('link', { name: 'Download this show (.ics)' })
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
    server.status = { has_token: true, reads: [google(2 * HOUR)] }
    const panel = await renderPanel()
    expect(
      within(panel).getByRole('heading', { name: PANEL_COPY.headingStatus })
    ).toBeInTheDocument()
    expect(within(panel).getByText('Google Calendar')).toBeInTheDocument()
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
    server.status = { has_token: true, reads: [google(9 * HOUR)] }
    server.newestSavedAt = ago(HOUR)
    const panel = await renderPanel()
    expect(
      within(panel).getByText(
        'Checked 9h ago, before your last save. It appears at its next check.'
      )
    ).toBeInTheDocument()
  })

  it('drops the comparison when the account has no saved shows', async () => {
    server.status = { has_token: true, reads: [google(2 * HOUR)] }
    server.newestSavedAt = null
    const panel = await renderPanel()
    expect(within(panel).getByText('Checked 2h ago.')).toBeInTheDocument()
  })

  it('C: several connected apps, each with its own line', async () => {
    server.status = {
      has_token: true,
      reads: [
        google(2 * HOUR),
        readOf('apple_ios', DAY + HOUR),
        readOf('other', 3 * HOUR, 3 * HOUR + 2 * HOUR),
      ],
    }
    server.newestSavedAt = ago(5 * HOUR)
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
    server.status = {
      has_token: true,
      reads: [readOf('other', 10 * MINUTE, 10 * MINUTE + 59 * MINUTE)],
    }
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
    server.status = { has_token: true, reads: [r] }
    const panel = await renderPanel()
    if (want === 'connected') {
      expect(within(panel).getByText(/^Checked /)).toBeInTheDocument()
    } else {
      expect(within(panel).getByText(/last checked .* ago\. It may have been removed there\.$/)).toBeInTheDocument()
    }
  })

  it('D: a stopped app offers the add again and I removed it', async () => {
    server.status = { has_token: true, reads: [google(12 * DAY)] }
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
    server.status = {
      has_token: true,
      reads: [readOf('apple_ios', DAY), google(12 * DAY)],
    }
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

  it('"I removed it" forgets the app, and the panel offers its row as new', async () => {
    const user = setupUser()
    server.status = { has_token: true, reads: [google(12 * DAY)] }
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

  it('says so inline when forgetting fails', async () => {
    const user = setupUser()
    server.status = { has_token: true, reads: [google(12 * DAY)] }
    server.forget = async () => {
      throw new Error('500')
    }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'I removed it' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.forgetFailed
    )
  })

  it.each([
    ['an unnamed app', UA.mac, readOf('other', 12 * DAY, 13 * DAY)],
    ['Apple seen from Windows', UA.windows, readOf('apple_mac', 12 * DAY)],
  ])('a stopped app this device cannot add (%s) gets only I removed it', async (_label, agent, r) => {
    useDevice(agent, true)
    server.status = { has_token: true, reads: [r] }
    const panel = await renderPanel()
    expect(within(panel).queryByText(/^Add to .* again$/)).not.toBeInTheDocument()
    expect(rowNames(panel)).toContain('I removed it')
    expect(rowNames(panel)).toContain('Copy link')
  })

  it('offers one Apple "again" link when both Apple families stopped', async () => {
    server.status = {
      has_token: true,
      reads: [readOf('apple_ios', 10 * DAY), readOf('apple_mac', 12 * DAY)],
    }
    const panel = await renderPanel()
    expect(
      within(panel).getAllByRole('button', { name: 'Add to Apple Calendar again' })
    ).toHaveLength(1)
    expect(
      within(panel).getAllByRole('button', { name: 'I removed it' })
    ).toHaveLength(2)
  })

  it('offers no Apple "again" link while the other Apple family is connected', async () => {
    server.status = {
      has_token: true,
      reads: [readOf('apple_mac', HOUR), readOf('apple_ios', 12 * DAY)],
    }
    const panel = await renderPanel()
    expect(
      within(panel).queryByRole('button', { name: 'Add to Apple Calendar again' })
    ).not.toBeInTheDocument()
    expect(rowNames(panel)).not.toContain('Apple Calendar')
  })

  it.each([
    ['sign_out_everywhere', RESET_SUB.sign_out_everywhere],
    ['password_reset', RESET_SUB.password_reset],
  ] as const)('E: the reset state for %s', async (reason, sub) => {
    server.status = {
      has_token: false,
      link_reset_at: ago(HOUR),
      link_reset_reason: reason,
    }
    const panel = await renderPanel()
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
  })
})

describe('SavedShowsCalendarPanel: never acts on a status it has not just read', () => {
  it('offers no row from a cached status while the fresh read is in flight', async () => {
    const client = createTestQueryClient()
    // Another surface cached "no link" earlier; meanwhile one was made elsewhere.
    client.setQueryData(['calendar', 'feedStatus'], { has_token: false })
    let finish!: () => void
    const fresh = new Promise<void>(resolve => {
      finish = resolve
    })
    server.status = { has_token: true, reads: [] }
    const base = apiRequest.getMockImplementation()!
    apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
      if (String(endpoint).endsWith('/calendar/token/feed-status')) await fresh
      return base(endpoint, options)
    })
    render(<Harness />, { wrapper: createWrapperWithClient(client) })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')

    expect(within(panel).getByText(PANEL_COPY.loading)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])

    finish()
    await waitFor(() => expect(rowNames(panel)).toContain('Copy link'))
    const user = setupUser()
    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))
    await within(panel).findByText(PANEL_COPY.copied)
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
  })

  it('reads the address from the server, not a cached token status', async () => {
    const user = setupUser()
    const client = createTestQueryClient()
    client.setQueryData(['calendar', 'tokenStatus'], {
      has_token: true,
      url_recoverable: true,
      feed_url: 'https://api.example.test/calendar/feed/replaced-token.ics',
    })
    server.status = { has_token: true, reads: [] }
    render(<Harness />, { wrapper: createWrapperWithClient(client) })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')
    await waitFor(() => expect(rowNames(panel)).toContain('Copy link'))

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    await within(panel).findByText(PANEL_COPY.copied)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
  })
})

describe('SavedShowsCalendarPanel: focus', () => {
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
})

describe('SavedShowsCalendarPanel: rows that need the link', () => {
  it('creates the link on the first pick, copies it, and never shows it', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    const copy = within(panel).getByRole('button', { name: 'Copy link' })
    expect(copy).toHaveClass('ph-no-capture')

    await user.click(copy)

    await within(panel).findByText(PANEL_COPY.copied)
    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
    expect(panel.textContent).not.toContain(TOKEN)
    for (const element of panel.querySelectorAll('[href]')) {
      expect(element.getAttribute('href')).not.toContain(TOKEN)
    }
  })

  it('reuses the link it created for a second pick', async () => {
    const user = setupUser()
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))
    await within(panel).findByText(PANEL_COPY.copied)
    await user.click(within(panel).getByRole('button', { name: 'Outlook.com' }))
    await within(panel).findByText(PANEL_COPY.outlookOpened)

    expect(calls('/calendar/token', 'POST')).toHaveLength(1)
  })

  it('loads an existing link instead of creating one', async () => {
    const user = setupUser()
    server.status = { has_token: true, reads: [] }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    await within(panel).findByText(PANEL_COPY.copied)
    expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
  })

  it("Google: opens Google's page in the click, before the link exists, then shows the two-step", async () => {
    const user = setupUser()
    let finishMint!: () => void
    const minted = server.mint
    server.mint = () =>
      new Promise(resolve => {
        finishMint = () => resolve(minted())
      })
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Google Calendar' }))

    expect(tabs).toHaveLength(1)
    expect(tabs[0].opener).toBeNull()
    expect(tabs[0].location.href).toBe(
      'https://calendar.google.com/calendar/r/settings/addbyurl'
    )
    expect(writeText).not.toHaveBeenCalled()

    finishMint()

    for (const line of PANEL_COPY.googlePending) {
      expect(await within(panel).findByText(line)).toBeInTheDocument()
    }
    expect(writeText).toHaveBeenCalledWith(FEED_URL)
  })

  it('Outlook: a blank tab opens in the click and goes to Microsoft with the webcal address', async () => {
    const user = setupUser()
    const panel = await renderPanel()

    await user.click(
      within(panel).getByRole('button', { name: 'Outlook (work or school)' })
    )

    expect(await within(panel).findByText(PANEL_COPY.outlookOpened)).toBeInTheDocument()
    expect(tabs).toHaveLength(1)
    const sent = new URL(tabs[0].location.href)
    expect(sent.origin + sent.pathname).toBe(
      'https://outlook.office.com/calendar/0/addfromweb/'
    )
    expect(sent.searchParams.get('url')).toBe(
      'webcal://api.example.test/calendar/feed/secret-token-abc.ics'
    )
  })

  it('Apple: hands the webcal address to the calendar app', async () => {
    const user = setupUser()
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Apple Calendar' }))

    expect(await within(panel).findByText(PANEL_COPY.appleOpened)).toBeInTheDocument()
    expect(openInCalendarApp).toHaveBeenCalledWith(
      'webcal://api.example.test/calendar/feed/secret-token-abc.ics'
    )
  })

  it('G: a failed create says so under the chosen row and closes the tab it opened', async () => {
    const user = setupUser()
    server.mint = async () => {
      throw new Error('500')
    }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Google Calendar' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.createFailed
    )
    expect(tabs[0].closed).toBe(true)
    expect(writeText).not.toHaveBeenCalled()
  })

  it('a failed load says so under the chosen row', async () => {
    const user = setupUser()
    server.status = { has_token: true, reads: [] }
    server.token = new Error('500')
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.loadFailed
    )
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

  it('H: a link that cannot be shown again turns the panel to Settings', async () => {
    const user = setupUser()
    server.status = { has_token: true, reads: [google(2 * HOUR)] }
    server.token = { has_token: true, url_recoverable: false }
    const panel = await renderPanel()

    await user.click(within(panel).getByRole('button', { name: 'Copy link' }))

    expect(await within(panel).findByText(PANEL_COPY.unrecoverable)).toBeInTheDocument()
    expect(rowNames(panel)).toEqual([])
    expect(
      within(panel).getByText('Checked 2h ago, since your last save.')
    ).toBeInTheDocument()
    expect(
      within(panel).getByRole('link', { name: PANEL_COPY.manageInSettings })
    ).toBeInTheDocument()
    expect(within(panel).queryByRole('alert')).not.toBeInTheDocument()
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
    server.status = { has_token: true, reads: [google(12 * DAY)] }
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
    server.status = { has_token: true, reads: [readOf('apple_ios', 20 * MINUTE)] }
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

  it('email unavailable (503) gets the generic line, and a later failure takes the sent line down', async () => {
    const user = setupUser()
    const panel = await renderPanel()
    await user.click(within(panel).getByRole('button', { name: 'Email me the steps' }))
    await within(panel).findByText(/^Sent to /)

    server.emailSteps = async () => {
      throw Object.assign(new Error('x'), { status: 503 })
    }
    await user.click(within(panel).getByRole('button', { name: 'Send again' }))

    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      PANEL_COPY.emailFailed
    )
    expect(within(panel).queryByText(/^Sent to /)).not.toBeInTheDocument()
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

  it('has no em or en dashes', () => {
    for (const s of strings) expect(s).not.toMatch(/[–—]/)
  })

  it('keeps to the vocabulary: no sync, feed, subscribe, key or read', () => {
    for (const s of strings) {
      expect(s).not.toMatch(/\b(synced|sync|feed|subscribe|key|read)\b/i)
    }
  })
})
