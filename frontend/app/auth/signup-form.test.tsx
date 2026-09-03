import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION, MIN_SIGNUP_AGE } from '@/lib/legal'
import AuthPage from './page'

// Accessible names for the two required signup checkboxes (PSY-1023 added age).
const TERMS_CHECKBOX = /Terms of Service/
const AGE_CHECKBOX = /at least 16 years old/
// Distinct error-message text (the checkbox LABEL also contains "at least 16
// years old", so the error matcher must key off the error-only phrasing).
const AGE_ERROR = /You must confirm that you are at least 16 years old/

// --- Mocks ---

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => new URLSearchParams(),
}))

const mockRegisterMutate = vi.fn()
vi.mock('@/features/auth', () => ({
  useRegister: () => ({
    mutate: mockRegisterMutate,
    isPending: false,
    error: null as Error | null,
  }),
  useLogin: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null as Error | null,
  }),
  useSendMagicLink: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  // Pulled in by the check-your-inbox interstitial (PSY-1900).
  useSendVerificationEmail: () => ({
    mutate: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    error: null as Error | null,
  }),
}))

// Mutable so a test can flip the viewer to authenticated mid-signup, which is
// what the real `useRegister` does when it awaits a profile refetch.
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

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (): null => null,
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

// --- Helpers ---

async function renderSignupForm() {
  const user = userEvent.setup()
  const rendered = renderWithProviders(<AuthPage />)

  // Switch to the signup tab (Radix unmounts inactive tab content)
  await user.click(screen.getByRole('tab', { name: 'Create account' }))

  return { user, rerender: () => rendered.rerender(<AuthPage />) }
}

// --- Tests ---

describe('SignupForm deferred validation', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockRegisterMutate.mockReset()
    mockAuthState = {
      setUser: vi.fn(),
      authStatus: 'anonymous',
    }
  })

  it('renders form fields without validation errors initially', async () => {
    await renderSignupForm()

    expect(screen.queryAllByRole('alert')).toHaveLength(0)
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: TERMS_CHECKBOX })).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: AGE_CHECKBOX })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
  })

  it('does not show errors while typing invalid input before submit', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'bur')
    await user.type(screen.getByLabelText('Password'), 'ab')

    expect(screen.queryAllByRole('alert')).toHaveLength(0)
  })

  it('shows validation errors on submit with empty fields', async () => {
    const { user } = await renderSignupForm()

    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      // Email + password + terms + age = 4 error alerts (PSY-1023 added age)
      expect(screen.getAllByRole('alert')).toHaveLength(4)
    })
    expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    expect(screen.getByText(/Password must be at least 12 characters/)).toBeInTheDocument()
    expect(screen.getByText(/You must agree to the Terms of Service/)).toBeInTheDocument()
    expect(screen.getByText(AGE_ERROR)).toBeInTheDocument()
  })

  it('shows only email error when other fields are valid', async () => {
    const { user } = await renderSignupForm()

    // Leave email empty, fill password and accept terms + age
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    })
    // Only email error — no terms, age, or password errors
    expect(screen.queryByText(/You must agree to the Terms of Service/)).not.toBeInTheDocument()
    expect(screen.queryByText(AGE_ERROR)).not.toBeInTheDocument()
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('disables submit while password is shorter than minimum length', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'short') // 5 chars
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))

    const submitButton = screen.getByRole('button', { name: 'Create account' })
    expect(submitButton).toBeDisabled()
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('enables submit once password reaches minimum length', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))

    // 11 chars => still disabled
    await user.type(screen.getByLabelText('Password'), '12345678901')
    expect(screen.getByRole('button', { name: 'Create account' })).toBeDisabled()

    // 12 chars => enabled
    await user.type(screen.getByLabelText('Password'), '2')
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
  })

  it('shows terms error on submit without checking terms', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/You must agree to the Terms of Service/)).toBeInTheDocument()
    })
  })

  // PSY-1023: submit is blocked (and the mutation never fires) when age is not
  // confirmed, even with email, password, and terms all valid.
  it('shows age error and blocks submit without confirming age', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    // Intentionally leave the age checkbox unchecked.
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(AGE_ERROR)).toBeInTheDocument()
    })
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('clears errors in real-time after failed submit', async () => {
    const { user } = await renderSignupForm()

    // Submit empty form to trigger errors (email + password + terms + age)
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => {
      expect(screen.getAllByRole('alert')).toHaveLength(4)
    })

    // Type valid email → email error clears
    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await waitFor(() => {
      expect(screen.queryByText(/Please enter a valid email address/)).not.toBeInTheDocument()
    })

    // Type valid password → password error clears
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await waitFor(() => {
      expect(screen.queryByText(/Password must be at least 12 characters/)).not.toBeInTheDocument()
    })

    // Check terms → terms error clears
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await waitFor(() => {
      expect(screen.queryByText(/You must agree to the Terms of Service/)).not.toBeInTheDocument()
    })

    // Check age → age error clears
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
    await waitFor(() => {
      expect(screen.queryByText(AGE_ERROR)).not.toBeInTheDocument()
    })
  })

  it('calls register mutation on valid submit', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(mockRegisterMutate).toHaveBeenCalledWith(
        {
          email: 'test@example.com',
          password: 'validPassword123!',
          terms_accepted: true,
          terms_version: CURRENT_TERMS_VERSION,
          privacy_version: CURRENT_PRIVACY_VERSION,
          age_confirmed: true,
          min_age_attested: MIN_SIGNUP_AGE,
        },
        expect.any(Object),
      )
    })
  })

  // PSY-1900: a successful signup no longer navigates. It swaps the card for
  // the check-your-inbox interstitial, which is the only place the new account
  // learns a verification email is already waiting.
  describe('post-signup handoff', () => {
    async function submitValidSignup() {
      const { user, rerender } = await renderSignupForm()

      await user.type(screen.getByLabelText('Email'), 'test@example.com')
      await user.type(screen.getByLabelText('Password'), 'validPassword123!')
      await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
      await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
      await user.click(screen.getByRole('button', { name: 'Create account' }))

      await waitFor(() => {
        expect(mockRegisterMutate).toHaveBeenCalled()
      })

      return {
        rerender,
        callbacks: mockRegisterMutate.mock.calls[0][1] as {
          onSuccess: (data: unknown) => void
          onError: (error: unknown) => void
        },
      }
    }

    it('shows the interstitial for the registered address instead of navigating', async () => {
      const { callbacks } = await submitValidSignup()

      act(() => {
        callbacks.onSuccess({
          user: { id: '1', email: 'test@example.com' },
        })
      })

      expect(
        await screen.findByRole('heading', { name: 'Check your inbox.' })
      ).toBeInTheDocument()
      expect(screen.getByText('test@example.com')).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // The redirect race this closes is real: `useRegister` awaits a full
    // profile refetch inside its own onSuccess, so the viewer is already
    // authenticated before this component's onSuccess ever runs. Claiming the
    // page at submit rather than at success is what keeps the redirect from
    // firing in that window.
    it('holds back the already-authenticated redirect while the request is in flight', async () => {
      const { rerender } = await submitValidSignup()

      // The session now exists, but the register callback has not run yet.
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'authenticated',
      }
      act(() => {
        rerender()
      })

      await waitFor(() => {
        expect(mockRegisterMutate).toHaveBeenCalled()
      })
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('releases the page back to the redirect when registration fails', async () => {
      const { callbacks, rerender } = await submitValidSignup()

      act(() => {
        callbacks.onError(new Error('Email already registered'))
      })
      mockAuthState = {
        setUser: vi.fn(),
        authStatus: 'authenticated',
      }
      act(() => {
        rerender()
      })

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/')
      })
    })

    // Documents pre-existing behavior rather than endorsing it: a `success`
    // response with no user leaves the form untouched with no error shown.
    // Worth fixing, but it predates the interstitial and is not this change.
    it('leaves the form in place when the response carries no user', async () => {
      const { callbacks } = await submitValidSignup()

      act(() => {
        callbacks.onSuccess({ success: true })
      })

      expect(
        screen.queryByRole('heading', { name: 'Check your inbox.' })
      ).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Create account' })).toBeInTheDocument()
    })
  })

  it('shows email error on submit with syntactically invalid email', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'not-an-email')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
    await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    })
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('does not show duplicate error messages', async () => {
    const { user } = await renderSignupForm()

    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getAllByRole('alert').length).toBeGreaterThanOrEqual(1)
    })

    // Find the email error alert and verify it has no duplicated text
    const emailError = screen.getByText('Please enter a valid email address')
    expect(emailError.textContent).toBe('Please enter a valid email address')
  })
})
