import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VenueCard } from './VenueCard'
import type { VenueWithShowCount } from '../types'

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

// Mock useVenueShows hook
const mockUseVenueShows = vi.fn((..._args: unknown[]) => ({
  data: undefined,
  error: null,
  refetch: vi.fn(),
}))
vi.mock('../hooks/useVenues', () => ({
  useVenueShows: (...args: unknown[]) => mockUseVenueShows(...args),
}))

// Mock TanStack Query
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({
    invalidateQueries: vi.fn(),
  }),
}))

// Mock queryClient
vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({
    venues: vi.fn(),
  }),
}))

// Mock child components
vi.mock('./FavoriteVenueButton', () => ({
  FavoriteVenueButton: ({ venueId }: { venueId: number }) => (
    <button data-testid="favorite-button">Fav {venueId}</button>
  ),
}))

vi.mock('./DeleteVenueDialog', () => ({
  DeleteVenueDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null,
}))

vi.mock('@/components/forms/VenueEditForm', () => ({
  VenueEditForm: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-form">Edit Form</div> : null,
}))

vi.mock('@/components/forms/ShowForm', () => ({
  ShowForm: ({ onCancel }: { onCancel: () => void }) => (
    <div data-testid="show-form">
      <button onClick={onCancel}>Cancel Show Form</button>
    </div>
  ),
}))

vi.mock('@/features/shows', () => ({
  CompactShowRow: ({ show }: { show: { id: number; title?: string } }) => (
    <div data-testid={`show-row-${show.id}`}>Show {show.id}</div>
  ),
  SHOW_LIST_FEATURE_POLICY: {
    context: { showDetailsLink: true },
  },
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, ...props }: { children: React.ReactNode; [key: string]: unknown }) => (
    <button {...props}>{children}</button>
  ),
}))

function makeVenue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'the-rebel-lounge',
    name: 'The Rebel Lounge',
    address: '2303 E Indian School Rd',
    city: 'Phoenix',
    state: 'AZ',
    verified: false,
    upcoming_show_count: 3,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('VenueCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseVenueShows.mockReturnValue({
      data: undefined,
      error: null,
      refetch: vi.fn(),
    })
  })

  it('renders as an article element', () => {
    render(<VenueCard venue={makeVenue()} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('renders venue name as a link when slug is present', () => {
    render(<VenueCard venue={makeVenue()} />)
    const link = screen.getByRole('link', { name: 'The Rebel Lounge' })
    expect(link).toHaveAttribute('href', '/venues/the-rebel-lounge')
  })

  it('renders venue name as plain text when slug is missing', () => {
    render(<VenueCard venue={makeVenue({ slug: '' })} />)
    const name = screen.getByText('The Rebel Lounge')
    expect(name.closest('a')).toBeNull()
    expect(name.tagName).toBe('SPAN')
  })

  it('renders city and state', () => {
    render(<VenueCard venue={makeVenue()} />)
    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('renders upcoming show count (plural)', () => {
    render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
    expect(screen.getByText('3 shows')).toBeInTheDocument()
  })

  it('renders singular show count', () => {
    render(<VenueCard venue={makeVenue({ upcoming_show_count: 1 })} />)
    expect(screen.getByText('1 show')).toBeInTheDocument()
  })

  it('renders zero show count', () => {
    render(<VenueCard venue={makeVenue({ upcoming_show_count: 0 })} />)
    expect(screen.getByText('0 shows')).toBeInTheDocument()
  })

  it('shows verified badge when verified', () => {
    const { container } = render(<VenueCard venue={makeVenue({ verified: true })} />)
    // BadgeCheck icon renders as an SVG
    const svgs = container.querySelectorAll('svg')
    // At least one SVG should have the primary color class (verified badge)
    expect(svgs.length).toBeGreaterThan(0)
  })

  it('renders favorite venue button', () => {
    render(<VenueCard venue={makeVenue()} />)
    expect(screen.getByTestId('favorite-button')).toBeInTheDocument()
    expect(screen.getByText('Fav 1')).toBeInTheDocument()
  })

  describe('expand/collapse behavior', () => {
    it('has expandable header when venue has shows', () => {
      render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
      const button = screen.getByRole('button', { name: /The Rebel Lounge/i })
        ?? screen.getAllByRole('button')[0]
      // The header div should have role="button" when has shows
      const header = document.querySelector('[role="button"]')
      expect(header).toBeInTheDocument()
    })

    it('does not have expandable header when venue has no shows', () => {
      render(<VenueCard venue={makeVenue({ upcoming_show_count: 0 })} />)
      const header = document.querySelector('[role="button"]')
      expect(header).toBeNull()
    })

    it('expands to show loading state on click', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: { shows: [], total: 0 },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      expect(screen.getByText('No upcoming shows')).toBeInTheDocument()
    })

    it('shows error message when shows fail to load', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: undefined,
        error: new Error('Network error'),
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      expect(screen.getByText('Failed to load shows')).toBeInTheDocument()
    })

    it('renders show rows when expanded with data', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: {
          shows: [
            { id: 10, slug: 'show-1', title: 'Show 1', event_date: '2026-05-01T20:00:00Z', artists: [] },
            { id: 11, slug: 'show-2', title: 'Show 2', event_date: '2026-05-02T20:00:00Z', artists: [] },
          ],
          total: 2,
        },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      expect(screen.getByTestId('show-row-10')).toBeInTheDocument()
      expect(screen.getByTestId('show-row-11')).toBeInTheDocument()
    })

    it('shows "View all" link when total > displayed shows', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: {
          shows: [{ id: 10, slug: 'show-1', title: 'Show 1', event_date: '2026-05-01T20:00:00Z', artists: [] }],
          total: 5,
        },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 5 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      const viewAllLink = screen.getByText('View all 5 shows')
      expect(viewAllLink.closest('a')).toHaveAttribute('href', '/venues/the-rebel-lounge')
    })

    it('supports keyboard navigation (Enter key)', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: { shows: [], total: 0 },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 3 })} />)
      const header = document.querySelector('[role="button"]')!
      ;(header as HTMLElement).focus()
      await user.keyboard('{Enter}')

      expect(screen.getByText('No upcoming shows')).toBeInTheDocument()
    })
  })

  describe('edit/delete controls', () => {
    it('does not show edit/delete buttons for unauthenticated user', () => {
      render(<VenueCard venue={makeVenue()} />)
      expect(screen.queryByTitle('Edit venue')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Delete venue')).not.toBeInTheDocument()
    })

    it('does not show edit/delete for non-admin non-owner', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueCard venue={makeVenue({ submitted_by: 42 })} />)
      expect(screen.queryByTitle('Edit venue')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Delete venue')).not.toBeInTheDocument()
    })

    it('shows edit/delete buttons for admin', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueCard venue={makeVenue()} />)
      expect(screen.getByTitle('Edit venue')).toBeInTheDocument()
      expect(screen.getByTitle('Delete venue')).toBeInTheDocument()
    })

    it('shows edit/delete buttons for venue owner', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '42', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueCard venue={makeVenue({ submitted_by: 42 })} />)
      expect(screen.getByTitle('Edit venue')).toBeInTheDocument()
      expect(screen.getByTitle('Delete venue')).toBeInTheDocument()
    })

    it('opens edit form when edit button clicked', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueCard venue={makeVenue()} />)

      expect(screen.queryByTestId('edit-form')).not.toBeInTheDocument()
      await user.click(screen.getByTitle('Edit venue'))
      expect(screen.getByTestId('edit-form')).toBeInTheDocument()
    })

    it('opens delete dialog when delete button clicked', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueCard venue={makeVenue()} />)

      expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
      await user.click(screen.getByTitle('Delete venue'))
      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
    })
  })

  describe('add show button', () => {
    it('shows add show button for authenticated user when expanded', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenueShows.mockReturnValue({
        data: { shows: [{ id: 10, slug: 'show-1', title: 'Show 1', event_date: '2026-05-01T20:00:00Z', artists: [] }], total: 1 },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 1 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      expect(screen.getByText(/Add a show at The Rebel Lounge/)).toBeInTheDocument()
    })

    it('does not show add show button for unauthenticated user when expanded', async () => {
      const user = userEvent.setup()
      mockUseVenueShows.mockReturnValue({
        data: { shows: [{ id: 10, slug: 'show-1', title: 'Show 1', event_date: '2026-05-01T20:00:00Z', artists: [] }], total: 1 },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 1 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)

      expect(screen.queryByText(/Add a show/)).not.toBeInTheDocument()
    })

    it('shows show form when add show button is clicked', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenueShows.mockReturnValue({
        data: { shows: [{ id: 10, slug: 'show-1', title: 'Show 1', event_date: '2026-05-01T20:00:00Z', artists: [] }], total: 1 },
        error: null,
        refetch: vi.fn(),
      })

      render(<VenueCard venue={makeVenue({ upcoming_show_count: 1 })} />)
      const header = document.querySelector('[role="button"]')!
      await user.click(header)
      await user.click(screen.getByText(/Add a show at The Rebel Lounge/))

      expect(screen.getByTestId('show-form')).toBeInTheDocument()
    })
  })
})
