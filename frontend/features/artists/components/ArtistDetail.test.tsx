import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { Artist, ArtistShow, ArtistAlias } from '../types'

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

// Mock hooks
const mockUseArtist = vi.fn()
vi.mock('../hooks/useArtists', () => ({
  useArtist: (opts: unknown) => mockUseArtist(opts),
  useArtistShows: () => ({
    data: { shows: [] as ArtistShow[], total: 0 },
    isLoading: false,
    error: null as Error | null,
  }),
}))

const mockUseIsAuthenticated = vi.fn()
vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

const mockUseArtistLabels = vi.fn()
vi.mock('@/features/labels/hooks/useLabels', () => ({
  useArtistLabels: (opts: unknown) => mockUseArtistLabels(opts),
  useLabelRoster: () => ({ data: null as unknown }),
}))

vi.mock('@/features/releases/hooks/useReleases', () => ({
  useArtistReleases: () => ({
    data: null as unknown,
    isLoading: false,
    error: null as Error | null,
  }),
}))

// PSY-1110: the discover + save mutations are exposed as stable module-level
// vi.fn()s so a test can drive their onSuccess/onError callbacks and exercise
// the candidate-pick UI (previously useDiscoverMusic was a no-op, so candidates
// never rendered). Default impl is a no-op; each picker test sets its own.
const mockDiscoverMusicMutate = vi.fn()
const mockUpdateBandcampMutate = vi.fn()
const mockUpdateSpotifyMutate = vi.fn()
// Bandcamp candidate accept routes through useArtistUpdate (sets social.bandcamp
// = profile root → backend async embed resolver, PSY-1190/1198), so this mutate
// must be inspectable in the picker tests.
const mockArtistUpdateMutate = vi.fn()
vi.mock('@/lib/hooks/admin/useAdminArtists', () => ({
  useDiscoverMusic: () => ({ mutate: mockDiscoverMusicMutate, isPending: false }),
  useUpdateArtistBandcamp: () => ({ mutate: mockUpdateBandcampMutate, isPending: false }),
  useClearArtistBandcamp: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateArtistSpotify: () => ({ mutate: mockUpdateSpotifyMutate, isPending: false }),
  useClearArtistSpotify: () => ({ mutate: vi.fn(), isPending: false }),
  useArtistUpdate: () => ({ mutate: mockArtistUpdateMutate, isPending: false }),
  useArtistAliases: () => ({ data: { aliases: [] as ArtistAlias[] }, isLoading: false }),
}))

// Mock child components
vi.mock('./ArtistShowsList', () => ({
  ArtistShowsList: ({ artistId }: { artistId: number }) => (
    <div data-testid="artist-shows-list">Shows for {artistId}</div>
  ),
}))

vi.mock('@/features/contributions', () => ({
  EntityEditDrawer: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-drawer">Edit Drawer</div> : null,
  EntitySaveSuccessBanner: ({ visible }: { visible: boolean }) =>
    visible ? <div data-testid="save-success-banner">Changes saved</div> : null,
  useEntitySaveSuccessBanner: () => ({
    isVisible: false,
    handleSaveSuccess: vi.fn(),
  }),
  AttributionLine: (): null => null,
  ReportEntityDialog: ({ open, entityName }: { open: boolean; entityName: string }) =>
    open ? <div data-testid="report-dialog">Report {entityName}</div> : null,
  ContributionPrompt: (): null => null,
  useSuggestEdit: () => ({ mutate: vi.fn(), isPending: false }),
}))

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: () => '/artists/test-artist',
  useRouter: () => ({ push: vi.fn() }),
}))

// Mock NotifyMeButton to avoid deep notification hooks dependency
vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityType: string; entityId: number; entityName: string; variant?: string }) => (
    <button data-testid="notify-me-button">Notify {entityName}</button>
  ),
}))

// PSY-364: ArtistDetail mounts <BillComposition>, which fires its own fetch.
// Stub it out so this suite doesn't need bill-composition fixtures.
vi.mock('./BillComposition', () => ({
  BillComposition: (): null => null,
}))

// PSY-945: ArtistDetail also mounts these query-firing children. Left
// un-stubbed they each issue a real fetch (festival-trajectory, tags,
// radio-plays, entity-collections, similar-artists graph) that this suite
// never awaits — under the global onUnhandledRequest:'error' policy MSW
// rejects them, and under the old 'bypass' policy they leaked to the real
// network and could still be pending at worker teardown ("Closing rpc while
// fetch was pending"). Stub them so the suite stays hermetic.
vi.mock('@/features/festivals/components/ArtistTrajectoryChart', () => ({
  ArtistTrajectoryChart: (): null => null,
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: (): null => null,
  AddTagDialog: (): null => null,
}))

vi.mock('@/features/radio', () => ({
  AsHeardOn: (): null => null,
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: (): null => null,
}))

// PSY-664: the graph dialog mock surfaces `open` + a close affordance wired
// to `onOpenChange(false)` so the close-path hash cleanup is testable without
// rendering the real ForceGraph2D-backed dialog. The real Dialog routes every
// close path (X, Escape, backdrop) through this same `onOpenChange`.
vi.mock('./RelatedArtists', () => ({
  ArtistSimilarSidebar: (): null => null,
  ArtistGraphDialog: ({
    open,
    onOpenChange,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
  }) =>
    open ? (
      <div data-testid="graph-dialog">
        <button
          data-testid="graph-dialog-close"
          onClick={() => onOpenChange(false)}
        >
          Close graph
        </button>
      </div>
    ) : null,
}))

// PSY-641: ArtistDetail is now a flat two-column layout — no page-level tabs.
// The mock renders header / sidebar / children slots directly. The new
// density primitives (BracketLink, SectionHeader, StatsList) get lightweight
// mocks so their props are inspectable.
vi.mock('@/components/shared', () => ({
  SocialLinks: () => <div data-testid="social-links">Social Links</div>,
  MusicEmbed: () => <div data-testid="music-embed">Music Embed</div>,
  ImageAttribution: () => null,
  EntityDetailLayout: ({
    children,
    sidebar,
    header,
    fallback,
    entityName,
  }: {
    children: React.ReactNode
    sidebar: React.ReactNode
    header: React.ReactNode
    fallback: { href: string; label: string }
    entityName: string
  }) => (
    <div data-testid="entity-layout">
      <a href={fallback.href}>{fallback.label}</a>
      <span data-testid="entity-name">{entityName}</span>
      <div data-testid="header-slot">{header}</div>
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
  RevisionHistory: () => <div data-testid="revision-history">Revision History</div>,
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number; variant?: string }) => (
    <button data-testid="follow-button">Follow {entityType} {entityId}</button>
  ),
  EntityDescription: ({ description, canEdit }: { description: string | null | undefined; canEdit: boolean }) => (
    <div data-testid="entity-description">{description || (canEdit ? 'Add description' : '')}</div>
  ),
  AddToCollectionButton: () => (
    <button data-testid="add-to-collection">[Add to collection]</button>
  ),
  BracketLink: ({
    label,
    href,
    onClick,
    title,
  }: {
    label: string
    href?: string
    onClick?: () => void
    title?: string
  }) =>
    href ? (
      <a href={href} title={title} data-testid={`bracket-${label}`}>
        [{label}]
      </a>
    ) : (
      <button onClick={onClick} title={title} data-testid={`bracket-${label}`}>
        [{label}]
      </button>
    ),
  SectionHeader: ({ title }: { title: string }) => <h3>{title}</h3>,
  StatsList: ({ items }: { items: { label: string; value: React.ReactNode }[] }) => (
    <dl data-testid="stats-list">
      {items.map(i => (
        <div key={i.label}>
          <dt>{i.label}</dt>
          <dd>{i.value}</dd>
        </div>
      ))}
    </dl>
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
      const backLink = screen.getByText('Back to Artists')
      expect(backLink.closest('a')).toHaveAttribute('href', '/artists')
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
      const headerSlot = screen.getByTestId('header-slot')
      expect(headerSlot).toHaveTextContent('Test Artist')
    })

    it('renders entity layout with breadcrumb fallback', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('entity-layout')).toBeInTheDocument()
      expect(screen.getByText('Artists')).toBeInTheDocument()
      expect(screen.getByTestId('entity-name')).toHaveTextContent('Test Artist')
    })

    it('renders a flat single-scroll main column with no page-level tabs', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      // The Discography / Labels page-level tabs were removed in PSY-641.
      expect(screen.queryByTestId('tab-discography')).not.toBeInTheDocument()
      expect(screen.queryByTestId('tab-labels')).not.toBeInTheDocument()
      // Main-column content renders directly.
      expect(screen.getByTestId('artist-shows-list')).toBeInTheDocument()
      expect(screen.getByTestId('revision-history')).toBeInTheDocument()
      // The community Discussion (comments) section was removed from artist
      // pages in PSY-980 — the shared CommentThread is no longer mounted here.
      expect(screen.queryByTestId('comment-thread')).not.toBeInTheDocument()
    })

    it('renders the header action linkbox as bracket links', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const headerActions = screen.getByTestId('header-actions')
      // Stateful trio + the always-on [Graph] link.
      expect(headerActions).toHaveTextContent('Follow')
      expect(headerActions).toHaveTextContent('Notify')
      expect(headerActions).toHaveTextContent('Add to collection')
      // [Graph] is a button that opens the page-level Dialog (PSY-645).
      // The legacy href="#graph" auto-open still works via the parent's
      // useUrlHash → graphDialogOpen plumbing, but the link itself no
      // longer renders as an anchor.
      expect(screen.getByTestId('bracket-Graph').tagName).toBe('BUTTON')
    })

    // PSY-664: the graph dialog drives the `#graph` URL hash so the open
    // state is shareable/deep-linkable. Opening pushes `#graph`; every close
    // path (X, Escape, backdrop — all route through onOpenChange) must strip
    // it again, otherwise a refresh or shared link re-opens the dialog.
    describe('graph dialog #graph hash (PSY-664)', () => {
      // window.location.hash is real jsdom state and not reset by the global
      // afterEach — restore it per test so a stray `#graph` cannot leak into
      // an unrelated suite (and re-trip the auto-open path here).
      let originalHref: string

      beforeEach(() => {
        originalHref = window.location.href
      })

      afterEach(() => {
        window.history.replaceState(null, '', originalHref)
      })

      it('auto-opens the dialog when the URL already carries #graph', () => {
        window.history.replaceState(null, '', '/artists/test-artist#graph')
        renderWithProviders(<ArtistDetail artistId="test-artist" />)
        expect(screen.getByTestId('graph-dialog')).toBeInTheDocument()
      })

      it('pushes #graph to the URL when [Graph] opens the dialog', async () => {
        const user = userEvent.setup()
        renderWithProviders(<ArtistDetail artistId="test-artist" />)
        expect(window.location.hash).toBe('')

        await user.click(screen.getByTestId('bracket-Graph'))

        expect(screen.getByTestId('graph-dialog')).toBeInTheDocument()
        expect(window.location.hash).toBe('#graph')
      })

      it('clears #graph from the URL when the dialog closes', async () => {
        const user = userEvent.setup()
        window.history.replaceState(null, '', '/artists/test-artist#graph')
        renderWithProviders(<ArtistDetail artistId="test-artist" />)
        // Auto-opened from the hash.
        expect(screen.getByTestId('graph-dialog')).toBeInTheDocument()
        expect(window.location.hash).toBe('#graph')

        await user.click(screen.getByTestId('graph-dialog-close'))

        expect(screen.queryByTestId('graph-dialog')).not.toBeInTheDocument()
        expect(window.location.hash).toBe('')
        // Path + search are preserved (only the hash is stripped).
        expect(window.location.pathname).toBe('/artists/test-artist')
      })

      it('leaves an unrelated hash untouched when the dialog closes', async () => {
        const user = userEvent.setup()
        window.history.replaceState(null, '', '/artists/test-artist#discussion')
        renderWithProviders(<ArtistDetail artistId="test-artist" />)
        // #discussion does not auto-open — open via the [Graph] button, which
        // replaces the hash with #graph.
        await user.click(screen.getByTestId('bracket-Graph'))
        expect(window.location.hash).toBe('#graph')

        await user.click(screen.getByTestId('graph-dialog-close'))
        expect(window.location.hash).toBe('')
      })
    })

    it('shows the report bracket link for authenticated users', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTitle('Report an issue')).toBeInTheDocument()
    })

    it('does not show the report bracket link for unauthenticated users', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTitle('Report an issue')).not.toBeInTheDocument()
    })

    it('shows a Suggest edit link for authenticated non-trusted users', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('bracket-Suggest edit')).toBeInTheDocument()
    })

    it('shows an Edit link for trusted-tier users', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false, user_tier: 'trusted_contributor' },
        isAuthenticated: true,
        isLoading: false,
      })
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('bracket-Edit')).toBeInTheDocument()
    })

    it('shows the [Add tag] bracket link for authenticated users (PSY-654)', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('bracket-Add tag')).toBeInTheDocument()
    })

    it('does not show the [Add tag] bracket link for unauthenticated users', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('bracket-Add tag')).not.toBeInTheDocument()
    })

    it('renders artist shows list', () => {
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('artist-shows-list')).toBeInTheDocument()
      expect(screen.getByText('Shows for 42')).toBeInTheDocument()
    })

    it('renders the statistics block in the sidebar when stats are present', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({
          stats: {
            releases: 4,
            labels: 2,
            shows_tracked: 13,
            similar_artists: 8,
            festival_appearances: 3,
          },
        }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const sidebarSlot = screen.getByTestId('sidebar-slot')
      expect(sidebarSlot).toHaveTextContent('Statistics')
      expect(sidebarSlot).toHaveTextContent('Releases')
      expect(sidebarSlot).toHaveTextContent('13')
    })

    it('omits the statistics block when stats are absent', () => {
      // Default makeArtist() has no `stats` field.
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('stats-list')).not.toBeInTheDocument()
    })

    it('renders social links in the sidebar when the artist has any', () => {
      mockUseArtist.mockReturnValue({
        data: makeArtist({
          social: {
            instagram: 'https://instagram.com/test',
            facebook: null,
            twitter: null,
            youtube: null,
            spotify: null,
            soundcloud: null,
            bandcamp: null,
            website: null,
          },
        }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.getByTestId('social-links')).toBeInTheDocument()
    })

    it('hides the links section when the artist has no social links', () => {
      // Default makeArtist() has all-null social fields.
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('social-links')).not.toBeInTheDocument()
    })

    it('renders the music embed in the sidebar when a music link exists', () => {
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
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    })

    it('hides the music embed when the artist has no music link', () => {
      // Default makeArtist() has no bandcamp_embed_url and all-null social.
      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()
    })

    it('renders Top tracks FIRST in the sidebar, before Statistics (PSY-1065)', () => {
      // Listening is the fastest way to judge an unfamiliar band — the
      // embed leads the column.
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
          stats: {
            releases: 2,
            labels: 1,
            shows_tracked: 3,
            similar_artists: 0,
            festival_appearances: 0,
          },
        }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      const embed = screen.getByTestId('music-embed')
      const statsHeader = screen.getByText('Statistics')
      expect(
        embed.compareDocumentPosition(statsHeader) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
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

    it('does not show edit drawer for non-admin users by default', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      expect(screen.queryByTestId('edit-drawer')).not.toBeInTheDocument()
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

  // PSY-1110: the candidate-pick UI was previously untested because
  // useDiscoverMusic was mocked to a no-op so candidates never rendered. These
  // drive the mutation onSuccess/onError callbacks directly.
  describe('admin music discovery candidate picker (PSY-1110, PSY-1198)', () => {
    const BC = 'https://soroche.bandcamp.com'
    const SP = 'https://open.spotify.com/artist/0OdUWJ0sBjDrqHygGUXeCF'

    // PSY-1198 contract: flat candidate list, each carrying its own platform +
    // region confidence tier (`high` | `review`).
    function makeCandidate(
      url: string,
      platform: 'bandcamp' | 'spotify',
      overrides: Record<string, unknown> = {}
    ) {
      return {
        platform,
        url,
        source: 'musicbrainz',
        mb_artist_id: 'mbid-soroche',
        mb_artist_name: 'Soroche',
        confidence: 'high',
        region_match: true,
        live: true,
        ...overrides,
      }
    }

    beforeEach(() => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
        isLoading: false,
      })
      // Default makeArtist() has no embed → the "Discover Music" button renders.
      mockUseArtist.mockReturnValue({
        data: makeArtist(),
        isLoading: false,
        error: null,
      })
    })

    afterEach(() => {
      // clearAllMocks (global beforeEach) clears calls but not implementations;
      // reset these so an onSuccess/onError impl can't leak into a later test.
      mockDiscoverMusicMutate.mockReset()
      mockUpdateBandcampMutate.mockReset()
      mockUpdateSpotifyMutate.mockReset()
      mockArtistUpdateMutate.mockReset()
    })

    it('renders bandcamp + spotify candidate cards grouped by platform', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({
            artist_id: 42,
            candidates: [
              makeCandidate(BC, 'bandcamp'),
              makeCandidate(SP, 'spotify', {
                confidence: 'review',
                region_match: false,
                notes: 'possible touring act or namesake — verify before linking',
              }),
            ],
          })
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))

      expect(
        screen.getByText('Pick streaming links for this artist')
      ).toBeInTheDocument()
      expect(screen.getByText('Bandcamp candidates')).toBeInTheDocument()
      expect(screen.getByText('Spotify candidates')).toBeInTheDocument()
      expect(screen.getByText(BC)).toBeInTheDocument()
      expect(screen.getByText(SP)).toBeInTheDocument()
      expect(screen.getAllByRole('button', { name: 'Use this' })).toHaveLength(2)
    })

    it('renders the confidence tier and verify caveat for a review candidate', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({
            artist_id: 42,
            candidates: [
              makeCandidate(BC, 'bandcamp'), // high
              makeCandidate(SP, 'spotify', {
                confidence: 'review',
                region_match: false,
                notes: 'instrumental act from Perth, Australia',
              }),
            ],
          })
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))

      // High candidate → clear tier badge; review candidate → "Verify" + caveat.
      expect(screen.getByText('High confidence')).toBeInTheDocument()
      expect(screen.getByText('Verify')).toBeInTheDocument()
      expect(
        screen.getByText(/possible touring act or namesake/i)
      ).toBeInTheDocument()
      // MB disambiguation note is surfaced verbatim.
      expect(
        screen.getByText('instrumental act from Perth, Australia')
      ).toBeInTheDocument()
    })

    it('accepts a Spotify candidate via the spotify update path and closes the picker', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({
            artist_id: 42,
            candidates: [makeCandidate(SP, 'spotify')],
          })
      )
      mockUpdateSpotifyMutate.mockImplementation(
        (_args: unknown, opts: { onSuccess: () => void }) => opts.onSuccess()
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))
      await user.click(screen.getByRole('button', { name: 'Use this' }))

      expect(mockUpdateSpotifyMutate).toHaveBeenCalledWith(
        { artistId: 42, spotifyUrl: SP },
        expect.anything()
      )
      expect(screen.getByText('Spotify URL saved')).toBeInTheDocument()
      expect(
        screen.queryByText('Pick streaming links for this artist')
      ).not.toBeInTheDocument()
    })

    it('accepts a Bandcamp candidate via the artist-update path (async embed resolve)', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({
            artist_id: 42,
            candidates: [makeCandidate(BC, 'bandcamp')],
          })
      )
      mockArtistUpdateMutate.mockImplementation(
        (_args: unknown, opts: { onSuccess: () => void }) => opts.onSuccess()
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))
      await user.click(screen.getByRole('button', { name: 'Use this' }))

      // Bandcamp accept sets social.bandcamp = profile root via the artist PATCH;
      // it does NOT go through the album/track-only bandcamp endpoint.
      expect(mockArtistUpdateMutate).toHaveBeenCalledWith(
        { artistId: 42, data: { bandcamp: BC } },
        expect.anything()
      )
      expect(mockUpdateBandcampMutate).not.toHaveBeenCalled()
      // The copy must NOT falsely claim the embed is live — it says "background".
      expect(screen.getByText(/fetching the embed in the background/i)).toBeInTheDocument()
      expect(
        screen.queryByText('Pick streaming links for this artist')
      ).not.toBeInTheDocument()
    })

    it('shows a destructive banner and keeps the picker open when a pick fails', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({
            artist_id: 42,
            candidates: [makeCandidate(BC, 'bandcamp')],
          })
      )
      mockArtistUpdateMutate.mockImplementation(
        (_args: unknown, opts: { onError: (e: Error) => void }) =>
          opts.onError(new Error('Invalid Bandcamp URL'))
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))
      await user.click(screen.getByRole('button', { name: 'Use this' }))

      const banner = screen
        .getByText('Invalid Bandcamp URL')
        .closest('[role="alert"]')
      expect(banner).toBeInTheDocument()
      expect(banner?.className).toContain('text-destructive')
      // The error path leaves the candidates on screen so the admin can retry.
      expect(
        screen.getByText('Pick streaming links for this artist')
      ).toBeInTheDocument()
    })

    it('shows a "no candidates" alert when discovery returns an empty list', async () => {
      const user = userEvent.setup()
      mockDiscoverMusicMutate.mockImplementation(
        (_id: number, opts: { onSuccess: (d: unknown) => void }) =>
          opts.onSuccess({ artist_id: 42, candidates: [] })
      )

      renderWithProviders(<ArtistDetail artistId="test-artist" />)
      await user.click(screen.getByRole('button', { name: /Discover Music/ }))

      expect(
        screen.getByText(/No streaming-link candidates found/i)
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Use this' })
      ).not.toBeInTheDocument()
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

// PSY-644 removed ArtistShowsList's internal Upcoming/Past Radix tabs; the
// pre-PSY-644 "switches aria-selected between upcoming and past tabs" test
// has been retired. The new two-section structure (upcoming always visible,
// past collapsed via `[Show]`/`[Hide]`) is covered in
// features/artists/components/ArtistShowsList.test.tsx.
