import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Activity } from 'react'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AUTH_ERROR_CODE_PARAM } from '@/lib/auth-href'
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

vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: mockLoginMutate, isPending: false, error: null as Error | null }),
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

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (props: { returnTo?: string }) => {
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
  GoogleOAuthButton: (props: { variant?: string; returnTo?: string }) => {
    googleOAuthProps(props)
    return <div data-testid="google-oauth" data-variant={props.variant} />
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
    passkeyLoginProps.mockReset()
    passkeySignupProps.mockReset()
    googleOAuthProps.mockReset()
    mockSearchParams = new URLSearchParams()
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

  // The OAuth callback refuses to create an account from a sign-in that
  // carried no consent, and names the refusal in ?error_code beside the
  // message in ?error.
  describe('OAuth refusal that needs signup consent', () => {
    const TERMS_MESSAGE =
      'Please accept the Terms of Service and Privacy Policy before creating an account.'
    const TERMS_REFUSAL_QUERY = new URLSearchParams({
      error: TERMS_MESSAGE,
      [AUTH_ERROR_CODE_PARAM]: 'TERMS_ACCEPTANCE_REQUIRED',
    }).toString()
    const termsCheckbox = () =>
      screen.getByRole('checkbox', {
        name: /I agree to the Terms of Service and Privacy Policy/,
      })

    it('opens on Create account with the message above the consent controls and Google sign-up', () => {
      setSearchParams(TERMS_REFUSAL_QUERY)
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Create account' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      const message = screen.getByText(TERMS_MESSAGE)
      const terms = termsCheckbox()
      const age = screen.getByRole('checkbox', { name: /years old/ })
      const google = screen.getByTestId('google-oauth')

      expect(google).toHaveAttribute('data-variant', 'signup')
      for (const control of [terms, age, google]) {
        expect(
          message.compareDocumentPosition(control) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
      }
      expect(screen.queryByText('Sign in to your account')).not.toBeInTheDocument()
    })

    // The refused callback carries the destination the sign-in started with, so
    // signing up from here still lands on it.
    it('hands the sign-up Google button the destination the refusal carries', () => {
      const query = new URLSearchParams(TERMS_REFUSAL_QUERY)
      query.set('returnTo', '/shows')
      setSearchParams(query.toString())
      renderWithProviders(<AuthPage />)

      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ variant: 'signup', returnTo: '/shows' })
      )
    })

    it('centres the terms checkbox and focuses it', () => {
      setSearchParams(TERMS_REFUSAL_QUERY)
      renderWithProviders(<AuthPage />)

      const terms = termsCheckbox()
      expect(terms).toHaveFocus()
      expect(terms).toHaveAccessibleDescription(TERMS_MESSAGE)
      const scrollIntoView = vi.mocked(Element.prototype.scrollIntoView)
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center' })
      expect(scrollIntoView.mock.contexts[0]).toBe(terms)
    })

    it('does not pull focus back when the viewer returns to Create account by hand', async () => {
      const user = userEvent.setup()
      setSearchParams(TERMS_REFUSAL_QUERY)
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Sign in' }))
      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(termsCheckbox()).not.toHaveFocus()
      expect(termsCheckbox()).not.toHaveAttribute('aria-describedby')
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(1)
    })

    it('brings the terms checkbox back into view and focus after the page was hidden', () => {
      setSearchParams(TERMS_REFUSAL_QUERY)
      const { rerender } = renderWithProviders(
        <Activity mode="visible">
          <AuthPage />
        </Activity>
      )
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

      expect(termsCheckbox()).toHaveFocus()
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(2)
    })

    it('neither scrolls nor moves focus on a gated arrival', () => {
      setSearchParams('returnTo=%2Fshows%2Fexample&intent=save')
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Create account' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(termsCheckbox()).not.toHaveFocus()
      expect(termsCheckbox()).not.toHaveAttribute('aria-describedby')
      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })

    it('names no description for a code that arrives without a message', () => {
      setSearchParams(`${AUTH_ERROR_CODE_PARAM}=TERMS_ACCEPTANCE_REQUIRED`)
      renderWithProviders(<AuthPage />)

      expect(termsCheckbox()).toHaveFocus()
      expect(termsCheckbox()).not.toHaveAttribute('aria-describedby')
    })

    it('neither scrolls nor moves focus when Create account is picked by hand', async () => {
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(termsCheckbox()).not.toHaveFocus()
      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })

    it('opens on Sign in for a refusal code that consent does not answer', () => {
      setSearchParams(
        'error=An+account+with+this+email+already+exists&error_code=USER_EXISTS'
      )
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })

    it('opens on Sign in only for a re-authentication carrying the code', async () => {
      mockAuthState = { setUser: vi.fn(), authStatus: 'authenticated' }
      setSearchParams(
        `returnTo=%2Fprofile&reason=OAUTH_LINK_REAUTH_REQUIRED&${TERMS_REFUSAL_QUERY}`
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
        expect.objectContaining({ variant: 'login', returnTo: '/shows' })
      )
    })

    it('passes the sanitized returnTo down to the signup Google button', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fcollections')
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(googleOAuthProps).toHaveBeenCalledWith(
        expect.objectContaining({ variant: 'signup', returnTo: '/collections' })
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
        expect.objectContaining({ variant: 'login', returnTo: '/' })
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
        expect.objectContaining({
          variant: 'login',
          returnTo: '/profile?tab=settings',
        })
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
