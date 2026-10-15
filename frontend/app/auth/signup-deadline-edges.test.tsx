import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity, type ReactElement } from 'react'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import { advanceFakeTime, stalledRequest } from '@/test/stalledFetch'
import { queryKeys } from '@/lib/queryClient'
import AuthPage from './page'
import { SIGNUP_REQUEST_TIMEOUT_MS } from './_components/signup-request-deadline'

// The page with the real `useRegister` mutation and the real passkey sign-up
// button behind one stubbed fetch, so these tests drive both sign-up paths and
// the deadlines bounding them together.

const RETURN_TO = '/collections'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams({ returnTo: RETURN_TO }),
}))

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

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

const mockStartRegistration = vi.fn()
vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startRegistration: (...args: unknown[]) => mockStartRegistration(...args),
}))

const fetchMock = vi.fn()

/** Answers each endpoint from its own handler; an unrouted call fails loudly. */
function routeFetch(routes: Record<string, (url: string, init?: RequestInit) => unknown>) {
  fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    const path = new URL(url).pathname
    const handler = routes[path]
    if (!handler) {
      throw new Error(`unrouted fetch: ${path}`)
    }
    return handler(url, init)
  })
}

function callsTo(path: string) {
  return fetchMock.mock.calls.filter(([url]) => new URL(url as string).pathname === path)
}

function signalOf(path: string): AbortSignal {
  const [call] = callsTo(path)
  return (call[1] as RequestInit).signal as AbortSignal
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const REGISTERED = {
  success: true,
  message: 'ok',
  user: { id: 100, email: 'signup@example.com', email_verified: false },
}

const BEGIN_OK = {
  json: async () => ({
    success: true,
    options: { publicKey: { challenge: 'begin-signup-challenge' } },
    challenge_id: 'challenge-id',
  }),
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>(settle => {
    resolve = settle
  })
  return { promise, resolve }
}

function visible(ui: ReactElement) {
  return <Activity mode="visible">{ui}</Activity>
}

function hidden(ui: ReactElement) {
  return <Activity mode="hidden">{ui}</Activity>
}

function setupUser() {
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
}

async function submitPasswordSignup(user: ReturnType<typeof setupUser>) {
  await user.click(screen.getByRole('tab', { name: 'Create account' }))
  await user.type(screen.getByLabelText('Email'), 'signup@example.com')
  await user.type(screen.getByLabelText('Password'), 'validPassword123!')
  await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(screen.getByRole('button', { name: 'Create account' }))
  await waitFor(() => {
    expect(callsTo('/auth/register')).toHaveLength(1)
  })
}

async function startPasskeySignup(user: ReturnType<typeof setupUser>) {
  await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
  const dialog = screen.getByRole('dialog', { name: 'Create account with passkey' })
  await user.type(within(dialog).getByLabelText('Email'), 'passkey@example.com')
  await user.click(within(dialog).getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(within(dialog).getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(within(dialog).getByRole('button', { name: /continue with passkey/i }))
  await waitFor(() => {
    expect(callsTo('/auth/passkey/signup/begin')).toHaveLength(1)
  })
  return dialog
}

describe('sign-up deadlines on /auth', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    mockPush.mockReset()
    mockStartRegistration.mockReset()
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    authStore.set('anonymous')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  describe('with both sign-up paths in flight', () => {
    it('leaves a stalled registration unbounded once a passkey sign-up starts', async () => {
      routeFetch({
        '/auth/register': stalledRequest,
        '/auth/passkey/signup/begin': () => Promise.resolve(BEGIN_OK),
      })
      mockStartRegistration.mockRejectedValue(
        new DOMException('The operation was cancelled.', 'NotAllowedError')
      )
      const user = setupUser()
      renderWithProviders(<AuthPage />)

      await submitPasswordSignup(user)
      await startPasskeySignup(user)
      await waitFor(() => {
        expect(mockStartRegistration).toHaveBeenCalledTimes(1)
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(signalOf('/auth/register').aborted).toBe(false)
      expect(screen.queryByText('Registration failed')).not.toBeInTheDocument()
    })

    it('leaves a stalled passkey begin unbounded once the registration is refused', async () => {
      const register = deferred<Response>()
      routeFetch({
        '/auth/register': () => register.promise,
        '/auth/passkey/signup/begin': stalledRequest,
      })
      const user = setupUser()
      renderWithProviders(<AuthPage />)

      await submitPasswordSignup(user)
      const dialog = await startPasskeySignup(user)
      await act(async () => {
        register.resolve(
          jsonResponse({ success: false, message: 'Email already registered' }, 409)
        )
      })
      expect(await screen.findByText('Email already registered')).toBeInTheDocument()

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(signalOf('/auth/passkey/signup/begin').aborted).toBe(false)
      expect(
        within(dialog).getByRole('button', { name: 'Creating account...' })
      ).toBeDisabled()
    })
  })

  describe('a registration answered before its profile refetch lands', () => {
    // The register request answers 5 s after the submit; the profile refetch
    // `useRegister` awaits after it is still out a second past the 30 s mark.
    const REGISTER_ANSWER_MS = 5_000
    const PAST_SUBMIT_DEADLINE_MS = SIGNUP_REQUEST_TIMEOUT_MS - REGISTER_ANSWER_MS + 1_000

    async function registerWithStalledRefetch() {
      const queryClient = createTestQueryClient()
      const refetch = deferred<{ success: boolean }>()
      const profile = vi
        .fn()
        .mockResolvedValueOnce({ success: false })
        .mockReturnValueOnce(refetch.promise)
      await queryClient.fetchQuery({
        queryKey: queryKeys.auth.profile,
        queryFn: profile,
        gcTime: Infinity,
      })
      const register = deferred<Response>()
      routeFetch({ '/auth/register': () => register.promise })
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient })

      await submitPasswordSignup(user)
      await advanceFakeTime(REGISTER_ANSWER_MS)
      await act(async () => {
        register.resolve(jsonResponse(REGISTERED))
      })
      await waitFor(() => {
        expect(profile).toHaveBeenCalledTimes(2)
      })
      return { refetch, profile }
    }

    it('releases the claim 30 s after the submit while the refetch is still out', async () => {
      await registerWithStalledRefetch()
      act(() => {
        authStore.set('authenticated')
      })

      await advanceFakeTime(PAST_SUBMIT_DEADLINE_MS)
      expect(signalOf('/auth/register').aborted).toBe(true)
      expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
    })

    it('shows the interstitial when the refetch lands past 30 s from the submit', async () => {
      const { refetch } = await registerWithStalledRefetch()

      await advanceFakeTime(PAST_SUBMIT_DEADLINE_MS)
      await act(async () => {
        refetch.resolve({ success: true })
      })

      expect(
        await screen.findByRole('heading', { name: 'Check your inbox.' })
      ).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })
  })

  describe('a page hidden with a registration in flight', () => {
    it('never resumes the redirect after the page is shown again', async () => {
      routeFetch({ '/auth/register': stalledRequest })
      const user = setupUser()
      const { rerender } = renderWithProviders(visible(<AuthPage />))

      await submitPasswordSignup(user)
      rerender(hidden(<AuthPage />))
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)
      rerender(visible(<AuthPage />))
      // Any profile refetch can report the session the register request set.
      act(() => {
        authStore.set('authenticated')
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
      expect(signalOf('/auth/register').aborted).toBe(false)
      expect(mockPush).not.toHaveBeenCalled()
    })
  })
})
