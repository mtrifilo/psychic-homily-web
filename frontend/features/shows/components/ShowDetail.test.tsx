import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders as render } from '@/test/utils'
import type { ShowResponse, ArtistResponse } from '../types'

// Mock AuthContext.
// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
type MockAuthContextValue = {
  user: { id: string; is_admin: boolean } | null
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

// Mock useShow hook
const mockUseShow = vi.fn()
// The corridor modules' archive read. Defaulted to "nothing yet" so every test
// that is not about the timeline renders the header exactly as it did before
// the modules existed; the timeline tests override it.
const mockUseShowTimeline = vi.fn(() => ({ data: undefined }))
vi.mock('../hooks/useShows', () => ({
  useShow: (...args: unknown[]) => mockUseShow(...args),
  // Arguments deliberately dropped: nothing here asserts on them, and the
  // stub's return is what the header renders from.
  useShowTimeline: () => mockUseShowTimeline(),
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

// Mock child components. We mock EntityDetailLayout to expose slots so tests
// can reason about the header / content split without pulling in the real
// Tabs machinery. ShowDetail renders in flat-layout mode (no `tabs` prop).
vi.mock('@/components/shared', () => ({
  SaveButton: () => <button data-testid="save-button">Save</button>,
  MusicEmbed: () => <div data-testid="music-embed" />,
  // A value, not a component, and the key has to be here for a mechanical
  // reason: the listen module reads it for its `maxWidth`, and a barrel mock
  // missing the export hands it `undefined`. This is a stand-in, not a mirror
  // of the real constant, and nothing here asserts a width.
  BANDCAMP_EMBED_MAX_WIDTH_PX: 700,
  AddToCollectionButton: () => <button data-testid="add-to-collection">Collect</button>,
  // aria-label mirrors the real component (ariaLabel ?? label) so a
  // name collision between two Edit affordances cannot hide behind the mock.
  BracketLink: ({
    label,
    onClick,
    href,
    ariaLabel,
  }: {
    label: string
    onClick?: () => void
    href?: string
    ariaLabel?: string
  }) =>
    href ? (
      <a href={href} aria-label={ariaLabel ?? label}>[{label}]</a>
    ) : (
      <button onClick={onClick} aria-label={ariaLabel ?? label}>[{label}]</button>
    ),
  UserAttribution: ({ name }: { name: string }) => <span>{name}</span>,
  SectionHeader: ({ title }: { title: string }) => <h2>{title}</h2>,
  // The listen card renders one per act. Its own coverage is in
  // SocialLinks.test and ShowListenModule.test; here it only has to exist, or
  // the barrel mock hands the card `undefined` and the page fails to render.
  SocialLinks: () => <div data-testid="social-links" />,
  // Surfaces `path` so the page-level test can assert WHICH url this show
  // hands out — the primitive's own behaviour is covered in ShareButton.test.
  ShareButton: ({ path }: { path: string }) => (
    <button data-testid="share-button" data-path={path}>
      Share
    </button>
  ),
  RevisionHistory: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="revision-history">History for {entityType} {entityId}</div>
  ),
  EntityDetailContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="entity-detail-container">{children}</div>
  ),
  EntityDetailLayout: ({
    header,
    children,
    sidebar,
    fallback,
    entityName,
  }: {
    header: React.ReactNode
    children: React.ReactNode
    sidebar?: React.ReactNode
    fallback: { href: string; label: string }
    entityName: string
  }) => (
    <div data-testid="entity-layout">
      <nav aria-label="Breadcrumb">
        <a href={fallback.href}>{fallback.label}</a>
        <span>{entityName}</span>
      </nav>
      <div data-testid="header-slot">{header}</div>
      {sidebar ? <div data-testid="sidebar-slot">{sidebar}</div> : null}
      <div data-testid="content-slot">{children}</div>
    </div>
  ),
}))

// Mock contributions feature so we can drive the success banner from tests
// without exercising the real auto-dismiss timer. PSY-563 routes show edits
// through {@link EntityEditDrawer} (replacing the old inline ShowForm).
const mockSaveBannerHandleSaveSuccess = vi.fn()
let mockSaveBannerVisible = false
vi.mock('@/features/contributions', () => ({
  EntitySaveSuccessBanner: ({ visible }: { visible: boolean }) =>
    visible ? <div data-testid="save-success-banner">Changes saved</div> : null,
  useEntitySaveSuccessBanner: () => ({
    isVisible: mockSaveBannerVisible,
    handleSaveSuccess: mockSaveBannerHandleSaveSuccess,
  }),
  // ShowProvenanceLine reads the last-editor + revision count through this
  // hook; null = "no revisions yet", which is the default these page-level
  // tests want (the line's own permutations live in its own test file).
  useEntityAttribution: () => ({ data: null }),
  EntityEditDrawer: ({
    open,
    entityType,
    onSuccess,
  }: {
    open: boolean
    entityType: string
    onSuccess?: (result: { applied: boolean }) => void
  }) =>
    open ? (
      <div data-testid="entity-edit-drawer">
        Drawer for {entityType}
        <button data-testid="drawer-save" onClick={() => onSuccess?.({ applied: true })}>
          Save Drawer
        </button>
      </div>
    ) : null,
}))

vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null,
}))

vi.mock('./ReportShowButton', () => ({
  ReportShowButton: () => <button data-testid="report-button">Report</button>,
}))

// The rails row runs two queries of its own (PSY-1689). Mocked at the component
// boundary like the other children, so this file stays about the page's own
// composition; which rows each rail draws, and when it hides, lives in
// ShowDiscoveryRails.test.tsx.
vi.mock('./ShowDiscoveryRails', () => ({
  ShowDiscoveryRails: () => <div data-testid="show-discovery-rails" />,
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

// Venue-module affordances whose hooks would otherwise fetch; behaviour is
// covered in their own test files (same convention as VenueDetail.test).
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ bracketLabel }: { bracketLabel?: string }) => (
    <button data-testid="follow-venue">[{bracketLabel ?? 'Follow'}]</button>
  ),
}))

// The `@/features/notifications` mock that stood here existed only for
// ShowVenueModule's [Notify me] bracket, which PSY-1905 retired. Left in place
// it would shadow the real barrel the moment this tree imports anything else
// from it, turning a working component into a silently broken test.

vi.mock('@/features/charts', () => ({
  EntityChartRankBadge: () => null,
  useChartEntityRank: () => ({ data: undefined, isSuccess: false }),
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: () => <div data-testid="entity-tag-list" />,
}))

vi.mock('@/features/comments', () => ({
  CommentThread: ({ entityType, entityId }: { entityType: string; entityId: number }) => (
    <div data-testid="comment-thread">Comments for {entityType} {entityId}</div>
  ),
  FieldNotesSection: ({ showId }: { showId: number }) => (
    <div data-testid="field-notes-section">Field Notes for {showId}</div>
  ),
}))

import { ShowDetail } from './ShowDetail'

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist-one',
    name: 'Artist One',
    city: 'Phoenix',
    state: 'AZ',
    set_type: 'headliner',
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
    price: 25,
    age_requirement: '21+',
    description: 'A great show description.',
    venues: [
      { id: 1, slug: 'the-venue', name: 'The Venue', city: 'Phoenix', state: 'AZ', verified: true },
    ],
    artists: [
      makeArtist({ id: 1, name: 'Headliner', slug: 'headliner' }),
      makeArtist({ id: 2, name: 'Opener', slug: 'opener', set_type: 'opener' }),
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
    mockSaveBannerVisible = false
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
      const { container } = render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByText('Show Not Found')).toBeInTheDocument()
      expect(screen.getByText(/doesn't exist or has been removed/)).toBeInTheDocument()
    })

    it('shows back to shows link on error', () => {
      mockUseShow.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Error'),
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByText('Headliner')).toBeInTheDocument()
      expect(screen.getByText('Opener')).toBeInTheDocument()
    })

    it('offers a share affordance built from the show slug, not the route param', () => {
      // `showId` here is the numeric id the route happened to be hit with;
      // the shared link must still be the durable slug URL.
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('share-button')).toHaveAttribute(
        'data-path',
        '/shows/test-show'
      )
    })

    it('links artists with slugs to artist pages', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      const link = screen.getByText('Headliner').closest('a')
      expect(link).toHaveAttribute('href', '/artists/headliner')
    })

    it('renders venue name as link', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      const link = screen.getByText('The Venue').closest('a')
      expect(link).toHaveAttribute('href', '/venues/the-venue')
    })

    // Scoped to the venue block on purpose: the bill above it prints each
    // act's hometown, and the fixture artists are from Phoenix too, so a bare
    // text query would match either module.
    it('renders venue location', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('venue-location')).toHaveTextContent(
        'Phoenix, AZ'
      )
    })

    // The street address is what people paste into a maps app, and for DIY
    // venues it is often not findable anywhere else. It is fetched on every
    // show request, so hiding it was pure data loss.
    it('renders the venue street address when the venue has one', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          venues: [
            {
              id: 1,
              slug: 'the-venue',
              name: 'The Venue',
              address: '308 N 2nd Ave',
              city: 'Phoenix',
              state: 'AZ',
              verified: true,
            },
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('venue-address')).toHaveTextContent(
        '308 N 2nd Ave'
      )
    })

    it('omits the address line when the venue has no address', () => {
      // The default fixture venue carries no `address`.
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('venue-location')).toHaveTextContent(
        'Phoenix, AZ'
      )
      expect(screen.queryByTestId('venue-address')).not.toBeInTheDocument()
    })

    // The API can hand back a whitespace-only address; rendering it would leave
    // a blank indented line under the city/state row.
    it('omits the address line when the address is whitespace only', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          venues: [
            {
              id: 1,
              slug: 'the-venue',
              name: 'The Venue',
              address: '   ',
              city: 'Phoenix',
              state: 'AZ',
              verified: true,
            },
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.queryByTestId('venue-address')).not.toBeInTheDocument()
    })

    it('renders price', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByText('$25')).toBeInTheDocument()
    })

    it('renders age requirement', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByText('21+')).toBeInTheDocument()
    })

    it('renders description', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByText('A great show description.')).toBeInTheDocument()
    })

    it('does not render description when missing', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ description: null }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.queryByText('A great show description.')).not.toBeInTheDocument()
    })

    it('renders breadcrumb with link to shows', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      const breadcrumbNav = screen.getByRole('navigation', { name: /Breadcrumb/ })
      expect(breadcrumbNav).toBeInTheDocument()
      const link = breadcrumbNav.querySelector('a')
      expect(link).toHaveAttribute('href', '/shows')
    })

    it('renders save button', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('save-button')).toBeInTheDocument()
    })

    it('renders report button', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('report-button')).toBeInTheDocument()
    })

    // Tags and attribution moved out of the header and into the provenance
    // footer with the mock's module order: above the fold belongs to the bill.
    // Asserted against the footer itself, not merely "somewhere in the content
    // slot", so moving them back above the embeds fails here.
    it('renders EntityTagList in the provenance footer, not the header', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('show-provenance-footer')).toContainElement(
        screen.getByTestId('entity-tag-list')
      )
      expect(screen.getByTestId('header-slot')).not.toContainElement(
        screen.getByTestId('entity-tag-list')
      )
    })

    // The slot the mock reserves for the rails row is BETWEEN the page's own
    // modules and the byline. Position is the claim, so containment alone is
    // not enough: a rails row rendered after the provenance footer would still
    // "be on the page" and would still be wrong. Both halves of the claim are
    // pinned — a row that drifted above the embeds would otherwise ship green.
    it('renders the rails row below the embeds and above the provenance footer', () => {
      // A show WITH music, so the embeds half of the claim has something to
      // anchor on — the default fixture's artists have none.
      mockUseShow.mockReturnValue({
        data: makeShow({
          artists: [
            makeArtist({
              id: 1,
              name: 'Band',
              // A URL `parseSpotifyEmbed` actually accepts. A bare
              // `spotify.com/band` fails its host-anchored parse, so the bill
              // yields no listen card and the embed this test anchors on never
              // renders.
              socials: {
                spotify: 'https://open.spotify.com/artist/1a2b3c4d5e6f7g8h9i0jkl',
              },
            }),
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      const rails = screen.getByTestId('show-discovery-rails')
      const footer = screen.getByTestId('show-provenance-footer')
      const embeds = screen.getByTestId('music-embed')

      expect(footer).not.toContainElement(rails)
      expect(
        embeds.compareDocumentPosition(rails) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
      expect(
        rails.compareDocumentPosition(footer) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
    })

    // Position is the whole design claim: one band, at the very top, in every
    // state, so nothing below it moves. Containment alone would still pass with
    // the band buried under the comment thread.
    it('renders the status stripe above the layout with venue-local copy', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      const stripe = screen.getByTestId('show-status-stripe')
      // 2026-04-15T20:00:00Z is 1 PM Wed Apr 15 in Phoenix.
      expect(stripe).toHaveTextContent(/WED.*APR 15/)

      const layout = screen.getByTestId('entity-layout')
      expect(layout).not.toContainElement(stripe)
      expect(
        stripe.compareDocumentPosition(layout) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
    })

    it('renders EntityCollections, FieldNotes, and CommentThread as content siblings', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('entity-collections')).toBeInTheDocument()
      expect(screen.getByTestId('field-notes-section')).toBeInTheDocument()
      expect(screen.getByTestId('comment-thread')).toBeInTheDocument()
    })
  })

  describe('cancelled show', () => {
    // The destructive alert that used to sit above the layout is gone: the
    // status stripe carries cancellation now, in the one place the page says
    // what state it is in.
    it('announces the cancellation in the status stripe', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_cancelled: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('show-status-stripe')).toHaveTextContent(
        /CANCELLED.*WED.*APR 15/
      )
    })

    // Cancellation outranks the clock: an admin can cancel a show hours before
    // doors, and the stripe must not still be counting it in.
    it('says cancelled rather than tonight for a cancelled show today', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_cancelled: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="today" />)
      const stripe = screen.getByTestId('show-status-stripe')
      expect(stripe).toHaveTextContent(/CANCELLED/)
      expect(stripe).not.toHaveTextContent(/TONIGHT/)
    })
  })

  describe('sold out show', () => {
    it('shows sold out in the badge and the ticket line', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_sold_out: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      // Twice by design (PSY-1686): the header badge and the ticket line's
      // swapped sale state both carry it.
      expect(screen.getAllByText('SOLD OUT')).toHaveLength(2)
    })
  })

  describe('announced door times', () => {
    // Twice by design, like SOLD OUT above: the locked mock renders doors in
    // BOTH the status stripe (as status) and the venue facts line (as a fact
    // sheet), through one shared formatter so the registers cannot split.
    it('prints doors in the stripe and the venue facts line', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ doors_at: '2026-04-16T02:00:00Z' }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('show-status-stripe')).toHaveTextContent(
        /DOORS 7PM/
      )
      expect(screen.getByTestId('venue-facts')).toHaveTextContent(/DOORS 7PM/)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
    })

    it('shows delete button for admin', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument()
    })

    // PSY-563: clicking Edit opens the right-side EntityEditDrawer instead
    // of expanding the inline form. The toggle button still lives in
    // ShowActions.
    it('toggles the edit drawer on click', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

      expect(screen.queryByTestId('entity-edit-drawer')).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Edit' }))
      expect(screen.getByTestId('entity-edit-drawer')).toBeInTheDocument()
    })

    // PSY-563: after a successful save, the drawer's onSuccess fires the
    // banner-flash hook with `{applied: true}` (shows always direct-save).
    // Mirrors the artist/venue/release/label/festival detail pages.
    it('flashes the save banner after a successful drawer save', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

      await user.click(screen.getByRole('button', { name: 'Edit' }))
      expect(screen.getByTestId('entity-edit-drawer')).toBeInTheDocument()

      await user.click(screen.getByTestId('drawer-save'))

      expect(mockSaveBannerHandleSaveSuccess).toHaveBeenCalledWith({ applied: true })
    })

    // PSY-563: revision history accordion mounts at the bottom of the
    // detail page (mirrors the other 5 detail pages).
    it('renders RevisionHistory for shows', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('revision-history')).toBeInTheDocument()
    })

    // PSY-563 put AttributionLine in the header slot; PSY-1686 replaces it on
    // the show page with the mock's provenance byline, in the footer with the
    // tags. The other five detail pages are unchanged.
    it('renders the provenance line in the provenance footer', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('show-provenance-footer')).toContainElement(
        screen.getByTestId('show-provenance-line')
      )
    })

    it('renders the save success banner when the hook reports it visible', () => {
      mockSaveBannerVisible = true
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('save-success-banner')).toBeInTheDocument()
    })

    it('does not render the save success banner when hidden', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.queryByTestId('save-success-banner')).not.toBeInTheDocument()
    })

    it('opens delete dialog on click', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

      expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /Delete/ }))
      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
    })

    it('shows Mark Sold Out button', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Mark Sold Out' })).toBeInTheDocument()
    })

    it('shows Mark Cancelled button', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Mark Cancelled' })).toBeInTheDocument()
    })

    it('shows Unmark Sold Out when already sold out', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_sold_out: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Unmark Sold Out' })).toBeInTheDocument()
    })

    it('shows Unmark Cancelled when already cancelled', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({ is_cancelled: true }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Unmark Cancelled' })).toBeInTheDocument()
    })

    it('calls sold out mutation on toggle', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

      await user.click(screen.getByRole('button', { name: 'Mark Sold Out' }))
      expect(mockSetSoldOut).toHaveBeenCalledWith({ showId: 1, value: true })
    })

    it('calls cancelled mutation on toggle', async () => {
      const user = userEvent.setup()
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.queryByRole('button', { name: /Edit/ })).not.toBeInTheDocument()
    })

    it('does not show delete button for non-admin non-owner', () => {
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByRole('button', { name: 'Mark Sold Out' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Mark Cancelled' })).toBeInTheDocument()
    })

    // A non-admin OWNER's edit path is the provenance line's [Edit] (user
    // decision, Wave 1C): ShowActions' Edit button stays admin-only
    // moderation chrome, and the bracket opens the same direct-save drawer.
    it('gives a non-admin owner the provenance Edit bracket, not the admin button, and it opens the drawer', async () => {
      const user = userEvent.setup()
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
      render(<ShowDetail showId="1" lifecycle="upcoming" />)

      expect(
        screen.queryByRole('button', { name: 'Edit' })
      ).not.toBeInTheDocument()
      await user.click(
        screen.getByRole('button', { name: 'Edit this show listing' })
      )
      expect(screen.getByTestId('entity-edit-drawer')).toBeInTheDocument()
    })
  })

  // The module's own card rendering is covered in ShowListenModule.test.tsx;
  // these two only pin that the page mounts it in the right slot and lets it
  // decide whether the section exists at all.
  describe('listen module', () => {
    it('renders the listen module when a bill artist has something to play', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          artists: [
            makeArtist({
              id: 1,
              name: 'Band',
              socials: { spotify: 'https://open.spotify.com/artist/1a2b3c4d5e6f7g8h9i0jkl' },
            }),
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.getByTestId('show-listen-module')).toBeInTheDocument()
      expect(screen.getByText('Listen / Before you go')).toBeInTheDocument()
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    })

    it('renders no listen module when no bill artist has a playable source', () => {
      mockUseShow.mockReturnValue({
        data: makeShow({
          artists: [
            makeArtist({ id: 1, name: 'Band', socials: {} }),
          ],
        }),
        isLoading: false,
        error: null,
      })
      render(<ShowDetail showId="1" lifecycle="upcoming" />)
      expect(screen.queryByTestId('show-listen-module')).not.toBeInTheDocument()
      expect(screen.queryByText('Listen / Before you go')).not.toBeInTheDocument()
    })
  })
})
