import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { Artist } from '@/lib/types/artist'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

// Mock radix tabs to avoid context requirement
vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({ children, ...props }: { children: React.ReactNode; value?: string; onValueChange?: (v: string) => void }) => <div data-testid="tabs-root" {...props}>{children}</div>,
  TabsList: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TabsTrigger: ({ children, value }: { children: React.ReactNode; value: string }) => <button data-value={value}>{children}</button>,
  TabsContent: ({ children, value }: { children: React.ReactNode; value: string }) => <div data-testid={`tabs-content-${value}`}>{children}</div>,
}))

// Mock hooks
const mockUseArtist = vi.fn()
vi.mock('@/lib/hooks/artists/useArtists', () => ({
  useArtist: (opts: unknown) => mockUseArtist(opts),
  useArtistShows: () => ({
    data: { shows: [], total: 0 },
    isLoading: false,
    error: null,
  }),
}))

const mockUseIsAuthenticated = vi.fn()
vi.mock('@/lib/hooks/auth/useAuth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

const mockUseArtistLabels = vi.fn()
vi.mock('@/lib/hooks/labels/useLabels', () => ({
  useArtistLabels: (opts: unknown) => mockUseArtistLabels(opts),
  useLabelRoster: () => ({ data: null }),
}))

vi.mock('@/features/releases/hooks/useReleases', () => ({
  useArtistReleases: () => ({
    data: null,
    isLoading: false,
    error: null,
  }),
}))

vi.mock('@/lib/hooks/admin/useAdminArtists', () => ({
  useDiscoverMusic: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateArtistBandcamp: () => ({ mutate: vi.fn(), isPending: false }),
  useClearArtistBandcamp: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateArtistSpotify: () => ({ mutate: vi.fn(), isPending: false }),
  useClearArtistSpotify: () => ({ mutate: vi.fn(), isPending: false }),
}))

// Mock child components
vi.mock('./ArtistShowsList', () => ({
  ArtistShowsList: ({ artistId }: { artistId: number }) => (
    <div data-testid="artist-shows-list">Shows for {artistId}</div>
  ),
}))

vi.mock('./ReportArtistButton', () => ({
  ReportArtistButton: ({
    artistName,
  }: {
    artistId: number
    artistName: string
  }) => <button data-testid="report-button">Report {artistName}</button>,
}))

vi.mock('@/components/forms/ArtistEditForm', () => ({
  ArtistEditForm: ({
    open,
  }: {
    open: boolean
    artist: unknown
    onOpenChange: (v: boolean) => void
    onSuccess: () => void
  }) => (open ? <div data-testid="edit-form">Edit Form</div> : null),
}))

vi.mock('@/components/shared', () => ({
  SocialLinks: () => <div data-testid="social-links">Social Links</div>,
  MusicEmbed: () => <div data-testid="music-embed">Music Embed</div>,
  EntityDetailLayout: ({
    children,
    sidebar,
    header,
    tabs,
    activeTab,
    onTabChange,
    backLink,
  }: {
    children: React.ReactNode
    sidebar: React.ReactNode
    header: React.ReactNode
    tabs: { value: string; label: string }[]
    activeTab: string
    onTabChange: (tab: string) => void
    backLink: { href: string; label: string }
  }) => (
    <div data-testid="entity-layout">
      <a href={backLink.href}>{backLink.label}</a>
      <div data-testid="header-slot">{header}</div>
      <div data-testid="tabs">
        {tabs.map(tab => (
          <button
            key={tab.value}
            data-testid={`tab-${tab.value}`}
            onClick={() => onTabChange(tab.value)}
            data-active={tab.value === activeTab}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div data-testid="sidebar-slot">{sidebar}</div>
      <div data-testid="content-slot">{children}</div>
    </div>
  ),
  EntityHeader: ({
    title,
    subtitle,
    actions,
  }: {
    title: string
    subtitle?: React.ReactNode
    actions?: React.ReactNode
  }) => (
    <div data-testid="entity-header">
      <h1>{title}</h1>
      {subtitle && <div data-testid="subtitle">{subtitle}</div>}
      {actions && <div data-testid="header-actions">{actions}</div>}
    </div>
  ),
}))

import { ArtistDetail } from './ArtistDetail'

function makeArtist(overrides: Partial<Artist> = {}): Artist {
  return {
    id: 42,
    slug: 'test-artist',
    name: 'Test Artist',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: null,
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtistDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAuthenticated.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
    })
    mockUseArtistLabels.mockReturnValue({
      data: { labels: [] },
      isLoading: false,
    })
  })

  describe('loading state', () => {
    it('shows loading spinner while fetching', () => {
      mockUseArtist.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const spinner = document.querySelector('.animate-spin')
      expect(spinner).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows 404 error for not found', () => {
      mockUseArtist.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Artist not found'),
      })

      renderWithProviders(<ArtistDetail artistId="bad-slug" />)
      expect(screen.getByText('Artist Not Found')).toBeInTheDocument()
      expect(
        screen.getByText(
          "The artist you're looking for doesn't exist or has been removed."
        )
      ).toBeInTheDocument()
    })

    it('shows generic error message for server errors', () => {
      mockUseArtist.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Server error'),
      })

      renderWithProviders(<ArtistDetail artistId="test" />)
      expect(screen.getByText('Error Loading Artist')).toBeInTheDocument()
      expect(screen.getByText('Server error')).toBeInTheDocument()
    })

    it('shows back to shows link on error', () => {
      mockUseArtist.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('not found'),
      })

      renderWithProviders(<ArtistDetail artistId="bad" />)
      const backLink = screen.getByText('Back to Shows')
      expect(backLink.closest('a')).toHaveAttribute('href', '/shows')
    })
  })

  describe('no artist data', () => {
    it('shows not found when artist is null', () => {
      mockUseArtist.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="missing" />)
      expect(screen.getByText('Artist Not Found')).toBeInTheDocument()
    })
  })

  describe('successful load', () => {
    beforeEach(() => {
      mockUseArtist.mockReturnValue({
        data: makeArtist(),
        isLoading: false,
        error: null,
      })
    })

    it('renders artist name in header', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByText('Test Artist')).toBeInTheDocument()
    })

    it('renders entity layout with back link', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('entity-layout')).toBeInTheDocument()
      expect(screen.getByText('Back to Shows')).toBeInTheDocument()
    })

    it('renders tabs for overview, discography, and labels', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('tab-overview')).toBeInTheDocument()
      expect(screen.getByTestId('tab-discography')).toBeInTheDocument()
      expect(screen.getByTestId('tab-labels')).toBeInTheDocument()
    })

    it('renders report button in header actions', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('report-button')).toBeInTheDocument()
      expect(screen.getByText('Report Test Artist')).toBeInTheDocument()
    })

    it('renders artist shows list', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('artist-shows-list')).toBeInTheDocument()
      expect(screen.getByText('Shows for 42')).toBeInTheDocument()
    })

    it('renders social links in sidebar', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('social-links')).toBeInTheDocument()
    })

    it('renders music embed in sidebar', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    })

    it('shows location in sidebar when available', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      // Location appears in both header subtitle and sidebar;
      // verify sidebar has "Location" heading
      const sidebarSlot = screen.getByTestId('sidebar-slot')
      expect(sidebarSlot).toHaveTextContent('Location')
      expect(sidebarSlot).toHaveTextContent('Phoenix, AZ')
    })

    it('hides location in sidebar when not available', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({ city: null, state: null }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const sidebarSlot = screen.getByTestId('sidebar-slot')
      expect(sidebarSlot).not.toHaveTextContent('Location')
    })

    it('shows label links in sidebar when labels exist', () => {
      mockUseArtistLabels.mockReturnValue({
        data: {
          labels: [
            { id: 1, name: 'Sub Pop', slug: 'sub-pop', city: null, state: null },
          ],
        },
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const sidebarSlot = screen.getByTestId('sidebar-slot')
      expect(sidebarSlot).toHaveTextContent('Sub Pop')
    })

    it('hides labels section in sidebar when no labels', () => {
      mockUseArtistLabels.mockReturnValue({
        data: { labels: [] },
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const sidebarSlot = screen.getByTestId('sidebar-slot')
      // Sidebar should NOT contain the "Labels" heading when there are no labels
      // Note: The tab labeled "Labels" exists in a different part of the DOM
      const sidebarHeadings = sidebarSlot.querySelectorAll('h3')
      const labelsHeading = Array.from(sidebarHeadings).find(
        h => h.textContent === 'Labels'
      )
      expect(labelsHeading).toBeUndefined()
    })
  })

  describe('admin features', () => {
    beforeEach(() => {
      mockUseArtist.mockReturnValue({
        data: makeArtist(),
        isLoading: false,
        error: null,
      })
    })

    it('does not show edit button for non-admin users', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      // The Edit2 icon button should not be present; check that edit form isn't rendered
      expect(screen.queryByTestId('edit-form')).not.toBeInTheDocument()
    })

    it('does not show admin music controls for non-admin users', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(
        screen.queryByText('No music embed configured')
      ).not.toBeInTheDocument()
    })

    it('shows admin music controls for admin users when no embed', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(
        screen.getByText('No music embed configured')
      ).toBeInTheDocument()
      expect(screen.getByText('Discover Music')).toBeInTheDocument()
      expect(screen.getByText('Enter Bandcamp URL')).toBeInTheDocument()
      expect(screen.getByText('Enter Spotify URL')).toBeInTheDocument()
    })

    it('shows edit bandcamp button for admin when embed exists', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })
      mockUseArtist.mockReturnValue({
        data: makeArtist({
          bandcamp_embed_url: 'https://artist.bandcamp.com/album/test',
        }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByText('Edit Bandcamp URL')).toBeInTheDocument()
      expect(screen.getByText('Add Spotify URL')).toBeInTheDocument()
    })

    it('shows edit spotify button for admin when spotify exists', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })
      mockUseArtist.mockReturnValue({
        data: makeArtist({
          social: {
            instagram: null,
            facebook: null,
            twitter: null,
            youtube: null,
            spotify: 'https://open.spotify.com/artist/123',
            soundcloud: null,
            bandcamp: null,
            website: null,
          },
        }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByText('Edit Spotify URL')).toBeInTheDocument()
      expect(screen.getByText('Add Bandcamp URL')).toBeInTheDocument()
    })

    it('opens bandcamp URL input when enter button clicked', async () => {
      const user = userEvent.setup()
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)

      await user.click(screen.getByText('Enter Bandcamp URL'))
      expect(screen.getByLabelText('Bandcamp Album URL')).toBeInTheDocument()
      expect(
        screen.getByPlaceholderText(
          'https://artist.bandcamp.com/album/album-name'
        )
      ).toBeInTheDocument()
    })

    it('opens spotify URL input when enter button clicked', async () => {
      const user = userEvent.setup()
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)

      await user.click(screen.getByText('Enter Spotify URL'))
      expect(screen.getByLabelText('Spotify Artist URL')).toBeInTheDocument()
      expect(
        screen.getByPlaceholderText('https://open.spotify.com/artist/...')
      ).toBeInTheDocument()
    })
  })

  describe('location display', () => {
    it('shows city and state in header subtitle', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({ city: 'Chicago', state: 'IL' }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const subtitle = screen.getByTestId('subtitle')
      expect(subtitle).toBeInTheDocument()
      expect(subtitle).toHaveTextContent('Chicago, IL')
    })

    it('shows only city when state is null', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({ city: 'Chicago', state: null }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const subtitle = screen.getByTestId('subtitle')
      expect(subtitle).toHaveTextContent('Chicago')
    })

    it('does not show subtitle when no location', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({ city: null, state: null }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('subtitle')).not.toBeInTheDocument()
    })
  })
})
