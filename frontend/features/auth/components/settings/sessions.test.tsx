import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { queryKeys } from '@/lib/queryClient'
import { SESSIONS_SETTINGS_ANCHOR, SESSIONS_SETTINGS_HREF, SessionsSettings } from './sessions'

// The card runs on the real mutation hook; only the request is stubbed.
const mockApiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
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
const REFUSAL_LINE = 'Failed to sign out other devices. Please try again.'
const CONFIRM_BODY =
  'Every other device and browser signed in to your account will be signed out and will need to sign in again. This device stays signed in. Calendar feeds keep working.'

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
    mockApiRequest.mockReset()
    mockCaptureException.mockReset()
    mockRouterPush.mockReset()
    mockRouterReplace.mockReset()
  })

  it('renders the card copy', () => {
    renderWithProviders(<SessionsSettings />)

    expect(screen.getByText('Sessions')).toBeInTheDocument()
    expect(
      screen.getByText("Sign out on every other device and browser where you're signed in.")
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign out everywhere else' })).toBeInTheDocument()
    expect(
      screen.getByText('This device stays signed in. Calendar feeds keep working.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('asks before signing anything out, and Cancel sends nothing', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SessionsSettings />)

    const dialog = await openConfirm(user)
    expect(dialog).toHaveTextContent('Sign out everywhere else?')
    expect(dialog).toHaveTextContent(CONFIRM_BODY)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('confirming posts the request and shows the success line, leaving this device signed in', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })
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
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
    expect(mockApiRequest).toHaveBeenCalledWith(
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
    mockApiRequest.mockRejectedValueOnce(new Error('HTTP 503: Service Unavailable'))
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
    mockApiRequest.mockRejectedValueOnce(
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
    mockApiRequest.mockRejectedValueOnce(new Error('HTTP 503'))
    let resolveSecond: (value: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
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
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('while the request runs the dialog cannot be dismissed and every control is disabled', async () => {
    let resolveRequest: (value: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
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
