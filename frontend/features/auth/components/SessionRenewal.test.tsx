import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { waitFor } from '@testing-library/react'
import { HydrationBoundary, dehydrate } from '@tanstack/react-query'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { SESSION_CHANGE_MUTATION_KEY, type UserProfile } from '@/features/auth/hooks/useAuth'
import {
  SESSION_RENEWAL_POLL_MS,
  SESSION_RENEWAL_RETRY_MS,
  SessionRenewal,
  sessionRenewalPollInterval,
} from './SessionRenewal'

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

let pathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => pathname,
}))

const mockApiRequest = vi.mocked(apiRequest)

// Every payload carries a distinct request_id, as the backend's do: TanStack
// keeps the previous object for a deep-equal write, and the effect keys on
// the object.
let payloads = 0
function signedInProfile(overrides: Partial<UserProfile> = {}): UserProfile {
  payloads += 1
  return {
    success: true,
    message: 'Profile retrieved',
    request_id: `req-${payloads}`,
    user: {
      id: 1,
      email: 'viewer@example.com',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    ...overrides,
  }
}

function renderWithProfile(profile: UserProfile | undefined) {
  const queryClient = createTestQueryClient()
  if (profile) queryClient.setQueryData(queryKeys.auth.profile, profile)
  const result = renderWithProviders(<SessionRenewal />, { queryClient })
  return { ...result, queryClient }
}

const settle = () => new Promise(resolve => setTimeout(resolve, 20))

const renewalCalls = () =>
  mockApiRequest.mock.calls.filter(([url]) => url === API_ENDPOINTS.AUTH.REFRESH).length

describe('SessionRenewal', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    pathname = '/'
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('posts to /auth/refresh once when the seeded profile says the session is due', async () => {
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })
    const { queryClient, rerender } = renderWithProfile(
      signedInProfile({ session_renewal_due: true })
    )

    await waitFor(() => expect(renewalCalls()).toBe(1))
    expect(mockApiRequest).toHaveBeenCalledWith(
      API_ENDPOINTS.AUTH.REFRESH,
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )

    // Success marks the cached profile as no longer due, so a re-render asks nothing more.
    await waitFor(() =>
      expect(
        queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)?.session_renewal_due
      ).toBe(false)
    )
    rerender(<SessionRenewal />)
    expect(renewalCalls()).toBe(1)
  })

  it('reads the flag from a server-dehydrated seed, the cold-load path', async () => {
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })
    const server = createTestQueryClient()
    server.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    const state = dehydrate(server)

    renderWithProviders(
      <HydrationBoundary state={state}>
        <SessionRenewal />
      </HydrationBoundary>,
      { queryClient: createTestQueryClient() }
    )

    await waitFor(() => expect(renewalCalls()).toBe(1))
    expect(mockApiRequest).toHaveBeenCalledWith(
      API_ENDPOINTS.AUTH.REFRESH,
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('does nothing when the session is not due', async () => {
    renderWithProfile(signedInProfile())
    await settle()
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does nothing on an /auth page, whose own session change must win', async () => {
    pathname = '/auth/magic-link'
    renderWithProfile(signedInProfile({ session_renewal_due: true }))
    await settle()
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('holds while a session change is in flight', async () => {
    const queryClient = createTestQueryClient()
    // The test client collects unobserved queries at once; keep the seed
    // until the component observes it.
    queryClient.setQueryDefaults(queryKeys.auth.profile, { gcTime: Infinity })
    const seed = signedInProfile({ session_renewal_due: true })
    queryClient.setQueryData(queryKeys.auth.profile, seed)
    let finishChange!: () => void
    const change = queryClient
      .getMutationCache()
      .build(queryClient, {
        mutationKey: [...SESSION_CHANGE_MUTATION_KEY, 'logout'],
        mutationFn: () => new Promise<void>(resolve => (finishChange = resolve)),
      })
    void change.execute(undefined)
    await waitFor(() =>
      expect(queryClient.isMutating({ mutationKey: SESSION_CHANGE_MUTATION_KEY })).toBe(1)
    )
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })

    renderWithProviders(<SessionRenewal />, { queryClient })
    await settle()
    expect(queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)).toBe(seed)
    expect(renewalCalls()).toBe(0)

    // Once the change settles, the next profile read may renew.
    finishChange()
    await waitFor(() =>
      expect(queryClient.isMutating({ mutationKey: SESSION_CHANGE_MUTATION_KEY })).toBe(0)
    )
    queryClient.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    await waitFor(() => expect(renewalCalls()).toBe(1))
  })

  it('leaves the signed-in profile untouched when the renewal fails', async () => {
    mockApiRequest.mockRejectedValue(new Error('network down'))
    const profile = signedInProfile({ session_renewal_due: true })
    const { queryClient } = renderWithProfile(profile)

    await waitFor(() => expect(renewalCalls()).toBe(1))
    await settle()
    expect(queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)).toEqual(profile)
  })

  it('holds inside the retry window, then asks again on a later profile read', async () => {
    const start = 1_700_000_000_000
    const now = vi.spyOn(Date, 'now').mockReturnValue(start)
    mockApiRequest.mockRejectedValue(new Error('network down'))
    const { queryClient } = renderWithProfile(signedInProfile({ session_renewal_due: true }))
    await waitFor(() => expect(renewalCalls()).toBe(1))

    // A fresh profile object inside the window re-runs the effect; the backoff holds it.
    now.mockReturnValue(start + SESSION_RENEWAL_RETRY_MS - 1000)
    queryClient.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    await settle()
    expect(renewalCalls()).toBe(1)

    // Past the window, the next profile read that still says due asks again.
    now.mockReturnValue(start + SESSION_RENEWAL_RETRY_MS + 1000)
    queryClient.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    await waitFor(() => expect(renewalCalls()).toBe(2))
  })

  it('asks again when the flag clears and comes back a day later', async () => {
    const start = 1_700_000_000_000
    const now = vi.spyOn(Date, 'now').mockReturnValue(start)
    mockApiRequest.mockResolvedValue({ success: true, message: 'Token refreshed' })
    const { queryClient } = renderWithProfile(signedInProfile({ session_renewal_due: true }))
    await waitFor(() => expect(renewalCalls()).toBe(1))
    await waitFor(() =>
      expect(
        queryClient.getQueryData<UserProfile>(queryKeys.auth.profile)?.session_renewal_due
      ).toBe(false)
    )

    now.mockReturnValue(start + 24 * 60 * 60 * 1000)
    queryClient.setQueryData(queryKeys.auth.profile, signedInProfile({ session_renewal_due: true }))
    await waitFor(() => expect(renewalCalls()).toBe(2))
  })

  it('polls the profile only for a signed-in viewer', () => {
    expect(sessionRenewalPollInterval(signedInProfile())).toBe(SESSION_RENEWAL_POLL_MS)
    expect(sessionRenewalPollInterval(undefined)).toBe(false)
    expect(
      sessionRenewalPollInterval({ success: false, message: 'Authentication required' })
    ).toBe(false)
  })
})
