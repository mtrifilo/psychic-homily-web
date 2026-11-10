import { describe, it, expect, vi, beforeEach, onTestFinished } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { queryKeys } from '@/lib/queryClient'
import { SESSIONS_SETTINGS_ANCHOR, SESSIONS_SETTINGS_HREF, SessionsSettings } from './sessions'
import { CALENDAR_FEED_SETTINGS_ANCHOR, CALENDAR_FEED_SETTINGS_HREF } from './calendar-feed-anchor'

// The card runs on the real mutation and query hooks; only the requests are
// stubbed, one stub per endpoint.
const mockSignOutRequest = vi.fn()
const mockFeedStatusRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (url: string, init?: unknown) =>
    /\/calendar\/token\/feed-status$/.test(url)
      ? mockFeedStatusRequest(url, init)
      : mockSignOutRequest(url, init),
}))

const mockCaptureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => mockCaptureException(...args),
}))

const mockRouterPush = vi.fn()
const mockRouterReplace = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockRouterPush, replace: mockRouterReplace }),
  usePathname: () => '/profile',
  useSearchParams: () => new URLSearchParams(),
}))

const SUCCESS_LINE = 'Signed out everywhere else. This device is still signed in.'
const RESET_SUCCESS_LINE =
  'Signed out everywhere else. This device is still signed in. Your calendar link was reset too; add it again under Calendar on this page.'
const REFUSAL_LINE = 'Failed to sign out other devices. Please try again.'
const HELPER_LINE = 'This device stays signed in.'
const HELPER_LINE_WITH_LINK = 'This device stays signed in. Your calendar link is reset too.'
const CONFIRM_BODY =
  'Every other device and browser signed in to your account will be signed out and will need to sign in again. This device stays signed in.'
const CONFIRM_BODY_WITH_LINK = `${CONFIRM_BODY} Your calendar link is reset too.`

const NO_LINK = { has_token: false }
const HAS_LINK = { has_token: true, created_at: '2026-11-01T12:00:00Z' }
const RESET_BY_SIGN_OUT = {
  has_token: false,
  link_reset_at: '2026-11-10T12:00:00Z',
  link_reset_reason: 'sign_out_everywhere',
}

async function openConfirm(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Sign out everywhere else' }))
  return screen.findByRole('dialog')
}

async function confirmSignOut(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await openConfirm(user)
  await user.click(within(dialog).getByRole('button', { name: 'Sign out everywhere else' }))
}

describe('SessionsSettings', () => {
  beforeEach(() => {
    mockSignOutRequest.mockReset()
    mockFeedStatusRequest.mockReset()
    mockFeedStatusRequest.mockResolvedValue(NO_LINK)
    mockCaptureException.mockReset()
    mockRouterPush.mockReset()
    mockRouterReplace.mockReset()
  })

  it('renders the card copy', async () => {
    renderWithProviders(<SessionsSettings />)

    expect(screen.getByText('Sessions')).toBeInTheDocument()
    expect(
      screen.getByText("Sign out on every other device and browser where you're signed in.")
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign out everywhere else' })).toBeInTheDocument()
    await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(1))
    expect(screen.getByText(HELPER_LINE)).toBeInTheDocument()
    expect(screen.queryByText(/calendar/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('says the calendar link is reset too when the account has one', async () => {
    mockFeedStatusRequest.mockResolvedValue(HAS_LINK)
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    expect(await screen.findByText(HELPER_LINE_WITH_LINK)).toBeInTheDocument()
    const dialog = await openConfirm(user)
    expect(dialog).toHaveTextContent(CONFIRM_BODY_WITH_LINK)
    expect(mockFeedStatusRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/calendar\/token\/feed-status$/),
      expect.objectContaining({ method: 'GET' })
    )
  })

  it('leaves the calendar sentence out while the status is loading', async () => {
    mockFeedStatusRequest.mockImplementation(() => new Promise(() => {}))
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    expect(screen.getByText(HELPER_LINE)).toBeInTheDocument()
    const dialog = await openConfirm(user)
    expect(dialog).toHaveTextContent(CONFIRM_BODY)
    expect(dialog).not.toHaveTextContent(/calendar/i)
  })

  it('leaves the calendar sentence out when the status cannot be read', async () => {
    mockFeedStatusRequest.mockRejectedValue(new Error('HTTP 503'))
    renderWithProviders(<SessionsSettings />)

    await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalled())
    expect(screen.getByText(HELPER_LINE)).toBeInTheDocument()
    expect(screen.queryByText(/calendar/i)).not.toBeInTheDocument()
  })

  it('asks before signing anything out, and Cancel sends nothing', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    const dialog = await openConfirm(user)
    expect(dialog).toHaveTextContent('Sign out everywhere else?')
    expect(dialog).toHaveTextContent(CONFIRM_BODY)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(mockSignOutRequest).not.toHaveBeenCalled()
  })

  it('confirming posts the request and shows the success line, leaving this device signed in', async () => {
    mockSignOutRequest.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    const queryClient = createTestQueryClient()
    const profile = { success: true, user: { id: 7, email: 'reader@example.com' } }
    // Nothing observes the profile here, so it is kept from garbage
    // collection; a removal would then be the code's, not the test client's.
    queryClient.setQueryDefaults(queryKeys.auth.profile, { gcTime: Infinity })
    queryClient.setQueryData(queryKeys.auth.profile, profile)
    renderWithProviders(<SessionsSettings />, { queryClient })

    await confirmSignOut(user)

    expect(await screen.findByRole('status')).toHaveTextContent(SUCCESS_LINE)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByText(REFUSAL_LINE)).not.toBeInTheDocument()
    expect(mockSignOutRequest).toHaveBeenCalledTimes(1)
    expect(mockSignOutRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/sessions\/sign-out-everywhere-else$/),
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )

    // No sign-out, no redirect: the profile is still the signed-in one.
    expect(queryClient.getQueryData(queryKeys.auth.profile)).toEqual(profile)
    expect(mockRouterPush).not.toHaveBeenCalled()
    expect(mockRouterReplace).not.toHaveBeenCalled()
    // The control stays for a second press.
    expect(screen.getByRole('button', { name: 'Sign out everywhere else' })).toBeEnabled()
  })

  it('shows the refusal line when the request fails, not the success line', async () => {
    mockSignOutRequest.mockRejectedValueOnce(new Error('HTTP 503: Service Unavailable'))
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    await confirmSignOut(user)

    expect(await screen.findByRole('alert')).toHaveTextContent(REFUSAL_LINE)
    expect(screen.queryByText(SUCCESS_LINE)).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mockCaptureException).toHaveBeenCalledTimes(1)
    expect(mockRouterPush).not.toHaveBeenCalled()
  })

  // The card's own render only. In the app the 401 also reaches the query
  // client's session-expiry handling, which settles the viewer as signed out.
  it('renders the refusal line, not the success line, for a 401 revoked-session refusal', async () => {
    mockSignOutRequest.mockRejectedValueOnce(
      new AuthError('Invalid token', AuthErrorCode.TOKEN_INVALID, { status: 401 })
    )
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    await confirmSignOut(user)

    expect(await screen.findByRole('alert')).toHaveTextContent(REFUSAL_LINE)
    expect(screen.queryByText(SUCCESS_LINE)).not.toBeInTheDocument()
    expect(mockCaptureException).toHaveBeenCalledTimes(1)
  })

  it('a second press clears the previous outcome while it runs', async () => {
    mockSignOutRequest.mockRejectedValueOnce(new Error('HTTP 503'))
    let resolveSecond: (value: unknown) => void = () => {}
    mockSignOutRequest.mockImplementationOnce(
      () => new Promise(resolve => { resolveSecond = resolve })
    )
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    await confirmSignOut(user)
    expect(await screen.findByRole('alert')).toHaveTextContent(REFUSAL_LINE)

    await confirmSignOut(user)
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())

    resolveSecond({ success: true })
    expect(await screen.findByRole('status')).toHaveTextContent(SUCCESS_LINE)
  })

  it('Escape closes the dialog when nothing is pending, and sends nothing', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    await openConfirm(user)
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(mockSignOutRequest).not.toHaveBeenCalled()
  })

  it('while the request runs the dialog cannot be dismissed and every control is disabled', async () => {
    let resolveRequest: (value: unknown) => void = () => {}
    mockSignOutRequest.mockImplementationOnce(
      () => new Promise(resolve => { resolveRequest = resolve })
    )
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    await confirmSignOut(user)
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByLabelText('Signing out')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: 'Signing out' })).toBeDisabled()
    // The card's trigger sits behind the modal, so it is reached as hidden.
    expect(
      screen.getByRole('button', { name: 'Sign out everywhere else', hidden: true })
    ).toBeDisabled()

    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Close' }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    resolveRequest({ success: true })
    expect(await screen.findByRole('status')).toHaveTextContent(SUCCESS_LINE)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mockCaptureException).not.toHaveBeenCalled()
  })

  describe('after the action', () => {
    it('says the link was reset when the status read after the action carries this action\'s marker', async () => {
      mockFeedStatusRequest.mockResolvedValueOnce(HAS_LINK).mockResolvedValue(RESET_BY_SIGN_OUT)
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(<SessionsSettings />)
      expect(await screen.findByText(HELPER_LINE_WITH_LINK)).toBeInTheDocument()

      await confirmSignOut(user)

      await waitFor(() => expect(screen.getByRole('status').textContent).toBe(RESET_SUCCESS_LINE))
      expect(screen.getByRole('link', { name: 'Calendar' })).toHaveAttribute(
        'href',
        CALENDAR_FEED_SETTINGS_HREF
      )
      // The link is gone, so the line under the button no longer mentions it.
      expect(screen.getByText(HELPER_LINE)).toBeInTheDocument()
      // The status was read again, after the sign-out request.
      expect(mockFeedStatusRequest).toHaveBeenCalledTimes(2)
      expect(mockFeedStatusRequest.mock.invocationCallOrder[1]).toBeGreaterThan(
        mockSignOutRequest.mock.invocationCallOrder[0]
      )
    })

    it('shows the shipped line when no link a calendar app was reading was reset', async () => {
      mockFeedStatusRequest.mockResolvedValueOnce(HAS_LINK).mockResolvedValue(NO_LINK)
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(<SessionsSettings />)
      await screen.findByText(HELPER_LINE_WITH_LINK)

      await confirmSignOut(user)

      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(2))
      await waitFor(() => expect(screen.getByText(HELPER_LINE)).toBeInTheDocument())
      expect(screen.getByRole('status').textContent).toBe(SUCCESS_LINE)
      expect(screen.queryByRole('link', { name: 'Calendar' })).not.toBeInTheDocument()
    })

    it('ignores a marker another action left', async () => {
      mockFeedStatusRequest.mockResolvedValue({
        ...RESET_BY_SIGN_OUT,
        link_reset_reason: 'password_reset',
      })
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(<SessionsSettings />)
      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(1))

      await confirmSignOut(user)

      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(2))
      expect(screen.getByRole('status').textContent).toBe(SUCCESS_LINE)
    })

    it('does not reuse the status read before the action while the new read is pending', async () => {
      // A marker already on the account before the action, then a read that
      // never answers.
      mockFeedStatusRequest
        .mockResolvedValueOnce(RESET_BY_SIGN_OUT)
        .mockImplementation(() => new Promise(() => {}))
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(<SessionsSettings />)
      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(1))

      await confirmSignOut(user)

      expect(await screen.findByRole('status')).toHaveTextContent(SUCCESS_LINE)
      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(2))
      expect(screen.getByRole('status').textContent).toBe(SUCCESS_LINE)
    })

    it('shows the shipped line when the status cannot be read after the action', async () => {
      mockFeedStatusRequest
        .mockResolvedValueOnce(HAS_LINK)
        .mockRejectedValue(new Error('HTTP 503'))
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(<SessionsSettings />)
      await screen.findByText(HELPER_LINE_WITH_LINK)

      await confirmSignOut(user)

      await waitFor(() => expect(mockFeedStatusRequest).toHaveBeenCalledTimes(2))
      expect((await screen.findByRole('status')).textContent).toBe(SUCCESS_LINE)
      expect(screen.getByText(HELPER_LINE)).toBeInTheDocument()
    })

    it('"Calendar" reveals this page\'s calendar card in place', async () => {
      const scrollIntoView = vi.fn()
      const originalScrollIntoView = Element.prototype.scrollIntoView
      Element.prototype.scrollIntoView = scrollIntoView
      onTestFinished(() => {
        Element.prototype.scrollIntoView = originalScrollIntoView
      })
      mockFeedStatusRequest.mockResolvedValueOnce(HAS_LINK).mockResolvedValue(RESET_BY_SIGN_OUT)
      mockSignOutRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(
        <div>
          <div id={CALENDAR_FEED_SETTINGS_ANCHOR} tabIndex={-1}>
            Calendar feed
          </div>
          <SessionsSettings />
        </div>
      )
      await screen.findByText(HELPER_LINE_WITH_LINK)
      await confirmSignOut(user)
      const link = await screen.findByRole('link', { name: 'Calendar' })
      const hashBefore = window.location.hash

      await user.click(link)

      const card = document.getElementById(CALENDAR_FEED_SETTINGS_ANCHOR)
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(scrollIntoView.mock.contexts[0]).toBe(card)
      expect(card).toHaveFocus()
      expect(window.location.hash).toBe(hashBefore)
    })
  })

  // The password reset's "Review signed-in devices" links here.
  it('is the target of its own Settings fragment', () => {
    renderWithProviders(<SessionsSettings />)

    const card = document.getElementById(SESSIONS_SETTINGS_ANCHOR)
    expect(card).not.toBeNull()
    expect(card).toHaveTextContent('Sessions')
    expect(card).toHaveAttribute('tabindex', '-1')
    expect(SESSIONS_SETTINGS_HREF).toBe('/profile?tab=settings#sessions')
  })
})
