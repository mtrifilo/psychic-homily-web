import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { MAGIC_LINK_EXPIRY_MINUTES } from '@/lib/magic-link'
import { PENDING_LINK_GONE_MESSAGE } from '@/app/auth/_components/google-collision-sign-in'
import AuthPage from './page'

// The Sign in tab's collision state, driven from the auth page as the OAuth
// callback lands on it: /auth?error_code=OAUTH_LINK_PENDING, with the address
// read from the pending link the backend holds for this browser.

const mockPush = vi.fn()
let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/auth',
}))

const mockSetUser = vi.fn()
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () => deriveMockAuthSignals({ setUser: mockSetUser, authStatus: 'anonymous' }),
  }
})

const mockLoginMutate = vi.fn()
let mockLoginError: Error | null = null

vi.mock('@/features/auth', () => ({
  useLogin: () => ({
    mutate: mockLoginMutate,
    reset: vi.fn(),
    isPending: false,
    error: mockLoginError,
  }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useSendMagicLink: () => ({ mutate: vi.fn(), isPending: false }),
  useSendVerificationEmail: () => ({
    mutate: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    error: null as Error | null,
  }),
}))

// The pending read and the mailbox send, as the backend answers them.
type PendingRead = {
  isPending: boolean
  isError?: boolean
  data?: { status: 'pending' | 'expired'; email?: string }
}
let mockPendingRead: PendingRead = { isPending: true }
const pendingReadEnabled = vi.fn()

type SendState = {
  isPending: boolean
  isSuccess: boolean
  isError: boolean
  error: Error | null
}
let mockSendState: SendState = { isPending: false, isSuccess: false, isError: false, error: null }
const mockSendMutate = vi.fn()

vi.mock('@/features/auth/hooks/useOAuthLink', () => ({
  SEND_LINK_FAILED_MESSAGE: 'Failed to send magic link. Please try again.',
  usePendingOAuthLink: (enabled: boolean) => {
    pendingReadEnabled(enabled)
    return enabled ? mockPendingRead : { isPending: true }
  },
  useSendOAuthLinkEmail: () => ({ mutate: mockSendMutate, ...mockSendState }),
}))

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: () => <div data-testid="passkey-login" />,
}))
vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: () => <div data-testid="passkey-signup" />,
}))
vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: () => <div data-testid="google-oauth" />,
}))

const COLLISION = 'error_code=OAUTH_LINK_PENDING&returnTo=%2Fshows'
const ADDRESS = 'alice@example.com'

function arriveOnCollision(read: PendingRead = { isPending: false, data: { status: 'pending', email: ADDRESS } }) {
  mockSearchParams = new URLSearchParams(COLLISION)
  mockPendingRead = read
  return renderWithProviders(<AuthPage />)
}

describe('AuthPage collision state', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockReset()
    mockLoginMutate.mockReset()
    mockLoginError = null
    mockSendMutate.mockReset()
    pendingReadEnabled.mockReset()
    mockSendState = { isPending: false, isSuccess: false, isError: false, error: null }
    mockSearchParams = new URLSearchParams()
    window.history.replaceState(null, '', '/')
  })

  it('names the address, prefilled and read-only, and offers the two proofs only', () => {
    arriveOnCollision()

    expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
    const reason = screen.getByText(/An account already uses/)
    expect(reason).toHaveTextContent(
      `An account already uses ${ADDRESS}. Sign in with your password to connect Google.`
    )
    const email = screen.getByLabelText('Email')
    expect(email).toHaveValue(ADDRESS)
    expect(email).toHaveAttribute('readonly')
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in and connect Google' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Email me a sign-in link instead' })).toBeInTheDocument()
    expect(
      screen.getByText(
        'The link connects Google, signs out your other sessions and turns off your current password.'
      )
    ).toBeInTheDocument()

    // A passkey is not a proof here, and Google would only repeat the
    // collision.
    expect(screen.queryByTestId('google-oauth')).not.toBeInTheDocument()
    expect(screen.queryByTestId('passkey-login')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Forgot password?' })).not.toBeInTheDocument()
    expect(screen.queryByText('Sign in to your account')).not.toBeInTheDocument()
  })

  it('offers a different Google account through the account chooser', () => {
    arriveOnCollision()

    const link = screen.getByRole('link', { name: 'Use a different Google account' })
    const href = new URL(link.getAttribute('href') ?? '')
    expect(href.pathname).toBe('/auth/login/google')
    expect(href.searchParams.get('prompt')).toBe('select_account')
    expect(href.searchParams.get('returnTo')).toBe('/shows')
    expect(screen.getByText(/Not your account\?/)).toBeInTheDocument()
  })

  // A reload mid-collision arrives without OAUTH_LINK_PENDING, which leaves
  // the URL after the first render; the cookie read alone puts it back.
  it('shows the collision state on a reload, from the pending-link read alone', () => {
    mockPendingRead = { isPending: false, data: { status: 'pending', email: ADDRESS } }
    renderWithProviders(<AuthPage />)

    expect(pendingReadEnabled).toHaveBeenCalledWith(true)
    expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
    expect(screen.getByLabelText('Email')).toHaveValue(ADDRESS)
    expect(screen.queryByTestId('google-oauth')).not.toBeInTheDocument()
  })

  it('shows the normal tab, with no line, when an ordinary arrival holds no pending link', () => {
    mockPendingRead = { isPending: false, data: { status: 'expired' } }
    renderWithProviders(<AuthPage />)

    expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.queryByText(PENDING_LINK_GONE_MESSAGE)).not.toBeInTheDocument()
    expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
  })

  it('shows the normal tab while an ordinary arrival waits on the read', () => {
    mockPendingRead = { isPending: true }
    renderWithProviders(<AuthPage />)

    expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
  })

  it('reads nothing on a re-authentication, which offers no new sign-in', () => {
    mockSearchParams = new URLSearchParams('reason=OAUTH_LINK_REAUTH_REQUIRED')
    renderWithProviders(<AuthPage />)

    expect(pendingReadEnabled).toHaveBeenCalled()
    expect(pendingReadEnabled.mock.calls.every(([enabled]) => enabled === false)).toBe(true)
  })

  it('signs in with the prefilled address and lands on returnTo', async () => {
    const user = userEvent.setup()
    arriveOnCollision()

    await user.type(screen.getByLabelText('Password'), 'hunter2-hunter2')
    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))

    expect(mockLoginMutate).toHaveBeenCalledTimes(1)
    const [credentials, options] = mockLoginMutate.mock.calls[0]
    expect(credentials).toEqual({ email: ADDRESS, password: 'hunter2-hunter2' })
    options.onSuccess({ success: true, user: { id: 9 } })
    expect(mockSetUser).toHaveBeenCalledWith({ id: 9 })
    expect(mockPush).toHaveBeenCalledWith('/shows')
  })

  it('asks for the password before signing in', async () => {
    const user = userEvent.setup()
    arriveOnCollision()

    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))

    expect(mockLoginMutate).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('Password is required')
    expect(screen.getByLabelText('Password')).toHaveAttribute('aria-invalid', 'true')
  })

  it('shows a refused sign-in in its own words', () => {
    mockLoginError = new AuthError('Invalid email or password', AuthErrorCode.INVALID_CREDENTIALS)
    arriveOnCollision()

    expect(screen.getByText('Invalid email or password')).toBeInTheDocument()
    expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
  })

  it('sends the link and shows the sent state with the address and its lifetime', async () => {
    const user = userEvent.setup()
    const { rerender } = arriveOnCollision()

    await user.click(screen.getByRole('button', { name: 'Email me a sign-in link instead' }))
    expect(mockSendMutate).toHaveBeenCalledTimes(1)

    mockSendState = { isPending: false, isSuccess: true, isError: false, error: null }
    rerender(<AuthPage />)

    expect(MAGIC_LINK_EXPIRY_MINUTES).toBe(15)
    const sent = screen.getByRole('status')
    expect(sent).toHaveTextContent('Check your inbox')
    expect(sent).toHaveTextContent(
      `We sent a sign-in link to ${ADDRESS}. It expires in ${MAGIC_LINK_EXPIRY_MINUTES} minutes. Opening it connects Google and signs out your other sessions.`
    )
    expect(
      screen.queryByRole('button', { name: 'Email me a sign-in link instead' })
    ).not.toBeInTheDocument()
    // The password proof stays usable above the note.
    expect(screen.getByRole('button', { name: 'Sign in and connect Google' })).toBeEnabled()
  })

  it('shows a failed send without leaving the state', () => {
    mockSendState = { isPending: false, isSuccess: false, isError: true, error: new Error('503') }
    arriveOnCollision()

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to send magic link. Please try again.')
    expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
  })

  it('falls back to the normal Sign in tab with the plain line when the read finds no pending link', () => {
    arriveOnCollision({ isPending: false, data: { status: 'expired' } })

    expect(screen.getByRole('status')).toHaveTextContent(PENDING_LINK_GONE_MESSAGE)
    expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
    expect(screen.queryByText('Sign in to connect Google')).not.toBeInTheDocument()
  })

  it('falls back the same way when the read fails', () => {
    arriveOnCollision({ isPending: false, isError: true })

    expect(screen.getByRole('status')).toHaveTextContent(PENDING_LINK_GONE_MESSAGE)
    expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
  })

  it('falls back to the normal tab when the send finds the pending link gone', async () => {
    const user = userEvent.setup()
    const { rerender } = arriveOnCollision()

    await user.click(screen.getByRole('button', { name: 'Email me a sign-in link instead' }))
    const [, options] = mockSendMutate.mock.calls[0]
    const gone = new AuthError(PENDING_LINK_GONE_MESSAGE, AuthErrorCode.OAUTH_LINK_PENDING_EXPIRED)
    mockSendState = { isPending: false, isSuccess: false, isError: true, error: gone }
    options.onError(gone)
    rerender(<AuthPage />)

    expect(await screen.findByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(PENDING_LINK_GONE_MESSAGE)
    expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
  })

  it('holds the tab on a spinner while the read is in flight', () => {
    arriveOnCollision({ isPending: true })

    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
    expect(screen.queryByTestId('google-oauth')).not.toBeInTheDocument()
  })
})
