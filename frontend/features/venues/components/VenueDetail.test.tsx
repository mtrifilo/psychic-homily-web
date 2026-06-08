import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VenueDetail } from './VenueDetail'
import type { Venue } from '../types'

// Mock AuthContext.
// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
type MockAuthContextValue = {
  user: { id: string; is_admin: boolean; user_tier?: string } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthContextValue>(() => ({
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

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/venues/test-venue',
}))

// Mock TanStack Query
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({
    invalidateQueries: vi.fn(),
  }),
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    venues: {
      detail: (id: string) => ['venues', 'detail', id],
      shows: (id: number | string) => ['venues', 'shows', id],
    },
  },
  createInvalidateQueries: () => ({
    venues: vi.fn(),
  }),
}))

// Mock useVenue, useVenueGenres, and useVenueShows hooks
const mockUseVenue = vi.fn()
type MockUseVenueGenresValue = {
  data:
    | { genres: Array<{ tag_id: number; name: string; slug: string; count: number }> }
    | null
}
const mockUseVenueGenres = vi.fn<(_id?: number) => MockUseVenueGenresValue>(
  () => ({ data: null })
)
const mockUseVenueShows = vi.fn((_opts: unknown) => ({
  data: { shows: [], total: 0 } as { shows: unknown[]; total: number },
  isLoading: false,
  error: null as Error | null,
}))
vi.mock('../hooks/useVenues', () => ({
  useVenue: (opts: unknown) => mockUseVenue(opts),
  useVenueGenres: (id: number) => mockUseVenueGenres(id),
  useVenueShows: (opts: unknown) => mockUseVenueShows(opts),
}))

// Shared mutate spies so tests can assert which path an inline description save
// takes: admins -> venueUpdate.mutate; trusted/submitter -> suggestVenueEdit.mutate.
// Hoisted so they can be referenced inside the vi.mock factories below.
const { mockVenueUpdateMutate, mockSuggestEditMutate } = vi.hoisted(() => ({
  mockVenueUpdateMutate: vi.fn(),
  mockSuggestEditMutate: vi.fn(),
}))

// Mock useVenueEdit hook
vi.mock('../hooks/useVenueEdit', () => ({
  useVenueUpdate: () => ({ mutate: mockVenueUpdateMutate, isPending: false }),
  useVenueDelete: () => ({ mutate: vi.fn(), isPending: false }),
}))

// Mock child components
vi.mock('@/components/shared', () => ({
  SocialLinks: () => <div data-testid="social-links" />,
  RevisionHistory: () => <div data-testid="revision-history" />,
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <button data-testid="follow-button">Follow {entityType} {entityId}</button>
  ),
  Breadcrumb: ({ fallback, currentPage }: { fallback: { href: string; label: string }; currentPage: string }) => (
    <nav aria-label="Breadcrumb"><a href={fallback.href}>{fallback.label}</a><span>{currentPage}</span></nav>
  ),
  TagPill: ({ label, href }: { label: string; href: string }) => (
    <a href={href} data-testid="tag-pill">{label}</a>
  ),
  EntityDescription: ({
    description,
    canEdit,
    onSave,
  }: {
    description: string | null | undefined
    canEdit: boolean
    onSave?: (description: string) => Promise<void>
  }) => (
    <div data-testid="entity-description">
      {description || (canEdit ? 'Add description' : '')}
      {canEdit && (
        <button
          data-testid="entity-description-save"
          onClick={() => onSave?.('New venue bio')}
        >
          Save description
        </button>
      )}
    </div>
  ),
  AddToCollectionButton: () => <button data-testid="add-to-collection">Collect</button>,
  EntityHeader: ({ title, subtitle, actions }: { title: string; subtitle?: React.ReactNode; actions?: React.ReactNode }) => (
    <div>
      <h1>{title}</h1>
      {subtitle && <div>{subtitle}</div>}
      {actions && <div>{actions}</div>}
    </div>
  ),
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityType: string; entityId: number; entityName: string }) => (
    <button data-testid="notify-me-button">Notify {entityName}</button>
  ),
}))

vi.mock('./VenueLocationCard', () => ({
  VenueLocationCard: ({ name }: { name: string }) => (
    <div data-testid="location-card">{name} Location</div>
  ),
}))

vi.mock('./VenueShowsList', () => ({
  VenueShowsList: ({ venueId }: { venueId: number }) => (
    <div data-testid="venue-shows-list">Shows for venue {venueId}</div>
  ),
}))

// PSY-365: VenueBillNetwork pulls in d3-force + react-force-graph (canvas
// can't render in jsdom). Stub it; coverage lives in
// VenueBillNetwork.test.tsx.
vi.mock('./VenueBillNetwork', () => ({
  VenueBillNetwork: ({ venueIdOrSlug }: { venueIdOrSlug: number | string }) => (
    <div data-testid="venue-bill-network">Bill Network for {String(venueIdOrSlug)}</div>
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
  ReportEntityDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="report-dialog">Report Dialog</div> : null,
  ContributionPrompt: (): null => null,
  useSuggestEdit: () => ({ mutate: mockSuggestEditMutate, isPending: false }),
}))

vi.mock('./DeleteVenueDialog', () => ({
  DeleteVenueDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null,
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">Comments for {entityType} {entityId}</div>
  ),
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: () => <div data-testid="entity-tag-list" />,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, asChild, ...props }: { children: React.ReactNode; asChild?: boolean; [key: string]: unknown }) => {
    if (asChild) return <>{children}</>
    return <button {...props}>{children}</button>
  },
}))

function makeVenue(overrides: Partial<Venue> = {}): Venue {
  return {
    id: 1,
    slug: 'the-rebel-lounge',
    name: 'The Rebel Lounge',
    address: '2303 E Indian School Rd',
    city: 'Phoenix',
    state: 'AZ',
    zipcode: '85016',
    verified: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('VenueDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseVenueGenres.mockReturnValue({ data: null })
  })

  describe('loading state', () => {
    it('shows spinner when loading', () => {
      mockUseVenue.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      })
      const { container } = render(<VenueDetail venueId="1" />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message', () => {
      mockUseVenue.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Something went wrong'),
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByText('Error Loading Venue')).toBeInTheDocument()
      expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    })

    it('shows 404 message for not found errors', () => {
      const error = new Error('Not found')
      ;(error as unknown as { status: number }).status = 404
      mockUseVenue.mockReturnValue({
        data: undefined,
        isLoading: false,
        error,
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByText('Venue Not Found')).toBeInTheDocument()
      expect(screen.getByText(/doesn't exist or has been removed/)).toBeInTheDocument()
    })

    it('shows back to venues link on error', () => {
      mockUseVenue.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Error'),
      })
      render(<VenueDetail venueId="1" />)
      const link = screen.getByText('Back to Venues').closest('a')
      expect(link).toHaveAttribute('href', '/venues')
    })
  })

  describe('no data state', () => {
    it('shows not found when data is null', () => {
      mockUseVenue.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByText('Venue Not Found')).toBeInTheDocument()
    })
  })

  describe('with venue data', () => {
    beforeEach(() => {
      mockUseVenue.mockReturnValue({
        data: makeVenue(),
        isLoading: false,
        error: null,
      })
    })

    it('renders venue name as heading', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByRole('heading', { level: 1, name: 'The Rebel Lounge' })).toBeInTheDocument()
    })

    it('renders venue location', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    })

    it('renders breadcrumb with link to venues', () => {
      render(<VenueDetail venueId="1" />)
      const breadcrumbNav = screen.getByRole('navigation', { name: /Breadcrumb/ })
      expect(breadcrumbNav).toBeInTheDocument()
      const link = breadcrumbNav.querySelector('a')
      expect(link).toHaveAttribute('href', '/venues')
    })

    it('renders venue shows list', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('venue-shows-list')).toBeInTheDocument()
      expect(screen.getByText('Shows for venue 1')).toBeInTheDocument()
    })

    it('renders location card in sidebar', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('location-card')).toBeInTheDocument()
    })

    // PSY-1034 regression guard: the two-column grid track must stay
    // min-width-capped so the ResizeObserver-measured VenueBillNetwork graph
    // can't balloon the layout rightward. A plain `1fr` track has an implicit
    // `min-width: auto` and reintroduces the bug; `minmax(0,1fr)` + `min-w-0`
    // on the main column are the two independent caps. Assert both are present.
    it('keeps the two-column grid track min-width-capped (no RO balloon)', () => {
      const { container } = render(<VenueDetail venueId="1" />)
      const grid = container.querySelector(
        '.lg\\:grid-cols-\\[minmax\\(0\\,1fr\\)_400px\\]',
      )
      expect(grid).not.toBeNull()
      // The main (graph-bearing) column carries the belt-and-suspenders cap.
      const mainColumn = grid?.querySelector('.min-w-0')
      expect(mainColumn).not.toBeNull()
    })

    it('does not render a favorite venue button', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.queryByTestId('favorite-button')).not.toBeInTheDocument()
    })

    it('renders follow button', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('follow-button')).toBeInTheDocument()
    })

    it('renders notify me button', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('notify-me-button')).toBeInTheDocument()
      expect(screen.getByText('Notify The Rebel Lounge')).toBeInTheDocument()
    })

    it('renders revision history', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('revision-history')).toBeInTheDocument()
    })

    it('shows verified badge when venue is verified', () => {
      mockUseVenue.mockReturnValue({
        data: makeVenue({ verified: true }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByLabelText('Verified venue')).toBeInTheDocument()
    })

    it('does not show verified badge when not verified', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.queryByLabelText('Verified venue')).not.toBeInTheDocument()
    })

    it('renders website link when social website is provided', () => {
      mockUseVenue.mockReturnValue({
        data: makeVenue({
          social: {
            website: 'https://www.therebelphx.com/events',
          },
        }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      const websiteLink = screen.getByText('therebelphx.com')
      expect(websiteLink.closest('a')).toHaveAttribute('href', 'https://www.therebelphx.com/events')
      expect(websiteLink.closest('a')).toHaveAttribute('target', '_blank')
    })

    it('normalizes URL without protocol', () => {
      mockUseVenue.mockReturnValue({
        data: makeVenue({
          social: {
            website: 'www.therebelphx.com',
          },
        }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      const websiteLink = screen.getByText('therebelphx.com')
      expect(websiteLink.closest('a')).toHaveAttribute('href', 'https://www.therebelphx.com')
    })

    it('renders social links when social data exists', () => {
      mockUseVenue.mockReturnValue({
        data: makeVenue({ social: { instagram: '@rebel' } }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByTestId('social-links')).toBeInTheDocument()
    })
  })

  describe('genre profile', () => {
    beforeEach(() => {
      mockUseVenue.mockReturnValue({
        data: makeVenue(),
        isLoading: false,
        error: null,
      })
    })

    it('renders genre tags when genres are available', () => {
      mockUseVenueGenres.mockReturnValue({
        data: {
          genres: [
            { tag_id: 1, name: 'Indie Rock', slug: 'indie-rock', count: 10 },
            { tag_id: 2, name: 'Punk', slug: 'punk', count: 5 },
          ],
        },
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByText('Genre Profile')).toBeInTheDocument()
      expect(screen.getByText('Indie Rock')).toBeInTheDocument()
      expect(screen.getByText('Punk')).toBeInTheDocument()
    })

    it('does not render genre profile when no genres', () => {
      mockUseVenueGenres.mockReturnValue({
        data: { genres: [] },
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.queryByText('Genre Profile')).not.toBeInTheDocument()
    })

    it('does not render genre profile when data is null', () => {
      mockUseVenueGenres.mockReturnValue({ data: null })
      render(<VenueDetail venueId="1" />)
      expect(screen.queryByText('Genre Profile')).not.toBeInTheDocument()
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
      mockUseVenue.mockReturnValue({
        data: makeVenue(),
        isLoading: false,
        error: null,
      })
    })

    it('shows edit button for admin', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument()
    })

    it('shows delete button for admin', () => {
      render(<VenueDetail venueId="1" />)
      expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument()
    })

    it('opens edit drawer on click', async () => {
      const user = userEvent.setup()
      render(<VenueDetail venueId="1" />)

      expect(screen.queryByTestId('edit-drawer')).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /Edit/ }))
      expect(screen.getByTestId('edit-drawer')).toBeInTheDocument()
    })

    it('opens delete dialog on click', async () => {
      const user = userEvent.setup()
      render(<VenueDetail venueId="1" />)

      expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /Delete/ }))
      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
    })
  })

  describe('venue owner controls', () => {
    it('shows edit for venue owner, no delete (non-admin)', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '42', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenue.mockReturnValue({
        data: makeVenue({ submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument()
    })

    it('shows edit for non-admin non-owner, no delete', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenue.mockReturnValue({
        data: makeVenue({ submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)
      // All authenticated users can suggest edits
      expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument()
      // Only admins see delete
      expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument()
    })
  })

  // PSY-668: inline bio editing opens to trusted_contributor + local_ambassador
  // + the venue's original submitter via the canEditDirectly gate. Admins write
  // through the admin PATCH (useVenueUpdate); everyone else routes through
  // suggest-edit (useSuggestEdit), which the backend auto-applies for trusted
  // tiers and queues for review otherwise.
  describe('inline description editing (PSY-668)', () => {
    beforeEach(() => {
      mockUseVenue.mockReturnValue({
        data: makeVenue({ description: 'Old bio' }),
        isLoading: false,
        error: null,
      })
    })

    it('routes admin saves through useVenueUpdate, not suggest-edit', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '1', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueDetail venueId="1" />)

      await user.click(screen.getByTestId('entity-description-save'))

      expect(mockVenueUpdateMutate).toHaveBeenCalledTimes(1)
      expect(mockVenueUpdateMutate).toHaveBeenCalledWith(
        { venueId: 1, data: { description: 'New venue bio' } },
        expect.anything()
      )
      expect(mockSuggestEditMutate).not.toHaveBeenCalled()
    })

    it('routes trusted-tier (non-admin) saves through useSuggestEdit, not useVenueUpdate', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '7', is_admin: false, user_tier: 'trusted_contributor' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueDetail venueId="1" />)

      await user.click(screen.getByTestId('entity-description-save'))

      expect(mockSuggestEditMutate).toHaveBeenCalledTimes(1)
      expect(mockSuggestEditMutate).toHaveBeenCalledWith(
        {
          entityType: 'venue',
          entityId: 1,
          changes: [
            { field: 'description', old_value: 'Old bio', new_value: 'New venue bio' },
          ],
          summary: 'Updated description via inline editor',
        },
        expect.anything()
      )
      expect(mockVenueUpdateMutate).not.toHaveBeenCalled()
    })

    it('routes original-submitter (non-admin, non-trusted) saves through useSuggestEdit', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '42', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenue.mockReturnValue({
        data: makeVenue({ description: 'Old bio', submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)

      await user.click(screen.getByTestId('entity-description-save'))

      expect(mockSuggestEditMutate).toHaveBeenCalledTimes(1)
      expect(mockVenueUpdateMutate).not.toHaveBeenCalled()
    })

    it('does NOT show the inline editor for a plain authenticated, non-trusted, non-submitter user', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseVenue.mockReturnValue({
        data: makeVenue({ description: 'Old bio', submitted_by: 42 }),
        isLoading: false,
        error: null,
      })
      render(<VenueDetail venueId="1" />)

      expect(screen.queryByTestId('entity-description-save')).not.toBeInTheDocument()
    })

    it('does NOT show the inline editor for anonymous visitors', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<VenueDetail venueId="1" />)

      expect(screen.queryByTestId('entity-description-save')).not.toBeInTheDocument()
    })
  })
})

// Replaces e2e: pages/venue-detail.spec.ts "shows tabs switch between upcoming and past"
// (moved to a component test per PSY-472, audit doc docs/research/e2e-layer-5-audit.md item #2).
