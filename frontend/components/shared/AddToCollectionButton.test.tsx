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
})
