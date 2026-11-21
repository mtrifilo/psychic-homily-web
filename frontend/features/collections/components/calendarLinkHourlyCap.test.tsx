import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useRef, useState } from 'react'
import { server as msw } from '@/test/mocks/server'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'
import { CalendarFeedSection } from './CalendarFeedSection'
import { FollowsActivityFeedSection } from './FollowsActivityFeedSection'
import { SavedShowsCalendarPanel } from './saved-shows-calendar/SavedShowsCalendarPanel'
import { CalendarStepsPage } from './saved-shows-calendar/CalendarStepsPage'
import { STEPS_PAGE_COPY } from './saved-shows-calendar/calendarStepsPageCopy'
import { PANEL_COPY } from './saved-shows-calendar/savedShowsCalendarCopy'

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

beforeEach(() => {
  mintErrors.length = 0
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
    if (url.endsWith('/calendar/token/feed-status')) return { has_token: false }
    if (url.endsWith('/calendar/token') && verb === 'GET') return { has_token: false }
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

  it.each([
    ['saved shows calendar', () => <CalendarFeedSection variant="settings" />],
    ['followed artists activity', () => <FollowsActivityFeedSection />],
  ])('the Settings %s feed card shows its existing mint-failure line', async (_name, card) => {
    const user = userEvent.setup()
    render(card(), { wrapper: createWrapperWithClient(createTestQueryClient()) })

    await user.click(await screen.findByRole('button', { name: 'Enable' }))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Could not update the feed. Please try again.'
      )
    )
    expect(screen.queryByRole('link', { name: 'Sign in again' })).toBeNull()
    expectEveryMintRefusedByTheCap()
  })
})
