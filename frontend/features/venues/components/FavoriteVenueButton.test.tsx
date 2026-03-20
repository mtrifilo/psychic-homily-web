import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FavoriteVenueButton } from './FavoriteVenueButton'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  isAuthenticated: false,
  user: null,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock useFavoriteVenueToggle
const mockToggle = vi.fn()
const mockFavoriteToggle = vi.fn(() => ({
  isFavorited: false,
  isLoading: false,
  toggle: mockToggle,
  error: null,
}))
vi.mock('@/features/auth', () => ({
  useFavoriteVenueToggle: (venueId: number, isAuth: boolean) => mockFavoriteToggle(venueId, isAuth),
}))

// Mock Button
vi.mock('@/components/ui/button', () => ({
  Button: ({ children, disabled, title, ...props }: {
    children: React.ReactNode
    disabled?: boolean
    title?: string
    [key: string]: unknown
  }) => (
    <button disabled={disabled} title={title} aria-label={props['aria-label'] as string} onClick={props.onClick as () => void}>
      {children}
    </button>
  ),
}))

describe('FavoriteVenueButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
      isLoading: false,
      logout: vi.fn(),
    })
    mockFavoriteToggle.mockReturnValue({
      isFavorited: false,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
  })

  it('renders nothing when not authenticated', () => {
    const { container } = render(<FavoriteVenueButton venueId={1} />)
    expect(container.innerHTML).toBe('')
  })

  it('renders button when authenticated', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    render(<FavoriteVenueButton venueId={1} />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('shows "Add to Favorites" aria-label when not favorited', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    render(<FavoriteVenueButton venueId={1} />)
    expect(screen.getByLabelText('Add to Favorites')).toBeInTheDocument()
  })

  it('shows "Remove from Favorites" aria-label when favorited', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockFavoriteToggle.mockReturnValue({
      isFavorited: true,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    render(<FavoriteVenueButton venueId={1} />)
    expect(screen.getByLabelText('Remove from Favorites')).toBeInTheDocument()
  })

  it('calls toggle on click', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockToggle.mockResolvedValue(undefined)

    render(<FavoriteVenueButton venueId={42} />)
    await user.click(screen.getByRole('button'))

    expect(mockToggle).toHaveBeenCalled()
  })

  it('disables button while loading', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockFavoriteToggle.mockReturnValue({
      isFavorited: false,
      isLoading: true,
      toggle: mockToggle,
      error: null,
    })
    render(<FavoriteVenueButton venueId={1} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('shows label text when showLabel is true', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    render(<FavoriteVenueButton venueId={1} showLabel />)
    expect(screen.getByText('Favorite')).toBeInTheDocument()
  })

  it('shows "Favorited" label text when favorited and showLabel is true', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockFavoriteToggle.mockReturnValue({
      isFavorited: true,
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    render(<FavoriteVenueButton venueId={1} showLabel />)
    expect(screen.getByText('Favorited')).toBeInTheDocument()
  })

  it('does not show label text by default', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    render(<FavoriteVenueButton venueId={1} />)
    expect(screen.queryByText('Favorite')).not.toBeInTheDocument()
    expect(screen.queryByText('Favorited')).not.toBeInTheDocument()
  })

  it('shows error tooltip when toggle fails', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockToggle.mockRejectedValue(new Error('Network error'))
    mockFavoriteToggle.mockReturnValue({
      isFavorited: false,
      isLoading: false,
      toggle: mockToggle,
      error: new Error('Network error'),
    })

    render(<FavoriteVenueButton venueId={1} />)
    await user.click(screen.getByRole('button'))

    expect(screen.getByText(/Failed to add favorite/)).toBeInTheDocument()
  })
})
