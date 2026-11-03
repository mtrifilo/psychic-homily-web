import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { MAGIC_LINK_EXPIRY_MINUTES } from '@/lib/magic-link'
import { PENDING_LINK_GONE_MESSAGE } from '@/app/auth/_components/google-collision-sign-in'
import { withoutBannerParams } from '@/app/auth/_components/auth-arrival-params'
import AuthPage from './page'

// The Sign in tab's collision state, driven from the auth page as the OAuth
// callback lands on it: /auth?error_code=OAUTH_LINK_PENDING, with the address
// read from the pending link the backend holds for this browser.

const mockPush = vi.fn()
const mockReplace = vi.fn()
let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/auth',
}))

const mockSetUser = vi.fn()
let mockAuthStatus: 'anonymous' | 'authenticated' = 'anonymous'
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () => deriveMockAuthSignals({ setUser: mockSetUser, authStatus: mockAuthStatus }),
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
  data?: { status: 'pending' | 'expired'; email?: string; replaces_identity?: boolean }
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
    const read = enabled ? mockPendingRead : { isPending: true }
    return { ...read, isSuccess: !read.isPending && 'data' in read && read.data !== undefined }
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

function arriveOnCollision(
  read: PendingRead = { isPending: false, data: { status: 'pending', email: ADDRESS } },
  query: string = COLLISION
) {
  mockSearchParams = new URLSearchParams(query)
  mockPendingRead = read
  return renderWithProviders(<AuthPage />)
}

describe('AuthPage collision state', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockReplace.mockReset()
    mockSetUser.mockReset()
    mockLoginMutate.mockReset()
    mockLoginError = null
    mockSendMutate.mockReset()
    pendingReadEnabled.mockReset()
    mockSendState = { isPending: false, isSuccess: false, isError: false, error: null }
    mockSearchParams = new URLSearchParams()
    mockAuthStatus = 'anonymous'
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
    expect(screen.getByLabelText('Password')).not.toHaveAttribute('placeholder')
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

  it('reads nothing on a re-authentication arrival without OAUTH_LINK_PENDING', () => {
    mockSearchParams = new URLSearchParams('reason=OAUTH_LINK_REAUTH_REQUIRED')
    renderWithProviders(<AuthPage />)

    expect(pendingReadEnabled).toHaveBeenCalled()
    expect(pendingReadEnabled.mock.calls.every(([enabled]) => enabled === false)).toBe(true)
  })

  // The Google start of a signed-in reader's re-authentication found the
  // address held by an account, and its callback came back here with the
  // reason, which holds the page for them.
  describe('on the re-authentication its Google start came back to', () => {
    const REAUTH_COLLISION = new URLSearchParams({
      error_code: 'OAUTH_LINK_PENDING',
      reason: 'OAUTH_LINK_REAUTH_REQUIRED',
      returnTo: '/profile?tab=settings',
    }).toString()

    function arriveOnReauthCollision(read?: PendingRead) {
      mockAuthStatus = 'authenticated'
      return arriveOnCollision(read, REAUTH_COLLISION)
    }

    it('shows the collision state under the re-authentication heading, without bouncing', () => {
      arriveOnReauthCollision()

      expect(pendingReadEnabled).toHaveBeenCalledWith(true)
      expect(
        screen.getByRole('heading', { level: 1, name: "Confirm it's you" })
      ).toBeInTheDocument()
      expect(screen.getByText(/An account already uses/)).toHaveTextContent(
        `An account already uses ${ADDRESS}. Sign in with your password to connect Google.`
      )
      expect(screen.getByLabelText('Email')).toHaveValue(ADDRESS)
      expect(screen.queryByRole('tab', { name: 'Create account' })).not.toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // The address bar after the first render is what a reload arrives with.
    it('keeps the collision state on a reload', () => {
      arriveOnReauthCollision().unmount()
      const reloaded = withoutBannerParams(REAUTH_COLLISION)
      expect(new URLSearchParams(reloaded).get('error_code')).toBe('OAUTH_LINK_PENDING')
      pendingReadEnabled.mockReset()

      arriveOnCollision(undefined, reloaded)

      expect(pendingReadEnabled).toHaveBeenLastCalledWith(true)
      expect(screen.getByText(/An account already uses/)).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // The page holds its own redirect on a re-authentication, so the proof's
    // navigation is the only one, and it takes the spent arrival out of
    // history.
    it('replaces the spent arrival with returnTo when the password proof succeeds', async () => {
      const user = userEvent.setup()
      arriveOnReauthCollision()

      await user.type(screen.getByLabelText('Password'), 'hunter2-hunter2')
      await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))
      const [, options] = mockLoginMutate.mock.calls[0]
      options.onSuccess({ success: true, user: { id: 9 } })

      expect(mockReplace).toHaveBeenCalledWith('/profile?tab=settings')
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('carries the reason into a start with a different Google account', () => {
      arriveOnReauthCollision()

      const link = screen.getByRole('link', { name: 'Use a different Google account' })
      const href = new URL(link.getAttribute('href') ?? '')
      expect(href.searchParams.get('reason')).toBe('OAUTH_LINK_REAUTH_REQUIRED')
      expect(href.searchParams.get('returnTo')).toBe('/profile?tab=settings')
      expect(href.searchParams.get('prompt')).toBe('select_account')
    })

    it('shows the re-authentication form, not a spinner, when the read finds no pending link', () => {
      arriveOnReauthCollision({ isPending: false, data: { status: 'expired' } })

      expect(screen.getByText(PENDING_LINK_GONE_MESSAGE)).toBeInTheDocument()
      expect(screen.getByTestId('google-oauth')).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })
  })

  it('signs in with the prefilled address and navigates to returnTo', async () => {
    const user = userEvent.setup()
    arriveOnCollision()

    await user.type(screen.getByLabelText('Password'), 'hunter2-hunter2')
    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))

    expect(mockLoginMutate).toHaveBeenCalledTimes(1)
    const [credentials, options] = mockLoginMutate.mock.calls[0]
    expect(credentials).toEqual({
      email: ADDRESS,
      password: 'hunter2-hunter2',
      connect_pending_link: true,
    })
    options.onSuccess({ success: true, user: { id: 9 } })
    expect(mockSetUser).toHaveBeenCalledWith({ id: 9 })
    expect(mockReplace).toHaveBeenCalledWith('/shows')
    expect(mockPush).not.toHaveBeenCalled()
  })

  // The backend refuses a correct password when the pending link is gone,
  // rather than signing in without connecting Google.
  it('falls back to the normal tab with the plain line when the password proof finds the link gone', async () => {
    const user = userEvent.setup()
    const { rerender } = arriveOnCollision()

    await user.type(screen.getByLabelText('Password'), 'hunter2-hunter2')
    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))
    const [, options] = mockLoginMutate.mock.calls[0]
    options.onError(new AuthError(PENDING_LINK_GONE_MESSAGE, AuthErrorCode.OAUTH_LINK_PENDING_EXPIRED))
    rerender(<AuthPage />)

    expect(await screen.findByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(PENDING_LINK_GONE_MESSAGE)
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('falls back with the refusal in its own words when Google was attached elsewhere meanwhile', async () => {
    const user = userEvent.setup()
    const { rerender } = arriveOnCollision()
    const refusal = 'This provider account is already connected to another Psychic Homily account. Disconnect it there first.'

    await user.type(screen.getByLabelText('Password'), 'hunter2-hunter2')
    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))
    const [, options] = mockLoginMutate.mock.calls[0]
    options.onError(new AuthError(refusal, AuthErrorCode.OAUTH_IDENTITY_IN_USE))
    rerender(<AuthPage />)

    expect(await screen.findByText('Sign in to your account')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(refusal)
  })

  it('keeps the state for a wrong password', async () => {
    const user = userEvent.setup()
    const { rerender } = arriveOnCollision()

    await user.type(screen.getByLabelText('Password'), 'nope')
    await user.click(screen.getByRole('button', { name: 'Sign in and connect Google' }))
    const [, options] = mockLoginMutate.mock.calls[0]
    options.onError(new AuthError('Invalid email or password', AuthErrorCode.INVALID_CREDENTIALS))
    rerender(<AuthPage />)

    expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
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

  describe('when the account holds another Google identity on an address never verified', () => {
    const REPLACING: PendingRead = {
      isPending: false,
      data: { status: 'pending', email: ADDRESS, replaces_identity: true },
    }

    it('offers the emailed link alone, saying it replaces the connected Google account', () => {
      arriveOnCollision(REPLACING)

      expect(screen.getByText('Sign in to connect Google')).toBeInTheDocument()
      expect(screen.getByText(/An account already uses/)).toHaveTextContent(
        `An account already uses ${ADDRESS}.`
      )
      expect(screen.queryByText(/Sign in with your password/)).not.toBeInTheDocument()
      const email = screen.getByLabelText('Email')
      expect(email).toHaveValue(ADDRESS)
      expect(email).toHaveAttribute('readonly')
      expect(email.closest('form')).toBeNull()

      expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Sign in and connect Google' })
      ).not.toBeInTheDocument()
      expect(screen.queryByText('or')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Email me a sign-in link' })).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Email me a sign-in link instead' })
      ).not.toBeInTheDocument()
      expect(
        screen.getByText(
          'The link replaces the connected Google account with yours, signs out your other sessions and turns off your current password.'
        )
      ).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'Use a different Google account' })).toBeInTheDocument()
    })

    it('sends the link and says opening it replaces the connected Google account', async () => {
      const user = userEvent.setup()
      const { rerender } = arriveOnCollision(REPLACING)

      await user.click(screen.getByRole('button', { name: 'Email me a sign-in link' }))
      expect(mockSendMutate).toHaveBeenCalledTimes(1)

      mockSendState = { isPending: false, isSuccess: true, isError: false, error: null }
      rerender(<AuthPage />)

      const sent = screen.getByRole('status')
      expect(sent).toHaveTextContent('Check your inbox')
      expect(sent).toHaveTextContent(
        `We sent a sign-in link to ${ADDRESS}. It expires in ${MAGIC_LINK_EXPIRY_MINUTES} minutes. Opening it replaces the connected Google account with yours, signs out your other sessions and turns off your current password.`
      )
      expect(screen.queryByRole('button', { name: /Email me a sign-in link/ })).not.toBeInTheDocument()
      expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
      expect(mockLoginMutate).not.toHaveBeenCalled()
    })

    it('submits no password when Enter is pressed in the address', async () => {
      const user = userEvent.setup()
      arriveOnCollision(REPLACING)

      await user.type(screen.getByLabelText('Email'), '{Enter}')
      expect(mockLoginMutate).not.toHaveBeenCalled()
      expect(screen.queryByText('Password is required')).not.toBeInTheDocument()
    })
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

  // A failed read says nothing about the pending link, so it does not claim
  // the link is gone.
  it('shows the normal tab without the line when the read fails', () => {
    arriveOnCollision({ isPending: false, isError: true })

    expect(screen.queryByText(PENDING_LINK_GONE_MESSAGE)).not.toBeInTheDocument()
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
