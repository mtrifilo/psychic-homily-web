import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowDetail } from './ShowDetail'
import type { ShowResponse, ArtistResponse } from '../types'

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

// Mock useShow hook
const mockUseShow = vi.fn()
vi.mock('../hooks/useShows', () => ({
  useShow: (...args: unknown[]) => mockUseShow(...args),
}))

// Mock admin hooks
const mockSetSoldOut = vi.fn()
const mockSetCancelled = vi.fn()
vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useSetShowSoldOut: () => ({
    mutate: mockSetSoldOut,
    isPending: false,
  }),
  useSetShowCancelled: () => ({
    mutate: mockSetCancelled,
    isPending: false,
  }),
}))

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: () => '/shows/test-show',
}))

// Mock child components
vi.mock('@/components/shared', () => ({
  SaveButton: () => <button data-testid="save-button">Save</button>,
  SocialLinks: () => <div data-testid="social-links" />,
  MusicEmbed: () => <div data-testid="music-embed" />,
  Breadcrumb: ({ fallback, currentPage }: { fallback: { href: string; label: string }; currentPage: string }) => (
    <nav aria-label="Breadcrumb"><a href={fallback.href}>{fallback.label}</a><span>{currentPage}</span></nav>
  ),
  AddToCollectionButton: () => <button data-testid="add-to-collection">Collect</button>,
}))

vi.mock('@/components/forms', () => ({
  ShowForm: ({ onCancel }: { onCancel: () => void }) => (
    <div data-testid="show-form">
      <button onClick={onCancel}>Cancel Form</button>
    </div>
  ),
}))

vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null,
}))

vi.mock('./ReportShowButton', () => ({
  ReportShowButton: () => <button data-testid="report-button">Report</button>,
}))

vi.mock('./AttendanceButton', () => ({
  AttendanceButton: ({ showId }: { showId: number }) => (
    <div data-testid="attendance-button">Attendance {showId}</div>
  ),
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist-one',
    name: 'Artist One',
    city: 'Phoenix',
    state: 'AZ',
    socials: {},
    ...overrides,
  }
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'approved',
    city: 'Phoenix',
    state: 'AZ',
    price: 25,
    age_requirement: '21+',
    description: 'A great show description.',
    venues: [
      { id: 1, slug: 'the-venue', name: 'The Venue', city: 'Phoenix', state: 'AZ', verified: true },
    ],
    artists: [
      makeArtist({ id: 1, name: 'Headliner', slug: 'headliner' }),
      makeArtist({ id: 2, name: 'Opener', slug: 'opener' }),
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowDetail', () => {
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
      mockUseShow.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      })
      const { container } = render(<ShowDetail showId="1" />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message', () => {
      mockUseShow.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Something went wrong'),
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('Error Loading Show')).toBeInTheDocument()
      expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    })

    it('shows 404 message for not found errors', () => {
      const error = new Error('Not found')
      ;(error as unknown as { status: number }).status = 404
      mockUseShow.mockReturnValue({
        data: undefined,
        isLoading: false,
        error,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('Show Not Found')).toBeInTheDocument()
      expect(screen.getByText(/doesn't exist or has been removed/)).toBeInTheDocument()
    })

    it('shows back to shows link on error', () => {
      mockUseShow.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Error'),
      })
      render(<ShowDetail showId="1" />)
      const link = screen.getByText('Back to Shows').closest('a')
      expect(link).toHaveAttribute('href', '/shows')
    })
  })

  describe('no data state', () => {
    it('shows not found when data is null', () => {
      mockUseShow.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('Show Not Found')).toBeInTheDocument()
    })
  })

  describe('with show data', () => {
    beforeEach(() => {
      mockUseShow.mockReturnValue({
        data: makeShow(),
        isLoading: false,
        error: null,
      })
    })

    it('renders artist names', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('Headliner')).toBeInTheDocument()
      expect(screen.getByText('Opener')).toBeInTheDocument()
    })

    it('links artists with slugs to artist pages', () => {
      render(<ShowDetail showId="1" />)
      const link = screen.getByText('Headliner').closest('a')
      expect(link).toHaveAttribute('href', '/artists/headliner')
    })

    it('renders venue name as link', () => {
      render(<ShowDetail showId="1" />)
      const link = screen.getByText('The Venue').closest('a')
      expect(link).toHaveAttribute('href', '/venues/the-venue')
    })

    it('renders venue location', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
    })

    it('renders price', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('$25.00')).toBeInTheDocument()
    })

    it('renders age requirement', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('21+')).toBeInTheDocument()
    })

    it('renders description', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('A great show description.')).toBeInTheDocument()
    })

    it('does not render description when missing', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ description: null }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.queryByText('A great show description.')).not.toBeInTheDocument()
    })

    it('renders breadcrumb with link to shows', () => {
      render(<ShowDetail showId="1" />)
      const breadcrumbNav = screen.getByRole('navigation', { name: /Breadcrumb/ })
      expect(breadcrumbNav).toBeInTheDocument()
      const link = breadcrumbNav.querySelector('a')
      expect(link).toHaveAttribute('href', '/shows')
    })

    it('renders save button', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByTestId('save-button')).toBeInTheDocument()
    })

    it('renders report button', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByTestId('report-button')).toBeInTheDocument()
    })
  })

  describe('cancelled show', () => {
    it('shows cancellation alert', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_cancelled: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('This show has been cancelled.')).toBeInTheDocument()
    })
  })

  describe('sold out show', () => {
    it('shows sold out badge', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_sold_out: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
    })
  })

  describe('admin controls', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseShow.mockReturnValue({
        data: makeShow(),
        isLoading: false,
        error: null,
      })
    })

    it('shows edit button for admin', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument()
    })

    it('shows delete button for admin', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument()
    })

    it('toggles edit form on click', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" />)

      expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /Edit/ }))
      expect(screen.getByTestId('show-form')).toBeInTheDocument()

      // The admin Cancel button has text "Cancel" with X icon
      const cancelButtons = screen.getAllByRole('button').filter(b =>
        b.textContent?.trim() === 'Cancel'
      )
      await user.click(cancelButtons[0])
      expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    })

    it('opens delete dialog on click', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" />)

      expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /Delete/ }))
      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
    })

    it('shows Mark Sold Out button', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: 'Mark Sold Out' })).toBeInTheDocument()
    })

    it('shows Mark Cancelled button', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: 'Mark Cancelled' })).toBeInTheDocument()
    })

    it('shows Unmark Sold Out when already sold out', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_sold_out: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: 'Unmark Sold Out' })).toBeInTheDocument()
    })

    it('shows Unmark Cancelled when already cancelled', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_cancelled: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: 'Unmark Cancelled' })).toBeInTheDocument()
    })

    it('calls sold out mutation on toggle', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" />)

      await user.click(screen.getByRole('button', { name: 'Mark Sold Out' }))
      expect(mockSetSoldOut).toHaveBeenCalledWith({ showId: 1, value: true })
    })

    it('calls cancelled mutation on toggle', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" />)

      await user.click(screen.getByRole('button', { name: 'Mark Cancelled' }))
      expect(mockSetCancelled).toHaveBeenCalledWith({ showId: 1, value: true })
    })
  })

  describe('non-admin controls', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: { id: '2', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseShow.mockReturnValue({
        data: makeShow(),
        isLoading: false,
        error: null,
      })
    })

    it('does not show edit button for non-admin', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.queryByRole('button', { name: /Edit/ })).not.toBeInTheDocument()
    })

    it('does not show delete button for non-admin non-owner', () => {
      render(<ShowDetail showId="1" />)
      expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument()
    })
  })

  describe('show owner controls', () => {
    it('shows delete button for show owner', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '42', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseShow.mockReturnValue({
        data: makeShow({ submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument()
    })

    it('shows status toggle buttons for show owner', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '42', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseShow.mockReturnValue({
        data: makeShow({ submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByRole('button', { name: 'Mark Sold Out' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Mark Cancelled' })).toBeInTheDocument()
    })
  })

  describe('artist music section', () => {
    it('renders music section when artists have music', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          artists: [
            makeArtist({
              id: 1,
              name: 'Band',
              socials: { spotify: 'https://spotify.com/band' },
            }),
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.getByText('Listen to the Artists')).toBeInTheDocument()
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    })

    it('does not render music section when no artists have music', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          artists: [
            makeArtist({ id: 1, name: 'Band', socials: {} }),
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" />)
      expect(screen.queryByText('Listen to the Artists')).not.toBeInTheDocument()
    })
  })
})
