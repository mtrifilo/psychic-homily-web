import { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AddToCollectionButton } from './AddToCollectionButton'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  user: { id: '1' },
  isAuthenticated: true,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock collection hooks
const mockAddMutate = vi.fn()
const mockMyCollections = vi.fn(() => ({
  data: {
    collections: [
      { id: 1, slug: 'my-favorites', title: 'My Favorites' },
      { id: 2, slug: 'best-of-2026', title: 'Best of 2026' },
    ],
  },
  isLoading: false,
}))

vi.mock('@/features/collections/hooks', () => ({
  useMyCollections: () => mockMyCollections(),
  useAddCollectionItem: () => ({
    mutate: mockAddMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

describe('AddToCollectionButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders nothing when not authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    const { container } = render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders a button with "Collect" text when authenticated', () => {
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )
    expect(screen.getByRole('button', { name: /add to collection/i })).toBeInTheDocument()
    expect(screen.getByText('Collect')).toBeInTheDocument()
  })

  it('opens popover with collections list when clicked', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(screen.getByText('Add to Collection')).toBeInTheDocument()
    expect(screen.getByText('My Favorites')).toBeInTheDocument()
    expect(screen.getByText('Best of 2026')).toBeInTheDocument()
  })

  it('shows entity name in popover header', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="venue" entityId={5} entityName="The Rebel Lounge" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(screen.getByText('The Rebel Lounge')).toBeInTheDocument()
  })

  it('calls addMutation when collection is clicked', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={42} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(screen.getByText('My Favorites'))

    expect(mockAddMutate).toHaveBeenCalledWith(
      { slug: 'my-favorites', entityType: 'artist', entityId: 42 },
      expect.any(Object)
    )
  })

  it('shows "Create new collection" link', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const link = screen.getByText('Create new collection')
    expect(link).toBeInTheDocument()
    expect(link.closest('a')).toHaveAttribute('href', '/collections')
  })

  it('shows empty state when user has no collections', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: [] },
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(screen.getByText('No collections yet')).toBeInTheDocument()
  })

  // ── Regression: unauthenticated → authenticated transition (PSY-466) ──
  // Rules of Hooks violation: earlier versions called `useState` for
  // `recentlyAdded` below the `if (!isAuthenticated) return null` early
  // return. On first render the auth profile hasn't resolved yet
  // (isAuthenticated=false) so the component took the early return with N
  // hooks called. Once /auth/profile resolved and isAuthenticated flipped
  // to true, the component proceeded past the early return and called one
  // additional hook — React flagged it as "Rendered more hooks than during
  // the previous render" and the error boundary rendered 500 for every
  // entity detail page where this button is rendered (artists, shows,
  // venues, releases, labels, festivals).
  //
  // The other tests all pass with the broken code because the mocked
  // `useAuthContext` returns the same auth state synchronously on every
  // render — the transition that triggered the violation never happens.
  // This regression test makes the mock call a real React hook (`useState`)
  // so React's hook-tracker has a stable hook anchor to compare against,
  // making the component-body hook-count transition detectable.
  it('renders without hook-order errors during the auth loading → authenticated transition', () => {
    // Force the mock to call a real React hook so React's hook-tracker
    // treats the auth hook as a stable slot and can actually see the
    // component body's hook-count change. Without this, the mock calls
    // zero hooks and React has nothing to anchor the comparison.
    let authState: {
      user: { id: string } | null
      isAuthenticated: boolean
      isLoading: boolean
      logout: ReturnType<typeof vi.fn>
    } = {
      user: null,
      isAuthenticated: false,
      isLoading: true,
      logout: vi.fn(),
    }
    mockAuthContext.mockImplementation(() => {
      // Real React hook — ensures this mock contributes a stable number
      // of hooks across renders so the component-body transition is
      // what React's hook-tracker actually sees.
      useState(0)
      return authState
    })

    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    // Initial render: auth profile still loading, user is null, so the
    // component hits the `if (!isAuthenticated) return null` early return.
    const { rerender } = render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    // Transition to authenticated — this is what triggered the
    // hook-order violation in production once /auth/profile resolved.
    authState = {
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    }

    let threwDuringRerender: Error | null = null
    try {
      rerender(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )
    } catch (e) {
      threwDuringRerender = e as Error
    }

    // A hook-order violation throws during render with a message like
    // "Rendered more hooks than during the previous render." or "change
    // in the order of Hooks". React also logs a dev-only console.error
    // about it before throwing.
    const allErrorOutput = [
      ...(threwDuringRerender ? [threwDuringRerender.message] : []),
      ...errorSpy.mock.calls.map(([msg]) =>
        typeof msg === 'string' ? msg : ''
      ),
    ]
    const hookErrors = allErrorOutput.filter(
      (msg) =>
        msg.includes('change in the order of Hooks') ||
        msg.includes('Rendered more hooks than during the previous render') ||
        msg.includes('Rendered fewer hooks than expected')
    )
    expect(hookErrors).toEqual([])
    expect(threwDuringRerender).toBeNull()

    // Sanity check: once authenticated, the button actually renders.
    expect(
      screen.getByRole('button', { name: /add to collection/i })
    ).toBeInTheDocument()

    errorSpy.mockRestore()
  })
})
