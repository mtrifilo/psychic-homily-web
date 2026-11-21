import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useRef, useState } from 'react'
import { server as msw } from '@/test/mocks/server'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'
import { SavedShowsCalendarPanel } from './saved-shows-calendar/SavedShowsCalendarPanel'
import { CalendarStepsPage } from './saved-shows-calendar/CalendarStepsPage'
import { SettingsCalendarCard } from './saved-shows-calendar/SettingsCalendarCard'
import { CONFIRM_ARMING_DELAY_MS } from './saved-shows-calendar/SettingsCalendarLinks'
import { STEPS_PAGE_COPY } from './saved-shows-calendar/calendarStepsPageCopy'
import { PANEL_COPY } from './saved-shows-calendar/savedShowsCalendarCopy'
import { SETTINGS_CALENDAR_COPY } from './saved-shows-calendar/settingsCalendarCopy'

// Making a link past the account's hourly cap answers 429
// CALENDAR_LINK_RATE_LIMITED_HOURLY. The cap has no wording of its own: each
// surface that makes a link shows the line it shows for any link it could not
// make. Every read is stubbed; the mint goes through the real API client to a
// handler answering exactly what the backend answers, so the error each
// surface receives is the one the client builds from that response.

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: 'authenticated',
    isAuthenticated: true,
    user: { id: '7', email: 'fan@example.com' },
    isLoading: false,
  }),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => '/library/calendar',
  redirect: vi.fn(),
}))

const realApi = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

const CAP_CODE = 'CALENDAR_LINK_RATE_LIMITED_HOURLY'

/** The backend's answer to the eleventh mint in a clock hour. */
function answerTheCap() {
  msw.use(
    http.post('*/calendar/token', () =>
      HttpResponse.json(
        {
          success: false,
          message: "Couldn't create your calendar link. Try again.",
          error_code: CAP_CODE,
          request_id: 'req-cap',
        },
        { status: 429, headers: { 'Retry-After': '1200' } }
      )
    )
  )
}

function method(options: unknown): string {
  return (options as { method?: string } | undefined)?.method ?? 'GET'
}

const mintErrors: unknown[] = []

const FEED_URL = 'https://api.example.test/feeds/secret-token-abc/saved-shows.ics'
const FOLLOWS_URL = 'https://api.example.test/feeds/secret-token-abc/follows.atom'

/** What the account's link reads answer; no link unless a case gives it one. */
let linkStatus: object = { has_token: false }

beforeEach(() => {
  mintErrors.length = 0
  linkStatus = { has_token: false }
  answerTheCap()
  apiRequest.mockImplementation(async (endpoint: string, options: unknown) => {
    const url = String(endpoint)
    const verb = method(options)
    if (url.endsWith('/calendar/token') && verb === 'POST') {
      try {
        return await realApi.apiRequest(endpoint, options as RequestInit)
      } catch (error) {
        mintErrors.push(error)
        throw error
      }
    }
    if (url.endsWith('/calendar/token/feed-status')) return linkStatus
    if (url.endsWith('/calendar/token') && verb === 'GET') return linkStatus
    if (url.includes('/saved-shows?')) return { shows: [], total: 0, limit: 1, offset: 0 }
    throw new Error(`unexpected request ${verb} ${url}`)
  })
})

/** Every mint so far reached the client as the cap's 429. */
function expectEveryMintRefusedByTheCap() {
  expect(mintErrors.length).toBeGreaterThan(0)
  for (const error of mintErrors) {
    expect(error).toMatchObject({ status: 429, errorCode: CAP_CODE, retryAfter: 1200 })
  }
}

function PanelHarness() {
  const anchorRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(true)
  return (
    <>
      <div ref={anchorRef}>
        <button type="button">Saved</button>
      </div>
      <SavedShowsCalendarPanel open={open} onOpenChange={setOpen} anchorRef={anchorRef} />
    </>
  )
}

describe('the hourly cap on making a calendar link', () => {
  it('the saved-shows panel shows its create-failure line', async () => {
    render(<PanelHarness />, { wrapper: createWrapperWithClient(createTestQueryClient()) })
    const panel = await screen.findByTestId('saved-shows-calendar-panel')

    const alert = await within(panel).findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.createFailed)
    expectEveryMintRefusedByTheCap()
  })

  it('/library/calendar shows its create-failure line under Add to calendar', async () => {
    const user = userEvent.setup()
    render(<CalendarStepsPage />, { wrapper: createWrapperWithClient(createTestQueryClient()) })

    await user.click(await screen.findByRole('button', { name: STEPS_PAGE_COPY.addToCalendar }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.createFailed)
    expectEveryMintRefusedByTheCap()
  })

  it('the Settings card shows its create-failure line under Add to calendar', async () => {
    const user = userEvent.setup()
    render(<SettingsCalendarCard id="calendar" />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    await user.click(
      await screen.findByRole('button', { name: SETTINGS_CALENDAR_COPY.addToCalendar })
    )

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.createFailed)
    expect(screen.queryAllByTestId('calendar-link-field')).toHaveLength(0)
    expectEveryMintRefusedByTheCap()
  })

  it('the Settings card keeps the link it has when New link is refused', async () => {
    linkStatus = {
      has_token: true,
      url_recoverable: true,
      feed_url: FEED_URL,
      follows_feed_url: FOLLOWS_URL,
      reads: [],
    }
    const user = userEvent.setup()
    render(<SettingsCalendarCard id="calendar" />, {
      wrapper: createWrapperWithClient(createTestQueryClient()),
    })

    await user.click(
      await screen.findByRole('button', { name: SETTINGS_CALENDAR_COPY.newLink })
    )
    // The confirming button ignores activation until its arming delay passes.
    await act(
      () => new Promise(resolve => setTimeout(resolve, CONFIRM_ARMING_DELAY_MS))
    )
    await user.click(
      screen.getByRole('button', { name: SETTINGS_CALENDAR_COPY.confirmNewLinkAction })
    )

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(PANEL_COPY.createFailed)
    await waitFor(() =>
      expect(
        screen
          .getAllByTestId('calendar-link-field')
          .map(field => (field as HTMLInputElement).value)
      ).toEqual([FEED_URL, FOLLOWS_URL])
    )
    expectEveryMintRefusedByTheCap()
  })
})
