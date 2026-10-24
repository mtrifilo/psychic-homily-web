import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity } from 'react'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent, { type UserEvent } from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { clientWithConsentVersions } from '@/test/consentVersions'
import { advanceFakeTime, stalledRequest } from '@/test/stalledFetch'
import AuthPage from './page'
import { SIGNUP_REQUEST_TIMEOUT_MS } from './_components/signup-request-deadline'

// The page with the real passkey sign-up button and the real backup prompt, so
// these tests cover the hand-off between them rather than either one alone.

const PROMPT = { name: 'Add a backup sign-in method' }
const INBOX = { name: 'Check your inbox.' }

const RETURN_TO = '/collections'
const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => searchParams.current,
  usePathname: () => '/auth',
}))

// A viewer store `setUser` writes to, as the real AuthProvider's override does:
// handing it a user makes the viewer authenticated on the next render.
const authStore = vi.hoisted(() => {
  let status: 'pending' | 'anonymous' | 'authenticated' = 'anonymous'
  const listeners = new Set<() => void>()
  return {
    get: () => status,
    set(next: typeof status) {
      status = next
      listeners.forEach(listener => listener())
    },
    subscribe(listener: () => void) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
})
const mockSetUser = vi.fn((user: unknown) => {
  authStore.set(user ? 'authenticated' : 'anonymous')
})

vi.mock('@/lib/context/AuthContext', async () => {
  const { useSyncExternalStore } = await import('react')
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({
        setUser: mockSetUser,
        authStatus: useSyncExternalStore(authStore.subscribe, authStore.get),
      }),
  }
})

vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
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

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

const mockStartRegistration = vi.fn()
vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startRegistration: (...args: unknown[]) => mockStartRegistration(...args),
}))

const fetchMock = vi.fn()

const BEGIN_OK = {
  json: async () => ({
    success: true,
    options: { publicKey: { challenge: 'begin-signup-challenge' } },
    challenge_id: 'challenge-id',
  }),
}

const FINISH_OK = {
  json: async () => ({
    success: true,
    user: {
      id: 100,
      email: 'signup@example.com',
      email_verified: false,
    },
  }),
}

async function submitPasskeySignup() {
  const user = userEvent.setup()
  renderWithProviders(<AuthPage />, { queryClient: clientWithConsentVersions() })

  await user.click(screen.getByRole('tab', { name: 'Create account' }))
  await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
  const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
  await user.type(within(dialog).getByLabelText('Email'), 'signup@example.com')
  await user.click(within(dialog).getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(within(dialog).getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(within(dialog).getByRole('button', { name: /continue with passkey/i }))
  return user
}

function skipPrompt(user: UserEvent, prompt: HTMLElement) {
  return user.click(within(prompt).getByRole('button', { name: "I'll do this later" }))
}

describe('passkey sign-up on /auth', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockClear()
    mockStartRegistration.mockReset()
    mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    authStore.set('anonymous')
    searchParams.current = new URLSearchParams({ returnTo: RETURN_TO })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the backup prompt instead of redirecting once the account exists', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    await submitPasskeySignup()

    expect(await screen.findByRole('dialog', PROMPT)).toBeInTheDocument()
    expect(mockSetUser).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'signup@example.com' })
    )
    expect(authStore.get()).toBe('authenticated')
    expect(mockPush).not.toHaveBeenCalled()
  })

  describe('closing the backup prompt', () => {
    const closeRoutes: Array<[string, (user: UserEvent, prompt: HTMLElement) => Promise<void>]> = [
      ['skipped', skipPrompt],
      ['closed with Escape', user => user.keyboard('{Escape}')],
      ['closed with its close button', (user, prompt) =>
        user.click(within(prompt).getByRole('button', { name: 'Close' }))],
    ]

    it.each(closeRoutes)(
      'shows the check-your-inbox interstitial for the new address when the prompt is %s',
      async (_route, close) => {
        // The account's address as the server stored it, not as it was typed.
        fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce({
          json: async () => ({
            success: true,
            user: { id: 100, email: 'stored@example.com', email_verified: false },
          }),
        })

        const user = await submitPasskeySignup()
        await close(user, await screen.findByRole('dialog', PROMPT))

        expect(await screen.findByRole('heading', INBOX)).toBeInTheDocument()
        expect(screen.getByText('stored@example.com')).toBeInTheDocument()
        expect(screen.queryByText('signup@example.com')).not.toBeInTheDocument()
        expect(screen.queryByRole('dialog', PROMPT)).not.toBeInTheDocument()
        expect(mockPush).not.toHaveBeenCalled()
      }
    )
  })

  it('continues from the interstitial to returnTo, the only way on', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await skipPrompt(user, prompt)
    await screen.findByRole('heading', INBOX)

    expect(
      screen.getByRole('link', { name: 'Continue where you left off' })
    ).toHaveAttribute('href', RETURN_TO)
    // The redirect effect stays held back, so the link is the one navigation.
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('offers the shows listing, not a hostile returnTo, from the interstitial', async () => {
    searchParams.current = new URLSearchParams({ returnTo: 'https://evil.com/phish' })
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await skipPrompt(user, prompt)

    expect(
      await screen.findByRole('link', { name: 'Browse upcoming shows' })
    ).toHaveAttribute('href', '/shows')
    expect(document.querySelector('a[href*="evil.com"]')).toBeNull()
    expect(mockPush).not.toHaveBeenCalled()
  })

  // A definitive profile failure outranks `setUser` in the real provider until
  // the next profile fetch succeeds, so the viewer can still read as anonymous
  // when the prompt closes. The interstitial shows either way, as it does
  // after a password sign-up.
  it('shows the interstitial, not the form, while the profile has not reported the session', async () => {
    mockSetUser.mockImplementationOnce(() => {})
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await skipPrompt(user, prompt)

    expect(await screen.findByRole('heading', INBOX)).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()

    act(() => {
      authStore.set('authenticated')
    })
    expect(screen.getByRole('heading', INBOX)).toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('holds the redirect while the finish request is in flight', async () => {
    let resolveFinish: (value: typeof FINISH_OK) => void = () => {}
    fetchMock
      .mockResolvedValueOnce(BEGIN_OK)
      .mockReturnValueOnce(new Promise(resolve => { resolveFinish = resolve }))

    await submitPasskeySignup()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    // Any other profile refetch can report the new session before this
    // component has read the finish response.
    act(() => {
      authStore.set('authenticated')
    })
    expect(mockPush).not.toHaveBeenCalled()

    await act(async () => {
      resolveFinish(FINISH_OK)
    })

    expect(await screen.findByRole('dialog', PROMPT)).toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  describe('a finish response that never arrives', () => {
    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
      fetchMock.mockResolvedValueOnce(BEGIN_OK).mockImplementationOnce(stalledRequest)
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('shows the form and the generic failure after 30 s', async () => {
      await submitPasskeySignup()
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledTimes(2)
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)

      expect(
        await screen.findByText('Failed to complete passkey signup')
      ).toBeInTheDocument()
      const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
      expect(
        within(dialog).getByRole('button', { name: /continue with passkey/i })
      ).toBeEnabled()
      expect(mockSetUser).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // The page renders neither the form nor the redirect while it holds a
    // pending claim for an authenticated viewer, so the control that sent the
    // request is gone by the time the deadline passes.
    it('resumes the redirect after 30 s once the session already exists', async () => {
      await submitPasskeySignup()
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledTimes(2)
      })

      act(() => {
        authStore.set('authenticated')
      })
      expect(
        screen.queryByRole('tab', { name: 'Create account' })
      ).not.toBeInTheDocument()
      // The fake clock also follows real time, so the lead is not exact.
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS - 1_000)
      expect(mockPush).not.toHaveBeenCalled()

      await advanceFakeTime(1_000)
      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
      })
      expect(mockPush).toHaveBeenCalledTimes(1)
    })

    // A stale re-auth link offers Create account; the session that appears
    // during that sign-up is the new account's, so it goes on to returnTo
    // like any other arrival rather than to the re-authentication form. A
    // later visit under the same URL, once the page was hidden, is a
    // re-authentication again.
    it('resumes the redirect on a stale re-auth link, and holds a later re-auth', async () => {
      searchParams.current = new URLSearchParams({
        returnTo: RETURN_TO,
        reason: 'REAUTH_REQUIRED',
      })
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      const { rerender } = renderWithProviders(
        <Activity mode="visible">
          <AuthPage />
        </Activity>,
        { queryClient: clientWithConsentVersions() }
      )
      await user.click(screen.getByRole('tab', { name: 'Create account' }))
      await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
      const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
      await user.type(within(dialog).getByLabelText('Email'), 'signup@example.com')
      await user.click(within(dialog).getByRole('checkbox', { name: /Terms of Service/ }))
      await user.click(within(dialog).getByRole('checkbox', { name: /at least 16 years old/ }))
      await user.click(within(dialog).getByRole('button', { name: /continue with passkey/i }))
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledTimes(2)
      })

      act(() => {
        authStore.set('authenticated')
      })
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)
      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
      })
      expect(screen.queryByText("Confirm it's you")).not.toBeInTheDocument()

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

      expect(
        await screen.findByRole('heading', { level: 1, name: "Confirm it's you" })
      ).toBeInTheDocument()
      expect(screen.queryByRole('tab', { name: /create account/i })).not.toBeInTheDocument()
      expect(mockPush).toHaveBeenCalledTimes(1)
    })
  })

  it('shows an account-exists refusal inside the open dialog, not in the page alert', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: false,
        message: 'An account with this email already exists',
      }),
    })

    await submitPasskeySignup()

    const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
    expect(
      await within(dialog).findByText('An account with this email already exists')
    ).toBeInTheDocument()
    // One copy, and it is the dialog's: the page behind the overlay has none.
    expect(
      screen.getAllByText('An account with this email already exists')
    ).toHaveLength(1)
    expect(mockPush).not.toHaveBeenCalled()
  })

  // A re-authentication link that outlived its session offers Create account,
  // and an account made from it goes on like any other new one, not back to
  // the re-authentication form.
  it('shows the check-inbox interstitial after a passkey sign-up from a stale re-auth link', async () => {
    searchParams.current = new URLSearchParams({
      returnTo: RETURN_TO,
      reason: 'REAUTH_REQUIRED',
    })
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    expect(
      await screen.findByRole('heading', { name: 'Check your inbox.' })
    ).toBeInTheDocument()
    expect(screen.queryByText("Confirm it's you")).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('keeps the form and reports the error when the finish request is refused', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    await submitPasskeySignup()

    expect(await screen.findByText('Email already registered')).toBeInTheDocument()
    expect(screen.queryByRole('dialog', PROMPT)).not.toBeInTheDocument()
    expect(mockSetUser).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
  })
})
