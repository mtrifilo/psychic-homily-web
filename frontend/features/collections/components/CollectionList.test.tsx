import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CollectionList } from './CollectionList'
import type { Collection } from '../types'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
}))

// Mock use-debounce to fire immediately
vi.mock('use-debounce', () => ({
  useDebounce: (value: unknown) => [value],
}))

// Mock collection hooks
const mockUseCollections = vi.fn()
const mockUseMyCollections = vi.fn()
const mockCreateMutate = vi.fn()

vi.mock('../hooks', () => ({
  useCollections: (params: unknown) => mockUseCollections(params),
  useMyCollections: () => mockUseMyCollections(),
  useCreateCollection: () => ({
    mutate: mockCreateMutate,
    isPending: false,
    error: null,
  }),
}))

// Mock child components
vi.mock('./CollectionCard', () => ({
  CollectionCard: ({ collection }: { collection: Collection }) => (
    <article data-testid={`collection-card-${collection.id}`}>{collection.title}</article>
  ),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, onClick, disabled, ...props }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button onClick={onClick} disabled={disabled} type={props.type as 'button' | 'reset' | 'submit' | undefined}>{children}</button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
}))

vi.mock('@/components/ui/textarea', () => ({
  Textarea: (props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...props} />,
}))

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) => (
    <div data-testid="dialog" data-open={open}>{children}</div>
  ),
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dialog-content">{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogTrigger: ({ children }: { children: React.ReactNode; asChild?: boolean }) => (
    <>{children}</>
  ),
}))

// Track the active tab value for selective rendering of TabsContent
let activeTabValue = 'all'

vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({ children, value, onValueChange }: {
    children: React.ReactNode
    value: string
    onValueChange: (v: string) => void
  }) => {
    activeTabValue = value
    return (
      <div data-testid="tabs" data-value={value}>
        {children}
      </div>
    )
  },
  TabsList: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="tabs-list">{children}</div>
  ),
  TabsTrigger: ({ children, value }: { children: React.ReactNode; value: string }) => (
    <button data-testid={`tab-${value}`} role="tab">{children}</button>
  ),
  TabsContent: ({ children, value }: { children: React.ReactNode; value: string }) => {
    // Only render the active tab's content to avoid duplicate elements
    if (value !== activeTabValue) return null
    return (
      <div data-testid={`tab-content-${value}`} role="tabpanel">{children}</div>
    )
  },
}))

function makeCollection(overrides: Partial<Collection> = {}): Collection {
  return {
    id: 1,
    title: 'Test Collection',
    slug: 'test-collection',
    description: 'A test collection',
    creator_id: 1,
    creator_name: 'testuser',
    collaborative: false,
    is_public: true,
    is_featured: false,
    display_mode: 'unranked',
    item_count: 5,
    subscriber_count: 3,
    contributor_count: 1,
    forks_count: 0,
    forked_from_collection_id: null,
    like_count: 0,
    user_likes_this: false,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('CollectionList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    // Default: my collections returns empty
    mockUseMyCollections.mockReturnValue({
      data: { collections: [] },
      isLoading: false,
      error: null,
    })
  })

  describe('loading state', () => {
    it('shows loading spinner when loading and no data', () => {
      mockUseCollections.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message when fetch fails', () => {
      mockUseCollections.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('Failed to load collections. Please try again later.')).toBeInTheDocument()
    })

    it('shows retry button on error', () => {
      const mockRefetch = vi.fn()
      mockUseCollections.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<CollectionList />)
      expect(screen.getByText('Retry')).toBeInTheDocument()
    })

    it('calls refetch when retry clicked', async () => {
      const user = userEvent.setup()
      const mockRefetch = vi.fn()
      mockUseCollections.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<CollectionList />)
      await user.click(screen.getByText('Retry'))
      expect(mockRefetch).toHaveBeenCalled()
    })
  })

  describe('empty state', () => {
    it('shows empty message when no collections', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('No collections yet')).toBeInTheDocument()
    })

    it('shows create CTA for authenticated user when empty', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('Be the first to create one!')).toBeInTheDocument()
    })

    it('shows sign-in prompt for unauthenticated user when empty', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('Sign in to create and curate your own collections.')).toBeInTheDocument()
    })
  })

  describe('with collections', () => {
    it('renders collection cards', () => {
      mockUseCollections.mockReturnValue({
        data: {
          collections: [
            makeCollection({ id: 1, title: 'Collection One' }),
            makeCollection({ id: 2, title: 'Collection Two' }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByTestId('collection-card-1')).toBeInTheDocument()
      expect(screen.getByTestId('collection-card-2')).toBeInTheDocument()
    })
  })

  describe('tabs', () => {
    it('renders All, Popular, Recent, Featured tabs for unauthenticated user', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByTestId('tab-all')).toBeInTheDocument()
      expect(screen.getByTestId('tab-popular')).toBeInTheDocument()
      expect(screen.getByTestId('tab-recent')).toBeInTheDocument()
      expect(screen.getByTestId('tab-featured')).toBeInTheDocument()
      expect(screen.queryByTestId('tab-yours')).not.toBeInTheDocument()
    })

    it('renders Yours tab for authenticated user', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByTestId('tab-yours')).toBeInTheDocument()
    })
  })

  describe('search', () => {
    it('renders search input', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByPlaceholderText('Search collections...')).toBeInTheDocument()
    })

    it('shows search empty state when no results found', async () => {
      const user = userEvent.setup()
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      const searchInput = screen.getByPlaceholderText('Search collections...')
      await user.type(searchInput, 'nonexistent')
      expect(screen.getByText('No collections found')).toBeInTheDocument()
    })
  })

  describe('create collection button', () => {
    it('shows create button for authenticated user', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      // Button and dialog title both render "Create Collection"; verify button exists
      const matches = screen.getAllByText('Create Collection')
      const button = matches.find(el => el.closest('button'))
      expect(button).toBeTruthy()
    })

    it('does not show create button for unauthenticated user', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      // Unauthenticated user should not see the header Create Collection button
      // (there may be a CTA in the empty state)
      const matches = screen.queryAllByText('Create Collection')
      const headerButton = matches.find(el => {
        const btn = el.closest('button')
        return btn && !btn.closest('[data-testid="tab-content-all"]')
      })
      expect(headerButton).toBeUndefined()
    })

    it('shows create dialog with form', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      // Dialog content renders (since we mock Dialog to always render children)
      expect(screen.getByLabelText('Title')).toBeInTheDocument()
      expect(screen.getByLabelText('Description (optional)')).toBeInTheDocument()
    })

    it('renders create form with Public checkbox checked by default', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      const publicCheckbox = screen.getByLabelText('Public') as HTMLInputElement
      expect(publicCheckbox.checked).toBe(true)
    })

    it('renders create form with Collaborative checkbox unchecked by default', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      const collabCheckbox = screen.getByLabelText('Collaborative') as HTMLInputElement
      expect(collabCheckbox.checked).toBe(false)
    })
  })

  // PSY-354: tag-filter URL plumbing.
  describe('tag filter (PSY-354)', () => {
    it('does not render the active-filter pill when ?tag is absent', () => {
      mockSearchParams.delete('tag')
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(
        screen.queryByTestId('collection-tag-filter-pill')
      ).not.toBeInTheDocument()
    })

    it('renders the active-filter pill with tag slug when ?tag is set', () => {
      mockSearchParams.set('tag', 'phoenix')
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      const pill = screen.getByTestId('collection-tag-filter-pill')
      expect(pill).toBeInTheDocument()
      expect(pill.textContent).toContain('phoenix')
      mockSearchParams.delete('tag')
    })

    it('passes tag from URL into useCollections params', () => {
      mockSearchParams.set('tag', 'best-of-2026')
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      // The hook is called with tag set when ?tag is in the URL.
      expect(mockUseCollections).toHaveBeenCalledWith(
        expect.objectContaining({ tag: 'best-of-2026' })
      )
      mockSearchParams.delete('tag')
    })

    it('clears the filter via router.replace when X is clicked', async () => {
      const user = userEvent.setup()
      mockSearchParams.set('tag', 'phoenix')
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      const clearBtn = screen.getByTestId('collection-tag-filter-clear')
      await user.click(clearBtn)
      expect(mockReplace).toHaveBeenCalled()
      mockSearchParams.delete('tag')
    })
  })
})
