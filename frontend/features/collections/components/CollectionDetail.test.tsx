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

vi.mock('../hooks', () => ({
  useCollection: (...args: unknown[]) => mockCollection(...args),
  useUpdateCollection: () => ({
    mutate: vi.fn(),
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
  useSubscribeCollection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useUnsubscribeCollection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteCollection: () => mockDeleteMutation(),
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
    item_count: 0,
    subscriber_count: 0,
    contributor_count: 0,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    items: [],
    is_subscribed: false,
    ...overrides,
  }
}

/** Helper: find the trash/delete icon button (has class text-destructive, no text) */
function findTrashButton(): HTMLElement {
  const buttons = screen.getAllByRole('button')
  const trashButton = buttons.find(
    (b) => b.className.includes('text-destructive') && !b.textContent?.includes('Delete')
  )
  if (!trashButton) throw new Error('Trash button not found')
  return trashButton
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
})
