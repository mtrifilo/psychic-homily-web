import { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AddToCollectionButton } from './AddToCollectionButton'

// Mock AuthContext
interface MockAuthState {
  user: { id: string } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: ReturnType<typeof vi.fn>
}
const mockAuthContext = vi.fn<() => MockAuthState>(() => ({
  user: { id: '1' },
  isAuthenticated: true,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock collection hooks
const mockMutateAsync = vi.fn()
const mockMyCollections = vi.fn(() => ({
  data: {
    collections: [
      { id: 1, slug: 'my-favorites', title: 'My Favorites' },
      { id: 2, slug: 'best-of-2026', title: 'Best of 2026' },
      { id: 3, slug: 'arizona-shows', title: 'Arizona Shows' },
    ],
  },
  isLoading: false,
}))
const mockContainingIds = vi.fn(() => ({
  data: new Set<number>(),
  isLoading: false,
}))

vi.mock('@/features/collections/hooks', () => ({
  useMyCollections: () => mockMyCollections(),
  useUserCollectionsContaining: () => mockContainingIds(),
  useAddCollectionItem: () => ({
    mutateAsync: mockMutateAsync,
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
    mockMyCollections.mockReturnValue({
      data: {
        collections: [
          { id: 1, slug: 'my-favorites', title: 'My Favorites' },
          { id: 2, slug: 'best-of-2026', title: 'Best of 2026' },
          { id: 3, slug: 'arizona-shows', title: 'Arizona Shows' },
        ],
      },
      isLoading: false,
    })
    mockContainingIds.mockReturnValue({
      data: new Set<number>(),
      isLoading: false,
    })
    mockMutateAsync.mockResolvedValue(undefined)
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

  it('opens popover with collection checkboxes when clicked', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(await screen.findByText('Add to Collection')).toBeInTheDocument()
    expect(screen.getByText('My Favorites')).toBeInTheDocument()
    expect(screen.getByText('Best of 2026')).toBeInTheDocument()
    // Each row exposes a checkbox role.
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(3)
    for (const cb of checkboxes) {
      expect(cb).toHaveAttribute('aria-checked', 'false')
    }
  })

  it('pre-checks collections that already contain the entity', async () => {
    mockContainingIds.mockReturnValue({
      data: new Set<number>([2]),
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const favoritesCheckbox = await screen.findByRole('checkbox', {
      name: /my favorites/i,
    })
    const bestOfCheckbox = screen.getByRole('checkbox', { name: /best of 2026/i })
    expect(favoritesCheckbox).toHaveAttribute('aria-checked', 'false')
    expect(bestOfCheckbox).toHaveAttribute('aria-checked', 'true')
  })

  it('shows entity name in popover header', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="venue" entityId={5} entityName="The Rebel Lounge" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(await screen.findByText('The Rebel Lounge')).toBeInTheDocument()
  })

  it('disables Submit until at least one new row is checked', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const submitBefore = await screen.findByRole('button', { name: /^add$/i })
    expect(submitBefore).toBeDisabled()

    await user.click(screen.getByRole('checkbox', { name: /my favorites/i }))

    const submitAfter = screen.getByRole('button', { name: /add to 1 collection/i })
    expect(submitAfter).toBeEnabled()
  })

  it('fans out parallel AddItem requests for each newly-checked collection', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={42} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(
      await screen.findByRole('checkbox', { name: /my favorites/i })
    )
    await user.click(screen.getByRole('checkbox', { name: /arizona shows/i }))
    await user.click(
      screen.getByRole('button', { name: /add to 2 collections/i })
    )

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(2)
    })
    expect(mockMutateAsync).toHaveBeenCalledWith({
      slug: 'my-favorites',
      entityType: 'artist',
      entityId: 42,
    })
    expect(mockMutateAsync).toHaveBeenCalledWith({
      slug: 'arizona-shows',
      entityType: 'artist',
      entityId: 42,
    })
  })

  it('surfaces per-collection errors without blocking successes', async () => {
    // Resolve "my-favorites", reject "arizona-shows" with a server-shaped error.
    mockMutateAsync.mockImplementation(
      ({ slug }: { slug: string }) =>
        slug === 'arizona-shows'
          ? Promise.reject(new Error('Already in collection'))
          : Promise.resolve(undefined)
    )

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={42} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(
      await screen.findByRole('checkbox', { name: /my favorites/i })
    )
    await user.click(screen.getByRole('checkbox', { name: /arizona shows/i }))
    await user.click(
      screen.getByRole('button', { name: /add to 2 collections/i })
    )

    // Failure surfaces inline.
    expect(await screen.findByText('Already in collection')).toBeInTheDocument()
    // Success row had its add fulfilled — both calls happened.
    expect(mockMutateAsync).toHaveBeenCalledTimes(2)
  })

  it('shows "Create new collection" link', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const link = await screen.findByText('Create new collection')
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

    expect(await screen.findByText('No collections yet')).toBeInTheDocument()
  })

  it('toggling a checkbox via the keyboard (Space) flips its state', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const checkbox = await screen.findByRole('checkbox', { name: /my favorites/i })
    checkbox.focus()
    expect(checkbox).toHaveFocus()
    expect(checkbox).toHaveAttribute('aria-checked', 'false')

    await user.keyboard(' ')
    expect(checkbox).toHaveAttribute('aria-checked', 'true')
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
    let authState: MockAuthState = {
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
