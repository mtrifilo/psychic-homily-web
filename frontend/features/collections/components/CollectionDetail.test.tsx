import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CollectionDetail } from './CollectionDetail'
import type { CollectionDetail as CollectionDetailType } from '../types'

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

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/collections/test-collection',
}))

// Mock shared components
vi.mock('@/components/shared', () => ({
  Breadcrumb: ({
    currentPage,
  }: {
    fallback: { href: string; label: string }
    currentPage: string
  }) => (
    <nav aria-label="Breadcrumb">
      <span data-testid="breadcrumb-current">{currentPage}</span>
    </nav>
  ),
}))

// Mock hooks
const mockCollection = vi.fn()
const mockDeleteMutate = vi.fn()
const mockDeleteMutation = vi.fn(() => ({
  mutate: mockDeleteMutate,
  isPending: false,
  isError: false,
  error: null,
}))
const mockReorderMutate = vi.fn()
const mockUpdateMutate = vi.fn()
// PSY-351: clone mutation mock — `mutate` invokes the success callback
// directly so we can assert the post-clone navigation deterministically
// without spinning up a real React Query client.
const mockCloneMutate = vi.fn()
const mockCloneMutation = vi.fn(() => ({
  mutate: mockCloneMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('../hooks', () => ({
  useCollection: (...args: unknown[]) => mockCollection(...args),
  useUpdateCollection: () => ({
    mutate: mockUpdateMutate,
    isPending: false,
    error: null,
  }),
  useAddCollectionItem: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useRemoveCollectionItem: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useReorderCollectionItems: () => ({
    mutate: mockReorderMutate,
    isPending: false,
  }),
  useUpdateCollectionItem: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useSubscribeCollection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useUnsubscribeCollection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteCollection: () => mockDeleteMutation(),
  useCloneCollection: () => mockCloneMutation(),
}))

// Mock comments feature
vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">Comments for {entityType} {entityId}</div>
  ),
}))

// Mock useEntitySearch
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => ({
    data: { artists: [], venues: [], releases: [], labels: [], festivals: [] },
    isSearching: false,
    totalResults: 0,
  }),
}))

function makeCollection(
  overrides: Partial<CollectionDetailType> = {}
): CollectionDetailType {
  return {
    id: 1,
    title: 'Test Collection',
    slug: 'test-collection',
    description: 'A test collection',
    is_public: true,
    collaborative: false,
    is_featured: false,
    cover_image_url: null,
    creator_id: 1,
    creator_name: 'testuser',
    display_mode: 'unranked',
    item_count: 0,
    subscriber_count: 0,
    contributor_count: 0,
    forks_count: 0,
    forked_from_collection_id: null,
    forked_from: null,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    items: [],
    is_subscribed: false,
    ...overrides,
  }
}

/** Helper: find the delete collection button by aria-label */
function findTrashButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Delete collection' })
}

describe('CollectionDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockDeleteMutation.mockReturnValue({
      mutate: mockDeleteMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockCloneMutation.mockReturnValue({
      mutate: mockCloneMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockCollection.mockReturnValue({
      data: makeCollection(),
      isLoading: false,
      error: null,
    })
  })

  it('renders collection title in heading', () => {
    render(<CollectionDetail slug="test-collection" />)
    expect(
      screen.getByRole('heading', { name: 'Test Collection', level: 1 })
    ).toBeInTheDocument()
  })

  it('renders loading state', () => {
    mockCollection.mockReturnValue({
      data: null,
      isLoading: true,
      error: null,
    })
    render(<CollectionDetail slug="test-collection" />)
    expect(
      screen.queryByRole('heading', { name: 'Test Collection' })
    ).not.toBeInTheDocument()
  })

  it('renders error state', () => {
    mockCollection.mockReturnValue({
      data: null,
      isLoading: false,
      error: new Error('Failed to load'),
    })
    render(<CollectionDetail slug="test-collection" />)
    expect(screen.getByText('Error Loading Collection')).toBeInTheDocument()
  })

  it('shows delete button for creator', () => {
    render(<CollectionDetail slug="test-collection" />)
    expect(findTrashButton()).toBeTruthy()
  })

  it('opens delete confirmation dialog instead of window.confirm', async () => {
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    await user.click(findTrashButton())

    // Dialog should open with confirmation text
    expect(screen.getByText(/cannot be undone/)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Cancel' })
    ).toBeInTheDocument()
    // "Delete Collection" appears in dialog title and button
    expect(screen.getAllByText('Delete Collection').length).toBeGreaterThanOrEqual(1)
  })

  it('calls deleteMutation.mutate when confirming delete in dialog', async () => {
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    // Open dialog
    await user.click(findTrashButton())

    // Click the destructive "Delete Collection" button in the dialog footer
    const deleteButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent?.includes('Delete Collection'))
    await user.click(deleteButtons[deleteButtons.length - 1])

    expect(mockDeleteMutate).toHaveBeenCalledWith(
      { slug: 'test-collection' },
      expect.any(Object)
    )
  })

  it('closes dialog when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    // Open dialog
    await user.click(findTrashButton())
    expect(screen.getByText(/cannot be undone/)).toBeInTheDocument()

    // Click Cancel
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
  })

  it('shows error message in dialog when deletion fails', async () => {
    mockDeleteMutation.mockReturnValue({
      mutate: mockDeleteMutate,
      isPending: false,
      isError: true,
      error: { message: 'Server error' },
    })
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    // Open dialog
    await user.click(findTrashButton())

    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows "Deleting..." text when deletion is pending in dialog', async () => {
    // Start with isPending false so we can open the dialog
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    // Open dialog first
    await user.click(findTrashButton())

    // Now simulate isPending becoming true (re-render with pending state)
    mockDeleteMutation.mockReturnValue({
      mutate: mockDeleteMutate,
      isPending: true,
      isError: false,
      error: null,
    })

    // Click the delete button to trigger the mutation
    const deleteButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent?.includes('Delete Collection'))
    await user.click(deleteButtons[deleteButtons.length - 1])

    // The mutate was called
    expect(mockDeleteMutate).toHaveBeenCalled()
  })

  it('does not show delete button for non-creator', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '999' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockCollection.mockReturnValue({
      data: makeCollection({ creator_id: 1 }),
      isLoading: false,
      error: null,
    })
    render(<CollectionDetail slug="test-collection" />)

    // No Edit or delete buttons for non-creators
    expect(screen.queryByText('Edit')).not.toBeInTheDocument()
    const buttons = screen.getAllByRole('button')
    const trashButton = buttons.find(
      (b) => b.className.includes('text-destructive')
    )
    expect(trashButton).toBeUndefined()
  })

  it('does not use window.confirm for delete', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    const user = userEvent.setup()
    render(<CollectionDetail slug="test-collection" />)

    // Open dialog
    await user.click(findTrashButton())

    // Confirm in dialog
    const deleteButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent?.includes('Delete Collection'))
    if (deleteButtons.length > 0) {
      await user.click(deleteButtons[deleteButtons.length - 1])
    }

    expect(confirmSpy).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  // ──────────────────────────────────────────────
  // PSY-351: Clone / Fork
  // ──────────────────────────────────────────────

  describe('PSY-351 fork attribution + clone button', () => {
    it('does not render fork attribution when collection has no source', () => {
      render(<CollectionDetail slug="test-collection" />)
      expect(
        screen.queryByTestId('forked-from-attribution')
      ).not.toBeInTheDocument()
    })

    it('renders inline "Forked from <link> by <curator>" when source exists', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          title: 'Forked Title',
          forked_from_collection_id: 42,
          forked_from: {
            id: 42,
            title: 'Original Title',
            slug: 'original-slug',
            creator_id: 7,
            creator_name: 'originator',
          },
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="forked-title" />)

      const attribution = screen.getByTestId('forked-from-attribution')
      expect(attribution).toBeInTheDocument()
      expect(attribution.textContent).toContain('Forked from')
      expect(attribution.textContent).toContain('Original Title')
      expect(attribution.textContent).toContain('by originator')

      // Title is rendered as a link to the source collection.
      const link = screen.getByRole('link', { name: 'Original Title' })
      expect(link).toHaveAttribute('href', '/collections/original-slug')
    })

    it('falls back to "Forked from a deleted collection" when source is gone', () => {
      // FK is set on the cloned record but the snapshot is null —
      // matches the backend behavior when the source was deleted (the
      // FK was reset to NULL via ON DELETE SET NULL).
      mockCollection.mockReturnValue({
        data: makeCollection({
          forked_from_collection_id: 42,
          forked_from: null,
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="orphan-fork" />)

      const attribution = screen.getByTestId('forked-from-attribution')
      expect(attribution.textContent).toContain(
        'Forked from a deleted collection'
      )
      // No link rendered in the fallback state.
      expect(
        screen.queryByRole('link', { name: /forked from/i })
      ).not.toBeInTheDocument()
    })

    it('does not render "N forks" stat when count is zero', () => {
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.queryByTestId('forks-count')).not.toBeInTheDocument()
    })

    it('renders "1 fork" / "N forks" public count when forks exist', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ forks_count: 1 }),
        isLoading: false,
        error: null,
      })
      const { rerender } = render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('forks-count').textContent).toContain('1 fork')

      mockCollection.mockReturnValue({
        data: makeCollection({ forks_count: 5 }),
        isLoading: false,
        error: null,
      })
      rerender(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('forks-count').textContent).toContain('5 forks')
    })

    it('hides Fork button on the user\'s own collection', () => {
      // current user (id=1) is the creator (creator_id=1) by default.
      render(<CollectionDetail slug="test-collection" />)
      expect(
        screen.queryByRole('button', { name: 'Fork collection' })
      ).not.toBeInTheDocument()
    })

    it('hides Fork button on private collections', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '999' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1, is_public: false }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(
        screen.queryByRole('button', { name: 'Fork collection' })
      ).not.toBeInTheDocument()
    })

    it('hides Fork button when not authenticated', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(
        screen.queryByRole('button', { name: 'Fork collection' })
      ).not.toBeInTheDocument()
    })

    it('shows Fork button to authenticated non-owners on public collections', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '999' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1, is_public: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(
        screen.getByRole('button', { name: 'Fork collection' })
      ).toBeInTheDocument()
    })

    it('navigates to the new collection slug after a successful clone', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '999' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1, is_public: true }),
        isLoading: false,
        error: null,
      })
      // Mutation triggers the onSuccess callback synchronously with the
      // server response shape (CollectionDetail).
      mockCloneMutate.mockImplementation(
        (_args, opts) => {
          opts?.onSuccess?.({
            ...makeCollection({
              id: 99,
              title: 'Test Collection (fork)',
              slug: 'test-collection-fork',
              creator_id: 999,
            }),
          })
        }
      )

      render(<CollectionDetail slug="test-collection" />)
      await user.click(
        screen.getByRole('button', { name: 'Fork collection' })
      )

      expect(mockCloneMutate).toHaveBeenCalledWith(
        { slug: 'test-collection' },
        expect.any(Object)
      )
      expect(mockPush).toHaveBeenCalledWith(
        '/collections/test-collection-fork'
      )
    })
  })

  // ──────────────────────────────────────────────
  // PSY-348: ranked vs. unranked display mode
  // ──────────────────────────────────────────────

  describe('display mode', () => {
    const sampleItems = [
      {
        id: 11,
        entity_type: 'artist',
        entity_id: 101,
        entity_name: 'First Artist',
        entity_slug: 'first-artist',
        position: 0,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 12,
        entity_type: 'artist',
        entity_id: 102,
        entity_name: 'Second Artist',
        entity_slug: 'second-artist',
        position: 1,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 13,
        entity_type: 'artist',
        entity_id: 103,
        entity_name: 'Third Artist',
        entity_slug: 'third-artist',
        position: 2,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
    ]

    it('does NOT render position numbers in unranked mode', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'unranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      // Numbered positions absent
      expect(screen.queryByText('1')).not.toBeInTheDocument()
      expect(screen.queryByText('2')).not.toBeInTheDocument()
      // No drag handle
      expect(screen.queryAllByTestId('drag-handle')).toHaveLength(0)
      // Items still rendered
      expect(screen.getByText('First Artist')).toBeInTheDocument()
      expect(screen.getByText('Second Artist')).toBeInTheDocument()
    })

    it('renders numbered positions and drag handles in ranked mode', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      // Numbered positions visible
      expect(screen.getByText('1')).toBeInTheDocument()
      expect(screen.getByText('2')).toBeInTheDocument()
      expect(screen.getByText('3')).toBeInTheDocument()
      // Drag handles visible (one per item) — only when ranked + creator
      expect(screen.getAllByTestId('drag-handle')).toHaveLength(3)
    })

    it('does NOT render drag handles in ranked mode for non-creator', () => {
      // Logged-in user is not the creator
      mockAuthContext.mockReturnValue({
        user: { id: '999' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({
          display_mode: 'ranked',
          items: sampleItems,
          creator_id: 1,
        }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      // Numbered positions still visible (everyone sees the ranking)
      expect(screen.getByText('1')).toBeInTheDocument()
      // ...but no drag handle for non-creators
      expect(screen.queryAllByTestId('drag-handle')).toHaveLength(0)
    })

    it('keyboard fallback: Move down sends correct reorder payload', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // First Move down button (corresponds to index 0 → swap with index 1)
      const moveDownButtons = screen.getAllByRole('button', { name: 'Move down' })
      await user.click(moveDownButtons[0])

      expect(mockReorderMutate).toHaveBeenCalledWith({
        slug: 'test-collection',
        items: [
          { item_id: 12, position: 0 },
          { item_id: 11, position: 1 },
          { item_id: 13, position: 2 },
        ],
      })
    })

    it('keyboard fallback: Move up sends correct reorder payload', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Last Move up button corresponds to index 2 → swap with index 1
      const moveUpButtons = screen.getAllByRole('button', { name: 'Move up' })
      await user.click(moveUpButtons[moveUpButtons.length - 1])

      expect(mockReorderMutate).toHaveBeenCalledWith({
        slug: 'test-collection',
        items: [
          { item_id: 11, position: 0 },
          { item_id: 13, position: 1 },
          { item_id: 12, position: 2 },
        ],
      })
    })

    it('keyboard fallback: Move up disabled on first item', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      const moveUpButtons = screen.getAllByRole('button', { name: 'Move up' })
      expect(moveUpButtons[0]).toBeDisabled()
    })

    it('keyboard fallback: Move down disabled on last item', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      const moveDownButtons = screen.getAllByRole('button', { name: 'Move down' })
      expect(moveDownButtons[moveDownButtons.length - 1]).toBeDisabled()
    })

    it('edit form: toggling display mode sends display_mode in update payload', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'unranked' }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Enter edit mode
      await user.click(screen.getByRole('button', { name: /Edit/i }))

      // Toggle to ranked. Locate by input value attribute since both radios
      // share an accessible-name prefix ("Ranked" matches "Unranked" too).
      const radios = screen.getAllByRole('radio') as HTMLInputElement[]
      const rankedRadio = radios.find((r) => r.value === 'ranked')!
      await user.click(rankedRadio)
      expect(rankedRadio).toBeChecked()

      // Save
      await user.click(screen.getByRole('button', { name: /Save/i }))

      expect(mockUpdateMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'test-collection',
          display_mode: 'ranked',
        }),
        expect.any(Object)
      )
    })

    it('edit form: defaults to current display_mode (ranked)', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked' }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const radios = screen.getAllByRole('radio') as HTMLInputElement[]
      const rankedRadio = radios.find((r) => r.value === 'ranked')!
      const unrankedRadio = radios.find((r) => r.value === 'unranked')!
      expect(rankedRadio).toBeChecked()
      expect(unrankedRadio).not.toBeChecked()
    })
  })

  // ──────────────────────────────────────────────
  // PSY-353: contributor badge + creator attribution link
  // ──────────────────────────────────────────────

  describe('PSY-353 contributor badge + creator link', () => {
    it('links creator name to /users/:username when creator_username is set', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_username: 'testuser' }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const link = screen.getByRole('link', { name: 'testuser' })
      expect(link).toHaveAttribute('href', '/users/testuser')
    })

    it('renders creator name as plain text when creator_username is absent', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ creator_username: null }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(
        screen.queryByRole('link', { name: 'testuser' })
      ).not.toBeInTheDocument()
      expect(screen.getByText(/testuser/)).toBeInTheDocument()
    })

    it('renders the contributor badge when contributor_count >= 3', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ contributor_count: 5 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const badge = screen.getByTestId('contributor-badge')
      expect(badge.textContent).toContain('Built by 5 contributors')
    })

    it('renders the badge at the threshold (exactly 3)', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ contributor_count: 3 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('contributor-badge').textContent).toContain(
        'Built by 3 contributors'
      )
    })

    it('omits the badge when contributor_count is below 3', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ contributor_count: 2 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.queryByTestId('contributor-badge')).not.toBeInTheDocument()
    })
  })
})
