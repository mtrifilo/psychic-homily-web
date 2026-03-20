import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CollectionDetail } from './CollectionDetail'
import type { CollectionDetail as CollectionDetailType, CollectionItem } from '../types'

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

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/collections/test-collection',
}))

// Mock NavigationBreadcrumbContext
vi.mock('@/lib/context/NavigationBreadcrumbContext', () => ({
  useNavigationBreadcrumbs: () => ({
    breadcrumbs: [],
    pushBreadcrumb: vi.fn(),
  }),
}))

// Mock collection hooks
const mockUseCollection = vi.fn()
const mockSubscribeMutate = vi.fn()
const mockUnsubscribeMutate = vi.fn()
const mockDeleteMutate = vi.fn()
const mockRemoveItemMutate = vi.fn()
const mockUpdateMutate = vi.fn()

vi.mock('../hooks', () => ({
  useCollection: (slug: string) => mockUseCollection(slug),
  useUpdateCollection: () => ({
    mutate: mockUpdateMutate,
    isPending: false,
    error: null,
  }),
  useRemoveCollectionItem: () => ({
    mutate: mockRemoveItemMutate,
    isPending: false,
  }),
  useSubscribeCollection: () => ({
    mutate: mockSubscribeMutate,
    isPending: false,
  }),
  useUnsubscribeCollection: () => ({
    mutate: mockUnsubscribeMutate,
    isPending: false,
  }),
  useDeleteCollection: () => ({
    mutate: mockDeleteMutate,
    isPending: false,
  }),
}))

// Mock shared components
vi.mock('@/components/shared', () => ({
  Breadcrumb: ({ fallback, currentPage }: { fallback: { href: string; label: string }; currentPage: string }) => (
    <nav aria-label="Breadcrumb"><a href={fallback.href}>{fallback.label}</a><span>{currentPage}</span></nav>
  ),
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, asChild, onClick, disabled, ...props }: {
    children: React.ReactNode
    asChild?: boolean
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => {
    if (asChild) return <>{children}</>
    return <button onClick={onClick} disabled={disabled} title={props.title as string}>{children}</button>
  },
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
}))

vi.mock('@/components/ui/textarea', () => ({
  Textarea: (props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...props} />,
}))

vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children }: { children: React.ReactNode }) => <span data-testid="badge">{children}</span>,
}))

function makeItem(overrides: Partial<CollectionItem> = {}): CollectionItem {
  return {
    id: 1,
    entity_type: 'artist',
    entity_id: 10,
    entity_name: 'Test Artist',
    entity_slug: 'test-artist',
    position: 0,
    added_by_user_id: 1,
    added_by_name: 'testuser',
    notes: null,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeCollection(overrides: Partial<CollectionDetailType> = {}): CollectionDetailType {
  return {
    id: 1,
    title: 'Best of Phoenix',
    slug: 'best-of-phoenix',
    description: 'The best artists in Phoenix',
    creator_id: 1,
    creator_name: 'testuser',
    collaborative: false,
    is_public: true,
    is_featured: false,
    item_count: 3,
    subscriber_count: 5,
    contributor_count: 1,
    items: [
      makeItem({ id: 1, entity_type: 'artist', entity_name: 'Artist One', entity_slug: 'artist-one' }),
      makeItem({ id: 2, entity_type: 'venue', entity_name: 'Cool Venue', entity_slug: 'cool-venue', entity_id: 20 }),
      makeItem({ id: 3, entity_type: 'show', entity_name: 'Big Show', entity_slug: 'big-show', entity_id: 30 }),
    ],
    is_subscribed: false,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('CollectionDetail', () => {
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
    it('shows spinner when loading', () => {
      mockUseCollection.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      })
      const { container } = render(<CollectionDetail slug="test" />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message', () => {
      mockUseCollection.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Something went wrong'),
      })
      render(<CollectionDetail slug="test" />)
      expect(screen.getByText('Error Loading Collection')).toBeInTheDocument()
      expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    })

    it('shows 404 message for not found errors', () => {
      mockUseCollection.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Collection not found'),
      })
      render(<CollectionDetail slug="test" />)
      expect(screen.getByText('Collection Not Found')).toBeInTheDocument()
    })

    it('shows back to collections link on error', () => {
      mockUseCollection.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Error'),
      })
      render(<CollectionDetail slug="test" />)
      const link = screen.getByText('Back to Collections').closest('a')
      expect(link).toHaveAttribute('href', '/collections')
    })
  })

  describe('no data state', () => {
    it('shows not found when data is null', () => {
      mockUseCollection.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="test" />)
      expect(screen.getByText('Collection Not Found')).toBeInTheDocument()
    })
  })

  describe('with collection data', () => {
    beforeEach(() => {
      mockUseCollection.mockReturnValue({
        data: makeCollection(),
        isLoading: false,
        error: null,
      })
    })

    it('renders collection title', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByRole('heading', { level: 1, name: 'Best of Phoenix' })).toBeInTheDocument()
    })

    it('renders creator name', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('by testuser')).toBeInTheDocument()
    })

    it('renders description', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('The best artists in Phoenix')).toBeInTheDocument()
    })

    it('renders item count', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('3 items')).toBeInTheDocument()
    })

    it('renders singular item count', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({ item_count: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('1 item')).toBeInTheDocument()
    })

    it('renders subscriber count', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('5 subscribers')).toBeInTheDocument()
    })

    it('renders singular subscriber count', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({ subscriber_count: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('1 subscriber')).toBeInTheDocument()
    })

    it('renders breadcrumb navigation', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      const breadcrumb = screen.getByRole('navigation', { name: /Breadcrumb/ })
      expect(breadcrumb).toBeInTheDocument()
      const link = breadcrumb.querySelector('a')
      expect(link).toHaveAttribute('href', '/collections')
    })

    it('shows Featured badge when is_featured', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({ is_featured: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Featured')).toBeInTheDocument()
    })

    it('shows Collaborative badge when collaborative', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({ collaborative: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Collaborative')).toBeInTheDocument()
    })

    it('renders items list', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Artist One')).toBeInTheDocument()
      expect(screen.getByText('Cool Venue')).toBeInTheDocument()
      expect(screen.getByText('Big Show')).toBeInTheDocument()
    })

    it('links items to their entity pages', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      const artistLink = screen.getByText('Artist One').closest('a')
      expect(artistLink).toHaveAttribute('href', '/artists/artist-one')

      const venueLink = screen.getByText('Cool Venue').closest('a')
      expect(venueLink).toHaveAttribute('href', '/venues/cool-venue')

      const showLink = screen.getByText('Big Show').closest('a')
      expect(showLink).toHaveAttribute('href', '/shows/big-show')
    })

    it('shows entity type badges', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Artist')).toBeInTheDocument()
      expect(screen.getByText('Venue')).toBeInTheDocument()
      expect(screen.getByText('Show')).toBeInTheDocument()
    })

    it('shows "added by" text for items', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      const addedByTexts = screen.getAllByText('added by testuser')
      expect(addedByTexts.length).toBe(3)
    })

    it('shows item notes when present', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({
          items: [makeItem({ notes: 'A great artist!' })],
        }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('A great artist!')).toBeInTheDocument()
    })

    it('shows empty state when no items', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection({ items: [] }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('This collection is empty.')).toBeInTheDocument()
    })
  })

  describe('subscribe/unsubscribe', () => {
    it('shows subscribe button for non-creator authenticated user', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Subscribe')).toBeInTheDocument()
    })

    it('shows unsubscribe button when already subscribed', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1, is_subscribed: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Unsubscribe')).toBeInTheDocument()
    })

    it('does not show subscribe button for creator', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.queryByText('Subscribe')).not.toBeInTheDocument()
      expect(screen.queryByText('Unsubscribe')).not.toBeInTheDocument()
    })

    it('does not show subscribe button for unauthenticated user', () => {
      mockUseCollection.mockReturnValue({
        data: makeCollection(),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.queryByText('Subscribe')).not.toBeInTheDocument()
    })

    it('calls subscribe mutation on click', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      await user.click(screen.getByText('Subscribe'))
      expect(mockSubscribeMutate).toHaveBeenCalledWith({ slug: 'best-of-phoenix' })
    })

    it('calls unsubscribe mutation on click', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1, is_subscribed: true }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      await user.click(screen.getByText('Unsubscribe'))
      expect(mockUnsubscribeMutate).toHaveBeenCalledWith({ slug: 'best-of-phoenix' })
    })
  })

  describe('creator controls', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
    })

    it('shows edit button for creator', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.getByText('Edit')).toBeInTheDocument()
    })

    it('shows delete button for creator', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      // Delete button has Trash2 icon
      const deleteButton = screen.getAllByRole('button').find(b =>
        b.querySelector('svg') && !b.textContent?.includes('Edit') && !b.textContent?.includes('Subscribe')
      )
      expect(deleteButton).toBeTruthy()
    })

    it('shows remove buttons on items for creator', () => {
      render(<CollectionDetail slug="best-of-phoenix" />)
      const removeButtons = screen.getAllByTitle('Remove from collection')
      expect(removeButtons.length).toBe(3)
    })

    it('does not show remove buttons for non-creator', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.queryByTitle('Remove from collection')).not.toBeInTheDocument()
    })

    it('toggles inline edit form on edit button click', async () => {
      const user = userEvent.setup()
      render(<CollectionDetail slug="best-of-phoenix" />)

      await user.click(screen.getByText('Edit'))
      // Edit form should show title input
      expect(screen.getByLabelText('Title')).toBeInTheDocument()
      expect(screen.getByLabelText('Description')).toBeInTheDocument()
    })

    it('calls remove item mutation when remove button clicked', async () => {
      const user = userEvent.setup()
      render(<CollectionDetail slug="best-of-phoenix" />)

      const removeButtons = screen.getAllByTitle('Remove from collection')
      await user.click(removeButtons[0])
      expect(mockRemoveItemMutate).toHaveBeenCalledWith({ slug: 'best-of-phoenix', itemId: 1 })
    })
  })

  describe('non-creator view', () => {
    it('does not show edit button for non-creator', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCollection.mockReturnValue({
        data: makeCollection({ creator_id: 1 }),
        isLoading: false,
        error: null,
      })
      render(<CollectionDetail slug="best-of-phoenix" />)
      expect(screen.queryByText('Edit')).not.toBeInTheDocument()
    })
  })
})
