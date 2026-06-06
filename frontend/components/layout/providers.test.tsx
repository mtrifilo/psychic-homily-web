import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Providers } from './providers'

// Mock the auth feature hooks consumed by AuthProvider. AuthProvider runs
// `useProfile()` (TanStack Query) + `useLogout()` (mutation) on mount; both
// would hit the network without a mock.
const mockUseProfile = vi.fn(() => ({
  data: undefined,
  isLoading: false,
  error: null,
}))
const mockUseLogout = vi.fn(() => ({
  mutateAsync: vi.fn(),
  isPending: false,
}))

vi.mock('@/features/auth', () => ({
  useProfile: () => mockUseProfile(),
  useLogout: () => mockUseLogout(),
}))

// PSY-961: Providers now mounts CreateCollectionDrawerProvider, which reads
// useRouter() — stub it so the provider tree mounts without a Next router.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Providers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseProfile.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    })
    mockUseLogout.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
    })
  })

  it('renders children through the provider tree', () => {
    render(
      <Providers>
        <div>child-content</div>
      </Providers>
    )
    expect(screen.getByText('child-content')).toBeInTheDocument()
  })

  it('does not crash when wrapping multiple children', () => {
    render(
      <Providers>
        <div>first</div>
        <div>second</div>
        <span>third</span>
      </Providers>
    )
    expect(screen.getByText('first')).toBeInTheDocument()
    expect(screen.getByText('second')).toBeInTheDocument()
    expect(screen.getByText('third')).toBeInTheDocument()
  })

  it('makes useAuthContext usable inside subtree (AuthProvider mounted)', async () => {
    // If AuthProvider isn't mounted, useAuthContext throws
    // "must be used within an AuthProvider". Render a consumer to prove the
    // provider is wired.
    const { useAuthContext } = await import('@/lib/context/AuthContext')

    function Consumer() {
      const ctx = useAuthContext()
      return <div data-testid="auth-state">{String(ctx.isAuthenticated)}</div>
    }

    render(
      <Providers>
        <Consumer />
      </Providers>
    )
    // Profile mock returns undefined → user is null → isAuthenticated false.
    expect(screen.getByTestId('auth-state')).toHaveTextContent('false')
  })

  it('makes TanStack Query useable inside subtree (QueryClientProvider mounted)', async () => {
    // Calling useQueryClient without a provider throws — proves the
    // provider is rendered.
    const { useQueryClient } = await import('@tanstack/react-query')

    function Consumer() {
      const qc = useQueryClient()
      return <div data-testid="qc">{qc ? 'ok' : 'missing'}</div>
    }

    render(
      <Providers>
        <Consumer />
      </Providers>
    )
    expect(screen.getByTestId('qc')).toHaveTextContent('ok')
  })

  it('reuses the same QueryClient across re-renders (singleton in browser)', async () => {
    const { useQueryClient } = await import('@tanstack/react-query')
    const seen: unknown[] = []

    function Consumer() {
      const qc = useQueryClient()
      seen.push(qc)
      return null
    }

    const { rerender } = render(
      <Providers>
        <Consumer />
      </Providers>
    )
    rerender(
      <Providers>
        <Consumer />
      </Providers>
    )

    // Same QueryClient instance on each render — confirms getQueryClient's
    // browser-singleton behavior is preserved by Providers.
    expect(seen.length).toBeGreaterThanOrEqual(2)
    expect(seen[0]).toBe(seen[seen.length - 1])
  })

  it('renders without crashing when children is null', () => {
    // Guard against accidental requirement that children be non-null —
    // some layout wrappers may render null children during loading states.
    const { container } = render(<Providers>{null}</Providers>)
    expect(container).toBeTruthy()
  })
})
