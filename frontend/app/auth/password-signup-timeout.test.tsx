import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'

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
function stalledRequest(_url: string, init?: RequestInit): Promise<never> {
  return new Promise((_, reject) => {
    init?.signal?.addEventListener('abort', () => {
      reject(new DOMException('The operation was aborted.', 'AbortError'))
    })
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

describe('password sign-up whose register response never arrives', () => {
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

  it('PREMISE: holds the redirect and shows no error long after 30 s', async () => {
    await submitPasswordSignup()

    act(() => {
      authStore.set('authenticated')
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(120_000)
    })

    expect(mockPush).not.toHaveBeenCalled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
