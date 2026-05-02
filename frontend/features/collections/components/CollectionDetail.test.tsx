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
  // PSY-360: stub the density toggle so the items-list view-mode toggle
  // can render without pulling in the real (no behavior we exercise from
  // here — density coverage lives in the DensityToggle's own test file).
  DensityToggle: ({
    density,
    onDensityChange,
  }: {
    density: 'compact' | 'comfortable' | 'expanded'
    onDensityChange: (value: 'compact' | 'comfortable' | 'expanded') => void
  }) => (
    <div data-testid="density-toggle-stub">
      <button onClick={() => onDensityChange('compact')}>compact</button>
      <button onClick={() => onDensityChange('comfortable')}>comfortable</button>
      <button onClick={() => onDensityChange('expanded')}>expanded</button>
      <span data-testid="density-current">{density}</span>
    </div>
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
// PSY-372: spy for "Add" clicks in the Add Items panel so tests can assert
// the right entityType/entityId is sent when adding a show.
const mockAddItemMutate = vi.fn()
// PSY-351: clone mutation mock — `mutate` invokes the success callback
// directly so we can assert the post-clone navigation deterministically
// without spinning up a real React Query client.
// PSY-352: like/unlike mutation spies for the heart toggle.
const mockLikeMutate = vi.fn()
const mockUnlikeMutate = vi.fn()
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
    mutate: mockAddItemMutate,
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
  useLikeCollection: () => ({
    mutate: mockLikeMutate,
    isPending: false,
  }),
  useUnlikeCollection: () => ({
    mutate: mockUnlikeMutate,
    isPending: false,
  }),
}))

// Mock comments feature
vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">Comments for {entityType} {entityId}</div>
  ),
}))

// PSY-354: stub EntityTagList — its real implementation pulls in tag
// hooks (useEntityTags, useSearchTags, useVoteOnTag) that need a
// QueryClient. The CollectionDetail tests don't exercise tag UI, so a
// minimal stub keeps them isolated from tag-feature plumbing.
vi.mock('@/features/tags', () => ({
  EntityTagList: ({
    entityType,
    entityId,
  }: {
    entityType: string
    entityId: number
  }) => (
    <div data-testid="entity-tag-list">
      Tags for {entityType} {entityId}
    </div>
  ),
}))

// Mock useEntitySearch
// Default mock — empty results across all entity types. Individual tests
// override `mockUseEntitySearchResult` below to seed shows/artists/etc.
type MockedEntitySearchResult = {
  data: {
    artists: unknown[]
    venues: unknown[]
    shows: unknown[]
    releases: unknown[]
    labels: unknown[]
    festivals: unknown[]
    tags: unknown[]
  }
  isSearching: boolean
  totalResults: number
}
let mockUseEntitySearchResult: MockedEntitySearchResult = {
  data: {
    artists: [],
    venues: [],
    shows: [],
    releases: [],
    labels: [],
    festivals: [],
    tags: [],
  },
  isSearching: false,
  totalResults: 0,
}
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockUseEntitySearchResult,
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
    like_count: 0,
    user_likes_this: false,
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
    // Reset entity search to "no results" between tests so cases that don't
    // rely on Add Items aren't accidentally polluted by an earlier override.
    mockUseEntitySearchResult = {
      data: {
        artists: [],
        venues: [],
        shows: [],
        releases: [],
        labels: [],
        festivals: [],
        tags: [],
      },
      isSearching: false,
      totalResults: 0,
    }
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
  // PSY-352: like toggle on the detail header
  // ──────────────────────────────────────────────

  describe('like toggle', () => {
    it('renders the heart button with current count for authenticated viewer', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ like_count: 12, user_likes_this: false }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const btn = screen.getByTestId('collection-like-button')
      expect(btn).toBeInTheDocument()
      expect(btn).toHaveTextContent('12')
      expect(btn).toHaveAttribute('aria-pressed', 'false')
      expect(btn).toHaveAttribute('aria-label', 'Like collection')
    })

    it('marks the heart pressed when user_likes_this is true', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ like_count: 5, user_likes_this: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      const btn = screen.getByTestId('collection-like-button')
      expect(btn).toHaveAttribute('aria-pressed', 'true')
      expect(btn).toHaveAttribute('aria-label', 'Unlike collection')
    })

    it('renders read-only count for anonymous viewers', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({ like_count: 9 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('collection-like-count')).toHaveTextContent('9')
      expect(screen.queryByTestId('collection-like-button')).not.toBeInTheDocument()
    })

    it('calls likeCollection when an unliked heart is clicked', async () => {
      const user = userEvent.setup()
      mockCollection.mockReturnValue({
        data: makeCollection({ like_count: 0, user_likes_this: false }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByTestId('collection-like-button'))
      expect(mockLikeMutate).toHaveBeenCalledWith({ slug: 'test-collection' })
      expect(mockUnlikeMutate).not.toHaveBeenCalled()
    })

    it('calls unlikeCollection when an already-liked heart is clicked', async () => {
      const user = userEvent.setup()
      mockCollection.mockReturnValue({
        data: makeCollection({ like_count: 1, user_likes_this: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByTestId('collection-like-button'))
      expect(mockUnlikeMutate).toHaveBeenCalledWith({ slug: 'test-collection' })
      expect(mockLikeMutate).not.toHaveBeenCalled()
    })
  })

  // ──────────────────────────────────────────────
  // PSY-348: ranked vs. unranked display mode
  // ──────────────────────────────────────────────

  describe('display mode', () => {
    // PSY-360 default flipped to grid view; the legacy ranked-mode
    // assertions below (drag handles, numbered position spans, Move
    // up/down buttons) all live in the list-view rendering path. Force
    // list view via localStorage so this block keeps testing what it
    // was originally written to test (PSY-348 ranked vs unranked).
    beforeEach(() => {
      window.localStorage.setItem('ph-collection-items-view-mode', 'list')
    })

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
  // PSY-371: Edit Collection cover image URL field
  // ──────────────────────────────────────────────

  describe('PSY-371 cover image URL field on edit dialog', () => {
    it('shows an empty cover image URL input when collection has no cover', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ cover_image_url: null }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const input = screen.getByTestId(
        'edit-cover-image-url-input'
      ) as HTMLInputElement
      expect(input).toBeInTheDocument()
      expect(input.value).toBe('')
      // No preview when empty.
      expect(
        screen.queryByTestId('edit-cover-image-url-preview')
      ).not.toBeInTheDocument()
      // No clear button when empty.
      expect(
        screen.queryByTestId('edit-cover-image-url-clear')
      ).not.toBeInTheDocument()
    })

    it('pre-populates the input + preview from the existing cover_image_url', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: 'https://example.com/cover.jpg',
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const input = screen.getByTestId(
        'edit-cover-image-url-input'
      ) as HTMLInputElement
      expect(input.value).toBe('https://example.com/cover.jpg')

      const preview = screen.getByTestId(
        'edit-cover-image-url-preview'
      ) as HTMLImageElement
      expect(preview).toBeInTheDocument()
      expect(preview.src).toBe('https://example.com/cover.jpg')
    })

    it('shows an inline error and hides the preview for an invalid URL', async () => {
      // Use a collection that passes the publish gate so that role="alert"
      // banner doesn't compete with our inline error.
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: null,
          item_count: 5,
          description: 'A '.repeat(40),
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const input = screen.getByTestId('edit-cover-image-url-input')
      await user.type(input, 'not-a-real-url')

      // Error message renders with the helper id and describes the field.
      // Use a stable id rather than role="alert" because the publish-gate
      // banner shares that role.
      const error = document.getElementById('edit-cover-image-url-error')
      expect(error).not.toBeNull()
      expect(error!.textContent).toMatch(/valid URL|http/i)
      expect(input).toHaveAttribute('aria-invalid', 'true')

      // No preview while invalid.
      expect(
        screen.queryByTestId('edit-cover-image-url-preview')
      ).not.toBeInTheDocument()

      // Save button is disabled.
      const saveBtn = screen.getByRole('button', { name: /Save/i })
      expect(saveBtn).toBeDisabled()
    })

    it('rejects non-http(s) protocols (e.g. javascript:)', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: null,
          item_count: 5,
          description: 'A '.repeat(40),
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))
      const input = screen.getByTestId('edit-cover-image-url-input')
      // Use paste rather than type so userEvent doesn't try to interpret the
      // colon as a keyboard shortcut.
      await user.click(input)
      await user.paste('javascript:alert(1)')

      const error = document.getElementById('edit-cover-image-url-error')
      expect(error).not.toBeNull()
      expect(error!.textContent).toMatch(/http/i)
      expect(
        screen.queryByTestId('edit-cover-image-url-preview')
      ).not.toBeInTheDocument()
    })

    it('renders the preview img once a valid URL is entered', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: null,
          item_count: 5,
          description: 'A '.repeat(40),
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const input = screen.getByTestId('edit-cover-image-url-input')
      await user.click(input)
      await user.paste('https://example.com/new-cover.jpg')

      const preview = screen.getByTestId(
        'edit-cover-image-url-preview'
      ) as HTMLImageElement
      expect(preview.src).toBe('https://example.com/new-cover.jpg')
      // No inline cover-image error visible (publish-gate banner may still
      // appear in some collection states; this test uses a passing one to
      // keep the assertion focused).
      expect(
        document.getElementById('edit-cover-image-url-error')
      ).toBeNull()
    })

    it('clear button removes the URL, hides the preview, and is itself hidden', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: 'https://example.com/cover.jpg',
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      // Preview + clear button visible at start.
      expect(
        screen.getByTestId('edit-cover-image-url-preview')
      ).toBeInTheDocument()
      const clearBtn = screen.getByTestId('edit-cover-image-url-clear')
      await user.click(clearBtn)

      const input = screen.getByTestId(
        'edit-cover-image-url-input'
      ) as HTMLInputElement
      expect(input.value).toBe('')
      expect(
        screen.queryByTestId('edit-cover-image-url-preview')
      ).not.toBeInTheDocument()
      // Clear button gone now that the field is empty.
      expect(
        screen.queryByTestId('edit-cover-image-url-clear')
      ).not.toBeInTheDocument()
    })

    it('Save sends explicit null when the cover URL was cleared', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          cover_image_url: 'https://example.com/cover.jpg',
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))
      await user.click(screen.getByTestId('edit-cover-image-url-clear'))
      await user.click(screen.getByRole('button', { name: /Save/i }))

      expect(mockUpdateMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'test-collection',
          cover_image_url: null,
        }),
        expect.any(Object)
      )
    })

    it('Save sends the trimmed URL string when a new URL is entered', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ cover_image_url: null }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))
      const input = screen.getByTestId('edit-cover-image-url-input')
      await user.click(input)
      // Surrounding whitespace should be stripped.
      await user.paste('  https://example.com/cover.jpg  ')

      await user.click(screen.getByRole('button', { name: /Save/i }))

      expect(mockUpdateMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'test-collection',
          cover_image_url: 'https://example.com/cover.jpg',
        }),
        expect.any(Object)
      )
    })

    it('blocks Save when the URL is invalid (mutation never called)', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ cover_image_url: null }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))
      const input = screen.getByTestId('edit-cover-image-url-input')
      await user.type(input, 'not-a-url')

      const saveBtn = screen.getByRole('button', { name: /Save/i })
      expect(saveBtn).toBeDisabled()
      // Even if the user could click it, the mutation must not run.
      await user.click(saveBtn)
      expect(mockUpdateMutate).not.toHaveBeenCalled()
    })

    it('omits the inline error before the user has touched the field', () => {
      // A pre-existing invalid URL on the record shouldn't blast a red error
      // message before the curator has even looked at the field — only after
      // they've interacted (typed or blurred) should the error appear.
      mockCollection.mockReturnValue({
        // Force-set an invalid stored URL via cast since the type doesn't
        // permit non-URL strings in the schema.
        data: makeCollection({
          cover_image_url: 'broken' as unknown as string,
        }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Synchronously re-render in edit mode without touching the input.
      // We just open the form; no role="alert" should be present yet.
      void user.click(screen.getByRole('button', { name: /Edit/i }))
    })
  })

  // ──────────────────────────────────────────────
  // PSY-356: publish-gate banner
  // ──────────────────────────────────────────────

  describe('PSY-356 publish-gate banner', () => {
    it('renders for creator on a public collection below the gate (items + description)', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          is_public: true,
          item_count: 0,
          description: '',
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      const banner = screen.getByTestId('publish-gate-banner')
      expect(banner).toBeInTheDocument()
      // Public + below: emphasises the browse-listing impact.
      expect(banner.textContent).toContain("isn't appearing")
      // Enumerates both gaps.
      expect(banner.textContent).toContain('3 more items')
      expect(banner.textContent).toContain('description of at least 50 characters')
    })

    it('renders pre-publish copy when creator owns a private below-gate collection', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          is_public: false,
          item_count: 1,
          description: 'short',
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      const banner = screen.getByTestId('publish-gate-banner')
      expect(banner.textContent).toContain('Before publishing')
      // Items: 1 of 3 → 2 more items.
      expect(banner.textContent).toContain('2 more items')
      // Existing-but-too-short description uses the "longer description" phrasing.
      expect(banner.textContent).toContain('longer description (50+ characters)')
    })

    it('enumerates only the items gap when description already passes', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          is_public: false,
          item_count: 2,
          description: 'x'.repeat(60),
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      const banner = screen.getByTestId('publish-gate-banner')
      expect(banner.textContent).toContain('1 more item')
      expect(banner.textContent).not.toMatch(/description/)
    })

    it('enumerates only the description gap when items already pass', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          is_public: false,
          item_count: 3,
          description: '',
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      const banner = screen.getByTestId('publish-gate-banner')
      expect(banner.textContent).toContain('description of at least 50 characters')
      expect(banner.textContent).not.toMatch(/more item/)
    })

    it('does not render when the gate passes', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          is_public: true,
          item_count: 3,
          description: 'x'.repeat(60),
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)
      expect(screen.queryByTestId('publish-gate-banner')).not.toBeInTheDocument()
    })

    it('does not render for a non-creator viewer', () => {
      // Viewer is user 999, collection creator is user 1.
      mockAuthContext.mockReturnValue({
        user: { id: '999' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({
          creator_id: 1,
          is_public: true,
          item_count: 0,
          description: '',
        }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)
      expect(screen.queryByTestId('publish-gate-banner')).not.toBeInTheDocument()
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

  // ──────────────────────────────────────────────
  // PSY-372: shows in the Add Items search
  // ──────────────────────────────────────────────

  describe('PSY-372 shows in Add Items search', () => {
    /**
     * Helper: open the Add Items panel and seed the entity-search mock with
     * results in the requested entity type so the dropdown renders rows the
     * user can interact with. Calls userEvent.click on "Add Items" but does
     * not type into the search box — typing is unnecessary because the hook
     * is mocked and returns the seeded data regardless.
     */
    async function openAddItemsWith({
      shows = [],
      artists = [],
    }: {
      shows?: Array<{
        id: number
        slug: string
        name: string
        subtitle: string | null
        entityType: 'show'
        href: string
      }>
      artists?: Array<{
        id: number
        slug: string
        name: string
        subtitle: string | null
        entityType: 'artist'
        href: string
      }>
    }) {
      mockUseEntitySearchResult = {
        data: {
          artists,
          venues: [],
          shows,
          releases: [],
          labels: [],
          festivals: [],
          tags: [],
        },
        isSearching: false,
        totalResults: shows.length + artists.length,
      }
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)
      await user.click(screen.getByRole('button', { name: /Add Items/i }))
      // The Add Items panel only renders the dropdown when the query field
      // has 2+ chars. Type something to satisfy the gate; the hook is mocked
      // so the typed value is irrelevant.
      const input = screen.getByPlaceholderText(/Search artists, shows/)
      await user.type(input, 'tt')
      return user
    }

    it('placeholder copy includes "shows"', async () => {
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)
      await user.click(screen.getByRole('button', { name: /Add Items/i }))

      const input = screen.getByPlaceholderText(
        'Search artists, shows, venues, releases, labels, festivals...'
      )
      expect(input).toBeInTheDocument()
    })

    it('renders show results in the dropdown with the configured label and a "Show" badge', async () => {
      // Synthesize a show entry mirroring how `useEntitySearch` would emit
      // it — name pre-formatted as "{Headliner} @ {Venue} · {Date}".
      const formattedLabel = 'Faetooth @ Valley Bar · Apr 15, 2026'
      await openAddItemsWith({
        shows: [
          {
            id: 99,
            slug: 'faetooth-valley-bar-2026-04-15',
            name: formattedLabel,
            subtitle: null,
            entityType: 'show',
            href: '/shows/faetooth-valley-bar-2026-04-15',
          },
        ],
      })

      // Label rendered verbatim in the dropdown row.
      expect(screen.getByText(formattedLabel)).toBeInTheDocument()
      // "Show" badge appears next to the label.
      expect(screen.getByText('Show')).toBeInTheDocument()
    })

    it('clicking Add on a show calls the add mutation with entityType "show"', async () => {
      const user = await openAddItemsWith({
        shows: [
          {
            id: 99,
            slug: 'faetooth-valley-bar-2026-04-15',
            name: 'Faetooth @ Valley Bar · Apr 15, 2026',
            subtitle: null,
            entityType: 'show',
            href: '/shows/faetooth-valley-bar-2026-04-15',
          },
        ],
      })

      // The Add Items panel triggers a button labeled "Add Items" (which
      // opened the dropdown), and each result row also has an "Add" button.
      // We want the row's button — filter to ones whose accessible name is
      // exactly "Add" (the row button has just "Add", not "Add Items").
      const buttons = screen.getAllByRole('button', { name: 'Add' })
      // There should be exactly one "Add" button (the show row).
      expect(buttons).toHaveLength(1)
      await user.click(buttons[0])

      expect(mockAddItemMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'test-collection',
          entityType: 'show',
          entityId: 99,
        }),
        expect.any(Object)
      )
    })

    it('does not regress: artist results still render with their existing label and badge', async () => {
      await openAddItemsWith({
        artists: [
          {
            id: 1,
            slug: 'the-growlers',
            name: 'The Growlers',
            subtitle: 'Dana Point, CA',
            entityType: 'artist',
            href: '/artists/the-growlers',
          },
        ],
      })

      expect(screen.getByText('The Growlers')).toBeInTheDocument()
      expect(screen.getByText('Dana Point, CA')).toBeInTheDocument()
      expect(screen.getByText('Artist')).toBeInTheDocument()
    })
  })

  // ──────────────────────────────────────────────
  // PSY-360: Grid view + view-mode toggle
  // ──────────────────────────────────────────────

  describe('PSY-360 grid view + view-mode toggle', () => {
    const sampleItems = [
      {
        id: 21,
        entity_type: 'release',
        entity_id: 201,
        entity_name: 'Cover Art Release',
        entity_slug: 'cover-art-release',
        image_url: 'https://example.com/cover.jpg',
        position: 0,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 22,
        entity_type: 'artist',
        entity_id: 202,
        entity_name: 'No-Image Artist',
        entity_slug: 'no-image-artist',
        image_url: null,
        position: 1,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
    ]

    beforeEach(() => {
      // Each test starts with no stored view-mode preference so the default
      // (grid) applies — except where a test explicitly seeds it.
      window.localStorage.removeItem('ph-collection-items-view-mode')
    })

    it('renders grid view by default with one card per item', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      const container = screen.getByTestId('collection-items')
      expect(container).toHaveAttribute('data-view-mode', 'grid')

      // One card per item
      expect(screen.getAllByTestId('collection-item-card')).toHaveLength(2)

      // The release with image_url renders the image variant; the
      // artist without one renders the typed-icon fallback.
      expect(screen.getByTestId('collection-item-card-image')).toBeInTheDocument()
      expect(screen.getByTestId('collection-item-card-fallback')).toBeInTheDocument()
    })

    it('renders the view-mode toggle with both options', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('collection-items-view-toggle')).toBeInTheDocument()
      expect(screen.getByTestId('view-mode-grid')).toBeInTheDocument()
      expect(screen.getByTestId('view-mode-list')).toBeInTheDocument()
    })

    it('clicking the list-view button switches to list rendering', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Sanity: starts in grid.
      expect(screen.getByTestId('collection-items')).toHaveAttribute(
        'data-view-mode',
        'grid'
      )

      await user.click(screen.getByTestId('view-mode-list'))

      expect(screen.getByTestId('collection-items')).toHaveAttribute(
        'data-view-mode',
        'list'
      )
      // Grid cards are gone; list rows have replaced them.
      expect(screen.queryAllByTestId('collection-item-card')).toHaveLength(0)
      // List rendering shows the entity-name link with its added-by line.
      expect(screen.getByText('Cover Art Release')).toBeInTheDocument()
      // Persistence side-effect: the choice was saved.
      expect(
        window.localStorage.getItem('ph-collection-items-view-mode')
      ).toBe('list')
    })

    it('respects a stored list-view preference on mount', () => {
      window.localStorage.setItem('ph-collection-items-view-mode', 'list')
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('collection-items')).toHaveAttribute(
        'data-view-mode',
        'list'
      )
      // No grid cards.
      expect(screen.queryAllByTestId('collection-item-card')).toHaveLength(0)
    })

    it('density toggle is visible only in grid view', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Visible in grid.
      expect(screen.getByTestId('density-toggle-stub')).toBeInTheDocument()

      // Switch to list — density toggle disappears (no effect on list).
      await user.click(screen.getByTestId('view-mode-list'))
      expect(
        screen.queryByTestId('density-toggle-stub')
      ).not.toBeInTheDocument()
    })

    it('grid view in ranked mode shows position badges on each card', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          display_mode: 'ranked',
          items: sampleItems,
        }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const badges = screen.getAllByTestId('collection-item-card-position')
      expect(badges).toHaveLength(2)
      expect(badges[0]).toHaveTextContent('1')
      expect(badges[1]).toHaveTextContent('2')
    })

    it('grid view in unranked mode does NOT show position badges', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({
          display_mode: 'unranked',
          items: sampleItems,
        }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(
        screen.queryAllByTestId('collection-item-card-position')
      ).toHaveLength(0)
    })

    it('density toggle changes the column-grid container class', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      const container = screen.getByTestId('collection-items')
      // Default density is comfortable (ph-density default).
      expect(container.className).toContain('grid-cols-2')
      expect(container.className).toContain('sm:grid-cols-3')

      // Compact density → tighter grid.
      await user.click(screen.getByText('compact'))
      const compactContainer = screen.getByTestId('collection-items')
      expect(compactContainer.className).toContain('grid-cols-3')
      expect(compactContainer.className).toContain('sm:grid-cols-4')

      // Expanded density → wider tiles, fewer columns.
      await user.click(screen.getByText('expanded'))
      const expandedContainer = screen.getByTestId('collection-items')
      expect(expandedContainer.className).toContain('grid-cols-1')
      expect(expandedContainer.className).toContain('sm:grid-cols-2')
    })
  })
})
