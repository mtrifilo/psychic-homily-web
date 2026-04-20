import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowCard } from './ShowCard'
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

// Mock child components to keep tests focused
vi.mock('@/components/shared', () => ({
  SaveButton: ({ showId }: { showId: number }) => (
    <button data-testid="save-button">Save {showId}</button>
  ),
  SocialLinks: () => <div data-testid="social-links" />,
  MusicEmbed: () => <div data-testid="music-embed" />,
}))

vi.mock('@/components/forms', () => ({
  ShowForm: ({ onCancel }: { onCancel: () => void }) => (
    <div data-testid="show-form">
      <button onClick={onCancel}>Cancel Form</button>
    </div>
  ),
}))

vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: ({ open }: { open: boolean }) => (
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null
  ),
}))

vi.mock('./ExportShowButton', () => ({
  ExportShowButton: () => <button data-testid="export-button">Export</button>,
}))

vi.mock('./AttendanceButton', () => ({
  AttendanceButton: ({ showId }: { showId: number }) => (
    <div data-testid="attendance-button">Attendance {showId}</div>
  ),
}))

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist-one',
    name: 'Artist One',
    // Default to a neutral set_type so tests can opt in to headliner status
    // via `is_headliner: true` or `set_type: 'headliner'` per-case.
    set_type: 'performer',
    position: 1,
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
    price: 20,
    age_requirement: '21+',
    venues: [
      {
        id: 1,
        slug: 'the-venue',
        name: 'The Venue',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      makeArtist({ id: 1, name: 'Headliner', slug: 'headliner', is_headliner: true }),
      makeArtist({ id: 2, name: 'Opener', slug: 'opener', is_headliner: false }),
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowCard', () => {
  beforeEach(() => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders as an article element', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('renders headliner artist name', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('Headliner')).toBeInTheDocument()
  })

  it('renders support artist with w/ prefix', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('w/')).toBeInTheDocument()
    expect(screen.getByText('Opener')).toBeInTheDocument()
  })

  it('treats first artist as headliner when no is_headliner flags set', () => {
    const show = makeShow({
      artists: [
        makeArtist({ id: 1, name: 'Band A', is_headliner: undefined }),
        makeArtist({ id: 2, name: 'Band B', is_headliner: undefined }),
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    // Band A is shown as headliner (in h2), Band B as support (with w/)
    expect(screen.getByText('Band A')).toBeInTheDocument()
    expect(screen.getByText('w/')).toBeInTheDocument()
    expect(screen.getByText('Band B')).toBeInTheDocument()
  })

  it('shows TBA when no artists', () => {
    const show = makeShow({ artists: [] })
    render(<ShowCard show={show} isAdmin={false} />)
    expect(screen.getByText('TBA')).toBeInTheDocument()
  })

  it('renders venue name as a link', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const venueLink = screen.getByText('The Venue')
    expect(venueLink.closest('a')).toHaveAttribute('href', '/venues/the-venue')
  })

  it('renders venue name as plain text when no slug', () => {
    const show = makeShow({
      venues: [
        { id: 1, slug: '', name: 'No Slug Venue', city: 'Phoenix', state: 'AZ', verified: true },
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    const venue = screen.getByText('No Slug Venue')
    expect(venue.closest('a')).toBeNull()
  })

  it('renders city and state', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
  })

  it('renders price', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('$20.00')).toBeInTheDocument()
  })

  it('renders age requirement', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('21+')).toBeInTheDocument()
  })

  it('does not render price when null', () => {
    render(<ShowCard show={makeShow({ price: null })} isAdmin={false} />)
    expect(screen.queryByText('$')).not.toBeInTheDocument()
  })

  it('does not render age requirement when not set', () => {
    render(<ShowCard show={makeShow({ age_requirement: null })} isAdmin={false} />)
    expect(screen.queryByText('21+')).not.toBeInTheDocument()
  })

  it('links artist with slug to artist page', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const link = screen.getByText('Headliner').closest('a')
    expect(link).toHaveAttribute('href', '/artists/headliner')
  })

  it('renders artist without slug as plain text', () => {
    const show = makeShow({
      artists: [
        makeArtist({ id: 1, name: 'No Slug Artist', slug: '', is_headliner: true }),
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    const artist = screen.getByText('No Slug Artist')
    expect(artist.closest('a')).toBeNull()
  })

  it('links date badge to show detail page', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const links = screen.getAllByRole('link')
    const showLink = links.find(l => l.getAttribute('href') === '/shows/test-show')
    expect(showLink).toBeDefined()
  })

  it('uses show ID in link when slug is missing', () => {
    render(<ShowCard show={makeShow({ slug: '' })} isAdmin={false} />)
    const links = screen.getAllByRole('link')
    const showLink = links.find(l => l.getAttribute('href') === '/shows/1')
    expect(showLink).toBeDefined()
  })

  it('applies cancelled opacity', () => {
    render(<ShowCard show={makeShow({ is_cancelled: true })} isAdmin={false} />)
    const article = screen.getByRole('article')
    expect(article.className).toContain('opacity-60')
  })

  it('does not show admin edit button for non-admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.queryByTitle('Edit show')).not.toBeInTheDocument()
  })

  it('shows admin edit button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(screen.getByTitle('Edit show')).toBeInTheDocument()
  })

  it('toggles inline edit form when admin clicks edit', async () => {
    const user = userEvent.setup()
    render(<ShowCard show={makeShow()} isAdmin={true} />)

    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()

    await user.click(screen.getByTitle('Edit show'))
    expect(screen.getByTestId('show-form')).toBeInTheDocument()

    // Title changes to "Cancel editing"
    await user.click(screen.getByTitle('Cancel editing'))
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
  })

  it('shows delete button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(screen.getByTitle('Delete show')).toBeInTheDocument()
  })

  it('shows delete button for show owner', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '42', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    const show = makeShow({ submitted_by: 42 })
    render(<ShowCard show={show} isAdmin={false} userId="42" />)
    expect(screen.getByTitle('Delete show')).toBeInTheDocument()
  })

  it('does not show delete button for non-owner non-admin', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '99', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    const show = makeShow({ submitted_by: 42 })
    render(<ShowCard show={show} isAdmin={false} userId="99" />)
    expect(screen.queryByTitle('Delete show')).not.toBeInTheDocument()
  })

  it('opens delete dialog when delete button clicked', async () => {
    const user = userEvent.setup()
    render(<ShowCard show={makeShow()} isAdmin={true} />)

    expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
    await user.click(screen.getByTitle('Delete show'))
    expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
  })

  it('renders with compact density as a borderless row', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} density="compact" />)
    const article = screen.getByRole('article')
    // Compact mode uses a flat row layout without card borders
    expect(article.className).toContain('hover:bg-muted/50')
    expect(article.className).not.toContain('border')
  })

  it('renders with comfortable density by default', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const article = screen.getByRole('article')
    // Comfortable mode uses card layout with border
    expect(article.className).toContain('border')
    expect(article.className).toContain('px-3')
  })

  it('renders with expanded density with more spacious padding', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} density="expanded" />)
    const article = screen.getByRole('article')
    // Expanded mode uses card layout with more generous padding
    expect(article.className).toContain('border')
    expect(article.className).toContain('px-5')
  })

  it('shows export button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(screen.getByTestId('export-button')).toBeInTheDocument()
  })

  it('does not show export button for non-admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.queryByTestId('export-button')).not.toBeInTheDocument()
  })

  describe('expand music section', () => {
    it('shows expand button when artist has music', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: { spotify: 'https://spotify.com/band' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.getByTitle('Discover artist music')).toBeInTheDocument()
    })

    it('does not show expand button when no artist has music', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: {},
            bandcamp_embed_url: null,
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.queryByTitle('Discover artist music')).not.toBeInTheDocument()
    })

    it('toggles expanded music section on click', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()

      await user.click(screen.getByTitle('Discover artist music'))
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()

      await user.click(screen.getByTitle('Hide artist music'))
      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()
    })
  })

  describe('multiple headliners', () => {
    it('renders multiple headliners separated by bullets', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Band A', is_headliner: true }),
          makeArtist({ id: 2, name: 'Band B', is_headliner: true }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.getByText('Band A')).toBeInTheDocument()
      expect(screen.getByText('Band B')).toBeInTheDocument()
    })

    it('does not show w/ section when no support acts', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Solo', is_headliner: true }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.queryByText('w/')).not.toBeInTheDocument()
    })
  })
})
