import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReleaseSaveButton } from './ReleaseSaveButton'

const mockToggle = vi.fn()
const mockPush = vi.fn()
type MockAuthStatus = 'pending' | 'authenticated' | 'anonymous'
// Single source of truth for the mocked auth state, mirroring the real
// AuthContext's invariant that `isAuthenticated` is DERIVED from `authStatus`.
// Setting the two independently would let a test assert against a viewer that
// cannot exist; driving both from one value still leaves 'pending' reachable,
// which is the cell where a signed-in viewer reads isAuthenticated=false.
const authState = (authStatus: MockAuthStatus) => ({
  authStatus,
  isAuthenticated: authStatus === 'authenticated',
  user: authStatus === 'authenticated' ? { id: 42 } : null,
})
const mockUseAuthContext = vi.fn(() => authState('authenticated'))
const mockUseReleaseSaveCount = vi.fn<
  (...args: unknown[]) => { data: undefined; isLoading: boolean }
>(() => ({ data: undefined, isLoading: false }))
const mockUseReleaseSaveToggle = vi.fn<
  (...args: unknown[]) => {
    toggle: typeof mockToggle
    isLoading: boolean
    error: Error | null
  }
>(() => ({ toggle: mockToggle, isLoading: false, error: null }))

vi.mock('next/navigation', () => ({
  usePathname: () => '/releases/the-record',
  useRouter: () => ({ push: mockPush }),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))
vi.mock('@/features/releases', () => ({
  useReleaseSaveCount: (...args: unknown[]) => mockUseReleaseSaveCount(...args),
  useReleaseSaveToggle: (...args: unknown[]) =>
    mockUseReleaseSaveToggle(...args),
}))

describe('ReleaseSaveButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/releases/the-record')
    mockUseAuthContext.mockReturnValue(authState('authenticated'))
    mockUseReleaseSaveCount.mockReturnValue({
      data: undefined,
      isLoading: false,
    })
    mockUseReleaseSaveToggle.mockReturnValue({
      toggle: mockToggle,
      isLoading: false,
      error: null,
    })
  })

  it('renders the approved bracket action and toggles a release', async () => {
    const user = userEvent.setup()
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
        variant="bracket"
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save release' }))
    expect(mockUseReleaseSaveToggle).toHaveBeenCalledWith(17, false, 42)
    expect(mockToggle).toHaveBeenCalledOnce()
  })

  it('renders Saved for an existing save', () => {
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: true }}
        variant="bracket"
      />
    )
    expect(
      screen.getByRole('button', { name: 'Remove saved release' })
    ).toHaveTextContent('[Saved]')
  })

  it('supports the dense Library text action and accessible release name', () => {
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: true }}
        variant="text"
        actionLabel="✕ remove"
        actionAriaLabel="Remove Clarity from saved releases"
      />
    )

    expect(
      screen.getByRole('button', {
        name: 'Remove Clarity from saved releases',
      })
    ).toHaveTextContent('✕ remove')
  })

  it('sends anonymous users to auth with the current release as returnTo', async () => {
    const user = userEvent.setup()
    mockUseAuthContext.mockReturnValue(authState('anonymous'))
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
      />
    )

    await user.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Freleases%2Fthe-record'
    )
    expect(mockToggle).not.toHaveBeenCalled()
  })

  it('preserves active query state in the sign-in return path', async () => {
    const user = userEvent.setup()
    window.history.replaceState({}, '', '/releases/the-record?window=all_time')
    mockUseAuthContext.mockReturnValue(authState('anonymous'))
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
      />
    )

    await user.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Freleases%2Fthe-record%3Fwindow%3Dall_time'
    )
  })

  // ── Unsettled auth (PSY-1972)
  //
  // Every variant ships ENABLED in server HTML and opts into pre-hydration
  // click replay, and `handleClick` routes on `!isAuthenticated`, which reads
  // false both for a viewer with no session and for one whose profile has not
  // arrived. Cells:
  //
  //   authStatus     save-count request   render
  //   pending        skipped              disabled
  //   anonymous      issued (count public) enabled, click -> /auth
  //   authenticated  issued               enabled, click -> toggle

  it.each([['bracket'], ['text'], ['button']] as const)(
    'ships the %s variant disabled while auth is unsettled',
    (variant) => {
      mockUseAuthContext.mockReturnValue(authState('pending'))
      render(
        <ReleaseSaveButton
          releaseId={17}
          saveData={{ save_count: 4, is_saved: false }}
          variant={variant}
        />
      )
      expect(screen.getByRole('button')).toBeDisabled()
    }
  )

  // The handler's own pending bail is not covered: React reads `props.disabled`
  // off the fiber before dispatching onClick, and `consumePendingReplay` refuses
  // a disabled target, so nothing can reach it while the control renders
  // disabled. It is defence in depth, and no single-file mutation fails on it.

  // The unsettled-window gate is inside `useReleaseSaveCount`, beside the key
  // it protects — its cells live in
  // features/releases/hooks/useSavedReleases.test.tsx. What belongs here is
  // that this component hands the hook the viewer it actually has, since the
  // key is built from those arguments.
  it('passes the settled viewer through to the save-count hook', () => {
    render(<ReleaseSaveButton releaseId={17} />)
    expect(mockUseReleaseSaveCount).toHaveBeenCalledWith(17, true, true, 42)
  })

  it('passes an anonymous viewer through to the save-count hook', () => {
    mockUseAuthContext.mockReturnValue(authState('anonymous'))
    render(<ReleaseSaveButton releaseId={17} />)
    expect(mockUseReleaseSaveCount).toHaveBeenCalledWith(17, false, true, undefined)
  })

  it('suppresses the self-fetch while a batch owns this row', () => {
    render(<ReleaseSaveButton releaseId={17} saveData="pending" />)
    expect(mockUseReleaseSaveCount).toHaveBeenCalledWith(17, true, false, 42)
  })

  // The error auto-hide used to be an untracked `setTimeout`, so it still fired
  // ~3s after the button unmounted and called `setState` into a torn-down React
  // DOM. Under vitest that lands after jsdom teardown and throws
  // `ReferenceError: window is not defined`, failing the whole run with every
  // test passing. No timer may outlive the component.
  it('leaves no pending error timer behind on unmount', async () => {
    vi.useFakeTimers()
    try {
      const toggleErr = new Error('Server is down')
      mockUseReleaseSaveToggle.mockReturnValue({
        toggle: vi.fn(async () => {
          throw toggleErr
        }),
        isLoading: false,
        error: toggleErr,
      })

      const { unmount } = render(
        <ReleaseSaveButton
          releaseId={17}
          saveData={{ save_count: 4, is_saved: false }}
        />
      )
      // The catch that arms the timer runs in a promise continuation, so let
      // the microtask queue drain before asserting the timer exists.
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /save release/i }))
      })
      expect(vi.getTimerCount()).toBeGreaterThan(0)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })
})
