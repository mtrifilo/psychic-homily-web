import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { LabelDetail as LabelDetailType } from '../types'

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

// Data hooks
const mockUseLabel = vi.fn()
const mockUseLabelRoster = vi.fn()
const mockUseLabelCatalog = vi.fn()
vi.mock('../hooks/useLabels', () => ({
  useLabel: (opts: unknown) => mockUseLabel(opts),
  useLabelRoster: (opts: unknown) => mockUseLabelRoster(opts),
  useLabelCatalog: (opts: unknown) => mockUseLabelCatalog(opts),
}))

const mockUseIsAuthenticated = vi.fn()
vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

// NB: do NOT mock @tanstack/react-query here — renderWithProviders needs the
// real QueryClient/QueryClientProvider. LabelDetail only calls useQueryClient
// inside onSuccess (never fired in these tests), so the real provider suffices.

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    labels: { detail: (id: string | number) => ['labels', 'detail', String(id)] },
  },
}))

vi.mock('@/features/releases/types', () => ({
  RELEASE_TYPES: ['lp', 'ep', 'single', 'compilation', 'live', 'remix', 'demo'],
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">
      Comments for {entityType} {entityId}
    </div>
  ),
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: () => <div data-testid="entity-tag-list" />,
  AddTagDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="add-tag-dialog">Add Tag</div> : null,
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="notify-me-button">Notify {entityName}</button>
  ),
}))

vi.mock('@/features/contributions', () => ({
  AttributionLine: (): null => null,
  EntityEditDrawer: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-drawer">Edit Drawer</div> : null,
  EntitySaveSuccessBanner: ({ visible }: { visible: boolean }) =>
    visible ? <div data-testid="save-success-banner">Saved</div> : null,
  // PSY-666: report dialog renders only when opened; the test toggles it via
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

vi.mock('@/components/shared', () => ({
  EntityDetailContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="entity-detail-container">{children}</div>
  ),
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
  SocialLinks: () => <div data-testid="social-links">Social Links</div>,
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <button data-testid="follow-button">
      Follow {entityType} {entityId}
    </button>
  ),
  AddToCollectionButton: () => (
    <button data-testid="add-to-collection">[Add to collection]</button>
  ),
  RevisionHistory: () => <div data-testid="revision-history">Revision History</div>,
  BracketLink: ({
    label,
    onClick,
    title,
  }: {
    label: string
    onClick?: () => void
    title?: string
  }) => (
    <button onClick={onClick} title={title} data-testid={`bracket-${label}`}>
      [{label}]
    </button>
  ),
  SectionHeader: ({ title }: { title: string }) => <h3>{title}</h3>,
  StatsList: ({ items }: { items: { label: string; value: React.ReactNode }[] }) => (
    <dl data-testid="stats-list">
      {items.map((i) => (
        <div key={i.label}>
          <dt>{i.label}</dt>
          <dd>{i.value}</dd>
        </div>
      ))}
    </dl>
  ),
  DenseTable: ({ children }: { children: React.ReactNode }) => (
    <table data-testid="dense-table">{children}</table>
  ),
  DenseTableGroupHeader: ({ title }: { title: string }) => (
    <tr>
      <td>{title}</td>
    </tr>
  ),
}))

import { LabelDetail } from './LabelDetail'

function makeLabel(overrides: Partial<LabelDetailType> = {}): LabelDetailType {
  return {
    id: 5,
    name: 'Sub Pop',
    slug: 'sub-pop',
    city: 'Seattle',
    state: 'WA',
    country: 'USA',
    founded_year: 1988,
    status: 'active',
    description: null,
    image_url: null,
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
    artist_count: 12,
    release_count: 340,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('LabelDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAuthenticated.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
    })
    mockUseLabelRoster.mockReturnValue({ data: { artists: [] }, isLoading: false })
    mockUseLabelCatalog.mockReturnValue({ data: { releases: [] }, isLoading: false })
  })

  describe('loading state', () => {
    it('shows a spinner while the label is fetching', () => {
      mockUseLabel.mockReturnValue({ data: undefined, isLoading: true, error: null })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(document.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows a 404 message for a not-found error', () => {
      mockUseLabel.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Label not found'),
      })

      renderWithProviders(<LabelDetail idOrSlug="bad-slug" />)
      expect(screen.getByText('Label Not Found')).toBeInTheDocument()
    })

    it('shows a generic message for a server error', () => {
      mockUseLabel.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Server exploded'),
      })

      renderWithProviders(<LabelDetail idOrSlug="x" />)
      expect(screen.getByText('Error Loading Label')).toBeInTheDocument()
      expect(screen.getByText('Server exploded')).toBeInTheDocument()
    })

    it('links back to the labels list on error', () => {
      mockUseLabel.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('not found'),
      })

      renderWithProviders(<LabelDetail idOrSlug="x" />)
      expect(screen.getByText('Back to Labels').closest('a')).toHaveAttribute(
        'href',
        '/labels'
      )
    })
  })

  describe('no data', () => {
    it('shows the not-found state when label is null', () => {
      mockUseLabel.mockReturnValue({ data: null, isLoading: false, error: null })

      renderWithProviders(<LabelDetail idOrSlug="missing" />)
      expect(screen.getByText('Label Not Found')).toBeInTheDocument()
    })
  })

  describe('successful load', () => {
    beforeEach(() => {
      mockUseLabel.mockReturnValue({
        data: makeLabel(),
        isLoading: false,
        error: null,
      })
    })

    it('renders the label name in the header and breadcrumb', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByTestId('header-slot')).toHaveTextContent('Sub Pop')
      expect(screen.getByTestId('entity-name')).toHaveTextContent('Sub Pop')
      expect(screen.getByText('Labels')).toBeInTheDocument()
    })

    it('renders location and founded year in the subtitle', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      const subtitle = screen.getByTestId('subtitle')
      expect(subtitle).toHaveTextContent('Seattle, WA')
      expect(subtitle).toHaveTextContent('Est. 1988')
    })

    it('renders the statistics block in the sidebar', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      const sidebar = screen.getByTestId('sidebar-slot')
      expect(sidebar).toHaveTextContent('Statistics')
      expect(sidebar).toHaveTextContent('Roster')
      expect(sidebar).toHaveTextContent('Catalog')
    })

    it('renders the comment thread and revision history', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByTestId('comment-thread')).toBeInTheDocument()
      expect(screen.getByTestId('revision-history')).toBeInTheDocument()
    })

    it('renders the About section when a description is present', () => {
      mockUseLabel.mockReturnValue({
        data: makeLabel({ description: 'A storied Seattle label.' }),
        isLoading: false,
        error: null,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByText('About')).toBeInTheDocument()
      expect(screen.getByText('A storied Seattle label.')).toBeInTheDocument()
    })

    it('omits the About section when there is no description', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.queryByText('About')).not.toBeInTheDocument()
    })

    it('renders the Links section only when a social link exists', () => {
      mockUseLabel.mockReturnValue({
        data: makeLabel({
          social: {
            instagram: 'https://instagram.com/subpop',
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

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByText('Links')).toBeInTheDocument()
      expect(screen.getByTestId('social-links')).toBeInTheDocument()
    })

    it('omits the Links section when every social value is empty', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.queryByText('Links')).not.toBeInTheDocument()
      expect(screen.queryByTestId('social-links')).not.toBeInTheDocument()
    })

    it('renders the roster section with artist links when artists exist', () => {
      mockUseLabelRoster.mockReturnValue({
        data: { artists: [{ id: 1, slug: 'mudhoney', name: 'Mudhoney' }] },
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      // "Roster" is also a stat label in the sidebar — scope to the content
      // slot so we assert the actual roster *section*, not the stat.
      const content = screen.getByTestId('content-slot')
      expect(within(content).getByText('Roster')).toBeInTheDocument()
      expect(within(content).getByText('Mudhoney').closest('a')).toHaveAttribute(
        'href',
        '/artists/mudhoney'
      )
    })

    it('groups the catalog by release type', () => {
      mockUseLabelCatalog.mockReturnValue({
        data: {
          releases: [
            {
              id: 1,
              title: 'Bleach',
              slug: 'bleach',
              release_type: 'lp',
              release_year: 1989,
              cover_art_url: null,
              catalog_number: 'SP34',
            },
          ],
        },
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      // "Catalog" is also a stat label in the sidebar — scope to the content
      // slot so we assert the catalog *section*, not the stat.
      const content = screen.getByTestId('content-slot')
      expect(within(content).getByText('Catalog')).toBeInTheDocument()
      expect(within(content).getByText('Albums')).toBeInTheDocument()
      expect(within(content).getByText('Bleach').closest('a')).toHaveAttribute(
        'href',
        '/releases/bleach'
      )
    })

    it('does not render edit / add-tag / report bracket links for unauthenticated users', () => {
      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.queryByTestId('bracket-Edit')).not.toBeInTheDocument()
      expect(screen.queryByTestId('bracket-Suggest edit')).not.toBeInTheDocument()
      expect(screen.queryByTestId('bracket-Add tag')).not.toBeInTheDocument()
      // PSY-666: the report affordance is auth-gated.
      expect(screen.queryByTestId('bracket-Report')).not.toBeInTheDocument()
    })

    it('shows a Suggest edit link for an authenticated non-trusted user', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByTestId('bracket-Suggest edit')).toBeInTheDocument()
      expect(screen.getByTestId('bracket-Add tag')).toBeInTheDocument()
    })

    it('shows an Edit link for a trusted-tier user', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false, user_tier: 'trusted_contributor' },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByTestId('bracket-Edit')).toBeInTheDocument()
    })

    it('offers a Suggest description link when there is no description', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)
      expect(screen.getByTestId('bracket-Suggest description')).toBeInTheDocument()
    })

    // PSY-666: signed-in users get a [Report] affordance that opens the
    // report dialog bound to the label.
    it('shows the Report bracket link and opens the report dialog', () => {
      mockUseIsAuthenticated.mockReturnValue({
        user: { is_admin: false },
        isAuthenticated: true,
        isLoading: false,
      })

      renderWithProviders(<LabelDetail idOrSlug="sub-pop" />)

      const reportLink = screen.getByTestId('bracket-Report')
      expect(reportLink).toBeInTheDocument()
      // Dialog is closed until the link is clicked.
      expect(screen.queryByTestId('report-dialog')).not.toBeInTheDocument()

      fireEvent.click(reportLink)
      expect(screen.getByText('Report label Sub Pop')).toBeInTheDocument()
    })
  })
})
