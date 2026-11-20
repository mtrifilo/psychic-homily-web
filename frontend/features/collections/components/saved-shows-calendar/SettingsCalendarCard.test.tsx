import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'
import type {
  CalendarFeedRead,
  CalendarTokenStatusResponse,
} from '@/features/shows/types'
import {
  CALENDAR_CLIENT_NAMES,
  calendarCheckedLine,
  calendarStoppedLine,
} from '@/features/collections/lib/calendarFeedStatus'
import { SettingsCalendarCard } from './SettingsCalendarCard'
import { PANEL_COPY, addAgainLabel } from './savedShowsCalendarCopy'
import {
  SETTINGS_CALENDAR_COPY as COPY,
  SETTINGS_RESET_SENTENCE,
} from './settingsCalendarCopy'

// The card against stubbed endpoints: the real hooks, model and pieces, with
// the network, the viewer, the device and the clipboard faked.

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
  usePathname: () => '/profile',
}))

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

const TOKEN = 'secret-token-abc'
const FEED_URL = `https://api.example.test/feeds/${TOKEN}/saved-shows.ics`
const FOLLOWS_URL = `https://api.example.test/feeds/${TOKEN}/follows.atom`
const NEW_TOKEN = 'fresh-token-xyz'
const NEW_FEED_URL = `https://api.example.test/feeds/${NEW_TOKEN}/saved-shows.ics`
const NEW_FOLLOWS_URL = `https://api.example.test/feeds/${NEW_TOKEN}/follows.atom`

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
  /** Answers GET /calendar/token; an Error rejects, a promise holds the answer. */
  token: CalendarTokenStatusResponse | Error | Promise<CalendarTokenStatusResponse>
  mint: () => Promise<unknown>
  turnOff: () => Promise<unknown>
  newestSavedAt: string | null
  /** Holds or fails the saved-shows read when set. */
  savedShows?: Promise<unknown> | Error
  emailSteps: () => Promise<unknown>
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

function linkStatus(reads: CalendarFeedRead[] = []): CalendarTokenStatusResponse {
  return {
    has_token: true,
    url_recoverable: true,
    feed_url: FEED_URL,
    follows_feed_url: FOLLOWS_URL,
    reads,
  }
}

/** The account has a link whose addresses can be shown, with these checks. */
function withLink(reads: CalendarFeedRead[] = []) {
  server.token = linkStatus(reads)
}

/** The account has a link made before its address was stored. */
function withOldLink(reads: CalendarFeedRead[] = []) {
  server.token = { has_token: true, url_recoverable: false, reads }
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

/** Keeps jsdom from following a link (it cannot); React handlers still run. */
function stopNavigation(event: MouseEvent) {
  if ((event.target as Element).closest('a')) event.preventDefault()
}

let mints = 0

beforeEach(() => {
  apiRequest.mockReset()
  mints = 0
  server = {
    token: { has_token: false },
    mint: async () => {
      // Every create mints a new key: the first is NEW_TOKEN, later ones
      // differ again, as on the server.
      mints += 1
      const token = mints === 1 ? NEW_TOKEN : `${NEW_TOKEN}-${mints}`
      const feedUrl = `https://api.example.test/feeds/${token}/saved-shows.ics`
      const followsUrl = `https://api.example.test/feeds/${token}/follows.atom`
      server.token = {
        has_token: true,
        url_recoverable: true,
        feed_url: feedUrl,
        follows_feed_url: followsUrl,
      }
      return {
        token,
        feed_url: feedUrl,
        follows_feed_url: followsUrl,
        created_at: ago(0),
      }
    },
    turnOff: async () => {
      server.token = { has_token: false }
      return { success: true, message: 'ok' }
    },
    newestSavedAt: ago(5 * HOUR),
    emailSteps: async () => undefined,
  }
  apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
    const url = String(endpoint)
    const verb = method(options)
    if (url.endsWith('/calendar/token') && verb === 'GET') {
      if (server.token instanceof Error) throw server.token
      return server.token
    }
    if (url.endsWith('/calendar/token') && verb === 'POST') return server.mint()
    if (url.endsWith('/calendar/token') && verb === 'DELETE') {
      return server.turnOff()
    }
    if (url.includes('/calendar/token/reads?client=') && verb === 'DELETE') {
      return { success: true, message: 'ok' }
    }
    if (url.endsWith('/calendar/token/feed-status')) return server.token
    if (url.endsWith('/calendar/token/email-steps') && verb === 'POST') {
      return server.emailSteps()
    }
    if (url.includes('/saved-shows?')) {
      if (server.savedShows instanceof Error) throw server.savedShows
      if (server.savedShows) return server.savedShows
      const shows = server.newestSavedAt
        ? [{ id: 1, saved_at: server.newestSavedAt }]
        : []
      return { shows, total: shows.length, limit: 1, offset: 0 }
    }
    throw new Error(`unexpected request ${verb} ${url}`)
  })
  writeText.mockReset().mockResolvedValue(undefined)
  vi.spyOn(window, 'open').mockReturnValue(null)
  useDevice(UA.mac)
  document.addEventListener('click', stopNavigation, true)
})

afterEach(() => {
  document.removeEventListener('click', stopNavigation, true)
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

function renderCard(client = createTestQueryClient()) {
  return render(<SettingsCalendarCard id="calendar" />, {
    wrapper: createWrapperWithClient(client),
  })
}

/** The card once its status has been read. */
async function renderReadCard() {
  const view = renderCard()
  await waitFor(() =>
    expect(screen.queryByText(PANEL_COPY.loading)).not.toBeInTheDocument()
  )
  return view
}

const addButtonNames: string[] = [
  COPY.addGoogle,
  COPY.addApple,
  COPY.outlookCom,
  COPY.outlookWork,
  PANEL_COPY.emailSteps,
]

/** The add controls the card offers, by name, in order. */
function addControls(): string[] {
  return [
    ...screen.queryAllByRole('button'),
    ...screen.queryAllByRole('link'),
  ]
    .map(el => el.textContent ?? '')
    .filter(name => addButtonNames.includes(name))
    .sort((a, b) => addButtonNames.indexOf(a) - addButtonNames.indexOf(b))
}

function fieldValues(): string[] {
  return screen
    .queryAllByTestId('calendar-link-field')
    .map(el => (el as HTMLInputElement).value)
}

describe('SettingsCalendarCard', () => {
  it('draws its title and sub, and says it is checking while the status loads', () => {
    apiRequest.mockImplementation(() => new Promise(() => {}))
    renderCard()
    expect(screen.getByText(COPY.title)).toBeInTheDocument()
    expect(screen.getByText(COPY.sub)).toBeInTheDocument()
    expect(screen.getByText(PANEL_COPY.loading)).toBeInTheDocument()
  })

  describe('no link yet', () => {
    it('offers only "Add to calendar" and makes no link on load', async () => {
      await renderReadCard()
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: COPY.addToCalendar })
      ).toBeInTheDocument()
      expect(addControls()).toEqual([])
      expect(fieldValues()).toEqual([])
      expect(screen.queryByRole('button', { name: COPY.newLink })).toBeNull()
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    })

    it('after a turn-off reads as never set up', async () => {
      // Turning off clears the reset marker, so nothing but has_token says it.
      server.token = { has_token: false }
      await renderReadCard()
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: COPY.addToCalendar })
      ).toBeInTheDocument()
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    })

    it.each(['sign_out_everywhere', 'password_reset'] as const)(
      'after a %s reset says so, then offers the one button',
      async reason => {
        server.token = {
          has_token: false,
          link_reset_at: ago(HOUR),
          link_reset_reason: reason,
        }
        await renderReadCard()
        expect(screen.getByText(SETTINGS_RESET_SENTENCE[reason])).toBeInTheDocument()
        expect(screen.queryByText(COPY.noneChecking)).toBeNull()
        expect(
          screen.getByRole('button', { name: COPY.addToCalendar })
        ).toBeInTheDocument()
        expect(addControls()).toEqual([])
        expect(calls('/calendar/token', 'POST')).toHaveLength(0)
      }
    )

    it('"Add to calendar" makes the link, then shows the add buttons and both addresses', async () => {
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))

      await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
      expect(calls('/calendar/token', 'POST')).toHaveLength(1)
      expect(addControls()).toEqual([
        COPY.addGoogle,
        COPY.addApple,
        COPY.outlookCom,
        COPY.outlookWork,
      ])
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
    })

    it('keeps the reset sentence after "Add to calendar" made the new link', async () => {
      server.token = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: 'password_reset',
      }
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      await waitFor(() => expect(fieldValues()).toHaveLength(2))
      expect(
        screen.getByText(SETTINGS_RESET_SENTENCE.password_reset)
      ).toBeInTheDocument()
    })

    it.each([
      ['New link', COPY.newLink, COPY.confirmNewLinkAction],
      ['Turn off', COPY.turnOff, COPY.confirmTurnOffAction],
    ])('ends the reset sentence once %s changes the link', async (_case, open, confirm) => {
      server.token = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: 'sign_out_everywhere',
      }
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      await waitFor(() => expect(fieldValues()).toHaveLength(2))
      expect(
        screen.getByText(SETTINGS_RESET_SENTENCE.sign_out_everywhere)
      ).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: open }))
      await user.click(screen.getByRole('button', { name: confirm }))
      await waitFor(() =>
        expect(
          screen.queryByText(SETTINGS_RESET_SENTENCE.sign_out_everywhere)
        ).toBeNull()
      )
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
    })

    it('keeps the reset sentence when a create whose answer was lost made the link', async () => {
      server.token = {
        has_token: false,
        link_reset_at: ago(HOUR),
        link_reset_reason: 'password_reset',
      }
      const mint = server.mint
      server.mint = async () => {
        await mint()
        throw new Error('connection reset')
      }
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      // The re-read after the failure finds the link the create made.
      await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
      expect(document.getElementById('calendar')).toHaveFocus()
      expect(
        screen.getByText(SETTINGS_RESET_SENTENCE.password_reset)
      ).toBeInTheDocument()
      expect(calls('/calendar/token', 'POST')).toHaveLength(1)
    })

    it('shows the one approved line when the fresh read before creating fails', async () => {
      const user = setupUser()
      await renderReadCard()
      server.token = new Error('down')
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        PANEL_COPY.createFailed
      )
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    })

    it('does not replace a link another tab made since the card read the status', async () => {
      const user = setupUser()
      await renderReadCard()
      withLink()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      await waitFor(() => expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL]))
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    })

    it('says so when the link cannot be made, and tries again', async () => {
      let fail = true
      const mint = server.mint
      server.mint = () => (fail ? Promise.reject(new Error('down')) : mint())
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addToCalendar }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        PANEL_COPY.createFailed
      )

      fail = false
      await user.click(screen.getByRole('button', { name: 'Try again.' }))
      await waitFor(() => expect(fieldValues()).toHaveLength(2))
    })
  })

  describe('status', () => {
    it('connected: the app has its two-line block and no add button', async () => {
      withLink([readOf('google', 2 * HOUR)])
      await renderReadCard()
      expect(screen.getByText(CALENDAR_CLIENT_NAMES.google)).toBeInTheDocument()
      expect(
        screen.getByText(
          calendarCheckedLine(ago(2 * HOUR), server.newestSavedAt, new Date())
        )
      ).toBeInTheDocument()
      expect(screen.queryByText(COPY.noneChecking)).toBeNull()
      expect(addControls()).toEqual([
        COPY.addApple,
        COPY.outlookCom,
        COPY.outlookWork,
      ])
      expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL])
    })

    it('a link nothing has checked: the sentence, every add button and the links', async () => {
      withLink()
      await renderReadCard()
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
      expect(addControls()).toEqual([
        COPY.addGoogle,
        COPY.addApple,
        COPY.outlookCom,
        COPY.outlookWork,
      ])
      expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL])
      expect(screen.getByText(COPY.note)).toBeInTheDocument()
    })

    it('stopped: its line carries "Add to ... again" and "I removed it", and the app has no add button', async () => {
      withLink([readOf('google', 12 * DAY)])
      await renderReadCard()
      const stoppedLine = calendarStoppedLine(
        {
          client: 'google',
          name: CALENDAR_CLIENT_NAMES.google,
          lastReadAt: ago(12 * DAY),
        },
        new Date()
      )
      expect(screen.getByText(stoppedLine)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: addAgainLabel('google') })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: PANEL_COPY.removedIt })
      ).toBeInTheDocument()
      expect(addControls()).toEqual([
        COPY.addApple,
        COPY.outlookCom,
        COPY.outlookWork,
      ])
    })

    it('"I removed it" forgets the app', async () => {
      withLink([readOf('google', 12 * DAY)])
      const user = setupUser()
      await renderReadCard()
      server.token = linkStatus()
      await user.click(screen.getByRole('button', { name: PANEL_COPY.removedIt }))
      expect(calls('/calendar/token/reads?client=google', 'DELETE')).toHaveLength(1)
      await waitFor(() =>
        expect(screen.queryByRole('button', { name: PANEL_COPY.removedIt })).toBeNull()
      )
    })
  })

  describe('old link whose address cannot be shown', () => {
    it('with an app connected: its sentence, New link and Turn off, no fields and no add buttons', async () => {
      withOldLink([readOf('apple_mac', HOUR)])
      await renderReadCard()
      expect(screen.getByText(CALENDAR_CLIENT_NAMES.apple_mac)).toBeInTheDocument()
      expect(screen.getByText(COPY.oldLinkConnected)).toBeInTheDocument()
      expect(fieldValues()).toEqual([])
      expect(addControls()).toEqual([])
      expect(screen.getByRole('button', { name: COPY.newLink })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: COPY.turnOff })).toBeInTheDocument()
      expect(screen.getByText(COPY.note)).toBeInTheDocument()
    })

    it.each([
      ['nothing connected', [] as CalendarFeedRead[]],
      ['only a stopped app', [readOf('google', 12 * DAY)]],
    ])('with %s: the shorter sentence', async (_case, reads) => {
      withOldLink(reads)
      await renderReadCard()
      expect(screen.getByText(COPY.oldLinkNoApp)).toBeInTheDocument()
      expect(screen.queryByText(COPY.oldLinkConnected)).toBeNull()
      expect(screen.queryByRole('button', { name: addAgainLabel('google') })).toBeNull()
      expect(fieldValues()).toEqual([])
    })

    it('a new link shows its addresses and the add buttons', async () => {
      withOldLink()
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.newLink }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmNewLinkAction })
      )
      await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
      expect(addControls()).toHaveLength(4)
    })
  })

  describe('devices', () => {
    it('a Windows computer: Google and both Outlooks, no Apple', async () => {
      useDevice(UA.windows)
      withLink()
      await renderReadCard()
      expect(addControls()).toEqual([
        COPY.addGoogle,
        COPY.outlookCom,
        COPY.outlookWork,
      ])
    })

    it('an iPhone in Safari: Apple with the Google Calendar app hint, then Email me the steps', async () => {
      useDevice(UA.iphoneSafari)
      withLink()
      await renderReadCard()
      expect(addControls()).toEqual([COPY.addApple, PANEL_COPY.emailSteps])
      expect(screen.getByText(PANEL_COPY.iphoneGoogleAppHint)).toBeInTheDocument()
      expect(screen.getByRole('link', { name: COPY.addApple })).toHaveAttribute(
        'href',
        FEED_URL.replace(/^https:/, 'webcal:')
      )
    })

    it('an iPhone outside Safari is told to open the page in Safari', async () => {
      useDevice(UA.iphoneChrome)
      withLink()
      await renderReadCard()
      expect(addControls()).toEqual([PANEL_COPY.emailSteps])
      expect(screen.getByText(PANEL_COPY.iphoneAppleNotSafari)).toBeInTheDocument()
    })

    it('an Android phone: Email me the steps only', async () => {
      useDevice(UA.android)
      withLink()
      await renderReadCard()
      expect(addControls()).toEqual([PANEL_COPY.emailSteps])
    })

    it.each([UA.iphoneSafari, UA.android])(
      'a phone never offers a Google add control, even for a stopped Google calendar (%#)',
      async agent => {
        useDevice(agent)
        withLink([readOf('google', 12 * DAY)])
        await renderReadCard()
        expect(screen.queryByText(COPY.addGoogle)).toBeNull()
        expect(screen.queryByText(addAgainLabel('google'))).toBeNull()
        expect(
          screen.getByRole('button', { name: PANEL_COPY.removedIt })
        ).toBeInTheDocument()
        expect(addControls()).toContain(PANEL_COPY.emailSteps)
      }
    )

    it('Email me the steps sends from the card', async () => {
      useDevice(UA.android)
      withLink()
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: PANEL_COPY.emailSteps }))
      expect(calls('/calendar/token/email-steps', 'POST')).toHaveLength(1)
      expect(
        await screen.findByText('Sent to fan@example.com. Open it on a computer.')
      ).toBeInTheDocument()
    })
  })

  describe('add buttons act on the link already read', () => {
    it('Google copies the address and opens Google in the click', async () => {
      withLink()
      const user = setupUser()
      await renderReadCard()
      const before = apiRequest.mock.calls.length
      await user.click(screen.getByRole('button', { name: COPY.addGoogle }))
      expect(writeText).toHaveBeenCalledWith(FEED_URL)
      expect(window.open).toHaveBeenCalledTimes(1)
      expect(apiRequest.mock.calls.length).toBe(before)
    })

    it('a refused copy in the Google two-step points to the field', async () => {
      withLink()
      writeText.mockRejectedValue(new Error('denied'))
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.addGoogle }))
      // Once under the button (stacked, below sm) and once under the row.
      expect(await screen.findAllByText(COPY.copyFailed)).toHaveLength(2)
      expect(screen.queryByText(PANEL_COPY.copyFailed)).toBeNull()
      expect(screen.queryByText(PANEL_COPY.manageInSettings)).toBeNull()
    })

    it("puts a row's note under its own button while the buttons stack", async () => {
      useDevice(UA.iphoneSafari)
      withLink()
      const user = setupUser()
      await renderReadCard()
      const apple = screen.getByRole('link', { name: COPY.addApple })
      await user.click(apple)
      const [stacked, beside] = await screen.findAllByText(PANEL_COPY.appleOpened)
      // The stacked copy shares the Apple button's group, ahead of Email me
      // the steps; the other sits after the whole row.
      const appleGroup = apple.closest('div.flex.flex-col')!.parentElement!
      expect(appleGroup).toContainElement(stacked)
      expect(stacked.closest('.sm\\:hidden')).not.toBeNull()
      expect(beside.closest('.hidden.sm\\:flex')).not.toBeNull()
      const email = screen.getByRole('button', { name: PANEL_COPY.emailSteps })
      expect(
        stacked.compareDocumentPosition(email) & Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
    })

    it('Outlook opens Microsoft with the address in a new tab', async () => {
      withLink()
      await renderReadCard()
      const outlook = screen.getByRole('link', { name: COPY.outlookCom })
      expect(outlook).toHaveAttribute('target', '_blank')
      expect(outlook.getAttribute('href')).toContain(encodeURIComponent('saved-shows.ics'))
    })
  })

  describe('links block', () => {
    it('labels each field and copies each address with its own confirmation', async () => {
      withLink()
      const user = setupUser()
      await renderReadCard()
      expect(screen.getByLabelText(COPY.calendarLinkLabel)).toHaveValue(FEED_URL)
      expect(screen.getByLabelText(new RegExp(`^${COPY.followsLinkLabel}`))).toHaveValue(
        FOLLOWS_URL
      )

      const [copyCalendar, copyFollows] = screen.getAllByRole('button', {
        name: COPY.copy,
      })
      await user.click(copyCalendar)
      expect(writeText).toHaveBeenLastCalledWith(FEED_URL)
      expect(await screen.findByText(PANEL_COPY.copied)).toBeInTheDocument()

      await user.click(copyFollows)
      expect(writeText).toHaveBeenLastCalledWith(FOLLOWS_URL)
      expect(await screen.findByText(COPY.followsCopied)).toBeInTheDocument()
    })

    it('says to copy by hand when the browser refuses', async () => {
      withLink()
      writeText.mockRejectedValue(new Error('denied'))
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getAllByRole('button', { name: COPY.copy })[0])
      expect(await screen.findByText(COPY.copyFailed)).toBeInTheDocument()
    })

    it('puts both private classes on every element that carries an address', async () => {
      withLink([readOf('google', 12 * DAY)])
      const { container } = await renderReadCard()
      const carriers = Array.from(container.querySelectorAll<HTMLElement>('*')).filter(
        el =>
          (el.getAttribute('href') ?? '').includes(TOKEN) ||
          ((el as HTMLInputElement).value ?? '').includes(TOKEN) ||
          Array.from(el.childNodes).some(
            node =>
              node.nodeType === Node.TEXT_NODE && (node.textContent ?? '').includes(TOKEN)
          )
      )
      // Both fields, Apple's webcal link and both Outlook links.
      expect(carriers.length).toBeGreaterThanOrEqual(5)
      for (const el of carriers) {
        expect(el).toHaveClass('ph-no-capture')
        expect(el).toHaveClass('sentry-block')
      }
      expect(container.firstElementChild).toHaveClass('ph-no-capture')
    })
  })

  describe('New link', () => {
    it('asks inline, and Cancel leaves the link as it was', async () => {
      withLink()
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.newLink }))
      const question = screen.getByRole('group', { name: COPY.confirmNewLink })
      expect(question).toHaveFocus()
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(
        within(question).getByRole('button', { name: COPY.confirmNewLinkAction })
      ).toHaveClass('bg-destructive')

      await user.click(within(question).getByRole('button', { name: COPY.cancel }))
      expect(screen.queryByRole('group', { name: COPY.confirmNewLink })).toBeNull()
      expect(screen.getByRole('button', { name: COPY.newLink })).toHaveFocus()
      expect(calls('/calendar/token', 'POST')).toHaveLength(0)
    })

    it('replaces both addresses', async () => {
      withLink([readOf('google', HOUR)])
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.newLink }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmNewLinkAction })
      )
      await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
      expect(calls('/calendar/token', 'POST')).toHaveLength(1)
      expect(screen.getByRole('button', { name: COPY.newLink })).toHaveFocus()
      // The new link has no checks yet.
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
    })

    it('closes the question when a create whose answer was lost did replace the link', async () => {
      withLink()
      const mint = server.mint
      server.mint = async () => {
        await mint()
        throw new Error('connection reset')
      }
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.newLink }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmNewLinkAction })
      )
      await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
      expect(screen.queryByRole('group', { name: COPY.confirmNewLink })).toBeNull()
      expect(screen.queryByText(PANEL_COPY.createFailed)).toBeNull()
      expect(screen.getByRole('button', { name: COPY.newLink })).toHaveFocus()
    })

    it('does not pull focus back from elsewhere when a question closes later', async () => {
      withLink()
      let commit: () => void = () => {}
      server.mint = () =>
        new Promise((_resolve, reject) => {
          commit = () => reject(new Error('connection reset'))
        })
      const user = setupUser()
      const outside = document.createElement('input')
      document.body.appendChild(outside)
      try {
        await renderReadCard()
        await user.click(screen.getByRole('button', { name: COPY.newLink }))
        await user.click(
          screen.getByRole('button', { name: COPY.confirmNewLinkAction })
        )
        // The person moves on to another card while the request is out.
        outside.focus()
        // Another window replaced the link; the failed request's re-read
        // finds it and the question closes.
        server.token = {
          has_token: true,
          url_recoverable: true,
          feed_url: NEW_FEED_URL,
          follows_feed_url: NEW_FOLLOWS_URL,
        }
        commit()
        await waitFor(() => expect(fieldValues()).toEqual([NEW_FEED_URL, NEW_FOLLOWS_URL]))
        expect(outside).toHaveFocus()
      } finally {
        outside.remove()
      }
    })

    it('keeps asking, with the create line, when it fails', async () => {
      withLink()
      server.mint = () => Promise.reject(new Error('down'))
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.newLink }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmNewLinkAction })
      )
      expect(await screen.findByRole('alert')).toHaveTextContent(
        PANEL_COPY.createFailed
      )
      expect(screen.getByRole('group', { name: COPY.confirmNewLink })).toBeInTheDocument()
      expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL])
    })
  })

  describe('Turn off', () => {
    it('asks inline, then turns off both addresses and offers "Add to calendar"', async () => {
      withLink()
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.turnOff }))
      const question = screen.getByRole('group', { name: COPY.confirmTurnOff })
      expect(question).toHaveFocus()
      const confirm = within(question).getByRole('button', {
        name: COPY.confirmTurnOffAction,
      })
      expect(confirm).toHaveClass('bg-destructive')
      await user.click(confirm)

      expect(
        await screen.findByRole('button', { name: COPY.addToCalendar })
      ).toBeInTheDocument()
      expect(calls('/calendar/token', 'DELETE')).toHaveLength(1)
      expect(fieldValues()).toEqual([])
      expect(screen.getByText(COPY.noneChecking)).toBeInTheDocument()
    })

    it('settles on the server when the link was already off', async () => {
      withLink()
      // Turned off elsewhere: the DELETE is refused and the link is gone.
      server.turnOff = async () => {
        server.token = { has_token: false }
        throw new Error('no calendar token')
      }
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.turnOff }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmTurnOffAction })
      )
      expect(
        await screen.findByRole('button', { name: COPY.addToCalendar })
      ).toBeInTheDocument()
      expect(fieldValues()).toEqual([])
      // Focus was on the confirm button inside the block that left.
      expect(document.getElementById('calendar')).toHaveFocus()
    })

    it('says so when it fails, and tries again', async () => {
      withLink()
      let fail = true
      const turnOff = server.turnOff
      server.turnOff = () => (fail ? Promise.reject(new Error('down')) : turnOff())
      const user = setupUser()
      await renderReadCard()
      await user.click(screen.getByRole('button', { name: COPY.turnOff }))
      await user.click(
        screen.getByRole('button', { name: COPY.confirmTurnOffAction })
      )
      expect(await screen.findByRole('alert')).toHaveTextContent(COPY.turnOffFailed)
      expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL])

      fail = false
      await user.click(screen.getByRole('button', { name: 'Try again.' }))
      expect(
        await screen.findByRole('button', { name: COPY.addToCalendar })
      ).toBeInTheDocument()
    })
  })

  it('never paints an address held in the cache from before it mounted', async () => {
    const client = createTestQueryClient()
    // Another account's link, left in the tab's cache by an earlier session.
    client.setQueryData(['calendar', 'tokenStatus'], {
      has_token: true,
      url_recoverable: true,
      feed_url: 'https://api.example.test/feeds/someone-else/saved-shows.ics',
      follows_feed_url: 'https://api.example.test/feeds/someone-else/follows.atom',
    })
    let answer: (value: CalendarTokenStatusResponse) => void = () => {}
    // The status read is held; the saved-shows read answers at once.
    server.token = new Promise<CalendarTokenStatusResponse>(resolve => {
      answer = resolve
    })
    renderCard(client)
    await waitFor(() => expect(calls('/calendar/token', 'GET')).toHaveLength(1))
    await waitFor(() =>
      expect(
        apiRequest.mock.calls.some(([endpoint]) =>
          String(endpoint).includes('/saved-shows?')
        )
      ).toBe(true)
    )
    // Give the saved-shows answer a render to land in.
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(screen.getByText(PANEL_COPY.loading)).toBeInTheDocument()
    expect(fieldValues()).toEqual([])
    expect(document.body.innerHTML).not.toContain('someone-else')

    server.token = linkStatus()
    answer(linkStatus())
    await waitFor(() => expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL]))
    expect(document.body.innerHTML).not.toContain('someone-else')
  })

  it('shows the link on a return visit when the status has not changed', async () => {
    withLink()
    const client = createTestQueryClient()
    const first = renderCard(client)
    await waitFor(() => expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL]))
    first.unmount()

    // Back to the card with the same answer waiting and the cache still warm.
    renderCard(client)
    await waitFor(() => expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL]))
    expect(screen.queryByText(PANEL_COPY.loading)).toBeNull()
  })

  it('says it could not load on a return visit whose read fails', async () => {
    withLink()
    const client = createTestQueryClient()
    const first = renderCard(client)
    await waitFor(() => expect(fieldValues()).toHaveLength(2))
    first.unmount()

    server.token = new Error('down')
    renderCard(client)
    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.loadFailed)
    expect(fieldValues()).toEqual([])
  })

  it.each([
    ['still pending', () => new Promise<unknown>(() => {})],
    ['failing', () => Object.assign(new Error('down'), { status: 500 })],
  ])(
    'shows the link and its checked lines while the saved-shows read is %s',
    async (_case, answer) => {
      server.savedShows = answer()
      withLink([readOf('google', 2 * HOUR)])
      renderCard()
      await waitFor(() => expect(fieldValues()).toEqual([FEED_URL, FOLLOWS_URL]))
      // No comparison with a save the card has not read.
      expect(
        screen.getByText(calendarCheckedLine(ago(2 * HOUR), null, new Date()))
      ).toBeInTheDocument()
      expect(screen.getByRole('button', { name: COPY.turnOff })).toBeInTheDocument()
    }
  )

  it('keeps focus in the card when the add button just used leaves as its app connects', async () => {
    withLink()
    const user = setupUser()
    await renderReadCard()
    const apple = screen.getByRole('link', { name: COPY.addApple })
    await user.click(apple)
    apple.focus()
    expect(apple).toHaveFocus()

    // Apple Calendar checks the link; the person comes back to the window.
    withLink([readOf('apple_mac', MINUTE)])
    window.dispatchEvent(new Event('focus'))
    await waitFor(() =>
      expect(screen.queryByRole('link', { name: COPY.addApple })).toBeNull()
    )
    expect(document.getElementById('calendar')).toHaveFocus()
  })

  it('reads the status again when the window regains focus', async () => {
    withLink()
    await renderReadCard()
    const before = calls('/calendar/token', 'GET').length
    withLink([readOf('apple_mac', MINUTE)])
    window.dispatchEvent(new Event('focus'))
    expect(await screen.findByText(CALENDAR_CLIENT_NAMES.apple_mac)).toBeInTheDocument()
    expect(calls('/calendar/token', 'GET').length).toBeGreaterThan(before)
  })

  it('carries the Settings anchor on the real card: id, focusable, and the ref', async () => {
    let anchored: HTMLDivElement | null = null
    render(
      <SettingsCalendarCard id="calendar" anchorRef={el => { anchored = el }} />,
      { wrapper: createWrapperWithClient(createTestQueryClient()) }
    )
    const card = document.getElementById('calendar')
    expect(card).not.toBeNull()
    expect(anchored).toBe(card)
    expect(card).toHaveAttribute('tabindex', '-1')
    card!.focus()
    expect(card).toHaveFocus()
  })

  it('says so when the status cannot be read, and reads again', async () => {
    server.token = new Error('down')
    const user = setupUser()
    renderCard()
    expect(await screen.findByRole('alert')).toHaveTextContent(PANEL_COPY.loadFailed)
    server.token = { has_token: false }
    await user.click(screen.getByRole('button', { name: 'Try again.' }))
    expect(
      await screen.findByRole('button', { name: COPY.addToCalendar })
    ).toBeInTheDocument()
  })
})
