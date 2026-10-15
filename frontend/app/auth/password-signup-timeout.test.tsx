import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'
import { SIGNUP_REQUEST_TIMEOUT_MS } from './_components/signup-request-deadline'

// The page with the real `useRegister` mutation and a stubbed fetch, so these
// tests drive the register request itself rather than a mocked mutation.

const RETURN_TO = '/collections'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams({ returnTo: RETURN_TO }),
}))

// A viewer store a test can flip to authenticated mid-request, as any profile
// refetch can once the register request has set the session cookie.
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

vi.mock('@/lib/context/AuthContext', async () => {
  const { useSyncExternalStore } = await import('react')
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({
        setUser: vi.fn(),
        authStatus: useSyncExternalStore(authStore.subscribe, authStore.get),
      }),
  }
})

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (): null => null,
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

const fetchMock = vi.fn()

// A request on a stalled connection: nothing comes back until its signal
// aborts, and then it rejects the way fetch does.
function stalled(signal: AbortSignal | null | undefined): Promise<never> {
  return new Promise((_, reject) => {
    signal?.addEventListener('abort', () => {
      reject(new DOMException('The operation was aborted.', 'AbortError'))
    })
  })
}

const stalledRequest = (_url: string, init?: RequestInit) => stalled(init?.signal)

// Headers arrived; the body never does.
const stalledBody = async (_url: string, init?: RequestInit) => ({
  ok: true,
  status: 200,
  headers: new Headers({ 'Content-Type': 'application/json' }),
  json: () => stalled(init?.signal),
})

function registerSignal(): AbortSignal {
  return (fetchMock.mock.calls[0]?.[1] as RequestInit).signal as AbortSignal
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

async function submitPasswordSignup() {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderWithProviders(<AuthPage />)

  await user.click(screen.getByRole('tab', { name: 'Create account' }))
  await user.type(screen.getByLabelText('Email'), 'signup@example.com')
  await user.type(screen.getByLabelText('Password'), 'validPassword123!')
  await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(screen.getByRole('button', { name: 'Create account' }))

  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
}

describe('password sign-up under the request deadline', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    mockPush.mockReset()
    fetchMock.mockReset()
    fetchMock.mockImplementation(stalledRequest)
    vi.stubGlobal('fetch', fetchMock)
    authStore.set('anonymous')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it.each([
    ['the register response', stalledRequest],
    ['the register response body', stalledBody],
  ])(
    'releases the claim and shows the generic failure when %s stalls for 30 s',
    async (_label, register) => {
      fetchMock.mockImplementation(register)
      await submitPasswordSignup()

      // The fake clock also follows real time, so the lead is not exact.
      await advance(SIGNUP_REQUEST_TIMEOUT_MS - 1_000)
      expect(registerSignal().aborted).toBe(false)
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()

      await advance(1_000)
      expect(await screen.findByText('Registration failed')).toBeInTheDocument()
      expect(registerSignal().aborted).toBe(true)
      expect(screen.getByLabelText('Email')).toHaveValue('signup@example.com')
      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
      })
      expect(mockPush).not.toHaveBeenCalled()
    }
  )

  it('holds the redirect until the deadline, then resumes it', async () => {
    await submitPasswordSignup()

    // Any profile refetch can report the session the register request set.
    act(() => {
      authStore.set('authenticated')
    })
    await advance(SIGNUP_REQUEST_TIMEOUT_MS - 1_000)
    expect(mockPush).not.toHaveBeenCalled()

    await advance(1_000)
    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })
    expect(mockPush).toHaveBeenCalledTimes(1)
  })

  it('leaves a completed sign-up alone once the deadline would have passed', async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          message: 'ok',
          user: { id: 100, email: 'signup@example.com', email_verified: false },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    await submitPasswordSignup()

    expect(
      await screen.findByRole('heading', { name: 'Check your inbox.' })
    ).toBeInTheDocument()
    await advance(SIGNUP_REQUEST_TIMEOUT_MS * 2)

    expect(registerSignal().aborted).toBe(false)
    expect(screen.getByRole('heading', { name: 'Check your inbox.' })).toBeInTheDocument()
    expect(screen.queryByText('Registration failed')).not.toBeInTheDocument()
  })
})
