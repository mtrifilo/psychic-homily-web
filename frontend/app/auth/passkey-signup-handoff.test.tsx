import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'

// The page with the real passkey sign-up button and the real backup prompt, so
// these tests cover the hand-off between them rather than either one alone.

const PROMPT = { name: 'Add a backup sign-in method' }

const RETURN_TO = '/collections'
const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => searchParams.current,
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
  renderWithProviders(<AuthPage />)

  await user.click(screen.getByRole('tab', { name: 'Create account' }))
  await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
  const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
  await user.type(within(dialog).getByLabelText('Email'), 'signup@example.com')
  await user.click(within(dialog).getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(within(dialog).getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(within(dialog).getByRole('button', { name: /continue with passkey/i }))
  return user
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

  it('redirects to returnTo once the prompt is skipped, and only then', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    expect(mockPush).not.toHaveBeenCalled()

    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })
    expect(mockPush).toHaveBeenCalledTimes(1)
  })

  it('redirects to returnTo when the prompt is closed with Escape', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    await screen.findByRole('dialog', PROMPT)

    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })
    expect(mockPush).toHaveBeenCalledTimes(1)
  })

  it('redirects to returnTo when the prompt is closed with its close button', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)

    await user.click(within(prompt).getByRole('button', { name: 'Close' }))

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })
    expect(mockPush).toHaveBeenCalledTimes(1)
  })

  it('sends a hostile returnTo to "/" after the prompt, not off site', async () => {
    searchParams.current = new URLSearchParams({ returnTo: 'https://evil.com/phish' })
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/')
    })
    expect(mockPush).toHaveBeenCalledTimes(1)
  })

  // A definitive profile failure outranks `setUser` in the real provider until
  // the next profile fetch succeeds, so the viewer can still read as anonymous
  // when the prompt closes. The page then shows the form and redirects as soon
  // as the profile reports the session.
  it('shows the form, not a redirect, while the profile has not reported the session', async () => {
    mockSetUser.mockImplementationOnce(() => {})
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

    const user = await submitPasskeySignup()
    const prompt = await screen.findByRole('dialog', PROMPT)
    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    expect(await screen.findByRole('tab', { name: 'Sign in' })).toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()

    act(() => {
      authStore.set('authenticated')
    })
    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })
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
