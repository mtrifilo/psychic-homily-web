import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
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
} = {
  setUser: vi.fn(),
  authStatus: 'anonymous',
}

// `authStatus` is the setting; `isAuthenticated` derives from it at the
// boundary, so no case describes a viewer whose two auth signals disagree,
// and 'pending' is expressible (it is not, when `isLoading` is the input:
// `isLoading` is false both before the profile fetch starts and after it
// fails without settling).
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    ...mockAuthState,
    isAuthenticated: mockAuthState.authStatus === 'authenticated',
    isLoading: mockAuthState.authStatus === 'pending',
  }),
}))

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

// Capture the props the auth surfaces hand down to their child auth buttons.
// The page threads the sanitized returnTo into LoginForm / SignupForm, which
// forward it to these buttons — so capturing here verifies returnTo
// propagation without exercising WebAuthn / OAuth redirect machinery.
const passkeyLoginProps = vi.fn()
const passkeySignupProps = vi.fn()

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (props: { returnTo?: string }) => {
    passkeyLoginProps(props)
    return <div data-testid="passkey-login" data-return-to={props.returnTo} />
  },
}))

vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (props: { returnTo?: string }) => {
    passkeySignupProps(props)
    return <div data-testid="passkey-signup" data-return-to={props.returnTo} />
  },
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: () => <div data-testid="google-oauth" />,
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
    mockSearchParams = new URLSearchParams()
    mockAuthState = {
      setUser: vi.fn(),
      authStatus: 'anonymous',
    }
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
      // Documents current behavior: the tab control is uncontrolled Radix
      // state and is NOT synced to the URL. Guards against an accidental
      // navigation side effect being introduced on tab change.
      const user = userEvent.setup()
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(mockPush).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('does not honor ?tab=signup as an initial-tab hint (param is inert)', () => {
      // The page hardcodes defaultValue="login" and never reads ?tab, so
      // ?tab=signup has no effect. If deep-linking to the signup tab is later
      // implemented, this expectation should flip.
      setSearchParams('tab=signup')
      renderWithProviders(<AuthPage />)

      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByText('Sign in to your account')).toBeInTheDocument()
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

    it('does not render an error banner when no ?error param is present', () => {
      renderWithProviders(<AuthPage />)

      // The login-form passkey error alert region is absent and no decoded
      // error text leaks in; the only "alert"-ish content would be the banner.
      expect(screen.queryByText('Email already exists')).not.toBeInTheDocument()
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

    it('passes the sanitized returnTo down to the signup passkey button', async () => {
      const user = userEvent.setup()
      setSearchParams('returnTo=%2Fcollections')
      renderWithProviders(<AuthPage />)

      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(passkeySignupProps).toHaveBeenCalledWith(
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

    it('shows the spinner, not the sign-in form, while auth is unsettled', () => {
      // The window this gate exists for: a viewer holding a session the
      // backend has not answered for reads 'pending', and showing them the
      // sign-in form claims they have no session.
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'pending',
      }

      renderWithProviders(<AuthPage />)

      expect(mockPush).not.toHaveBeenCalled()
      // Loading branch shows a spinner, not the tabs.
      expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
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
})
