import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Activity } from 'react'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { deferred } from '@/test/inflightOrdering'
import {
  BUILD_VERSIONS_BODY,
  NEWER_VERSIONS_BODY,
  publishLegalVersionsInTurn as publishInTurn,
  publishedVersions as versionsBody,
  versionsUnavailable as unavailable,
} from '@/test/consentVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION, MIN_SIGNUP_AGE } from '@/lib/legal'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import {
  CONSENT_RETRY_NOTICE,
  CONSENT_VERSIONS_CHANGED,
  CONSENT_VERSIONS_UNAVAILABLE,
} from './_components/consent-versions-notice'
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
  usePathname: () => '/auth',
}))

const mockRegisterMutate = vi.fn()
// The callbacks the form hands `useRegister` on its latest render.
let registerCallbacks: {
  onSuccess: (data: unknown, variables: unknown) => void
  onError: (error: unknown) => void
}
// The error the register mutation reports, as a refused sign-up leaves it.
let mockRegisterError: Error | null = null
vi.mock('@/features/auth', () => ({
  useRegister: (callbacks: typeof registerCallbacks) => {
    registerCallbacks = callbacks
    return {
      mutate: mockRegisterMutate,
      isPending: false,
      error: mockRegisterError,
    }
  },
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

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockAuthState) }
})

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

const passkeySignupProps = vi.fn()
vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (props: unknown): null => {
    passkeySignupProps(props)
    return null
  },
}))

const googleOAuthProps = vi.fn()
vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (props: unknown): null => {
    googleOAuthProps(props)
    return null
  },
}))

function latestButtonProps(props: ReturnType<typeof vi.fn>) {
  return props.mock.calls.at(-1)?.[0]
}

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
    mockRegisterError = null
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
          signal: expect.any(AbortSignal),
          onRegistered: expect.any(Function),
        }
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
        callbacks: registerCallbacks,
        variables: mockRegisterMutate.mock.calls[0][0] as unknown,
      }
    }

    it('shows the interstitial for the registered address instead of navigating', async () => {
      const { callbacks, variables } = await submitValidSignup()

      act(() => {
        callbacks.onSuccess(
          { user: { id: '1', email: 'registered@example.com' } },
          variables
        )
      })

      expect(
        await screen.findByRole('heading', { name: 'Check your inbox.' })
      ).toBeInTheDocument()
      expect(screen.getByText('registered@example.com')).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('falls back to the submitted address when the response user has none', async () => {
      const { callbacks, variables } = await submitValidSignup()

      act(() => {
        callbacks.onSuccess({ user: { id: '1', email: '' } }, variables)
      })

      expect(
        await screen.findByRole('heading', { name: 'Check your inbox.' })
      ).toBeInTheDocument()
      expect(screen.getByText('test@example.com')).toBeInTheDocument()
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
      const { callbacks, variables } = await submitValidSignup()

      act(() => {
        callbacks.onSuccess({ success: true }, variables)
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

  // Every registration sends the versions GET /legal/versions publishes,
  // never the build constants, and a consent refusal of those versions is
  // retried once with versions read again past every cache.
  describe('consent versions', () => {
    const PUBLISHED = BUILD_VERSIONS_BODY
    const CONSENT_REFUSAL = new AuthError(
      'Please accept the Terms of Service and Privacy Policy before creating an account.',
      AuthErrorCode.TERMS_ACCEPTANCE_REQUIRED
    )

    async function submitFilledForm() {
      const { user, rerender } = await renderSignupForm()
      await user.type(screen.getByLabelText('Email'), 'test@example.com')
      await user.type(screen.getByLabelText('Password'), 'validPassword123!')
      await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
      await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
      await user.click(screen.getByRole('button', { name: 'Create account' }))
      return { user, rerender }
    }

    // The form has one registration in flight at a time, and the callbacks it
    // hands `useRegister` settle that one, so they stand for the latest call's.
    function registerCall(index: number) {
      const [request] = mockRegisterMutate.mock.calls[index] as [
        { terms_version: string; privacy_version: string },
      ]
      return { request, callbacks: registerCallbacks }
    }

    it('registers once the route confirms this build\'s versions', async () => {
      const versions = publishInTurn(() => versionsBody(PUBLISHED))

      await submitFilledForm()

      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      expect(versions.reads()).toBeGreaterThan(0)
      expect(registerCall(0).request).toMatchObject({
        terms_version: CURRENT_TERMS_VERSION,
        privacy_version: CURRENT_PRIVACY_VERSION,
      })
      expect(screen.queryByText(CONSENT_VERSIONS_CHANGED)).not.toBeInTheDocument()
    })

    it('asks for a reload, and blocks every sign-up control, when the route publishes other versions', async () => {
      publishInTurn(() => versionsBody(NEWER_VERSIONS_BODY))
      const { user } = await renderSignupForm()

      expect(await screen.findByText(CONSENT_VERSIONS_CHANGED)).toBeInTheDocument()
      await user.type(screen.getByLabelText('Email'), 'test@example.com')
      await user.type(screen.getByLabelText('Password'), 'validPassword123!')
      await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
      await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))

      expect(screen.getByRole('button', { name: 'Create account' })).toBeDisabled()
      expect(latestButtonProps(googleOAuthProps)).toMatchObject({ disabled: true })
      expect(latestButtonProps(passkeySignupProps)).toMatchObject({ versionsChanged: true })
      expect(mockRegisterMutate).not.toHaveBeenCalled()
    })

    it('neither retries nor registers when the re-read after a refusal finds other versions', async () => {
      publishInTurn(() => versionsBody(PUBLISHED), () => versionsBody(NEWER_VERSIONS_BODY))
      await submitFilledForm()
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))

      act(() => {
        registerCall(0).callbacks.onError(CONSENT_REFUSAL)
      })

      expect(await screen.findByText(CONSENT_VERSIONS_CHANGED)).toBeInTheDocument()
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(screen.queryByText(CONSENT_VERSIONS_UNAVAILABLE)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Create account' })).toBeDisabled()
      expect(mockRegisterMutate).toHaveBeenCalledTimes(1)
    })

    it('does not register, and says why, when the versions cannot be read', async () => {
      publishInTurn(unavailable)

      await submitFilledForm()

      expect(await screen.findByText(CONSENT_VERSIONS_UNAVAILABLE)).toBeInTheDocument()
      expect(mockRegisterMutate).not.toHaveBeenCalled()
    })

    it('retries once, with a notice, after a refusal of the published versions', async () => {
      const republished = deferred<Response>()
      const versions = publishInTurn(
        () => versionsBody(PUBLISHED),
        () => republished.promise
      )
      const { rerender } = await submitFilledForm()
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      const readsBeforeRefusal = versions.reads()

      // The mutation reports the refusal, as the real one does.
      mockRegisterError = CONSENT_REFUSAL
      act(() => {
        registerCall(0).callbacks.onError(CONSENT_REFUSAL)
      })
      act(() => {
        rerender()
      })

      expect(await screen.findByText(CONSENT_RETRY_NOTICE)).toBeInTheDocument()
      expect(screen.queryByText(CONSENT_REFUSAL.message)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Creating account/ })).toBeDisabled()
      await waitFor(() => expect(versions.reads()).toBe(readsBeforeRefusal + 1))

      republished.resolve(versionsBody(PUBLISHED))

      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(2))
      expect(registerCall(1).request).toMatchObject({
        terms_version: CURRENT_TERMS_VERSION,
        privacy_version: CURRENT_PRIVACY_VERSION,
      })
      // The notice ends once the retried request is out.
      await waitFor(() => {
        expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      })
    })

    it('drops a retry, and leaves the form usable, when the page is hidden during the re-read', async () => {
      const republished = deferred<Response>()
      publishInTurn(() => versionsBody(PUBLISHED), () => republished.promise)
      const user = userEvent.setup()
      const page = (mode: 'visible' | 'hidden') => (
        <Activity mode={mode}>
          <AuthPage />
        </Activity>
      )
      const rendered = renderWithProviders(page('visible'))
      await user.click(screen.getByRole('tab', { name: 'Create account' }))
      await user.type(screen.getByLabelText('Email'), 'test@example.com')
      await user.type(screen.getByLabelText('Password'), 'validPassword123!')
      await user.click(screen.getByRole('checkbox', { name: TERMS_CHECKBOX }))
      await user.click(screen.getByRole('checkbox', { name: AGE_CHECKBOX }))
      await user.click(screen.getByRole('button', { name: 'Create account' }))
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      act(() => {
        registerCall(0).callbacks.onError(CONSENT_REFUSAL)
      })
      expect(await screen.findByText(CONSENT_RETRY_NOTICE)).toBeInTheDocument()

      rendered.rerender(page('hidden'))
      await act(async () => {
        republished.resolve(versionsBody(PUBLISHED))
      })
      rendered.rerender(page('visible'))
      // A shown page reopens on its arrival's tab.
      await user.click(screen.getByRole('tab', { name: 'Create account' }))

      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
      expect(mockRegisterMutate).toHaveBeenCalledTimes(1)
    })

    it('shows only the latest of the form errors', async () => {
      publishInTurn(unavailable)
      await submitFilledForm()
      expect(await screen.findByText(CONSENT_VERSIONS_UNAVAILABLE)).toBeInTheDocument()

      // Submitting again with the versions readable clears it.
      publishInTurn(() => versionsBody(PUBLISHED))
      await userEvent.setup().click(screen.getByRole('button', { name: 'Create account' }))

      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      expect(screen.queryByText(CONSENT_VERSIONS_UNAVAILABLE)).not.toBeInTheDocument()
    })

    it('shows a second refusal instead of retrying again', async () => {
      publishInTurn(() => versionsBody(PUBLISHED), () => versionsBody(PUBLISHED))
      const { rerender } = await submitFilledForm()
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      act(() => {
        registerCall(0).callbacks.onError(CONSENT_REFUSAL)
      })
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(2))

      mockRegisterError = CONSENT_REFUSAL
      act(() => {
        registerCall(1).callbacks.onError(CONSENT_REFUSAL)
      })
      act(() => {
        rerender()
      })

      expect(await screen.findByText(CONSENT_REFUSAL.message)).toBeInTheDocument()
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(mockRegisterMutate).toHaveBeenCalledTimes(2)
      expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
    })

    it('does not retry a refusal of anything other than consent', async () => {
      const versions = publishInTurn(() => versionsBody(PUBLISHED))
      await submitFilledForm()
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))
      const readsBeforeRefusal = versions.reads()

      act(() => {
        registerCall(0).callbacks.onError(
          new AuthError('Email already registered', AuthErrorCode.USER_EXISTS)
        )
      })

      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(mockRegisterMutate).toHaveBeenCalledTimes(1)
      expect(versions.reads()).toBe(readsBeforeRefusal)
    })

    it('does not retry when the versions for the retry cannot be read', async () => {
      publishInTurn(() => versionsBody(PUBLISHED), unavailable)
      await submitFilledForm()
      await waitFor(() => expect(mockRegisterMutate).toHaveBeenCalledTimes(1))

      act(() => {
        registerCall(0).callbacks.onError(CONSENT_REFUSAL)
      })

      expect(await screen.findByText(CONSENT_VERSIONS_UNAVAILABLE)).toBeInTheDocument()
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(mockRegisterMutate).toHaveBeenCalledTimes(1)
    })
  })
})
