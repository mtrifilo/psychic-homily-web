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
let mockAuthState = {
  setUser: vi.fn(),
  isAuthenticated: false,
  isLoading: false,
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthState,
}))

vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useSendMagicLink: () => ({ mutate: vi.fn(), isPending: false }),
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
    passkeyLoginProps.mockReset()
    passkeySignupProps.mockReset()
    mockSearchParams = new URLSearchParams()
    mockAuthState = {
      setUser: vi.fn(),
      isAuthenticated: false,
      isLoading: false,
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

      expect(screen.getByText('Create an account')).toBeInTheDocument()
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
        isAuthenticated: true,
        isLoading: false,
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
        isAuthenticated: true,
        isLoading: false,
      }

      renderWithProviders(<AuthPage />)

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/')
      })
    })

    it('does not redirect while auth state is still loading', () => {
      mockAuthState = {
        setUser: vi.fn(),
        isAuthenticated: false,
        isLoading: true,
      }

      renderWithProviders(<AuthPage />)

      expect(mockPush).not.toHaveBeenCalled()
      // Loading branch shows a spinner, not the tabs.
      expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
    })
  })
})
