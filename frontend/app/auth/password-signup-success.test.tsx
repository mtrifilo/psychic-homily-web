import { describe, it, expect, vi, beforeEach, afterEach, onTestFinished } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { clientWithConsentVersions } from '@/test/consentVersions'
import { deferred } from '@/test/inflightOrdering'
import { advanceFakeTime, headersThenBody } from '@/test/stalledFetch'
import { AuthProvider } from '@/lib/context/AuthContext'
import { queryKeys } from '@/lib/queryClient'
import AuthPage from './page'
import { SIGNUP_REQUEST_TIMEOUT_MS } from './_components/signup-request-deadline'

// The page under the real AuthProvider, so the profile query is what reports
// the new session, with the real `useRegister` behind one stubbed fetch. Once
// the profile names a viewer while the sign-up claim is pending, the page
// stops rendering the form.

const RETURN_TO = '/collections'
const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => searchParams.current,
  usePathname: () => '/auth',
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

const fetchMock = vi.fn()

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

const USER = { id: 100, email: 'signup@example.com', email_verified: false }
const REGISTERED = { success: true, message: 'ok', user: USER }
const SIGNED_IN_PROFILE = { success: true, user: USER }
const ANONYMOUS_PROFILE = { success: false }

/** Routes the two endpoints; `profile` gets the 1-based number of the request. */
function routeFetch(routes: {
  profile: (request: number) => Promise<Response>
  register: () => Promise<Response>
}) {
  let profileRequests = 0
  fetchMock.mockImplementation((url: string) => {
    const path = new URL(url).pathname
    if (path === '/auth/profile') {
      profileRequests += 1
      return routes.profile(profileRequests)
    }
    if (path === '/auth/register') {
      return routes.register()
    }
    throw new Error(`unrouted fetch: ${path}`)
  })
  return { profileRequests: () => profileRequests }
}

function registerCall() {
  return fetchMock.mock.calls.find(
    ([url]) => new URL(url as string).pathname === '/auth/register'
  )
}

function registerSignal(): AbortSignal {
  return (registerCall()?.[1] as RequestInit).signal as AbortSignal
}

/** Notes whether the page ever renders nothing once the form has shown. */
function watchForBlankPage(container: HTMLElement) {
  let formShown = false
  let blankSeen = false
  const observer = new MutationObserver(() => {
    if (container.querySelector('form')) formShown = true
    if (formShown && container.firstChild === null) blankSeen = true
  })
  observer.observe(container, { childList: true, subtree: true })
  onTestFinished(() => observer.disconnect())
  return { blankSeen: () => blankSeen }
}

function renderPage() {
  return renderWithProviders(
    <AuthProvider>
      <AuthPage />
    </AuthProvider>,
    { queryClient: clientWithConsentVersions() }
  )
}

async function submitPasswordSignup(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('tab', { name: 'Create account' }))
  await user.type(screen.getByLabelText('Email'), 'signup@example.com')
  await user.type(screen.getByLabelText('Password'), 'validPassword123!')
  await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
  await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
  await user.click(screen.getByRole('button', { name: 'Create account' }))
}

describe('password sign-up success with the real profile query', () => {
  beforeEach(() => {
    mockPush.mockReset()
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    searchParams.current = new URLSearchParams({ returnTo: RETURN_TO })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  // No blank frame rests on TanStack Query's notify scheduler, which hands
  // every store change to React in a later task (`setTimeout(cb, 0)` by
  // default), while the step from the refetch landing to `useRegister`'s
  // callbacks is all microtasks. The page reads the new profile only after the
  // interstitial is already its state. Anything that makes that step wait on a
  // task (an awaited timer or request after the refetch) lets the page render
  // its authenticated, claim-pending blank first; the interstitial still
  // follows, but this test fails.
  it('goes straight from the form to the interstitial when the awaited refetch lands late', async () => {
    const refetch = deferred<Response>()
    const requests = routeFetch({
      profile: n =>
        n === 1 ? Promise.resolve(jsonResponse(ANONYMOUS_PROFILE)) : refetch.promise,
      register: () => Promise.resolve(jsonResponse(REGISTERED)),
    })
    const user = userEvent.setup()
    const { container } = renderPage()
    const page = watchForBlankPage(container)

    await submitPasswordSignup(user)
    await waitFor(() => expect(requests.profileRequests()).toBe(2))
    // A turn of the task queue, so the refetch lands after the register
    // answer's own notifications have reached React.
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    await act(async () => {
      refetch.resolve(jsonResponse(SIGNED_IN_PROFILE))
    })

    expect(
      await screen.findByRole('heading', { name: 'Check your inbox.' })
    ).toBeInTheDocument()
    // The backup prompt belongs to a passkey sign-up only.
    expect(
      screen.queryByRole('dialog', { name: 'Add a backup sign-in method' })
    ).toBeNull()
    expect(page.blankSeen()).toBe(false)
    expect(mockPush).not.toHaveBeenCalled()
  })

  // A re-authentication link that outlived its session offers Create account,
  // and an account made from it goes on like any other new account.
  it('shows the interstitial, not the re-authentication form, after a sign-up from a stale re-auth link', async () => {
    searchParams.current = new URLSearchParams({
      returnTo: RETURN_TO,
      reason: 'REAUTH_REQUIRED',
    })
    routeFetch({
      profile: n =>
        Promise.resolve(jsonResponse(n === 1 ? ANONYMOUS_PROFILE : SIGNED_IN_PROFILE)),
      register: () => Promise.resolve(jsonResponse(REGISTERED)),
    })
    const user = userEvent.setup()
    renderPage()

    await submitPasswordSignup(user)

    expect(
      await screen.findByRole('heading', { name: 'Check your inbox.' })
    ).toBeInTheDocument()
    expect(screen.queryByText("Confirm it's you")).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Sign in' })).not.toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  // Another profile fetch (a focus or reconnect refetch, say) can land between
  // the register response's headers setting the cookie and its body arriving.
  // The page then stops rendering the form before the registration settles.
  describe('when another profile fetch reports the session before the registration settles', () => {
    async function registerBehindAnOutsideRefetch() {
      vi.useFakeTimers({ shouldAdvanceTime: true })
      const registerBody = deferred<unknown>()
      const requests = routeFetch({
        profile: n =>
          Promise.resolve(jsonResponse(n === 1 ? ANONYMOUS_PROFILE : SIGNED_IN_PROFILE)),
        // Its headers, and so the session cookie, are in; its body is not.
        register: () => Promise.resolve(headersThenBody(() => registerBody.promise)),
      })
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      const { container, queryClient } = renderPage()

      await submitPasswordSignup(user)
      await waitFor(() => expect(registerCall()).toBeDefined())
      await act(async () => {
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      })
      await waitFor(() => expect(container.querySelector('form')).toBeNull())
      return { registerBody, requests }
    }

    it('shows the interstitial once the registration lands, and keeps it past the deadline', async () => {
      const { registerBody, requests } = await registerBehindAnOutsideRefetch()

      await act(async () => {
        registerBody.resolve(REGISTERED)
      })
      await waitFor(() => expect(requests.profileRequests()).toBe(3))

      expect(
        await screen.findByRole('heading', { name: 'Check your inbox.' })
      ).toBeInTheDocument()
      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
      expect(screen.getByRole('heading', { name: 'Check your inbox.' })).toBeInTheDocument()
      expect(registerSignal().aborted).toBe(false)
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('releases the claim to the redirect at once when the registration then fails', async () => {
      const { registerBody } = await registerBehindAnOutsideRefetch()

      await act(async () => {
        registerBody.reject(new TypeError('network error'))
      })

      await waitFor(() => expect(mockPush).toHaveBeenCalledWith(RETURN_TO))
      expect(registerSignal().aborted).toBe(false)
      expect(screen.queryByRole('heading', { name: 'Check your inbox.' })).toBeNull()
    })
  })
})
