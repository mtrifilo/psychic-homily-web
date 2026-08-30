import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotifyMeButton } from './NotifyMeButton'

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/artists/test-artist',
}))

// Mock AuthContext
const mockAuthContext = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Single source of truth for the mocked auth state, mirroring the real
// AuthContext's invariant that `isAuthenticated` is DERIVED from `authStatus`.
// Setting the two independently would let a test assert against a viewer that
// cannot exist; driving both from one value still leaves 'pending' reachable,
// which is the cell where a signed-in viewer reads isAuthenticated=false.
function setAuth(authStatus: 'pending' | 'authenticated' | 'anonymous') {
  mockAuthContext.mockReturnValue({
    authStatus,
    isAuthenticated: authStatus === 'authenticated',
    user: authStatus === 'authenticated' ? { id: '1' } : null,
  })
}

// Mock notification hooks
const mockQuickCreate = vi.fn()
const mockDeleteFilter = vi.fn()
const mockFilterCheck = vi.fn()

vi.mock('../hooks', () => ({
  useNotificationFilterCheck: (...args: unknown[]) => mockFilterCheck(...args),
  useQuickCreateFilter: () => mockQuickCreate(),
  useDeleteFilter: () => mockDeleteFilter(),
}))

describe('NotifyMeButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setAuth('authenticated')
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
      isSuccess: true,
    })
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockDeleteFilter.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders "Notify me" for authenticated user without filter', () => {
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('renders "Notifications on" when user has a matching filter', () => {
    mockFilterCheck.mockReturnValue({
      data: { id: 1, name: 'Filter' },
      hasFilter: true,
      isLoading: false,
      isSuccess: true,
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.getByText('Notifications on')).toBeInTheDocument()
  })

  it('redirects to auth when unauthenticated', async () => {
    setAuth('anonymous')
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    await user.click(screen.getByText('Notify me'))
    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fartists%2Ftest-artist')
  })

  it('calls quickCreate.mutate when clicking notify without filter', async () => {
    const mutateFn = vi.fn()
    mockQuickCreate.mockReturnValue({
      mutate: mutateFn,
      isPending: false,
      isError: false,
      error: null,
    })
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={42}
        entityName="Test Artist"
      />
    )
    await user.click(screen.getByText('Notify me'))
    expect(mutateFn).toHaveBeenCalledWith({ entityType: 'artist', entityId: 42 })
  })

  it('displays error message when quick-create mutation fails', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Network error'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Network error')
  })

  it('displays error message when delete mutation fails', () => {
    mockFilterCheck.mockReturnValue({
      data: { id: 1, name: 'Filter' },
      hasFilter: true,
      isLoading: false,
      isSuccess: true,
    })
    mockDeleteFilter.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Failed to remove notification'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Failed to remove notification')
  })

  it('displays fallback error message when error has no message', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error(''),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Failed to update notification. Please try again.')
  })

  it('displays error in compact mode when mutation fails', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Server error'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        compact
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Server error')
  })

  it('does not display error when no mutation has failed', () => {
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  // ── Unsettled auth (PSY-1972)
  //
  // The anonymous branch is a bare router push to /auth, and `!isAuthenticated`
  // reads true both for a viewer with no session and for one whose profile has
  // not arrived. Cells:
  //
  //   authStatus     button variant                bracket variant
  //   pending        disabled, no click            disabled
  //   anonymous      enabled, click -> /auth       enabled, click -> /auth
  //   authenticated  normal                        normal

  it.each([[false], [true]])(
    'ships disabled while auth is unsettled (compact=%s)',
    (compact) => {
      setAuth('pending')
      render(
        <NotifyMeButton
          entityType="artist"
          entityId={1}
          entityName="Test Artist"
          compact={compact}
        />
      )
      expect(screen.getByRole('button')).toBeDisabled()
    }
  )

  // No test covers `handleClick`'s own `isUnsettled` bail: it is unreachable
  // while the control renders disabled. React reads `props.disabled` off the
  // fiber before dispatching onClick, so stripping the DOM attribute does not
  // reach it either, and `consumePendingReplay` refuses a disabled target as
  // well. It is defence in depth against a future edit that drops one of the
  // disabled branches, and no single-file mutation can fail on it.
})

describe('NotifyMeButton — bracket variant (PSY-641)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setAuth('authenticated')
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
      isSuccess: true,
    })
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockDeleteFilter.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders [Notify me] as a bracket link when no filter exists', () => {
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    const btn = screen.getByRole('button', { name: 'Notify me' })
    expect(btn).toBeInTheDocument()
    expect(btn).not.toHaveAttribute('aria-pressed')
  })

  it('renders [Notifications on] with aria-pressed when a filter exists', () => {
    mockFilterCheck.mockReturnValue({
      data: { id: 7 },
      hasFilter: true,
      isLoading: false,
      isSuccess: true,
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    expect(
      screen.getByRole('button', { name: 'Notifications on' })
    ).toHaveAttribute('aria-pressed', 'true')
  })

  it('creates a filter when the bracket link is clicked', async () => {
    const mutate = vi.fn()
    mockQuickCreate.mockReturnValue({
      mutate,
      isPending: false,
      isError: false,
      error: null,
    })
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    await user.click(screen.getByRole('button', { name: 'Notify me' }))
    expect(mutate).toHaveBeenCalledWith({ entityType: 'artist', entityId: 1 })
  })

  // Unsettled auth (PSY-1972). BracketLink ships this control enabled in server
  // HTML and opts it into pre-hydration click replay, so `disabled` (which also
  // sets pointer-events-none) is what keeps a replayed click from routing a
  // signed-in viewer to /auth.
  it('ships the bracket disabled while auth is unsettled', () => {
    setAuth('pending')
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    expect(screen.getByRole('button', { name: 'Notify me' })).toBeDisabled()
  })

  it('ships the bracket enabled for a settled-anonymous viewer', async () => {
    setAuth('anonymous')
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    const bracket = screen.getByRole('button', { name: 'Notify me' })
    expect(bracket).toBeEnabled()
    await user.click(bracket)
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fartists%2Ftest-artist'
    )
  })
})
