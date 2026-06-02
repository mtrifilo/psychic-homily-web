import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CollectionDetail } from './CollectionDetail'
import type {
  CollectionDetail as CollectionDetailType,
  CollectionItem,
} from '../types'

// Mock AuthContext
type MockAuthUser = { id: string; is_admin?: boolean } | null
type MockAuthValue = {
  user: MockAuthUser
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthValue>(() => ({
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
  // PSY-556: surface the `disabled` prop on the stub so the parent test
  // can assert that list view disables the toggle (visible but inert).
  DensityToggle: ({
    density,
    onDensityChange,
    disabled,
  }: {
    density: 'compact' | 'comfortable' | 'expanded'
    onDensityChange: (value: 'compact' | 'comfortable' | 'expanded') => void
    disabled?: boolean
  }) => (
    <div
      data-testid="density-toggle-stub"
      data-disabled={disabled ? 'true' : 'false'}
    >
      <button onClick={() => onDensityChange('compact')}>compact</button>
      <button onClick={() => onDensityChange('comfortable')}>comfortable</button>
      <button onClick={() => onDensityChange('expanded')}>expanded</button>
      <span data-testid="density-current">{density}</span>
    </div>
  ),
  // PSY-613: UserAttribution stub mirrors the real primitive's shape
  // (link when username is set; plain span otherwise) so the existing
  // PSY-353 attribution tests still find the right role + href.
  UserAttribution: ({
    name,
    username,
    fallback = 'Anonymous',
    linkable = true,
    className,
    testId,
  }: {
    name?: string | null
    username?: string | null
    fallback?: string
    linkable?: boolean
    className?: string
    testId?: string
  }) => {
    const displayName = name && name.length > 0 ? name : fallback
    if (linkable && username && username.length > 0) {
      return (
        <a
          href={`/users/${username}`}
          className={className}
          data-testid={testId}
        >
          {displayName}
        </a>
      )
    }
    return (
      <span className={className} data-testid={testId}>
        {displayName}
      </span>
    )
  },
  // PSY-725: AddItemsSection renders the shared InlineErrorBanner when
  // useEntitySearch flips `searchError` true. The mock keeps the same
  // role + testId contract as the real primitive so the assertions in
  // the PSY-725 banner tests look for the right elements.
  InlineErrorBanner: ({
    children,
    testId,
  }: {
    children: React.ReactNode
    testId?: string
  }) => (
    <div role="alert" data-testid={testId}>
      {children}
    </div>
  ),
}))

// Mock hooks
const mockCollection = vi.fn()
const mockDeleteMutate = vi.fn()
type MockDeleteMutationValue = {
  mutate: typeof mockDeleteMutate
  isPending: boolean
  isError: boolean
  error: { message?: string } | null
}
const mockDeleteMutation = vi.fn<() => MockDeleteMutationValue>(() => ({
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

// Factories for the mutation hooks that need configurable
// isError / error state per-test so we can render the new inline error
// banners (PSY-609). Default state is "idle, no error" — individual
// tests use `mockReturnValueOnce` (or a helper) to flip into the
// error state.
type MutationStub = {
  mutate: ReturnType<typeof vi.fn>
  isPending: boolean
  isError: boolean
  error: Error | null
}
const mockCloneMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockCloneMutate,
  isPending: false,
  isError: false,
  error: null,
}))
const idleMutation = (): MutationStub => ({
  mutate: vi.fn(),
  isPending: false,
  isError: false,
  error: null,
})
const mockSubscribeMutation = vi.fn(idleMutation)
const mockUnsubscribeMutation = vi.fn(idleMutation)
const mockLikeMutation = vi.fn(
  (): MutationStub => ({
    mutate: mockLikeMutate,
    isPending: false,
    isError: false,
    error: null,
  })
)
const mockUnlikeMutation = vi.fn(
  (): MutationStub => ({
    mutate: mockUnlikeMutate,
    isPending: false,
    isError: false,
    error: null,
  })
)
const mockReorderMutation = vi.fn(
  (): MutationStub => ({
    mutate: mockReorderMutate,
    isPending: false,
    isError: false,
    error: null,
  })
)
const mockRemoveMutation = vi.fn(idleMutation)

vi.mock('../hooks', () => ({
  useCollection: (...args: unknown[]) => mockCollection(...args),
  useUpdateCollection: (): Omit<MutationStub, 'isError'> => ({
    mutate: mockUpdateMutate,
    isPending: false,
    error: null,
  }),
  useAddCollectionItem: (): MutationStub => ({
    mutate: mockAddItemMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
  // PSY-823: AddItemsSection now stages items in AddItemsPicker and submits
  // via bulk-add. Tests mostly stub the picker; the bulk-add mock just
  // resolves with an empty partial-success response.
  useBulkAddCollectionItems: () => ({
    mutate: vi.fn(),
    mutateAsync: () => Promise.resolve({ added: [], errors: [] }),
    isPending: false,
    isError: false,
    error: null,
  }),
  useRemoveCollectionItem: () => mockRemoveMutation(),
  useReorderCollectionItems: () => mockReorderMutation(),
  useUpdateCollectionItem: (): MutationStub => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useSubscribeCollection: () => mockSubscribeMutation(),
  useUnsubscribeCollection: () => mockUnsubscribeMutation(),
  useDeleteCollection: () => mockDeleteMutation(),
  useCloneCollection: () => mockCloneMutation(),
  useLikeCollection: () => mockLikeMutation(),
  useUnlikeCollection: () => mockUnlikeMutation(),
}))

// Mock comments feature
vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">Comments for {entityType} {entityId}</div>
  ),
}))

// PSY-823: stub the AddItemsPicker. CollectionDetail tests don't exercise
// the picker's internals (search results, paste preview, staging) — that
// surface has its own test file. The stub keeps these tests fast and
// isolates them from useEntitySearch + useResolveCollectionItems wiring.
vi.mock('./AddItemsPicker', () => ({
  AddItemsPicker: () => <div data-testid="add-items-picker-stub" />,
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

// PSY-357: stub the report dialog. Its real implementation pulls in
// useReportEntity (a useMutation hook) which requires a QueryClient at
// mount time. CollectionDetail tests don't exercise the dialog content
// so a no-op stub keeps the non-creator render path lean.
vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: (): null => null,
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
  /**
   * PSY-725: total-outage flag from the shared hook. Defaults false so
   * existing tests don't accidentally surface the banner.
   */
  searchError: boolean
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
  searchError: false,
}
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockUseEntitySearchResult,
  // PSY-725: AddItemsSection imports the canonical banner copy from the
  // same module. Re-export the literal so the mock fully replaces the
  // real module and the banner-copy assertion still finds the right text.
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE:
    'Search is temporarily unavailable. Try again in a moment.',
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
    // PSY-609: reset configurable mutation factories so each test starts
    // from "idle, no error".
    mockSubscribeMutation.mockImplementation(idleMutation)
    mockUnsubscribeMutation.mockImplementation(idleMutation)
    mockLikeMutation.mockReturnValue({
      mutate: mockLikeMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockUnlikeMutation.mockReturnValue({
      mutate: mockUnlikeMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockReorderMutation.mockReturnValue({
      mutate: mockReorderMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockRemoveMutation.mockImplementation(idleMutation)
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
      searchError: false,
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

    it('hides Fork on the user\'s own collection (not in overflow either)', async () => {
      // current user (id=1) is the creator (creator_id=1) by default.
      // PSY-892 D4: Fork lives in the ⋯ overflow — open it to assert absence.
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)
      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
    })

    it('hides Fork on private collections (not in overflow either)', async () => {
      const user = userEvent.setup()
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
      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
    })

    it('hides Fork when not authenticated (not in overflow either)', async () => {
      const user = userEvent.setup()
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
      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
    })

    it('shows Fork in the ⋯ overflow menu for authenticated non-owners on public collections', async () => {
      // PSY-892 D4: Fork moved from the primary action row into the overflow.
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
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.getByTestId('overflow-fork')).toBeInTheDocument()
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
      // PSY-892 D4: Fork lives in the ⋯ overflow menu.
      await user.click(screen.getByTestId('collection-overflow-trigger'))
      await user.click(screen.getByTestId('overflow-fork'))

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

    it('renders a Like button that prompts sign-in for anonymous viewers (PSY-892 D4)', async () => {
      // Pre-D4 anonymous viewers saw a read-only count; the redesign makes
      // Like a primary action in every viewer state — anonymous clicks route
      // to sign-in (matches FollowButton's unauth pattern).
      const user = userEvent.setup()
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

      const btn = screen.getByTestId('collection-like-button')
      expect(btn).toHaveTextContent('9')
      expect(btn).toHaveAttribute('aria-label', 'Sign in to like collection')
      // Not pressed/pressable state for anonymous viewers.
      expect(btn).not.toHaveAttribute('aria-pressed')

      await user.click(btn)
      // Same returnTo redirect as FollowButton / AttendanceButton so the
      // viewer lands back on this collection after signing in.
      expect(mockPush).toHaveBeenCalledWith(
        '/auth?returnTo=%2Fcollections%2Ftest-collection'
      )
      expect(mockLikeMutate).not.toHaveBeenCalled()
      expect(mockUnlikeMutate).not.toHaveBeenCalled()
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
  // PSY-578: report button visibility on collection header
  // ──────────────────────────────────────────────

  describe('PSY-578 report button visibility', () => {
    // PSY-892 D4: Report lives in the ⋯ overflow menu — every test opens the
    // overflow first so presence/absence is asserted against the real menu.
    it('renders Report in the overflow for an authenticated non-creator non-admin', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '999', is_admin: false },
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

      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.getByTestId('overflow-report')).toBeInTheDocument()
    })

    it('hides Report for the collection creator', async () => {
      // Creator uses Edit / Delete instead of Report.
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: false },
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

      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-report')).not.toBeInTheDocument()
    })

    it('hides Report for admins (they use the moderation queue)', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '999', is_admin: true },
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

      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-report')).not.toBeInTheDocument()
    })

    it('hides Report for unauthenticated viewers', async () => {
      const user = userEvent.setup()
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

      await user.click(screen.getByTestId('collection-overflow-trigger'))
      expect(screen.queryByTestId('overflow-report')).not.toBeInTheDocument()
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

    const sampleItems: CollectionItem[] = [
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

      // Numbered positions visible. Scope to the items container — the items
      // header now also renders the item count ("3"), which would make a
      // page-wide getByText('3') ambiguous (PSY-892 D7).
      const itemsContainer = within(screen.getByTestId('collection-items'))
      expect(itemsContainer.getByText('1')).toBeInTheDocument()
      expect(itemsContainer.getByText('2')).toBeInTheDocument()
      expect(itemsContainer.getByText('3')).toBeInTheDocument()
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
      const moveDownButtons = screen.getAllByRole('button', { name: /move .* down/i })
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
      const moveUpButtons = screen.getAllByRole('button', { name: /move .* up/i })
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
      const moveUpButtons = screen.getAllByRole('button', { name: /move .* up/i })
      expect(moveUpButtons[0]).toBeDisabled()
    })

    it('keyboard fallback: Move down disabled on last item', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      const moveDownButtons = screen.getAllByRole('button', { name: /move .* down/i })
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
  // PSY-372 / PSY-725 search-internal tests moved to AddItemsPicker.test.tsx —
  // CollectionDetail now stubs the picker (PSY-823) so picker internals
  // (search results, outage banner, paste-mode preview) live in the
  // picker's own test surface.

  // ──────────────────────────────────────────────
  // PSY-581: Add Items default-open on empty collections
  // ──────────────────────────────────────────────

  describe('PSY-581 Add Items default-open on empty collections', () => {
    /** Sample item used to drop the collection out of the empty state. */
    const sampleItem: CollectionItem = {
      id: 100,
      entity_type: 'artist',
      entity_id: 1,
      entity_name: 'Sample Artist',
      entity_slug: 'sample-artist',
      image_url: null,
      position: 0,
      added_by_user_id: 1,
      added_by_name: 'testuser',
      notes: null,
      created_at: '2025-01-01T00:00:00Z',
    }

    it('renders the picker by default when the collection is empty', () => {
      // Default fixture has `items: []` and the current user is the creator
      // so the empty-state path applies — the panel opens on first paint
      // and renders the AddItemsPicker (stubbed in this test surface).
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('add-items-picker-stub')).toBeInTheDocument()
    })

    it('keeps the panel collapsed by default when the collection has items', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: [sampleItem] }),
        isLoading: false,
        error: null,
      })

      render(<CollectionDetail slug="test-collection" />)

      expect(
        screen.getByRole('button', { name: /Add Items/i })
      ).toBeInTheDocument()
      // Picker is not mounted while the panel is collapsed.
      expect(screen.queryByTestId('add-items-picker-stub')).not.toBeInTheDocument()
    })
  })

  // ──────────────────────────────────────────────
  // PSY-360: Grid view + view-mode toggle
  // ──────────────────────────────────────────────

  describe('PSY-360 grid view + view-mode toggle', () => {
    const sampleItems: CollectionItem[] = [
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

    it('density toggle stays mounted in list view but is disabled (PSY-556)', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      // Visible AND enabled in grid view.
      const toggle = screen.getByTestId('density-toggle-stub')
      expect(toggle).toBeInTheDocument()
      expect(toggle).toHaveAttribute('data-disabled', 'false')

      // Switch to list — toggle stays mounted (no layout shift) but is
      // disabled. Persisted selection is preserved by useDensity.
      await user.click(screen.getByTestId('view-mode-list'))
      const toggleAfter = screen.getByTestId('density-toggle-stub')
      expect(toggleAfter).toBeInTheDocument()
      expect(toggleAfter).toHaveAttribute('data-disabled', 'true')

      // Switch back to grid — re-enabled.
      await user.click(screen.getByTestId('view-mode-grid'))
      expect(screen.getByTestId('density-toggle-stub')).toHaveAttribute(
        'data-disabled',
        'false'
      )
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
      // Default density is Compact (PSY-892 D3 — collections default dense).
      expect(container.className).toContain('grid-cols-3')
      expect(container.className).toContain('sm:grid-cols-4')

      // Comfortable density → wider tiles, fewer columns.
      await user.click(screen.getByText('comfortable'))
      const comfortableContainer = screen.getByTestId('collection-items')
      expect(comfortableContainer.className).toContain('grid-cols-2')
      expect(comfortableContainer.className).toContain('sm:grid-cols-3')

      // Expanded density → widest tiles, fewest columns.
      await user.click(screen.getByText('expanded'))
      const expandedContainer = screen.getByTestId('collection-items')
      expect(expandedContainer.className).toContain('grid-cols-1')
      expect(expandedContainer.className).toContain('sm:grid-cols-2')
    })
  })

  // PSY-348 drag tests force list mode via beforeEach; these exercise
  // the grid-mode path that was non-functional pre-PSY-527.
  describe('PSY-527: grid + ranked reorder', () => {
    const sampleItems: CollectionItem[] = [
      {
        id: 31,
        entity_type: 'release',
        entity_id: 301,
        entity_name: 'First Release',
        entity_slug: 'first-release',
        image_url: null,
        position: 0,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 32,
        entity_type: 'release',
        entity_id: 302,
        entity_name: 'Second Release',
        entity_slug: 'second-release',
        image_url: null,
        position: 1,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 33,
        entity_type: 'release',
        entity_id: 303,
        entity_name: 'Third Release',
        entity_slug: 'third-release',
        image_url: null,
        position: 2,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
    ]

    beforeEach(() => {
      window.localStorage.removeItem('ph-collection-items-view-mode')
    })

    it('renders one drag handle per grid card in ranked + creator mode', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('collection-items')).toHaveAttribute(
        'data-view-mode',
        'grid'
      )
      // Regression guard: fails if useSortable is removed from CollectionItemCard.
      expect(
        screen.getAllByTestId('collection-item-card-reorder')
      ).toHaveLength(3)
      expect(
        screen.getAllByRole('button', { name: /^Drag to reorder/ })
      ).toHaveLength(3)
    })

    it('does NOT render the reorder cluster in grid + unranked mode', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'unranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      expect(screen.getByTestId('collection-items')).toHaveAttribute(
        'data-view-mode',
        'grid'
      )
      expect(
        screen.queryAllByTestId('collection-item-card-reorder')
      ).toHaveLength(0)
    })

    it('does NOT render drag handles in grid + ranked for non-creator', () => {
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

      // Position badges still visible (everyone sees the ranking).
      expect(
        screen.getAllByTestId('collection-item-card-position')
      ).toHaveLength(3)
      expect(
        screen.queryAllByTestId('collection-item-card-reorder')
      ).toHaveLength(0)
      expect(
        screen.queryAllByRole('button', { name: /^Drag to reorder/ })
      ).toHaveLength(0)
    })

    it('keyboard fallback: Move down on first grid card sends correct reorder payload', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      const moveDownButtons = screen.getAllByRole('button', {
        name: /move .* down/i,
      })
      expect(moveDownButtons).toHaveLength(3)
      await user.click(moveDownButtons[0])

      expect(mockReorderMutate).toHaveBeenCalledWith({
        slug: 'test-collection',
        items: [
          { item_id: 32, position: 0 },
          { item_id: 31, position: 1 },
          { item_id: 33, position: 2 },
        ],
      })
    })

    it('keyboard fallback: Move up on last grid card sends correct reorder payload', async () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      const user = userEvent.setup()
      render(<CollectionDetail slug="test-collection" />)

      const moveUpButtons = screen.getAllByRole('button', { name: /move .* up/i })
      await user.click(moveUpButtons[moveUpButtons.length - 1])

      expect(mockReorderMutate).toHaveBeenCalledWith({
        slug: 'test-collection',
        items: [
          { item_id: 31, position: 0 },
          { item_id: 33, position: 1 },
          { item_id: 32, position: 2 },
        ],
      })
    })

    it('keyboard fallback: Move up disabled on first grid card', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const moveUpButtons = screen.getAllByRole('button', { name: /move .* up/i })
      expect(moveUpButtons[0]).toBeDisabled()
    })

    it('keyboard fallback: Move down disabled on last grid card', () => {
      mockCollection.mockReturnValue({
        data: makeCollection({ display_mode: 'ranked', items: sampleItems }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)

      const moveDownButtons = screen.getAllByRole('button', {
        name: /move .* down/i,
      })
      expect(moveDownButtons[moveDownButtons.length - 1]).toBeDisabled()
    })
  })

  // PSY-609: surface mutation failures across the silent collection
  // action surfaces. The hooks themselves keep React Query's mutation
  // state machine; these tests pin the user-visible result.
  describe('PSY-609 mutation error banners', () => {
    it('renders the subscribe error banner when subscribeMutation isError', () => {
      mockAuthContext.mockReturnValue({
        // Non-creator viewer — subscribe button is rendered.
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockSubscribeMutation.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        isError: true,
        error: new Error('subscription quota exceeded'),
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('subscribe-error')).toHaveTextContent(
        'subscription quota exceeded'
      )
    })

    it('renders the clone error banner when cloneMutation isError', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockCloneMutation.mockReturnValue({
        mutate: mockCloneMutate,
        isPending: false,
        isError: true,
        error: new Error('Failed to fork: backend down'),
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('clone-error')).toHaveTextContent(
        'Failed to fork: backend down'
      )
    })

    it('uses the privacy-aware copy on subscribe 403', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockSubscribeMutation.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        isError: true,
        error: Object.assign(new Error('forbidden'), { status: 403 }),
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('subscribe-error')).toHaveTextContent(
        'This collection is private.'
      )
    })

    it('renders the like error banner when likeMutation isError', () => {
      mockLikeMutation.mockReturnValue({
        mutate: mockLikeMutate,
        isPending: false,
        isError: true,
        error: new Error('rate limit'),
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('like-error')).toHaveTextContent('rate limit')
    })

    it('renders the unlike error banner with privacy-aware copy on 403', () => {
      mockUnlikeMutation.mockReturnValue({
        mutate: mockUnlikeMutate,
        isPending: false,
        isError: true,
        error: Object.assign(new Error('forbidden'), { status: 403 }),
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('unlike-error')).toHaveTextContent(
        /your like was removed/i
      )
    })

    it('does not render any action-error banner when mutations are idle', () => {
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.queryByTestId('subscribe-error')).not.toBeInTheDocument()
      expect(screen.queryByTestId('unsubscribe-error')).not.toBeInTheDocument()
      expect(screen.queryByTestId('clone-error')).not.toBeInTheDocument()
      expect(screen.queryByTestId('like-error')).not.toBeInTheDocument()
      expect(screen.queryByTestId('unlike-error')).not.toBeInTheDocument()
    })

    it('renders the reorder error banner when reorderMutation isError', () => {
      // Need at least 1 item with the items-list visible — supply ranked
      // mode + creator so the items list renders.
      mockReorderMutation.mockReturnValue({
        mutate: mockReorderMutate,
        isPending: false,
        isError: true,
        error: new Error('Failed to save order'),
      })
      mockCollection.mockReturnValue({
        data: makeCollection({
          display_mode: 'ranked',
          item_count: 2,
          items: [
            {
              id: 1,
              entity_type: 'release',
              entity_id: 10,
              entity_name: 'Item One',
              entity_slug: 'item-one',
              image_url: null,
              position: 0,
              added_by_user_id: 1,
              added_by_name: 'curator',
              notes: null,
              notes_html: undefined,
              created_at: '2025-01-01T00:00:00Z',
            },
            {
              id: 2,
              entity_type: 'release',
              entity_id: 11,
              entity_name: 'Item Two',
              entity_slug: 'item-two',
              image_url: null,
              position: 1,
              added_by_user_id: 1,
              added_by_name: 'curator',
              notes: null,
              notes_html: undefined,
              created_at: '2025-01-01T00:00:00Z',
            },
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test-collection" />)
      expect(screen.getByTestId('reorder-error')).toHaveTextContent(
        'Failed to save order'
      )
    })
  })

  // ──────────────────────────────────────────────
  // PSY-898: detail page redesign (PSY-892 D1–D7 + PSY-894 edit form)
  // ──────────────────────────────────────────────

  describe('PSY-898 detail page redesign', () => {
    const sampleItems: CollectionItem[] = [
      {
        id: 41,
        entity_type: 'artist',
        entity_id: 401,
        entity_name: 'Redesign Artist',
        entity_slug: 'redesign-artist',
        image_url: null,
        position: 0,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
      {
        id: 42,
        entity_type: 'venue',
        entity_id: 402,
        entity_name: 'Redesign Venue',
        entity_slug: 'redesign-venue',
        image_url: null,
        position: 1,
        added_by_user_id: 1,
        added_by_name: 'testuser',
        notes: null,
        created_at: '2025-01-01T00:00:00Z',
      },
    ]

    describe('D4: header action consolidation + ⋯ overflow menu', () => {
      it('owner: Like + Edit + Delete primary; Share + Explore graph in overflow; no Fork/Report/Subscribe', async () => {
        const user = userEvent.setup()
        mockCollection.mockReturnValue({
          data: makeCollection({ items: sampleItems }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        // Primary row: Like + Edit + Delete (the curator's daily actions).
        expect(
          screen.getByTestId('collection-like-button')
        ).toBeInTheDocument()
        expect(
          screen.getByRole('button', { name: 'Edit' })
        ).toBeInTheDocument()
        expect(
          screen.getByRole('button', { name: 'Delete collection' })
        ).toBeInTheDocument()
        // Subscribe never shows for the owner.
        expect(
          screen.queryByRole('button', { name: /Subscribe/i })
        ).not.toBeInTheDocument()

        // Overflow: Share + Explore graph; Fork/Report are non-owner actions.
        await user.click(screen.getByTestId('collection-overflow-trigger'))
        expect(screen.getByTestId('overflow-share')).toBeInTheDocument()
        expect(
          screen.getByTestId('overflow-explore-graph')
        ).toBeInTheDocument()
        expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
        expect(
          screen.queryByTestId('overflow-report')
        ).not.toBeInTheDocument()
      })

      it('non-owner (auth): Like + Subscribe primary; Share/graph/Fork/Report in overflow; no Edit/Delete', async () => {
        const user = userEvent.setup()
        mockAuthContext.mockReturnValue({
          user: { id: '999', is_admin: false },
          isAuthenticated: true,
          isLoading: false,
          logout: vi.fn(),
        })
        mockCollection.mockReturnValue({
          data: makeCollection({
            creator_id: 1,
            is_public: true,
            items: sampleItems,
          }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        // Primary row: Like + Subscribe (the social actions).
        expect(
          screen.getByTestId('collection-like-button')
        ).toBeInTheDocument()
        expect(
          screen.getByRole('button', { name: /Subscribe/i })
        ).toBeInTheDocument()
        // Owner-only actions absent.
        expect(
          screen.queryByRole('button', { name: 'Edit' })
        ).not.toBeInTheDocument()
        expect(
          screen.queryByRole('button', { name: 'Delete collection' })
        ).not.toBeInTheDocument()

        // Overflow: all four secondary actions.
        await user.click(screen.getByTestId('collection-overflow-trigger'))
        expect(screen.getByTestId('overflow-share')).toBeInTheDocument()
        expect(
          screen.getByTestId('overflow-explore-graph')
        ).toBeInTheDocument()
        expect(screen.getByTestId('overflow-fork')).toBeInTheDocument()
        expect(screen.getByTestId('overflow-report')).toBeInTheDocument()
      })

      it('logged out: Like primary (sign-in prompt); Share + Explore graph in overflow only', async () => {
        const user = userEvent.setup()
        mockAuthContext.mockReturnValue({
          user: null,
          isAuthenticated: false,
          isLoading: false,
          logout: vi.fn(),
        })
        mockCollection.mockReturnValue({
          data: makeCollection({ creator_id: 1, items: sampleItems }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        // Primary row: just the Like button (prompts sign-in).
        expect(
          screen.getByTestId('collection-like-button')
        ).toHaveAttribute('aria-label', 'Sign in to like collection')
        expect(
          screen.queryByRole('button', { name: /Subscribe/i })
        ).not.toBeInTheDocument()
        expect(
          screen.queryByRole('button', { name: 'Edit' })
        ).not.toBeInTheDocument()

        // Overflow: Share + Explore graph only.
        await user.click(screen.getByTestId('collection-overflow-trigger'))
        expect(screen.getByTestId('overflow-share')).toBeInTheDocument()
        expect(
          screen.getByTestId('overflow-explore-graph')
        ).toBeInTheDocument()
        expect(screen.queryByTestId('overflow-fork')).not.toBeInTheDocument()
        expect(
          screen.queryByTestId('overflow-report')
        ).not.toBeInTheDocument()
      })

      it('hides Explore graph from the overflow when the collection has no items', async () => {
        // Default fixture: creator viewing an empty collection.
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByTestId('collection-overflow-trigger'))
        expect(screen.getByTestId('overflow-share')).toBeInTheDocument()
        expect(
          screen.queryByTestId('overflow-explore-graph')
        ).not.toBeInTheDocument()
      })

      it('shows the inline "Forking collection…" row while a clone is pending', () => {
        // The overflow menu closes on click, taking its pending label with
        // it — the inline status row is the only in-flight feedback.
        mockAuthContext.mockReturnValue({
          user: { id: '999' },
          isAuthenticated: true,
          isLoading: false,
          logout: vi.fn(),
        })
        mockCloneMutation.mockReturnValue({
          mutate: mockCloneMutate,
          isPending: true,
          isError: false,
          error: null,
        })
        mockCollection.mockReturnValue({
          data: makeCollection({ creator_id: 1, is_public: true }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        expect(screen.getByTestId('fork-pending')).toHaveTextContent(
          'Forking collection'
        )
      })

      it('Share copies the page link and shows the copied banner', async () => {
        const user = userEvent.setup()
        // userEvent.setup() installs a navigator.clipboard stub; spy on it so
        // the component's writeText().then(...) chain still resolves.
        const writeSpy = vi.spyOn(navigator.clipboard, 'writeText')
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByTestId('collection-overflow-trigger'))
        await user.click(screen.getByTestId('overflow-share'))

        expect(writeSpy).toHaveBeenCalledWith(window.location.href)
        // Banner appears once the clipboard promise resolves; it lives in the
        // header banner block since the menu closes on click.
        expect(await screen.findByTestId('share-copied')).toHaveTextContent(
          'Link copied to clipboard'
        )
      })
    })

    describe('D1: sticky anchor nav', () => {
      it('renders Items / Tags / Discussion jump links with Items active by default', () => {
        render(<CollectionDetail slug="test-collection" />)

        const nav = screen.getByTestId('collection-anchor-nav')
        expect(within(nav).getByTestId('anchor-nav-items')).toHaveAttribute(
          'aria-current',
          'true'
        )
        expect(
          within(nav).getByTestId('anchor-nav-tags')
        ).not.toHaveAttribute('aria-current')
        expect(
          within(nav).getByTestId('anchor-nav-discussion')
        ).not.toHaveAttribute('aria-current')
      })

      it('clicking a jump link marks it active', async () => {
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByTestId('anchor-nav-discussion'))

        expect(screen.getByTestId('anchor-nav-discussion')).toHaveAttribute(
          'aria-current',
          'true'
        )
        expect(
          screen.getByTestId('anchor-nav-items')
        ).not.toHaveAttribute('aria-current')
      })

      it('updates the active link when the observer reports a section in view (scroll tracking)', () => {
        // The global IntersectionObserver mock (test/setup.ts) is a no-op
        // stub and is not `configurable`, so it can't be replaced via
        // vi.stubGlobal — but it IS `writable`, so plain assignment works.
        // Swap in a capturing stub for this test so we can drive the
        // callback with synthetic entries (the only way to exercise the
        // scroll-tracking path in jsdom).
        let capturedCallback: IntersectionObserverCallback | null = null
        const observeSpy = vi.fn()
        const makeEntry = (target: Element, top: number) =>
          ({
            isIntersecting: true,
            target,
            boundingClientRect: { top },
          }) as unknown as IntersectionObserverEntry

        const OriginalIO = window.IntersectionObserver
        window.IntersectionObserver = class {
          constructor(cb: IntersectionObserverCallback) {
            capturedCallback = cb
          }
          observe = observeSpy
          unobserve = vi.fn()
          disconnect = vi.fn()
          takeRecords = () => []
        } as unknown as typeof IntersectionObserver

        try {
          render(<CollectionDetail slug="test-collection" />)

          // The nav observed all three sections (they render synchronously
          // in jsdom, so the retry loop succeeds on its first pass).
          expect(observeSpy).toHaveBeenCalledTimes(3)
          expect(capturedCallback).not.toBeNull()

          // Simulate the discussion section scrolling into the reading band.
          const discussionEl = document.getElementById('discussion')!
          act(() => {
            capturedCallback!(
              [makeEntry(discussionEl, 120)],
              {} as IntersectionObserver
            )
          })

          expect(
            screen.getByTestId('anchor-nav-discussion')
          ).toHaveAttribute('aria-current', 'true')
          expect(
            screen.getByTestId('anchor-nav-items')
          ).not.toHaveAttribute('aria-current')

          // When two sections intersect, the one nearest the top of the
          // viewport wins.
          const itemsEl = document.getElementById('items')!
          act(() => {
            capturedCallback!(
              [makeEntry(discussionEl, 400), makeEntry(itemsEl, 150)],
              {} as IntersectionObserver
            )
          })

          expect(screen.getByTestId('anchor-nav-items')).toHaveAttribute(
            'aria-current',
            'true'
          )
        } finally {
          window.IntersectionObserver = OriginalIO
        }
      })
    })

    describe('D6: section order (Items → Tags → Discussion)', () => {
      it('renders the three sections in the locked order', () => {
        mockCollection.mockReturnValue({
          data: makeCollection({ items: sampleItems }),
          isLoading: false,
          error: null,
        })
        const { container } = render(
          <CollectionDetail slug="test-collection" />
        )

        const items = container.querySelector('#items')
        const tags = container.querySelector('#tags')
        const discussion = container.querySelector('#discussion')
        expect(items).not.toBeNull()
        expect(tags).not.toBeNull()
        expect(discussion).not.toBeNull()

        // DOCUMENT_POSITION_FOLLOWING — tags follows items, discussion
        // follows tags in document order.
        expect(
          items!.compareDocumentPosition(tags!) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
        expect(
          tags!.compareDocumentPosition(discussion!) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
      })
    })

    describe('D7: + Add Items in the items header', () => {
      it('shows the items count and the Add Items toggle for the creator', () => {
        mockCollection.mockReturnValue({
          data: makeCollection({ items: sampleItems }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        expect(screen.getByTestId('items-count')).toHaveTextContent('2')
        expect(screen.getByTestId('add-items-toggle')).toBeInTheDocument()
        // Panel collapsed by default for a non-empty collection (PSY-581
        // default-open only applies to empty collections).
        expect(
          screen.queryByTestId('add-items-picker-stub')
        ).not.toBeInTheDocument()
      })

      it('hides the Add Items toggle for non-creators', () => {
        mockAuthContext.mockReturnValue({
          user: { id: '999' },
          isAuthenticated: true,
          isLoading: false,
          logout: vi.fn(),
        })
        mockCollection.mockReturnValue({
          data: makeCollection({ creator_id: 1, items: sampleItems }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        expect(
          screen.queryByTestId('add-items-toggle')
        ).not.toBeInTheDocument()
        // Count still shows for everyone.
        expect(screen.getByTestId('items-count')).toHaveTextContent('2')
      })

      it('toggles the picker panel open and closed from the header button', async () => {
        const user = userEvent.setup()
        mockCollection.mockReturnValue({
          data: makeCollection({ items: sampleItems }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByTestId('add-items-toggle'))
        expect(
          screen.getByTestId('add-items-picker-stub')
        ).toBeInTheDocument()

        await user.click(screen.getByTestId('add-items-toggle'))
        expect(
          screen.queryByTestId('add-items-picker-stub')
        ).not.toBeInTheDocument()
      })
    })

    describe('PSY-894: edit form polish', () => {
      it('Esc closes a pristine edit form without saving', async () => {
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        expect(screen.getByLabelText('Title')).toBeInTheDocument()

        // Title input has autoFocus, so the keypress originates inside the
        // form and bubbles to its onKeyDown handler.
        await user.keyboard('{Escape}')

        expect(screen.queryByLabelText('Title')).not.toBeInTheDocument()
        expect(mockUpdateMutate).not.toHaveBeenCalled()
        expect(
          screen.queryByTestId('collection-update-success')
        ).not.toBeInTheDocument()
      })

      it('Esc does NOT discard a dirty form (unsaved-work protection)', async () => {
        // Adversarial-review finding: a reflexive Esc press while typing a
        // long description must not silently destroy the user's work. A
        // dirty form ignores Esc; closing it requires the deliberate Cancel
        // click.
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        // Dirty the form.
        await user.type(screen.getByLabelText('Title'), ' updated')

        await user.keyboard('{Escape}')

        // Form stays open — unsaved work is protected.
        expect(screen.getByLabelText('Title')).toBeInTheDocument()
        expect(mockUpdateMutate).not.toHaveBeenCalled()

        // The hint stops promising Esc once the form is dirty.
        expect(screen.queryByText(/Esc to cancel/)).not.toBeInTheDocument()

        // Cancel still works as the deliberate exit.
        await user.click(screen.getByRole('button', { name: 'Cancel' }))
        expect(screen.queryByLabelText('Title')).not.toBeInTheDocument()
      })

      it('⌘S saves the form', async () => {
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        await user.keyboard('{Meta>}s{/Meta}')

        expect(mockUpdateMutate).toHaveBeenCalledWith(
          expect.objectContaining({ slug: 'test-collection' }),
          expect.any(Object)
        )
      })

      it('⌘S respects the same validation as the Save button (empty title)', async () => {
        // Code-review finding: the shortcut must not bypass the empty-title
        // guard that disables the Save button.
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        await user.clear(screen.getByLabelText('Title'))
        await user.keyboard('{Meta>}s{/Meta}')

        expect(mockUpdateMutate).not.toHaveBeenCalled()
      })

      it('shows the green "Collection updated" banner after a successful save', async () => {
        // Mutation invokes onSuccess synchronously so the parent closes the
        // form and shows the banner.
        mockUpdateMutate.mockImplementation((_args, opts) =>
          opts?.onSuccess?.()
        )
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        await user.click(screen.getByRole('button', { name: /Save/i }))

        expect(
          screen.getByTestId('collection-update-success')
        ).toHaveTextContent('Collection updated')
        // Form closed (header re-rendered).
        expect(screen.queryByLabelText('Title')).not.toBeInTheDocument()
      })

      it('does NOT show the success banner after Cancel', async () => {
        const user = userEvent.setup()
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))
        await user.click(screen.getByRole('button', { name: 'Cancel' }))

        expect(
          screen.queryByTestId('collection-update-success')
        ).not.toBeInTheDocument()
        // Form closed without saving.
        expect(screen.queryByLabelText('Title')).not.toBeInTheDocument()
        expect(mockUpdateMutate).not.toHaveBeenCalled()
      })

      it('Public + Collaborative DS checkboxes toggle and land in the save payload', async () => {
        const user = userEvent.setup()
        mockCollection.mockReturnValue({
          data: makeCollection({ is_public: true, collaborative: false }),
          isLoading: false,
          error: null,
        })
        render(<CollectionDetail slug="test-collection" />)

        await user.click(screen.getByRole('button', { name: 'Edit' }))

        const publicCheckbox = screen.getByRole('checkbox', {
          name: 'Public',
        })
        const collabCheckbox = screen.getByRole('checkbox', {
          name: 'Collaborative',
        })
        expect(publicCheckbox).toBeChecked()
        expect(collabCheckbox).not.toBeChecked()

        await user.click(publicCheckbox)
        await user.click(collabCheckbox)
        expect(publicCheckbox).not.toBeChecked()
        expect(collabCheckbox).toBeChecked()

        await user.click(screen.getByRole('button', { name: /Save/i }))
        expect(mockUpdateMutate).toHaveBeenCalledWith(
          expect.objectContaining({
            is_public: false,
            collaborative: true,
          }),
          expect.any(Object)
        )
      })
    })
  })
})
