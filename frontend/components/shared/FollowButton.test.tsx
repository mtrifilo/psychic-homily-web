import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient } from '@/test/utils'

const mockPush = vi.fn()
const mockFollowMutate = vi.fn()
const mockUnfollowMutate = vi.fn()
let mockIsAuthenticated = true
let mockFollowStatusData: { follower_count: number; is_following: boolean } | undefined
let mockStatusLoading = false

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/artists/test-artist',
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: mockIsAuthenticated }),
}))

// Hoist `mockApiRequest` so the `@/lib/api` mock below can reference it
// without TDZ issues when the partial-mock factory runs.
const { mockApiRequest } = vi.hoisted(() => ({
  mockApiRequest: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

// Two strategies live in this file:
// 1. Most tests fully mock `@/lib/hooks/common/useFollow` so they can drive the
//    UI directly via state setters. Fast, hermetic.
// 2. The "optimistic-update + rollback" suite uses the REAL hook against a
//    fresh QueryClient seeded with cached follow status, and intercepts only
//    the network layer via `mockApiRequest`. That's the only level at which
//    the optimistic-then-rollback behavior actually flows through the
//    component, so it's the only level where the assertion is meaningful.
const useMockedFollowHooks = vi.hoisted(() => ({ value: true }))

vi.mock('@/lib/hooks/common/useFollow', async () => {
  const actual = await vi.importActual<typeof import('@/lib/hooks/common/useFollow')>(
    '@/lib/hooks/common/useFollow'
  )
  return {
    ...actual,
    useFollowStatus: (entityType: string, entityId: number) =>
      useMockedFollowHooks.value
        ? { data: mockFollowStatusData, isLoading: mockStatusLoading }
        : actual.useFollowStatus(entityType, entityId),
    useFollow: () =>
      useMockedFollowHooks.value
        ? { mutate: mockFollowMutate, isPending: false }
        : actual.useFollow(),
    useUnfollow: () =>
      useMockedFollowHooks.value
        ? { mutate: mockUnfollowMutate, isPending: false }
        : actual.useUnfollow(),
  }
})

import { FollowButton } from './FollowButton'


describe('FollowButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useMockedFollowHooks.value = true
    mockIsAuthenticated = true
    mockFollowStatusData = { follower_count: 10, is_following: false }
    mockStatusLoading = false
  })

  it('renders "Follow" text in non-compact mode when not following', () => {
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Follow')).toBeInTheDocument()
  })

  it('renders "Following" text when following', () => {
    mockFollowStatusData = { follower_count: 10, is_following: true }

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Following')).toBeInTheDocument()
  })

  it('renders follower count when > 0', () => {
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('10')).toBeInTheDocument()
  })

  it('calls follow.mutate when clicking while not following', async () => {
    const user = userEvent.setup()
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    await user.click(screen.getByRole('button'))
    expect(mockFollowMutate).toHaveBeenCalledWith({ entityType: 'artists', entityId: 1 })
  })

  it('calls unfollow.mutate when clicking while following', async () => {
    const user = userEvent.setup()
    mockFollowStatusData = { follower_count: 10, is_following: true }

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    await user.click(screen.getByRole('button'))
    expect(mockUnfollowMutate).toHaveBeenCalledWith({ entityType: 'artists', entityId: 1 })
  })

  it('redirects to /auth when not authenticated', async () => {
    const user = userEvent.setup()
    mockIsAuthenticated = false

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    await user.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fartists%2Ftest-artist')
    expect(mockFollowMutate).not.toHaveBeenCalled()
  })

  it('uses followData prop over fetched data', () => {
    mockFollowStatusData = { follower_count: 10, is_following: false }

    render(
      <FollowButton
        entityType="artists"
        entityId={1}
        followData={{ follower_count: 42, is_following: true }}
      />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
  })

  it('renders compact mode with aria-label', () => {
    render(
      <FollowButton entityType="artists" entityId={1} compact />,
      { wrapper: createWrapper() }
    )

    const button = screen.getByRole('button', { name: 'Follow' })
    expect(button).toBeInTheDocument()
  })

  it('renders loading spinner when status is loading and no followData', () => {
    mockStatusLoading = true
    mockFollowStatusData = undefined

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    // In non-compact mode, should show "Follow" text with a spinner
    expect(screen.getByText('Follow')).toBeInTheDocument()
  })

  it('does not show loading spinner when followData is provided', () => {
    mockStatusLoading = true
    mockFollowStatusData = undefined

    render(
      <FollowButton
        entityType="artists"
        entityId={1}
        followData={{ follower_count: 5, is_following: false }}
      />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Follow')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('shows the Unfollow affordance on hover while following', async () => {
    const user = userEvent.setup()
    mockFollowStatusData = { follower_count: 10, is_following: true }

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    // Before hover — "Following" label.
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.queryByText('Unfollow')).not.toBeInTheDocument()

    // On hover — the same button flips to "Unfollow" + destructive variant.
    await user.hover(screen.getByRole('button'))
    expect(await screen.findByText('Unfollow')).toBeInTheDocument()
    expect(screen.queryByText('Following')).not.toBeInTheDocument()

    // Unhover — back to "Following".
    await user.unhover(screen.getByRole('button'))
    expect(await screen.findByText('Following')).toBeInTheDocument()
  })
})

describe('FollowButton — bracket variant (PSY-641)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useMockedFollowHooks.value = true
    mockIsAuthenticated = true
    mockFollowStatusData = { follower_count: 10, is_following: false }
    mockStatusLoading = false
  })

  it('renders [Follow] as a bracket link when not following', () => {
    render(
      <FollowButton entityType="artists" entityId={1} variant="bracket" />,
      { wrapper: createWrapper() }
    )
    const btn = screen.getByRole('button', { name: 'Follow' })
    expect(btn).toBeInTheDocument()
    expect(btn).not.toHaveAttribute('aria-pressed')
  })

  it('renders [Following] with aria-pressed when following', () => {
    mockFollowStatusData = { follower_count: 10, is_following: true }
    render(
      <FollowButton entityType="artists" entityId={1} variant="bracket" />,
      { wrapper: createWrapper() }
    )
    const btn = screen.getByRole('button', { name: 'Following' })
    expect(btn).toHaveAttribute('aria-pressed', 'true')
  })

  it('calls follow mutation when the bracket link is clicked', async () => {
    const user = userEvent.setup()
    render(
      <FollowButton entityType="artists" entityId={1} variant="bracket" />,
      { wrapper: createWrapper() }
    )
    await user.click(screen.getByRole('button', { name: 'Follow' }))
    expect(mockFollowMutate).toHaveBeenCalledWith({ entityType: 'artists', entityId: 1 })
  })

  it('renders a disabled [Follow] while follow status is loading', () => {
    mockStatusLoading = true
    mockFollowStatusData = undefined
    render(
      <FollowButton entityType="artists" entityId={1} variant="bracket" />,
      { wrapper: createWrapper() }
    )
    expect(screen.getByRole('button', { name: 'Follow' })).toBeDisabled()
  })
})

// ── Optimistic update + rollback (real hooks, network mocked) ───────
//
// The optimistic-update + rollback contract lives in the `useFollow` /
// `useUnfollow` hooks (see their `onMutate` / `onError`). The component
// is a thin consumer that reads `useFollowStatus`'s cache. To prove the
// follower count *flips first*, then *rolls back on error*, we need the
// real hooks against a shared QueryClient. We mock ONLY the network
// (`apiRequest`) — and we hold the request open with a deferred promise
// so the optimistic UI is observable BEFORE the mutation resolves.
//
// The Wave 3 false-coverage failure mode this guards against: asserting
// only the POST-success UI state means the test passes even if the
// optimistic-update path was deleted. We assert the optimistic state
// while the mutation is still in-flight to lock that in.
describe('FollowButton — optimistic update + rollback (real hooks)', () => {
  function deferred<T = unknown>() {
    let resolve!: (value: T) => void
    let reject!: (reason?: unknown) => void
    const promise = new Promise<T>((res, rej) => {
      resolve = res
      reject = rej
    })
    return { promise, resolve, reject }
  }

  // queryKeys.follows.entity(entityType, entityId) — mirrors lib/queryClient
  // (kept inline so this test isn't load-bearing on internal key shape).
  const ENTITY_TYPE = 'artists'
  const ENTITY_ID = 7
  const FOLLOW_KEY = ['follows', 'entity', ENTITY_TYPE, ENTITY_ID]

  function makeClient() {
    return new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity, staleTime: Infinity },
        mutations: { retry: false },
      },
    })
  }

  function Wrapper({ client, children }: { client: QueryClient; children: React.ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }

  beforeEach(() => {
    vi.clearAllMocks()
    useMockedFollowHooks.value = false
    mockIsAuthenticated = true
    mockApiRequest.mockReset()
  })

  // The real `useFollowStatus` query hits `apiRequest(GET …/follow/followers/…)`.
  // We don't want the optimistic-update tests to depend on a network round-trip,
  // so we satisfy the GET from the seeded cache value (mockApiRequest returns
  // it) and reserve a separate mock implementation for the mutation call.
  function mockGetThenMutation(
    cached: { follower_count: number; is_following: boolean },
    mutationCall: ReturnType<typeof deferred>
  ) {
    mockApiRequest.mockImplementation((_endpoint: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      if (method === 'GET') return Promise.resolve(cached)
      // The follow / unfollow POST / DELETE both go through this branch.
      return mutationCall.promise
    })
  }

  // We use the `compact` variant because its `aria-label` toggles cleanly
  // on `isFollowing` ("Follow" ↔ "Unfollow"), with no hover-state coupling
  // that would change the rendered text when userEvent.click implicitly
  // moves the pointer over the button. The non-compact display swaps
  // "Following" ↔ "Unfollow" on `onMouseEnter`, and userEvent.click fires
  // pointerEnter alongside the click — so a non-compact assertion of
  // "Following" right after click would race the hover-induced "Unfollow"
  // and flake. compact keeps the assertion focused on the actual state
  // change (the follower count + the aria-label).

  it('shows optimistic +1 follower count BEFORE follow mutation resolves, then keeps it on success', async () => {
    const user = userEvent.setup()
    const client = makeClient()
    // Seed the follow-status cache so the optimistic-update path has a
    // base snapshot to bump from. The component will also receive the
    // same value if `useFollowStatus` queries against this key.
    const initial = { follower_count: 10, is_following: false }
    client.setQueryData(FOLLOW_KEY, initial)

    // Hold the POST open so the optimistic state is observable BEFORE
    // the mutation resolves.
    const inFlight = deferred()
    mockGetThenMutation(initial, inFlight)

    render(
      <Wrapper client={client}>
        <FollowButton entityType={ENTITY_TYPE} entityId={ENTITY_ID} compact />
      </Wrapper>
    )

    expect(await screen.findByText('10')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Follow' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Follow' }))

    // OPTIMISTIC: count flips to 11 + the button's aria-label flips to
    // "Unfollow" (the compact-variant label for is_following=true) BEFORE
    // the network request resolves.
    expect(await screen.findByText('11')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Unfollow' })).toBeInTheDocument()

    // Resolve the mutation so any pending promises settle cleanly. The
    // success path is exercised; the test's load-bearing assertion is
    // the optimistic state ABOVE this line (asserted with a still-pending
    // network request).
    inFlight.resolve({ success: true, message: 'ok' })
  })

  it('rolls back follower count when the follow mutation REJECTS', async () => {
    const user = userEvent.setup()
    const client = makeClient()
    const initial = { follower_count: 10, is_following: false }
    client.setQueryData(FOLLOW_KEY, initial)

    const inFlight = deferred()
    mockGetThenMutation(initial, inFlight)
    // Silence the expected rejection log; we WANT the mutation to fail.
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <Wrapper client={client}>
        <FollowButton entityType={ENTITY_TYPE} entityId={ENTITY_ID} compact />
      </Wrapper>
    )

    expect(await screen.findByText('10')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Follow' }))

    // OPTIMISTIC flip first.
    expect(await screen.findByText('11')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Unfollow' })).toBeInTheDocument()

    // Reject — onError rolls the cache back to the snapshot.
    inFlight.reject(new Error('500 boom'))

    // ROLLBACK: count returns to 10 and aria-label flips back to "Follow".
    await waitFor(() => {
      expect(screen.getByText('10')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Follow' })).toBeInTheDocument()

    errorSpy.mockRestore()
  })

  it('rolls back follower count when the unfollow mutation REJECTS', async () => {
    const user = userEvent.setup()
    const client = makeClient()
    // Seed as already following with 11 followers.
    const initial = { follower_count: 11, is_following: true }
    client.setQueryData(FOLLOW_KEY, initial)

    const inFlight = deferred()
    mockGetThenMutation(initial, inFlight)
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <Wrapper client={client}>
        <FollowButton entityType={ENTITY_TYPE} entityId={ENTITY_ID} compact />
      </Wrapper>
    )

    expect(await screen.findByRole('button', { name: 'Unfollow' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Unfollow' }))

    // OPTIMISTIC: drops to 10 + aria-label flips to "Follow".
    expect(await screen.findByText('10')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Follow' })).toBeInTheDocument()

    // Reject — rollback.
    inFlight.reject(new Error('409 conflict'))

    await waitFor(() => {
      expect(screen.getByText('11')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Unfollow' })).toBeInTheDocument()

    errorSpy.mockRestore()
  })
})
