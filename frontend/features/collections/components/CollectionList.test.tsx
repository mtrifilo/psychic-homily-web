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

// Mock collection hooks
const mockUseCollections = vi.fn()
const mockCreateMutate = vi.fn()

vi.mock('../hooks', () => ({
  useCollections: () => mockUseCollections(),
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
    <button onClick={onClick} disabled={disabled} type={props.type as string}>{children}</button>
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
  DialogTrigger: ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => (
    <>{children}</>
  ),
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
    item_count: 5,
    subscriber_count: 3,
    contributor_count: 1,
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
      expect(screen.getByText('No public collections yet.')).toBeInTheDocument()
    })

    it('shows encouragement for authenticated user when empty', () => {
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

    it('does not show encouragement for unauthenticated user', () => {
      mockUseCollections.mockReturnValue({
        data: { collections: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.queryByText('Be the first to create one!')).not.toBeInTheDocument()
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

    it('separates featured and regular collections', () => {
      mockUseCollections.mockReturnValue({
        data: {
          collections: [
            makeCollection({ id: 1, title: 'Featured One', is_featured: true }),
            makeCollection({ id: 2, title: 'Regular One', is_featured: false }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('Featured')).toBeInTheDocument()
      expect(screen.getByText('All Collections')).toBeInTheDocument()
    })

    it('does not show Featured heading when no featured collections', () => {
      mockUseCollections.mockReturnValue({
        data: {
          collections: [
            makeCollection({ id: 1, title: 'Regular One', is_featured: false }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.queryByText('Featured')).not.toBeInTheDocument()
      expect(screen.queryByText('All Collections')).not.toBeInTheDocument()
    })

    it('does not show All Collections heading when only featured', () => {
      mockUseCollections.mockReturnValue({
        data: {
          collections: [
            makeCollection({ id: 1, title: 'Featured One', is_featured: true }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CollectionList />)
      expect(screen.getByText('Featured')).toBeInTheDocument()
      // The "All Collections" heading only shows when both featured and regular exist
      expect(screen.queryByText('All Collections')).not.toBeInTheDocument()
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
      expect(screen.queryByText('Create Collection')).not.toBeInTheDocument()
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
      // "Create Collection" appears as both button text and dialog title
      expect(screen.getAllByText('Create Collection').length).toBeGreaterThanOrEqual(2)
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
})
