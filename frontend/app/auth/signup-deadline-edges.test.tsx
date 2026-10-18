import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity } from 'react'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { clientWithConsentVersions } from '@/test/consentVersions'
import { advanceFakeTime, stalledRequest } from '@/test/stalledFetch'
import { deferred } from '@/test/inflightOrdering'
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
    it('still bounds a stalled registration after a passkey sign-up starts', async () => {
      routeFetch({
        '/auth/register': stalledRequest,
        '/auth/passkey/signup/begin': () => Promise.resolve(BEGIN_OK),
      })
      mockStartRegistration.mockRejectedValue(
        new DOMException('The operation was cancelled.', 'NotAllowedError')
      )
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient: clientWithConsentVersions() })

      await submitPasswordSignup(user)
      await startPasskeySignup(user)
      await waitFor(() => {
        expect(mockStartRegistration).toHaveBeenCalledTimes(1)
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(signalOf('/auth/register').aborted).toBe(true)
      expect(await screen.findByText('Registration failed')).toBeInTheDocument()
    })

    it('still bounds a stalled passkey begin after the registration is refused', async () => {
      const register = deferred<Response>()
      routeFetch({
        '/auth/register': () => register.promise,
        '/auth/passkey/signup/begin': stalledRequest,
      })
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient: clientWithConsentVersions() })

      await submitPasswordSignup(user)
      const dialog = await startPasskeySignup(user)
      await act(async () => {
        register.resolve(
          jsonResponse({ success: false, message: 'Email already registered' }, 409)
        )
      })
      expect(await screen.findByText('Email already registered')).toBeInTheDocument()

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(signalOf('/auth/passkey/signup/begin').aborted).toBe(true)
      await waitFor(() => {
        expect(
          within(dialog).getByRole('button', { name: /continue with passkey/i })
        ).toBeEnabled()
      })
    })
  })

  describe('a registration from a form the tab switch remounted', () => {
    // Radix unmounts an inactive tab's panel, so leaving and coming back gives
    // a fresh form whose submit is enabled while the first request is still out.
    it('leaves the newer registration its own deadline when the first one lands', async () => {
      const first = deferred<Response>()
      let registers = 0
      routeFetch({
        '/auth/register': (url, init) => {
          registers += 1
          return registers === 1 ? first.promise : stalledRequest(url, init)
        },
      })
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient: clientWithConsentVersions() })

      await submitPasswordSignup(user)
      await user.click(screen.getByRole('tab', { name: 'Sign in' }))
      await user.click(screen.getByRole('tab', { name: 'Create account' }))
      await user.type(screen.getByLabelText('Email'), 'signup@example.com')
      await user.type(screen.getByLabelText('Password'), 'validPassword123!')
      await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
      await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
      await user.click(screen.getByRole('button', { name: 'Create account' }))
      await waitFor(() => {
        expect(callsTo('/auth/register')).toHaveLength(2)
      })

      await act(async () => {
        first.resolve(jsonResponse(REGISTERED))
      })
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)

      const [, secondInit] = callsTo('/auth/register')[1] as [string, RequestInit]
      expect((secondInit.signal as AbortSignal).aborted).toBe(true)
      expect(await screen.findByText('Registration failed')).toBeInTheDocument()
    })
  })

  describe('a registration that settles after a passkey sign-up completed', () => {
    const PROMPT = { name: 'Add a backup sign-in method' }
    const FINISH_OK = {
      json: async () => ({
        success: true,
        user: { id: 200, email: 'passkey@example.com', email_verified: false },
      }),
    }

    async function completePasskeyBehind(register: (url: string, init?: RequestInit) => unknown) {
      routeFetch({
        '/auth/register': register,
        '/auth/passkey/signup/begin': () => Promise.resolve(BEGIN_OK),
        '/auth/passkey/signup/finish': () => Promise.resolve(FINISH_OK),
      })
      mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient: clientWithConsentVersions() })

      await submitPasswordSignup(user)
      await startPasskeySignup(user)
      expect(await screen.findByRole('dialog', PROMPT)).toBeInTheDocument()
      act(() => {
        authStore.set('authenticated')
      })
    }

    it('leaves the backup prompt when the registration then fails on its deadline', async () => {
      await completePasskeyBehind(stalledRequest)

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(signalOf('/auth/register').aborted).toBe(true)
      expect(screen.getByRole('dialog', PROMPT)).toBeInTheDocument()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('leaves the backup prompt when the registration then succeeds', async () => {
      const register = deferred<Response>()
      await completePasskeyBehind(() => register.promise)

      await act(async () => {
        register.resolve(jsonResponse(REGISTERED))
      })
      await advanceFakeTime(1_000)

      expect(screen.getByRole('dialog', PROMPT)).toBeInTheDocument()
      expect(screen.queryByRole('heading', { name: 'Check your inbox.' })).toBeNull()
      expect(mockPush).not.toHaveBeenCalled()
    })
  })

  describe('a registration answered before its profile refetch lands', () => {
    // The register request answers 5 s after the submit; the profile refetch
    // `useRegister` awaits after it is still out a second past the 30 s mark.
    const REGISTER_ANSWER_MS = 5_000
    const PAST_SUBMIT_DEADLINE_MS = SIGNUP_REQUEST_TIMEOUT_MS - REGISTER_ANSWER_MS + 1_000

    async function registerWithStalledRefetch() {
      const queryClient = clientWithConsentVersions()
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

    // The refetch gets a deadline of its own, from the register answer.
    it('holds the claim past 30 s from the submit while the refetch is still out', async () => {
      await registerWithStalledRefetch()
      act(() => {
        authStore.set('authenticated')
      })

      await advanceFakeTime(PAST_SUBMIT_DEADLINE_MS)
      expect(signalOf('/auth/register').aborted).toBe(false)
      expect(mockPush).not.toHaveBeenCalled()

      await advanceFakeTime(REGISTER_ANSWER_MS)
      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
      })
      expect(mockPush).toHaveBeenCalledTimes(1)
      expect(signalOf('/auth/register').aborted).toBe(false)
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

  describe('a registration whose profile refetch outlasts its own deadline', () => {
    it('releases the claim without a failure, then shows the interstitial once the refetch lands', async () => {
      const queryClient = clientWithConsentVersions()
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
      routeFetch({ '/auth/register': () => Promise.resolve(jsonResponse(REGISTERED)) })
      const user = setupUser()
      renderWithProviders(<AuthPage />, { queryClient })

      await submitPasswordSignup(user)
      await waitFor(() => {
        expect(profile).toHaveBeenCalledTimes(2)
      })
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)

      expect(screen.queryByText('Registration failed')).not.toBeInTheDocument()
      expect(screen.getByLabelText('Email')).toHaveValue('signup@example.com')

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
    // A shown page gives the request a fresh deadline, on the signal it holds.
    it('resumes the redirect 30 s after the page is shown again', async () => {
      routeFetch({ '/auth/register': stalledRequest })
      const user = setupUser()
      const { rerender } = renderWithProviders(
        <Activity mode="visible">
          <AuthPage />
        </Activity>,
        { queryClient: clientWithConsentVersions() }
      )

      await submitPasswordSignup(user)
      rerender(
        <Activity mode="hidden">
          <AuthPage />
        </Activity>
      )
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)
      rerender(
        <Activity mode="visible">
          <AuthPage />
        </Activity>
      )
      // Any profile refetch can report the session the register request set.
      act(() => {
        authStore.set('authenticated')
      })

      // The fake clock also follows real time, so the lead is not exact.
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS - 1_000)
      expect(signalOf('/auth/register').aborted).toBe(false)
      expect(mockPush).not.toHaveBeenCalled()

      await advanceFakeTime(1_000)
      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith(RETURN_TO)
      })
      expect(mockPush).toHaveBeenCalledTimes(1)
      expect(signalOf('/auth/register').aborted).toBe(true)
    })
  })
})
