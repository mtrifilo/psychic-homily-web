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

// Mock next/navigation — the unauth bracket variant pushes to /auth.
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/releases/test-release',
}))

// Mock @tanstack/react-query's useQueryClient — the component invalidates the
// contains + backlinks caches after a remove. The test doesn't mount a real
// QueryClientProvider, so stub the client.
const mockInvalidateQueries = vi.fn()
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  }
})

// Mock collection hooks. Collections carry item_count + is_public +
// cover_image_url so the rich-row subtitle ("N items · Public/Private") and
// thumbnail render (PSY-829 D2).
const DEFAULT_COLLECTIONS = [
  {
    id: 1,
    slug: 'my-favorites',
    title: 'My Favorites',
    item_count: 12,
    is_public: true,
    cover_image_url: null,
  },
  {
    id: 2,
    slug: 'best-of-2026',
    title: 'Best of 2026',
    item_count: 1,
    is_public: false,
    cover_image_url: null,
  },
  {
    id: 3,
    slug: 'arizona-shows',
    title: 'Arizona Shows',
    item_count: 7,
    is_public: true,
    cover_image_url: null,
  },
]
const mockMutateAsync = vi.fn()
const mockRemoveMutateAsync = vi.fn()
const mockMyCollections = vi.fn(() => ({
  data: { collections: DEFAULT_COLLECTIONS },
  isLoading: false,
}))
// PSY-829: contains query returns a Map (collectionId → collection_item id).
const mockContaining = vi.fn(() => ({
  data: new Map<number, number>(),
  isLoading: false,
}))

vi.mock('@/features/collections/hooks', () => ({
  useMyCollections: () => mockMyCollections(),
  useUserCollectionsContaining: () => mockContaining(),
  useAddCollectionItem: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
    isError: false,
    error: null as Error | null,
  }),
  useRemoveCollectionItem: () => ({
    mutateAsync: mockRemoveMutateAsync,
    isPending: false,
    isError: false,
    error: null as Error | null,
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

// PSY-961: the "Create … with {entity}" CTA opens the app-level Create drawer.
const mockOpenCreateDrawer = vi.fn()
vi.mock('@/features/collections/components/CreateCollectionDrawer', () => ({
  useCreateCollectionDrawer: () => ({ openCreateDrawer: mockOpenCreateDrawer }),
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
      data: { collections: DEFAULT_COLLECTIONS },
      isLoading: false,
    })
    mockContaining.mockReturnValue({
      data: new Map<number, number>(),
      isLoading: false,
    })
    // The add hook resolves to the created item (incl. its `id`) — PSY-829
    // captures it so a same-session uncheck→remove knows the item id.
    mockMutateAsync.mockResolvedValue({ id: 999 })
    mockRemoveMutateAsync.mockResolvedValue(undefined)
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

  it('pre-checks collections that already contain the entity + shows "Added"', async () => {
    mockContaining.mockReturnValue({
      data: new Map<number, number>([[2, 20]]),
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
    // The already-in row carries the "Added" indicator (PSY-829 D2).
    expect(screen.getByText('Added')).toBeInTheDocument()
  })

  it('renders the rich-row subtitle "N items · Public/Private" (PSY-829 D2)', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    // My Favorites: 12 items · Public; Best of 2026: 1 item (singular) · Private.
    expect(await screen.findByText('12 items · Public')).toBeInTheDocument()
    expect(screen.getByText('1 item · Private')).toBeInTheDocument()
    expect(screen.getByText('7 items · Public')).toBeInTheDocument()
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
          : Promise.resolve({ id: 111 })
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

  it('unchecks a failed-add row and drops it from the batch (PSY-829 code-review)', async () => {
    // The failed row must NOT stay checked — the row state should match
    // reality (not added) while the inline error explains why.
    mockMutateAsync.mockImplementation(({ slug }: { slug: string }) =>
      slug === 'arizona-shows'
        ? Promise.reject(new Error('Server error'))
        : Promise.resolve({ id: 111 })
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

    expect(await screen.findByText('Server error')).toBeInTheDocument()
    // The failed row is back to unchecked; the succeeded row shows Added.
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: /arizona shows/i })
      ).toHaveAttribute('aria-checked', 'false')
    )
    expect(
      screen.getByRole('checkbox', { name: /my favorites/i })
    ).toHaveAttribute('aria-checked', 'true')
  })

  it('same-session add → remove uses the item id captured from the add (PSY-829 code-review)', async () => {
    // Race the code-review flagged: after a Submit add, `containing` hasn't
    // refetched yet, so confirmRemove must fall back to the item id captured
    // from the add response (here: 555) — not silently no-op.
    mockMutateAsync.mockResolvedValue({ id: 555 })
    // containing stays EMPTY (the refetch hasn't landed) for the whole test.
    mockContaining.mockReturnValue({
      data: new Map<number, number>(),
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={42} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(
      await screen.findByRole('checkbox', { name: /my favorites/i })
    )
    await user.click(screen.getByRole('button', { name: /add to 1 collection/i }))

    // Row now shows Added; uncheck → confirm → Remove.
    await waitFor(() => expect(screen.getByText('Added')).toBeInTheDocument())
    await user.click(screen.getByRole('checkbox', { name: /my favorites/i }))
    await user.click(screen.getByRole('button', { name: /^remove$/i }))

    await waitFor(() => {
      expect(mockRemoveMutateAsync).toHaveBeenCalledWith({
        slug: 'my-favorites',
        itemId: 555,
      })
    })
  })

  it('clears never-submitted pending checks when the popover closes (PSY-829 code-review)', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    // Open, check a new row, then close WITHOUT submitting.
    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(
      await screen.findByRole('checkbox', { name: /my favorites/i })
    )
    expect(
      screen.getByRole('checkbox', { name: /my favorites/i })
    ).toHaveAttribute('aria-checked', 'true')
    await user.keyboard('{Escape}')

    // Reopen — the never-submitted check must not leak into the new session.
    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: /my favorites/i })
      ).toHaveAttribute('aria-checked', 'false')
    )
  })

  // Lock in the "Adding…" loading state. Earlier the only assertion around
  // the submitting UX was the failure-surface message; that would pass even
  // if the submit button stopped switching to its loading copy mid-flight.
  it('shows the "Adding…" loading state while the submit promises are in flight', async () => {
    // Hold every add open so the submitting=true window is observable.
    let resolveOne!: (v: { id: number }) => void
    mockMutateAsync.mockImplementation(
      () =>
        new Promise<{ id: number }>((resolve) => {
          resolveOne = resolve
        })
    )

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={42} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    await user.click(
      await screen.findByRole('checkbox', { name: /my favorites/i })
    )
    await user.click(
      screen.getByRole('button', { name: /add to 1 collection/i })
    )

    // BEFORE the mutation resolves: button copy switches to "Adding…" +
    // every row checkbox is disabled (so the user can't toggle mid-submit).
    expect(
      await screen.findByRole('button', { name: /adding/i })
    ).toBeDisabled()
    for (const cb of screen.getAllByRole('checkbox')) {
      expect(cb).toBeDisabled()
    }

    // Resolve so the test doesn't hang on the pending promise.
    resolveOne({ id: 1 })
    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1)
    })
  })

  it('opens the Create drawer pre-filled with the entity from the main CTA (D4)', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    const cta = await screen.findByRole('button', {
      name: /create new collection with test artist/i,
    })
    await user.click(cta)
    expect(mockOpenCreateDrawer).toHaveBeenCalledWith({
      initialStagedItems: [
        { entityType: 'artist', entityId: 1, name: 'Test Artist', subtitle: null },
      ],
    })
  })

  it('shows a primary Create CTA that pre-fills the entity in the empty state (D5)', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: [] },
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )

    await user.click(screen.getByRole('button', { name: /add to collection/i }))

    expect(
      await screen.findByText('No collections yet — start one.')
    ).toBeInTheDocument()
    // D5: Create is the primary action and pre-fills the current entity.
    await user.click(
      screen.getByRole('button', { name: /create with test artist/i })
    )
    expect(mockOpenCreateDrawer).toHaveBeenCalledWith({
      initialStagedItems: [
        { entityType: 'artist', entityId: 1, name: 'Test Artist', subtitle: null },
      ],
    })
    // No submit row / no checkbox list in the empty state.
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
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

  // ── PSY-829 D1: uncheck an already-in row → remove-with-confirm ──
  // The pre-PSY-829 popover only fanned out newly-checked IDs, so unchecking
  // an already-in row did nothing (dead affordance). Now it opens an inline
  // confirm and, on Remove, DELETEs by the collection_item id the contains
  // query supplies.
  describe('remove-with-confirm (D1)', () => {
    it('unchecking a saved row opens the confirm (no removal yet)', async () => {
      mockContaining.mockReturnValue({
        data: new Map<number, number>([[2, 20]]),
        isLoading: false,
      })
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )

      await user.click(screen.getByRole('button', { name: /add to collection/i }))
      await user.click(
        await screen.findByRole('checkbox', { name: /best of 2026/i })
      )

      expect(
        screen.getByText('Remove from this collection?')
      ).toBeInTheDocument()
      // Nothing deleted until the user confirms.
      expect(mockRemoveMutateAsync).not.toHaveBeenCalled()
    })

    it('Cancel dismisses the confirm and keeps the row added', async () => {
      mockContaining.mockReturnValue({
        data: new Map<number, number>([[2, 20]]),
        isLoading: false,
      })
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )

      await user.click(screen.getByRole('button', { name: /add to collection/i }))
      await user.click(
        await screen.findByRole('checkbox', { name: /best of 2026/i })
      )
      await user.click(screen.getByRole('button', { name: /^cancel$/i }))

      expect(
        screen.queryByText('Remove from this collection?')
      ).not.toBeInTheDocument()
      expect(mockRemoveMutateAsync).not.toHaveBeenCalled()
      // Row stays checked + Added.
      expect(
        screen.getByRole('checkbox', { name: /best of 2026/i })
      ).toHaveAttribute('aria-checked', 'true')
    })

    it('Remove DELETEs by the collection_item id and clears the Added state', async () => {
      mockContaining.mockReturnValue({
        data: new Map<number, number>([[2, 20]]),
        isLoading: false,
      })
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )

      await user.click(screen.getByRole('button', { name: /add to collection/i }))
      await user.click(
        await screen.findByRole('checkbox', { name: /best of 2026/i })
      )
      await user.click(screen.getByRole('button', { name: /^remove$/i }))

      await waitFor(() => {
        expect(mockRemoveMutateAsync).toHaveBeenCalledWith({
          slug: 'best-of-2026',
          itemId: 20,
        })
      })
      // Row is no longer Added; checkbox unchecked.
      await waitFor(() =>
        expect(
          screen.getByRole('checkbox', { name: /best of 2026/i })
        ).toHaveAttribute('aria-checked', 'false')
      )
    })

    it('surfaces a remove failure inline without clearing the Added state', async () => {
      mockContaining.mockReturnValue({
        data: new Map<number, number>([[2, 20]]),
        isLoading: false,
      })
      mockRemoveMutateAsync.mockRejectedValueOnce(new Error('Network down'))
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )

      await user.click(screen.getByRole('button', { name: /add to collection/i }))
      await user.click(
        await screen.findByRole('checkbox', { name: /best of 2026/i })
      )
      await user.click(screen.getByRole('button', { name: /^remove$/i }))

      expect(await screen.findByText('Network down')).toBeInTheDocument()
      // Still added — the row didn't lose its membership on a failed remove.
      expect(
        screen.getByRole('checkbox', { name: /best of 2026/i })
      ).toHaveAttribute('aria-checked', 'true')
    })
  })

  // ── PSY-829: client-side filter input above a long list ──
  describe('search filter', () => {
    const manyCollections = Array.from({ length: 10 }, (_, i) => ({
      id: i + 1,
      slug: `coll-${i + 1}`,
      title: i === 0 ? 'Desert Psych' : `Collection ${i + 1}`,
      item_count: i + 1,
      is_public: true,
      cover_image_url: null,
    }))

    it('shows the filter only when the list exceeds the threshold', async () => {
      const user = userEvent.setup()
      // Default fixture has 3 collections — below threshold → no filter.
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )
      await user.click(screen.getByRole('button', { name: /add to collection/i }))
      expect(
        screen.queryByRole('textbox', { name: /filter collections/i })
      ).not.toBeInTheDocument()
    })

    it('filters rows by title (case-insensitive)', async () => {
      mockMyCollections.mockReturnValue({
        data: { collections: manyCollections },
        isLoading: false,
      })
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )
      await user.click(screen.getByRole('button', { name: /add to collection/i }))

      const filter = await screen.findByRole('textbox', {
        name: /filter collections/i,
      })
      await user.type(filter, 'desert')

      expect(screen.getByText('Desert Psych')).toBeInTheDocument()
      expect(screen.queryByText('Collection 2')).not.toBeInTheDocument()
    })

    it('shows the no-match empty state when the filter excludes every row', async () => {
      mockMyCollections.mockReturnValue({
        data: { collections: manyCollections },
        isLoading: false,
      })
      const user = userEvent.setup()
      render(
        <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
      )
      await user.click(screen.getByRole('button', { name: /add to collection/i }))

      const filter = await screen.findByRole('textbox', {
        name: /filter collections/i,
      })
      await user.type(filter, 'zzzznomatch')

      expect(screen.getByText(/No collections match/)).toBeInTheDocument()
      // No rows rendered, but the panel didn't crash and the filter persists.
      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
      expect(filter).toHaveValue('zzzznomatch')
    })
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

describe('AddToCollectionButton — bracket variant (PSY-641)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders [Add to collection] as a bracket link in bracket variant', () => {
    render(
      <AddToCollectionButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    const trigger = screen.getByRole('button', { name: /add to collection/i })
    expect(trigger).toBeInTheDocument()
    expect(trigger).toHaveTextContent('[Add to collection]')
  })

  it('bracket trigger opens the collections popover when clicked', async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        variant="bracket"
      />
    )
    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    expect(await screen.findByText('My Favorites')).toBeInTheDocument()
  })

  // ── PSY-663: unauthenticated bracket variant renders a public affordance ──
  // Releases and labels aren't follow/notify-able, so the bracket
  // [Add to collection] is their only public header bracket. For unauth
  // viewers it must still render (not return an empty linkbox) and redirect
  // to /auth on click, mirroring FollowButton / NotifyMeButton.
  it('renders [Add to collection] for an unauthenticated viewer in bracket variant', () => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    render(
      <AddToCollectionButton
        entityType="release"
        entityId={7}
        entityName="Test Release"
        variant="bracket"
      />
    )
    const trigger = screen.getByRole('button', { name: /add to collection/i })
    expect(trigger).toBeInTheDocument()
    expect(trigger).toHaveTextContent('[Add to collection]')
  })

  it('redirects an unauthenticated viewer to /auth with returnTo on click', async () => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    const user = userEvent.setup()
    render(
      <AddToCollectionButton
        entityType="release"
        entityId={7}
        entityName="Test Release"
        variant="bracket"
      />
    )
    await user.click(screen.getByRole('button', { name: /add to collection/i }))
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Freleases%2Ftest-release'
    )
    // No popover should open for unauth viewers.
    expect(screen.queryByText('My Favorites')).not.toBeInTheDocument()
  })

  // Non-bracket variants have no public surface — they still return null for
  // unauth viewers (the fix is scoped to the bracket variant only).
  it.each(['default', 'ghost', 'outline'] as const)(
    'renders nothing for an unauthenticated viewer in %s variant',
    (variant) => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      const { container } = render(
        <AddToCollectionButton
          entityType="release"
          entityId={7}
          entityName="Test Release"
          variant={variant}
        />
      )
      expect(container.innerHTML).toBe('')
    }
  )
})

describe('AddToCollectionButton — recently-used promotion (PSY-960)', () => {
  // 9 collections so the grouping (>=5) AND the search box (>8) are both
  // active — lets us exercise the searching-collapses-grouping rule too.
  const NAMES = [
    'Alpha',
    'Bravo',
    'Charlie',
    'Delta',
    'Echo',
    'Foxtrot',
    'Golf',
    'Hotel',
    'India',
  ]
  const MANY_COLLECTIONS = NAMES.map((title, i) => ({
    id: i + 1,
    slug: `c${i + 1}`,
    title,
    item_count: 3,
    is_public: true,
    cover_image_url: null,
  }))

  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockContaining.mockReturnValue({
      data: new Map<number, number>(),
      isLoading: false,
    })
    mockMutateAsync.mockResolvedValue({ id: 999 })
    mockRemoveMutateAsync.mockResolvedValue(undefined)
  })

  const openPopover = async () => {
    const user = userEvent.setup()
    render(
      <AddToCollectionButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    await user.click(
      screen.getByRole('button', { name: /add to collection/i })
    )
    return user
  }

  it('promotes recently-used collections above a separator, newest first', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })
    // Charlie (id 3) added most recently, then Alpha (id 1).
    window.localStorage.setItem(
      'psy:collection-add-recency',
      JSON.stringify({ '3': 2000, '1': 1000 })
    )

    await openPopover()

    expect(await screen.findByText('Recently used')).toBeInTheDocument()
    expect(screen.getByText('All collections')).toBeInTheDocument()

    // Promoted rows render first, newest-first: Charlie then Alpha.
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(9)
    expect(checkboxes[0]).toHaveAccessibleName(/charlie/i)
    expect(checkboxes[1]).toHaveAccessibleName(/alpha/i)
    // The third row is the start of the (un-promoted) remainder.
    expect(checkboxes[2]).not.toHaveAccessibleName(/charlie|alpha/i)
    // Each section is its own a11y group, labelled by its header.
    const labelledGroups = screen
      .getAllByRole('group')
      .filter((g) => g.hasAttribute('aria-labelledby'))
    expect(labelledGroups).toHaveLength(2)
  })

  it('orders a multi-select batch newest-first (last-checked promoted highest)', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )
    const trigger = screen.getByRole('button', { name: /add to collection/i })

    await user.click(trigger)
    // Check Bravo (id 2) THEN Delta (id 4) in one batch — Delta is checked
    // last, so it must get the newer stamp (strictly-increasing per batch).
    await user.click(await screen.findByRole('checkbox', { name: /bravo/i }))
    await user.click(screen.getByRole('checkbox', { name: /delta/i }))
    await user.click(screen.getByRole('button', { name: /add to 2 collections/i }))
    await waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem('psy:collection-add-recency') ?? '{}'
      )
      expect(stored['2']).toBeGreaterThan(0)
      // Delta (last-checked) is strictly newer than Bravo, not a Date.now() tie.
      expect(stored['4']).toBeGreaterThan(stored['2'])
    })

    // Reopen — Delta promoted above Bravo under "Recently used".
    await user.click(trigger)
    await user.click(trigger)
    expect(await screen.findByText('Recently used')).toBeInTheDocument()
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes[0]).toHaveAccessibleName(/delta/i)
    expect(checkboxes[1]).toHaveAccessibleName(/bravo/i)
  })

  it('stays flat (no grouping) below the collection threshold', async () => {
    // DEFAULT_COLLECTIONS has 3 (< RECENTLY_USED_MIN_COLLECTIONS); recency is
    // present but must be ignored.
    mockMyCollections.mockReturnValue({
      data: { collections: DEFAULT_COLLECTIONS },
      isLoading: false,
    })
    window.localStorage.setItem(
      'psy:collection-add-recency',
      JSON.stringify({ '1': 1000 })
    )

    await openPopover()

    expect(await screen.findByText('My Favorites')).toBeInTheDocument()
    expect(screen.queryByText('Recently used')).not.toBeInTheDocument()
    expect(screen.queryByText('All collections')).not.toBeInTheDocument()
  })

  it('stays flat on a cold start (no add-recency yet)', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })
    // localStorage intentionally empty.

    await openPopover()

    expect(await screen.findByText('Alpha')).toBeInTheDocument()
    expect(screen.queryByText('Recently used')).not.toBeInTheDocument()
  })

  it('collapses to a flat filtered list while searching', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })
    window.localStorage.setItem(
      'psy:collection-add-recency',
      JSON.stringify({ '3': 2000 })
    )

    const user = await openPopover()
    // Grouped before searching.
    expect(await screen.findByText('Recently used')).toBeInTheDocument()

    // Type a filter — the section headers disappear, results go flat.
    await user.type(
      screen.getByRole('textbox', { name: /filter collections/i }),
      'char'
    )
    expect(screen.queryByText('Recently used')).not.toBeInTheDocument()
    expect(screen.queryByText('All collections')).not.toBeInTheDocument()
    expect(screen.getByText('Charlie')).toBeInTheDocument()
  })

  it('records the add in localStorage on a successful submit', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })

    const user = await openPopover()
    // Check Delta (id 4) and submit.
    await user.click(
      await screen.findByRole('checkbox', { name: /delta/i })
    )
    await user.click(
      screen.getByRole('button', { name: /add to 1 collection/i })
    )

    await waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem('psy:collection-add-recency') ?? '{}'
      )
      expect(stored['4']).toBeGreaterThan(0)
    })
  })

  it('surfaces a collection under "Recently used" after an add + reopen', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })

    const user = userEvent.setup()
    render(
      <AddToCollectionButton entityType="artist" entityId={1} entityName="Test Artist" />
    )
    const trigger = screen.getByRole('button', { name: /add to collection/i })

    // First open: cold start, flat (no grouping).
    await user.click(trigger)
    expect(await screen.findByText('Echo')).toBeInTheDocument()
    expect(screen.queryByText('Recently used')).not.toBeInTheDocument()

    // Add to Echo (id 5) and submit.
    await user.click(screen.getByRole('checkbox', { name: /echo/i }))
    await user.click(screen.getByRole('button', { name: /add to 1 collection/i }))
    await waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem('psy:collection-add-recency') ?? '{}'
      )
      expect(stored['5']).toBeGreaterThan(0)
    })

    // Close and reopen — Echo now promoted to the top of "Recently used".
    await user.click(trigger)
    await user.click(trigger)
    expect(await screen.findByText('Recently used')).toBeInTheDocument()
    expect(screen.getAllByRole('checkbox')[0]).toHaveAccessibleName(/echo/i)
  })

  it('caps "Recently used" at 5 and overflows extra stamped collections into "All collections"', async () => {
    mockMyCollections.mockReturnValue({
      data: { collections: MANY_COLLECTIONS },
      isLoading: false,
    })
    // Six collections stamped (ids 1–6), distinct increasing timestamps.
    window.localStorage.setItem(
      'psy:collection-add-recency',
      JSON.stringify({ '1': 1000, '2': 2000, '3': 3000, '4': 4000, '5': 5000, '6': 6000 })
    )

    await openPopover()

    expect(await screen.findByText('Recently used')).toBeInTheDocument()
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(9)
    // Promoted = the newest 5: Foxtrot(6) Echo(5) Delta(4) Charlie(3) Bravo(2).
    expect(checkboxes[0]).toHaveAccessibleName(/foxtrot/i)
    expect(checkboxes[4]).toHaveAccessibleName(/bravo/i)
    // Alpha (id 1, oldest stamp) exceeds the cap → overflows into the
    // remainder, NOT promoted, and is not duplicated.
    expect(checkboxes[5]).not.toHaveAccessibleName(/foxtrot|echo|delta|charlie|bravo/i)
    expect(screen.getAllByText('Alpha')).toHaveLength(1)
  })
})
