import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Activity } from 'react'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AUTH_ERROR_CODE_PARAM } from '@/lib/auth-href'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import {
  ACCOUNT_RECOVERABLE_LINK_TEXT,
  ACCOUNT_RECOVERABLE_MESSAGE,
} from '@/app/auth/_components/account-recoverable-message'
import AuthPage from './page'

// --- Mocks ---
//
// next/navigation is mocked with mutable module-level state so each test can
// configure the search params and assert on router calls. The page reads
// searchParams via useSearchParams() and navigates via useRouter().push().

const mockPush = vi.fn()
const mockReplace = vi.fn()
let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/auth',
}))

// AuthContext is mocked with mutable state so individual tests can toggle the
// authenticated / loading state the page branches on.
let mockAuthState: {
  setUser: ReturnType<typeof vi.fn>
  authStatus: 'pending' | 'anonymous' | 'authenticated'
  isLoading?: boolean
} = {
  setUser: vi.fn(),
  authStatus: 'anonymous',
}

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockAuthState) }
})

// Captures the per-call options `LoginForm` hands `useLogin().mutate`, so a
// test can drive the success callback with a real login payload.
const mockLoginMutate = vi.fn()
// The error the login mutation reports, as a refused sign-in leaves it.
let mockLoginError: Error | null = null

vi.mock('@/features/auth', () => ({
  useLogin: () => ({
    mutate: mockLoginMutate,
    // As the real mutation's reset does, clears the error it reports.
    reset: () => {
      mockLoginError = null
    },
    isPending: false,
    error: mockLoginError,
  }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useSendMagicLink: () => ({ mutate: vi.fn(), isPending: false }),
  // Pulled in by the check-your-inbox interstitial (PSY-1900). Mocking the
  // module wholesale means every export the page's tree imports has to exist
  // here or the import throws before a single test runs.
  useSendVerificationEmail: () => ({
    mutate: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    error: null as Error | null,
  }),
}))

// Capture the props the auth surfaces hand down to their child auth buttons,
// so a test can check what each button is given without exercising WebAuthn or
// OAuth redirect machinery. The page threads the sanitized returnTo into the
// login button; the signup button gets the page's handoff reporter instead.
const passkeyLoginProps = vi.fn()
const passkeySignupProps = vi.fn()

type PasskeyLoginProps = { returnTo?: string; onError?: (error: AuthError) => void }

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (props: PasskeyLoginProps) => {
    passkeyLoginProps(props)
    return <div data-testid="passkey-login" data-return-to={props.returnTo} />
  },
}))

vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (props: { onHandoffChange?: unknown }) => {
    passkeySignupProps(props)
    return <div data-testid="passkey-signup" />
  },
}))

const googleOAuthProps = vi.fn()

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (props: { returnTo?: string }) => {
    googleOAuthProps(props)
    return <div data-testid="google-oauth" />
  },
}))

function setSearchParams(query: string) {
  mockSearchParams = new URLSearchParams(query)
}

describe('AuthPage', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockReplace.mockReset()
    mockLoginMutate.mockReset()
    mockLoginError = null
    passkeyLoginProps.mockReset()
    passkeySignupProps.mockReset()
    googleOAuthProps.mockReset()
    mockSearchParams = new URLSearchParams()
    window.history.replaceState(null, '', '/')
    mockAuthState = {
      setUser: vi.fn(),
      authStatus: 'anonymous',
    }
    vi.mocked(Element.prototype.scrollIntoView).mockClear()
  })

  describe('default tab', () => {
    it('renders the login tab by default when no ?tab param is present', () => {
      renderWithProviders(<AuthPage />)

      // Both tab triggers exist; login is the selected/active one.
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByRole('tab', { name: 'Create account' })).toHaveAttribute(
        'aria-selected',
        'false'
      )

      // Login-tab copy + the login passkey button are rendered.
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
      expect(screen.getByTestId('passkey-login')).toBeInTheDocument()
      // Radix unmounts inactive tab content.
      expect(screen.queryByTestId('passkey-signup')).not.toBeInTheDocument()
    })
  })

  describe('tab switching', () => {
    it('shows the signup form after clicking the Create account tab', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(
        screen.getByRole('heading', { name: 'Never miss a show.' })
      ).toBeInTheDocument()
      expect(screen.getByTestId('passkey-signup')).toBeInTheDocument()
      expect(screen.queryByTestId('passkey-login')).not.toBeInTheDocument()
    })

    it('does not push a URL update when switching tabs (Radix-local state)', async () => {
      // The selected tab is page-local state and is NOT written to the URL.
      // Guards against an accidental navigation side effect on tab change.
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(mockPush).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('does not honor ?tab=signup as an initial-tab hint (param is inert)', () => {
      // The page never reads ?tab, so ?tab=signup has no effect; the only
      // arrival that opens Create account is a gated control's intent.
      setSearchParams('tab=signup')
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    })
  })

  // A gated control names the action it refused. That arrival, and only that
  // one, opens on Create account; every other route in opens on Sign in.
  describe('gated arrival', () => {
    function expectSelected(tab: 'Sign in' | 'Create account') {
      const other = tab === 'Sign in' ? 'Create account' : 'Sign in'
      expect(screen.getByRole('tab', { name: tab })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByRole('tab', { name: other })).toHaveAttribute(
        'aria-selected',
        'false'
      )
    }

    it('opens on Create account, with its account ledger, when a gated control sent the visitor', () => {
      setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
      renderWithProviders(<AuthPage />)

      expectSelected('Create account')
      expect(
        screen.getByRole('heading', { name: 'Never miss a show.' })
      ).toBeInTheDocument()
      expect(screen.getByText('shows you plan to catch')).toBeInTheDocument()
      expect(screen.getByTestId('passkey-signup')).toBeInTheDocument()
      expect(screen.queryByTestId('passkey-login')).not.toBeInTheDocument()
    })

    it('opens on Create account from a gated click on a page with no returnTo', () => {
      setSearchParams('intent=follow')
      renderWithProviders(<AuthPage />)

      expectSelected('Create account')
    })

    // The header link and the mobile Account sheet both build a returnTo with no
    // intent. A returnTo alone says nothing about whether the reader has an
    // account.
    it('opens on Sign in for a returnTo that names no intent', () => {
      setSearchParams('returnTo=%2Fartists%2Fcalexico')
      renderWithProviders(<AuthPage />)

      expectSelected('Sign in')
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    })

    // The value edge cases are pinned on `hasGatedIntent` itself; this proves
    // the page goes through it.
    it('ignores an unlisted intent and opens on Sign in', () => {
      setSearchParams('returnTo=%2Fshows&intent=signup')
      renderWithProviders(<AuthPage />)

      expectSelected('Sign in')
    })

    it('keeps Sign in one click away after a gated arrival, without navigating', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Sign in' }))

      expectSelected('Sign in')
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
    })

    // A query-string-only navigation keeps this page mounted, so each of these
    // re-renders the same instance under a new URL, the way the router does.
    describe('a new arrival on the mounted page', () => {
      it('opens on Create account for a gated arrival after a plain one', () => {
        setSearchParams('returnTo=%2Fshows%2Fexample')
        const { rerender } = renderWithProviders(<AuthPage />)
        expectSelected('Sign in')

        setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
        rerender(<AuthPage />)

        expectSelected('Create account')
      })

      it('opens on Sign in for a plain arrival after a gated one', () => {
        setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
        const { rerender } = renderWithProviders(<AuthPage />)
        expectSelected('Create account')

        setSearchParams('returnTo=%2Fshows%2Fexample')
        rerender(<AuthPage />)

        expectSelected('Sign in')
      })

      it('drops a tab the viewer picked once the arrival changes', async () => {
        const user = userEvent.setup()
        setSearchParams('returnTo=%2Fshows%2Fexample')
        const { rerender } = renderWithProviders(<AuthPage />)
        await user.click(screen.getByRole('tab', { name: 'Create account' }))
        expectSelected('Create account')

        setSearchParams('')
        rerender(<AuthPage />)

        expectSelected('Sign in')
      })

      // Back to the first query string is a new arrival too, not a return to
      // the pick made under it.
      it('does not bring a dropped pick back when the first arrival returns', async () => {
        const user = userEvent.setup()
        setSearchParams('returnTo=%2Fshows%2Fexample')
        const { rerender } = renderWithProviders(<AuthPage />)
        await user.click(screen.getByRole('tab', { name: 'Create account' }))
        expectSelected('Create account')

        setSearchParams('')
        rerender(<AuthPage />)
        expectSelected('Sign in')

        setSearchParams('returnTo=%2Fshows%2Fexample')
        rerender(<AuthPage />)

        expectSelected('Sign in')
      })

      it('keeps a tab the viewer picked while the arrival is unchanged', async () => {
        const user = userEvent.setup()
        setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
        const { rerender } = renderWithProviders(<AuthPage />)
        await user.click(screen.getByRole('tab', { name: 'Sign in' }))

        rerender(<AuthPage />)

        expectSelected('Sign in')
      })

      // Leaving /auth and coming back to the same URL hides and re-shows this
      // instance rather than remounting it. The return is a new arrival.
      it('opens the header link on Sign in again after the page was hidden', async () => {
        const user = userEvent.setup()
        setSearchParams('returnTo=%2Fshows')
        const { rerender } = renderWithProviders(
          <Activity mode="visible">
            <AuthPage />
          </Activity>
        )
        await user.click(screen.getByRole('tab', { name: 'Create account' }))
        expectSelected('Create account')

        rerender(
          <Activity mode="hidden">
            <AuthPage />
          </Activity>
        )
        rerender(
          <Activity mode="visible">
            <AuthPage />
          </Activity>
        )

        expectSelected('Sign in')
      })

      it('opens a gated click on Create account again after the page was hidden', async () => {
        const user = userEvent.setup()
        setSearchParams('returnTo=%2Fshows&intent=save')
        const { rerender } = renderWithProviders(
          <Activity mode="visible">
            <AuthPage />
          </Activity>
        )
        await user.click(screen.getByRole('tab', { name: 'Sign in' }))
        expectSelected('Sign in')

        rerender(
          <Activity mode="hidden">
            <AuthPage />
          </Activity>
        )
        rerender(
          <Activity mode="visible">
            <AuthPage />
          </Activity>
        )

        expectSelected('Create account')
      })

      it('shows no Create account content on a re-auth after a gated arrival', async () => {
        setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
        const { rerender } = renderWithProviders(<AuthPage />)
        expectSelected('Create account')

        mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
        setSearchParams('returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED')
        rerender(<AuthPage />)

        await screen.findByText("Confirm it's you")
        expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
          'aria-selected',
          'true'
        )
        expect(
          screen.queryByRole('heading', { name: 'Never miss a show.' })
        ).not.toBeInTheDocument()
        expect(screen.queryByTestId('passkey-signup')).not.toBeInTheDocument()
      })
    })

    it('moves to Sign in, focus included, from the footer link on a gated arrival', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
      renderWithProviders(<AuthPage />)
      expectSelected('Create account')

      await user.click(screen.getByRole('button', { name: 'Sign in' }))

      expectSelected('Sign in')
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveFocus()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('opens a signed-out re-auth link with an intent on Sign in only', () => {
      setSearchParams(
        'returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED&intent=save'
      )
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(
        screen.queryByRole('tab', { name: /create account/i })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('heading', { name: 'Never miss a show.' })
      ).not.toBeInTheDocument()
    })

    it('opens on Sign in for a re-authentication even when an intent is present', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      setSearchParams(
        'returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED&intent=save'
      )
      renderWithProviders(<AuthPage />)

      await screen.findByText("Confirm it's you")
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(
        screen.queryByRole('tab', { name: /create account/i })
      ).not.toBeInTheDocument()
      expect(screen.getByTestId('passkey-login')).toBeInTheDocument()
    })
  })

  // PSY-1900: the signup tab leads with what an account is FOR instead of the
  // old "Sign up to submit shows and join the community" line.
  describe('signup intent panel', () => {
    it('renders the value ledger and the profile-visibility footer', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(screen.getByText('shows you plan to catch')).toBeInTheDocument()
      expect(
        screen.getByText('artists and venues you care about')
      ).toBeInTheDocument()
      expect(
        screen.getByText('hear when they announce something near you')
      ).toBeInTheDocument()
      expect(
        screen.getByText('for completists: add what we are missing')
      ).toBeInTheDocument()
      expect(
        screen.getByText(/You choose what shows on your public profile/)
      ).toBeInTheDocument()
      expect(
        screen.queryByText('Sign up to submit shows and join the community')
      ).not.toBeInTheDocument()
    })

    it('returns to the sign-in tab from the footer link without navigating', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))
      await user.click(screen.getByRole('button', { name: 'Sign in' }))

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // Radix unmounts the signup panel on the tab change, destroying the button
    // that had focus. Without handing focus to the tab list first, the click
    // drops a keyboard user at the top of the document.
    it('moves focus to the sign-in tab rather than dropping it', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))
      await user.click(screen.getByRole('button', { name: 'Sign in' }))

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveFocus()
    })
  })

  describe('OAuth / URL error banner', () => {
    it('renders the error banner from the ?error param', () => {
      setSearchParams('error=Email%20already%20exists&provider=google')
      renderWithProviders(<AuthPage />)

      expect(screen.getByText('Email already exists')).toBeInTheDocument()
    })

    it('opens on Sign in for an ?error with no code', () => {
      setSearchParams('error=authentication+failed')
      renderWithProviders(<AuthPage />)

      expect(screen.getByText('authentication failed')).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
    })

    it('does not render an error banner when no ?error param is present', () => {
      renderWithProviders(<AuthPage />)

      // The login-form passkey error alert region is absent and no decoded
      // error text leaks in; the only "alert"-ish content would be the banner.
      expect(screen.queryByText('Email already exists')).not.toBeInTheDocument()
    })
  })

  describe('OAuth link refused for a closed account', () => {
    const MESSAGE =
      'Your account is unavailable, so the provider was not connected; please contact support.'

    it('shows the refusal above the ordinary sign-in form', () => {
      setSearchParams('reason=ACCOUNT_INACTIVE')
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('alert')).toHaveTextContent(MESSAGE)
      // Not a re-authentication: signing in again cannot fix this, so the
      // page keeps its own heading and both tabs.
      expect(screen.getByText('Welcome to Psychic Homily')).toBeInTheDocument()
      expect(screen.queryByText("Confirm it's you")).not.toBeInTheDocument()
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByRole('tab', { name: 'Create account' })).toBeInTheDocument()
    })

    it('sends a signed-in reader on without showing it', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      setSearchParams('reason=ACCOUNT_INACTIVE')
      renderWithProviders(<AuthPage />)

      await waitFor(() => expect(mockPush).toHaveBeenCalledWith('/'))
      expect(screen.queryByText(MESSAGE)).not.toBeInTheDocument()
    })

    it('shows nothing for another reason or none', () => {
      setSearchParams('reason=account_inactive')
      const { unmount } = renderWithProviders(<AuthPage />)
      expect(screen.queryByText(MESSAGE)).not.toBeInTheDocument()
      unmount()

      setSearchParams('')
      renderWithProviders(<AuthPage />)
      expect(screen.queryByText(MESSAGE)).not.toBeInTheDocument()
    })
  })

  // A deleted account inside its recovery window is refused with
  // ACCOUNT_RECOVERABLE on password and passkey sign-in and on the OAuth
  // callback. Each surface answers in the page's own words with a link to
  // account recovery, never in the text the refusal carried.
  describe('recoverable account refusal', () => {
    function expectRecoveryOffer() {
      const link = screen.getByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      expect(link).toHaveAttribute('href', '/auth/recover')
      expect(link.closest('[role="alert"]')).not.toBeNull()
      expect(screen.getByText(ACCOUNT_RECOVERABLE_MESSAGE, { exact: false })).toBeInTheDocument()
    }

    it('offers recovery when password sign-in is refused with the code', () => {
      mockLoginError = new AuthError(
        'Server copy for the recoverable refusal',
        AuthErrorCode.ACCOUNT_RECOVERABLE
      )
      renderWithProviders(<AuthPage />)

      expectRecoveryOffer()
      expect(
        screen.queryByText('Server copy for the recoverable refusal')
      ).not.toBeInTheDocument()
    })

    // The passkey button reports a refusal with the code the server answered.
    function refusePasskeySignIn(error: AuthError) {
      const props = passkeyLoginProps.mock.lastCall?.[0] as PasskeyLoginProps
      act(() => props.onError?.(error))
    }

    it('offers recovery when passkey sign-in is refused with the code', () => {
      renderWithProviders(<AuthPage />)
      refusePasskeySignIn(
        new AuthError(
          'Server copy for the recoverable refusal',
          AuthErrorCode.ACCOUNT_RECOVERABLE
        )
      )

      expectRecoveryOffer()
      expect(
        screen.queryByText('Server copy for the recoverable refusal')
      ).not.toBeInTheDocument()
    })

    it('keeps the message of a passkey refusal with any other code', () => {
      renderWithProviders(<AuthPage />)
      refusePasskeySignIn(
        new AuthError(
          'Account unavailable. Please contact support.',
          AuthErrorCode.ACCOUNT_INACTIVE
        )
      )

      expect(
        screen.getByText('Account unavailable. Please contact support.')
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      ).not.toBeInTheDocument()
    })

    it('shows a passkey refusal that follows a password refusal', () => {
      mockLoginError = new AuthError(
        'Invalid email or password',
        AuthErrorCode.INVALID_CREDENTIALS
      )
      renderWithProviders(<AuthPage />)
      expect(screen.getByText('Invalid email or password')).toBeInTheDocument()

      refusePasskeySignIn(
        new AuthError('Server copy', AuthErrorCode.ACCOUNT_RECOVERABLE)
      )

      expectRecoveryOffer()
      expect(screen.queryByText('Invalid email or password')).not.toBeInTheDocument()
    })

    it('drops a passkey refusal when the password form is submitted', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)
      refusePasskeySignIn(
        new AuthError('Server copy', AuthErrorCode.ACCOUNT_RECOVERABLE)
      )
      expectRecoveryOffer()

      await user.type(screen.getByLabelText('Email'), 'someone@example.com')
      await user.type(screen.getByLabelText('Password'), 'a-password')
      await user.click(screen.getByRole('button', { name: /^sign in$/i }))

      await waitFor(() => expect(mockLoginMutate).toHaveBeenCalled())
      expect(
        screen.queryByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      ).not.toBeInTheDocument()
    })

    it('keeps the message of a password refusal with any other code', () => {
      mockLoginError = new AuthError(
        'Invalid email or password',
        AuthErrorCode.INVALID_CREDENTIALS
      )
      renderWithProviders(<AuthPage />)

      expect(screen.getByText('Invalid email or password')).toBeInTheDocument()
      expect(
        screen.queryByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      ).not.toBeInTheDocument()
    })

    it('offers recovery in the banner for an OAuth refusal with the code, on Sign in', () => {
      setSearchParams(
        new URLSearchParams({
          error: 'Text the page does not show',
          [AUTH_ERROR_CODE_PARAM]: 'ACCOUNT_RECOVERABLE',
        }).toString()
      )
      renderWithProviders(<AuthPage />)

      expectRecoveryOffer()
      expect(screen.queryByText('Text the page does not show')).not.toBeInTheDocument()
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    })

    it('offers recovery for the code even when the refusal carries no message', () => {
      setSearchParams(`${AUTH_ERROR_CODE_PARAM}=ACCOUNT_RECOVERABLE`)
      renderWithProviders(<AuthPage />)

      expectRecoveryOffer()
    })

    it('shows the message of an OAuth refusal with any other code', () => {
      setSearchParams(
        new URLSearchParams({
          error: 'Account unavailable. Please contact support.',
          [AUTH_ERROR_CODE_PARAM]: 'ACCOUNT_INACTIVE',
        }).toString()
      )
      renderWithProviders(<AuthPage />)

      expect(
        screen.getByText('Account unavailable. Please contact support.')
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      ).not.toBeInTheDocument()
    })
  })

  // One "Continue with Google" for both tabs: a new identity is asked for its
  // consent on the step after Google, so neither tab gathers any for it.
  describe('one Continue with Google entry', () => {
    const TERMS_MESSAGE =
      'Please accept the Terms of Service and Privacy Policy before creating an account.'

    it('gives both tabs the same Google button with the same props', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fshows')
      renderWithProviders(<AuthPage />)
      const onSignIn = googleOAuthProps.mock.calls.at(-1)?.[0]

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(googleOAuthProps.mock.calls.at(-1)?.[0]).toEqual(onSignIn)
      expect(onSignIn).toEqual({ className: 'w-full', returnTo: '/shows' })
    })

    it('puts the Google button ahead of every consent control on Create account', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      const google = screen.getByTestId('google-oauth')
      for (const control of [
        screen.getByRole('checkbox', { name: /Terms of Service/ }),
        screen.getByRole('checkbox', { name: /years old/ }),
      ]) {
        expect(
          google.compareDocumentPosition(control) & Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
      }
    })

    // A consent refusal can still arrive (another provider, or a backend from
    // before the step), and it is shown like any other refusal, on Sign in.
    it('shows a consent refusal on Sign in, without moving focus', () => {
      setSearchParams(
        new URLSearchParams({
          error: TERMS_MESSAGE,
          [AUTH_ERROR_CODE_PARAM]: 'TERMS_ACCEPTANCE_REQUIRED',
        }).toString()
      )
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText(TERMS_MESSAGE)).toBeInTheDocument()
      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })

    it('words the footer notice the way the Google button is pressed', () => {
      renderWithProviders(<AuthPage />)

      const notice = screen.getByText(/^By continuing, you agree to the/)
      expect(notice).toHaveTextContent(
        'By continuing, you agree to the Terms of Service and acknowledge the Privacy Policy.'
      )
      expect(within(notice).getByRole('link', { name: 'Terms of Service' })).toHaveAttribute('href', '/terms')
      expect(within(notice).getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', '/privacy')
    })
  })

  // The sign-up form's banner for its passkey button.
  describe('sign-up form errors', () => {
    it("shows the passkey button's error", async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)
      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      act(() => {
        const props = passkeySignupProps.mock.calls.at(-1)?.[0] as {
          onError?: (message: string) => void
        }
        props.onError?.('Failed to start passkey signup')
      })

      expect(await screen.findByText('Failed to start passkey signup')).toBeInTheDocument()
    })
  })

  // The banners render from the query string, which leaves the address bar
  // after the first render. The App Router syncs useSearchParams with
  // window.history.replaceState; `followAddressBar` does that by hand.
  describe('banner parameters leave the URL', () => {
    const INACTIVE_MESSAGE =
      'Your account is unavailable, so the provider was not connected; please contact support.'

    function arriveAt(query: string) {
      window.history.replaceState(null, '', `/auth?${query}`)
      setSearchParams(query)
      return renderWithProviders(<AuthPage />)
    }

    function addressBar() {
      return `${window.location.pathname}${window.location.search}`
    }

    function followAddressBar(rerender: (ui: React.ReactElement) => void) {
      setSearchParams(window.location.search)
      rerender(<AuthPage />)
    }

    // A reload renders a fresh page from whatever the address bar holds.
    function reload(unmount: () => void) {
      unmount()
      setSearchParams(window.location.search)
      return renderWithProviders(<AuthPage />)
    }

    async function signInWithPassword() {
      const user = userEvent.setup()
      await user.type(screen.getByLabelText('Email'), 'reader@test.local')
      await user.type(screen.getByLabelText('Password'), 'e2e-test-password-123')
      await user.click(screen.getByRole('button', { name: 'Sign in' }))
      await waitFor(() => expect(mockLoginMutate).toHaveBeenCalled())
      const { onSuccess } = mockLoginMutate.mock.calls.at(-1)![1] as {
        onSuccess: (data: unknown) => void
      }
      act(() => {
        onSuccess({ success: true, user: { id: '1', email: 'reader@test.local' } })
      })
    }

    it('shows the closed-account banner, then drops reason from the URL and keeps returnTo', () => {
      const { rerender } = arriveAt('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')

      expect(screen.getByRole('alert')).toHaveTextContent(INACTIVE_MESSAGE)
      expect(addressBar()).toBe('/auth?returnTo=%2Fshows')

      followAddressBar(rerender)

      expect(screen.getByRole('alert')).toHaveTextContent(INACTIVE_MESSAGE)
      expect(passkeyLoginProps).toHaveBeenLastCalledWith(
        expect.objectContaining({ returnTo: '/shows' })
      )
    })

    it('shows no banner after a reload, and signing in still lands on returnTo', async () => {
      const { unmount } = arriveAt('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')

      reload(unmount)

      expect(screen.queryByText(INACTIVE_MESSAGE)).not.toBeInTheDocument()
      await signInWithPassword()
      expect(mockPush).toHaveBeenCalledWith('/shows')
    })

    it('does the same for an OAuth refusal message and code', () => {
      const { rerender, unmount } = arriveAt(
        'error_code=RATE_LIMITED&error=Too%20many%20attempts&returnTo=%2Fvenues'
      )

      expect(screen.getByText('Too many attempts')).toBeInTheDocument()
      expect(addressBar()).toBe('/auth?returnTo=%2Fvenues')

      followAddressBar(rerender)
      expect(screen.getByText('Too many attempts')).toBeInTheDocument()

      reload(unmount)
      expect(screen.queryByText('Too many attempts')).not.toBeInTheDocument()
      expect(passkeyLoginProps).toHaveBeenLastCalledWith(
        expect.objectContaining({ returnTo: '/venues' })
      )
    })

    // replaceState rewrites the entry the reader is on, so the entry Back
    // returns to after they leave is the rewritten one.
    it('leaves no history entry that still carries the banner', () => {
      const pushState = vi.spyOn(window.history, 'pushState')
      const entries = window.history.length
      const { unmount } = arriveAt('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')

      expect(pushState).not.toHaveBeenCalled()
      expect(window.history.length).toBe(entries)

      reload(unmount)
      expect(screen.queryByText(INACTIVE_MESSAGE)).not.toBeInTheDocument()
      pushState.mockRestore()
    })

    // A consent refusal opens on Sign in like any other refusal: Google's
    // consent is asked on the step after Google, not on Create account.
    it('keeps a consent refusal once the code leaves the URL', () => {
      const message = 'Please accept the Terms of Service and Privacy Policy before creating an account.'
      const { rerender } = arriveAt(
        new URLSearchParams({
          error: message,
          [AUTH_ERROR_CODE_PARAM]: 'TERMS_ACCEPTANCE_REQUIRED',
          returnTo: '/shows',
        }).toString()
      )
      expect(addressBar()).toBe('/auth?returnTo=%2Fshows')

      followAddressBar(rerender)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText(message)).toBeInTheDocument()
    })

    it('keeps the recovery offer once the code leaves the URL', () => {
      const { rerender } = arriveAt(`${AUTH_ERROR_CODE_PARAM}=ACCOUNT_RECOVERABLE&error=x`)
      expect(addressBar()).toBe('/auth')

      followAddressBar(rerender)

      expect(
        screen.getByRole('link', { name: ACCOUNT_RECOVERABLE_LINK_TEXT })
      ).toHaveAttribute('href', '/auth/recover')
    })

    it('keeps a gated arrival on Create account beside a banner', () => {
      const { rerender } = arriveAt('error=x&returnTo=%2Fshows&intent=save')
      expect(addressBar()).toBe('/auth?returnTo=%2Fshows&intent=save')

      followAddressBar(rerender)

      expect(screen.getByRole('tab', { name: 'Create account' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText('x')).toBeInTheDocument()
    })

    it('keeps a re-authentication reason in the URL', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      const { rerender } = arriveAt('returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED&error=x')

      expect(addressBar()).toBe('/auth?returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED')
      followAddressBar(rerender)

      expect(screen.getByText("Confirm it's you")).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })
  })

  describe('returnTo propagation', () => {
    it('passes a sanitized internal returnTo down to the login passkey button', () => {
      setSearchParams('returnTo=%2Flibrary%3Ftab%3Dvenues')
      renderWithProviders(<AuthPage />)

      expect(passkeyLoginProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/library?tab=venues' })
      )
    })

    // The page owns the navigation after a passkey sign-up (see
    // passkey-signup-handoff.test.tsx), so the button gets the page's handoff
    // reporter and no destination of its own.
    it('hands the signup passkey button the handoff reporter, not a destination', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fcollections')
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(passkeySignupProps).toHaveBeenCalledWith(
        expect.objectContaining({ onHandoffChange: expect.any(Function) })
      )
    })

    it('passes the sanitized returnTo down to the login Google button', () => {
      setSearchParams('returnTo=%2Fshows')
      renderWithProviders(<AuthPage />)

      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/shows' })
      )
    })

    it('passes the sanitized returnTo down to the signup Google button', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fcollections')
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/collections' })
      )
    })

    it('falls back to "/" when returnTo points at an external origin', () => {
      // Confirms the page routes the raw param through sanitizeReturnTo rather
      // than forwarding it verbatim (open-redirect guard). The sanitizer's full
      // matrix is covered in auth-redirect-utils.test.ts.
      setSearchParams('returnTo=https%3A%2F%2Fevil.com%2Fphish')
      renderWithProviders(<AuthPage />)

      expect(passkeyLoginProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/' })
      )
      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/' })
      )
    })
  })

  describe('already-authenticated redirect', () => {
    it('redirects to the sanitized returnTo and renders no form when authenticated', async () => {
      setSearchParams('returnTo=%2Flibrary')
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'authenticated',
      }

      renderWithProviders(<AuthPage />)

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/library')
      })
      // Authenticated branch returns null — no auth card / tabs render.
      expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
    })

    it('redirects to "/" when authenticated with no returnTo', async () => {
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'authenticated',
      }

      renderWithProviders(<AuthPage />)

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/')
      })
    })

    it('spins while the profile fetch is in flight', () => {
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'pending',
        isLoading: true,
      }

      renderWithProviders(<AuthPage />)

      expect(mockPush).not.toHaveBeenCalled()
      expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
    })

    // The page this rule exists for. A profile fetch that failed without
    // answering (429, 5xx, network, 403) leaves `authStatus` terminally
    // 'pending', and a viewer in that state may be carrying a dead session and
    // needing exactly this form. Gating the form on 'pending' would spin
    // forever; `isLoading` goes false when the query errors, so it renders.
    it('serves the sign-in form once a failed profile settles nothing', () => {
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'pending',
        isLoading: false,
      }

      renderWithProviders(<AuthPage />)

      expect(mockPush).not.toHaveBeenCalled()
      expect(
        screen.getByRole('tab', { name: 'Sign in' })
      ).toBeInTheDocument()
    })
  })

  // PSY-1945. The login site used to assemble the context user by hand from a
  // handful of response fields, dropping `is_admin` and stating a placeholder
  // for `email_verified`. The override outranks the profile query for the rest
  // of the SPA session, so an admin lost their admin UI until a hard reload.
  describe('password login handoff to auth context', () => {
    async function submitLogin() {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.type(screen.getByLabelText('Email'), 'admin@test.local')
      await user.type(screen.getByLabelText('Password'), 'e2e-test-password-123')
      await user.click(screen.getByRole('button', { name: 'Sign in' }))

      await waitFor(() => {
        expect(mockLoginMutate).toHaveBeenCalled()
      })

      return mockLoginMutate.mock.calls[0][1] as {
        onSuccess: (data: unknown) => void
      }
    }

    it('hands the whole response user to setUser, privilege fields included', async () => {
      const callbacks = await submitLogin()

      callbacks.onSuccess({
        success: true,
        user: {
          id: '7',
          email: 'admin@test.local',
          username: 'reggie',
          first_name: 'Reg',
          last_name: 'Gie',
          is_admin: true,
          email_verified: true,
          user_tier: 'trusted_contributor',
          nav_mode: 'top',
        },
      })

      // The whole payload, unnarrowed: `AuthProvider` is what maps it, and the
      // bug was this site handing over a subset with the privilege fields cut.
      expect(mockAuthState.setUser).toHaveBeenCalledWith({
        id: '7',
        email: 'admin@test.local',
        username: 'reggie',
        first_name: 'Reg',
        last_name: 'Gie',
        is_admin: true,
        email_verified: true,
        user_tier: 'trusted_contributor',
        nav_mode: 'top',
      })
      expect(mockPush).toHaveBeenCalledWith('/')
    })
  })

  describe('re-authentication', () => {
    // The backend sends an ALREADY authenticated reader here when a
    // security-relevant change needs the account proved again. The page's
    // ordinary behaviour, bouncing an authenticated reader to returnTo, would
    // return them to the control that just refused them with nothing changed:
    // a silent no-op with no way to tell what happened.
    const reauthParams = new URLSearchParams({
      returnTo: '/profile?tab=settings',
      reason: 'OAUTH_LINK_REAUTH_REQUIRED',
    })

    it('renders the form for an authenticated reader instead of bouncing', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams(reauthParams)

      renderWithProviders(<AuthPage />)

      expect(await screen.findByText("Confirm it's you")).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('names the reason, so the reader knows why they are here', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams(reauthParams)

      renderWithProviders(<AuthPage />)

      expect(
        await screen.findByText(/connect a provider to your account/i)
      ).toBeInTheDocument()
    })

    it('offers no Create account tab, which proves nothing about holding one', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams(reauthParams)

      renderWithProviders(<AuthPage />)

      await screen.findByText("Confirm it's you")
      expect(
        screen.queryByRole('tab', { name: /create account/i })
      ).not.toBeInTheDocument()
    })

    // The Settings link start sends a stale session here; signing in again
    // with Google has to land back on Settings, where Connect is.
    it('sends a Google re-authentication back to Settings', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams(reauthParams)

      renderWithProviders(<AuthPage />)

      await screen.findByText("Confirm it's you")
      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ returnTo: '/profile?tab=settings' })
      )
    })

    it('still bounces an authenticated reader when no reason is given', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams({ returnTo: '/profile?tab=settings' })

      renderWithProviders(<AuthPage />)

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/profile?tab=settings')
      })
    })

    it('ignores a reason it does not recognise', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      mockSearchParams = new URLSearchParams({
        returnTo: '/profile?tab=settings',
        reason: 'SOMETHING_ELSE',
      })

      renderWithProviders(<AuthPage />)

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/profile?tab=settings')
      })
    })
  })
})
