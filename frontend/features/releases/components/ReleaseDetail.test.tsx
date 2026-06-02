import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ReleaseDetail } from './ReleaseDetail'
import type { ReleaseDetail as ReleaseDetailType } from '../types'

// next/link → plain anchor.
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

// useRelease drives all three render states.
const mockUseRelease = vi.fn()
vi.mock('../hooks/useReleases', () => ({
  useRelease: (opts: unknown) => mockUseRelease(opts),
}))

const mockUseIsAuthenticated = vi.fn(() => ({
  user: null as unknown,
  isAuthenticated: false,
}))
vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

const mockInvalidateQueries = vi.fn()
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    releases: {
      detail: (id: string | number) => ['releases', 'detail', String(id)],
    },
  },
}))

// EntityDetailLayout must render header + sidebar + children so their content
// is assertable in the same tree.
vi.mock('@/components/shared', () => ({
  EntityDetailLayout: ({
    header,
    sidebar,
    children,
  }: {
    header: React.ReactNode
    sidebar: React.ReactNode
    children: React.ReactNode
  }) => (
    <div>
      <div data-testid="header">{header}</div>
      <aside data-testid="sidebar">{sidebar}</aside>
      <main>{children}</main>
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
    <div>
      <h1>{title}</h1>
      {subtitle && <div>{subtitle}</div>}
      {actions && <div>{actions}</div>}
    </div>
  ),
  RevisionHistory: ({ entityType }: { entityType: string }) => (
    <div data-testid="revision-history">{entityType} revisions</div>
  ),
  AddToCollectionButton: () => (
    <button data-testid="add-to-collection">Collect</button>
  ),
  BracketLink: ({
    label,
    onClick,
    title,
  }: {
    label: string
    onClick: () => void
    title?: string
  }) => (
    <button onClick={onClick} title={title}>
      {label}
    </button>
  ),
}))

vi.mock('@/features/contributions', () => ({
  AttributionLine: (): null => null,
  ContributionPrompt: (): null => null,
  EntityEditDrawer: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-drawer">Edit Drawer</div> : null,
  EntitySaveSuccessBanner: ({ visible }: { visible: boolean }) =>
    visible ? <div data-testid="save-banner">Saved</div> : null,
  // PSY-661: report dialog renders only when opened; the test toggles it via
  // the [Report] bracket link.
  ReportEntityDialog: ({
    open,
    entityType,
    entityName,
  }: {
    open: boolean
    entityType: string
    entityName: string
  }) =>
    open ? (
      <div data-testid="report-dialog">
        Report {entityType} {entityName}
      </div>
    ) : null,
  useEntitySaveSuccessBanner: () => ({
    isVisible: false,
    handleSaveSuccess: vi.fn(),
  }),
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: () => <div data-testid="entity-tag-list" />,
  AddTagDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="add-tag-dialog">Add Tag</div> : null,
}))

vi.mock('@/features/radio', () => ({
  AsHeardOn: ({
    entityType,
    entitySlug,
  }: {
    entityType: string
    entitySlug: string
  }) => (
    <div data-testid="as-heard-on">
      Radio plays: {entityType}/{entitySlug}
    </div>
  ),
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

vi.mock('@/features/comments', () => ({
  CommentThread: ({
    entityType,
    entityId,
  }: {
    entityType: string
    entityId: number
  }) => (
    <div data-testid="comment-thread">
      Comments for {entityType} {entityId}
    </div>
  ),
}))

vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    asChild,
    ...props
  }: {
    children: React.ReactNode
    asChild?: boolean
    [key: string]: unknown
  }) => {
    if (asChild) return <>{children}</>
    return <button {...props}>{children}</button>
  },
}))

function makeRelease(
  overrides: Partial<ReleaseDetailType> = {}
): ReleaseDetailType {
  return {
    id: 1,
    title: 'In Rainbows',
    slug: 'in-rainbows',
    release_type: 'lp',
    release_year: 2007,
    release_date: '2007-10-10',
    cover_art_url: 'https://example.com/in-rainbows.jpg',
    description: 'The seventh studio album.',
    artists: [
      { id: 1, slug: 'radiohead', name: 'Radiohead', role: 'main' },
    ],
    labels: [
      { id: 1, name: 'XL Recordings', slug: 'xl-recordings', catalog_number: 'XLLP324' },
    ],
    external_links: [
      { id: 1, platform: 'bandcamp', url: 'https://radiohead.bandcamp.com' },
      { id: 2, platform: 'spotify', url: 'https://open.spotify.com/album/x' },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ReleaseDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAuthenticated.mockReturnValue({
      user: null,
      isAuthenticated: false,
    })
  })

  describe('loading state', () => {
    it('shows a spinner while loading', () => {
      mockUseRelease.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      })
      const { container } = render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('renders a generic error message', () => {
      mockUseRelease.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Something broke'),
      })
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.getByText('Error Loading Release')).toBeInTheDocument()
      expect(screen.getByText('Something broke')).toBeInTheDocument()
    })

    it('renders a not-found message for 404-style errors', () => {
      mockUseRelease.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('release not found'),
      })
      render(<ReleaseDetail idOrSlug="missing" />)
      expect(screen.getByText('Release Not Found')).toBeInTheDocument()
    })

    it('offers a back-to-releases link on error', () => {
      mockUseRelease.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('boom'),
      })
      render(<ReleaseDetail idOrSlug="x" />)
      expect(
        screen.getByText('Back to Releases').closest('a')
      ).toHaveAttribute('href', '/releases')
    })
  })

  describe('no data state', () => {
    it('renders not-found when data is null', () => {
      mockUseRelease.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      })
      render(<ReleaseDetail idOrSlug="x" />)
      expect(screen.getByText('Release Not Found')).toBeInTheDocument()
    })
  })

  describe('with release data', () => {
    beforeEach(() => {
      mockUseRelease.mockReturnValue({
        data: makeRelease(),
        isLoading: false,
        error: null,
      })
    })

    it('renders the release title as a heading', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(
        screen.getByRole('heading', { level: 1, name: 'In Rainbows' })
      ).toBeInTheDocument()
    })

    it('renders the cover art in the sidebar', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      const img = screen.getByAltText('In Rainbows cover art')
      expect(img).toHaveAttribute('src', 'https://example.com/in-rainbows.jpg')
    })

    it('renders the description', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(
        screen.getByText('The seventh studio album.')
      ).toBeInTheDocument()
    })

    it('renders artists linked to their pages', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      const artist = screen.getByRole('link', { name: 'Radiohead' })
      expect(artist).toHaveAttribute('href', '/artists/radiohead')
    })

    it('renders the formatted release date', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      // Day can shift by one in non-UTC test runners since the date string is
      // parsed as UTC midnight; match month + year, which are stable.
      expect(screen.getByText(/Released:\s+October \d+, 2007/)).toBeInTheDocument()
    })

    it('renders a label with its catalog number', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.getByText('XL Recordings')).toBeInTheDocument()
      expect(screen.getByText('(XLLP324)')).toBeInTheDocument()
    })

    it('renders external listen/buy links with mapped platform labels', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      const bandcamp = screen.getByText('Bandcamp').closest('a')
      expect(bandcamp).toHaveAttribute(
        'href',
        'https://radiohead.bandcamp.com'
      )
      expect(screen.getByText('Spotify')).toBeInTheDocument()
    })

    it('renders the radio "As Heard On" panel for the release slug', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(
        screen.getByText('Radio plays: release/in-rainbows')
      ).toBeInTheDocument()
    })

    it('renders the comment thread bound to the release id', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(
        screen.getByText('Comments for release 1')
      ).toBeInTheDocument()
    })

    it('does not render edit affordances for anonymous users', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.queryByText('Edit')).not.toBeInTheDocument()
      expect(screen.queryByText('Suggest edit')).not.toBeInTheDocument()
    })

    // PSY-661: the report affordance is auth-gated.
    it('does not render the Report bracket link for anonymous users', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.queryByTitle('Report an issue')).not.toBeInTheDocument()
    })
  })

  describe('contributor affordances', () => {
    it('shows "Suggest edit" for a signed-in non-privileged user', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false, user_tier: 'contributor' },
        isAuthenticated: true,
      })
      mockUseRelease.mockReturnValue({
        data: makeRelease(),
        isLoading: false,
        error: null,
      })
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.getByText('Suggest edit')).toBeInTheDocument()
    })

    it('shows "Edit" for an admin who can edit directly', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: true },
        isAuthenticated: true,
      })
      mockUseRelease.mockReturnValue({
        data: makeRelease(),
        isLoading: false,
        error: null,
      })
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.getByText('Edit')).toBeInTheDocument()
    })

    it('offers "Suggest description" when the release has no description', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false, user_tier: 'contributor' },
        isAuthenticated: true,
      })
      mockUseRelease.mockReturnValue({
        data: makeRelease({ description: null }),
        isLoading: false,
        error: null,
      })
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.getByText('Suggest description')).toBeInTheDocument()
    })

    // PSY-661: signed-in users get a [Report] affordance that opens the
    // report dialog bound to the release.
    it('shows the Report bracket link and opens the report dialog', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false, user_tier: 'contributor' },
        isAuthenticated: true,
      })
      mockUseRelease.mockReturnValue({
        data: makeRelease(),
        isLoading: false,
        error: null,
      })
      render(<ReleaseDetail idOrSlug="in-rainbows" />)

      const reportLink = screen.getByTitle('Report an issue')
      expect(reportLink).toBeInTheDocument()
      // Dialog is closed until the link is clicked.
      expect(screen.queryByTestId('report-dialog')).not.toBeInTheDocument()

      fireEvent.click(reportLink)
      expect(
        screen.getByText('Report release In Rainbows')
      ).toBeInTheDocument()
    })
  })

  describe('optional fields omitted', () => {
    beforeEach(() => {
      mockUseRelease.mockReturnValue({
        data: makeRelease({
          description: null,
          external_links: [],
          labels: [],
          release_date: null,
        }),
        isLoading: false,
        error: null,
      })
    })

    it('omits the About section without a description', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.queryByText('About')).not.toBeInTheDocument()
    })

    it('omits the Listen / Buy section without external links', () => {
      render(<ReleaseDetail idOrSlug="in-rainbows" />)
      expect(screen.queryByText('Listen / Buy')).not.toBeInTheDocument()
    })
  })
})
