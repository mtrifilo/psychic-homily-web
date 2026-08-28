import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { MutableRefObject, ReactNode } from 'react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListResponse } from '../types'
import type { PlaceableScene } from './globeTypes'

// AtlasGlobe statically imports only `globeTypes` (no react-globe.gl) and
// dynamic-imports GlobeCanvas (ssr:false). In jsdom we exercise the testable
// surface: data wiring, the loading/error states, and the <640px mobile gate
// (the WebGL globe itself is validated by screenshot, not jsdom).

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseScenes = vi.fn()
// FollowButton pulls AuthContext + usePathname (neither available here) —
// mock at the module boundary, same idiom as VenueDetail/LabelDetail tests.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number | string }) => (
    <button data-testid="follow-button">
      Follow {entityType} {String(entityId)}
    </button>
  ),
}))

// SceneNotifyModeToggle also pulls AuthContext and has focused coverage in its
// own suite; keep this globe composition test isolated from that auth concern.
vi.mock('./SceneNotifyModeToggle', () => ({
  SceneNotifyModeToggle: () => null,
}))

// useMyFollowing pulls AuthContext (unavailable here) — stub the follows hook
// (PSY-1340); tests override via mockUseMyFollowing.
const mockUseMyFollowing = vi.fn(() => ({ data: undefined }))
vi.mock('@/lib/hooks/common/useFollow', () => ({
  useMyFollowing: () => mockUseMyFollowing(),
  // SceneNotifyModeToggle (rendered inside the preview panel) reads this.
  useFollowStatus: () => ({ data: undefined }),
}))

vi.mock('../hooks', () => ({
  useScenes: () => mockUseScenes(),
  useSceneArtists: () => ({ data: undefined, isLoading: false }),
  // The preview panel (opened by the drift tests) reads the scene's this-week
  // shows (PSY-1309); a quiet week is the neutral default here.
  useSceneShows: () => ({ data: { shows: [] }, isLoading: false }),
  // City view (PSY-1539) reads the scene's roster size for the rail header.
  useSceneDetail: () => ({ data: undefined }),
}))

// City view's venue list (PSY-1539). Tests that need venues override via
// mockUseVenues; the default is an un-entered city view (no venues).
// Options are forwarded (not swallowed) so the scoping AtlasGlobe asks for —
// the city, and the PSY-1574 metro rollup — is assertable.
const mockUseVenues = vi.fn<
  (options?: Record<string, unknown>) => Record<string, unknown>
>(() => ({
  data: undefined,
  isFetching: false,
  isPlaceholderData: false,
}))
// The venue panel's shows request (PSY-1540). A quiet calendar is the neutral
// default; the panel's own suite covers the list rendering in detail.
const mockUseVenueShows = vi.fn<() => Record<string, unknown>>(() => ({
  data: { shows: [], venue_id: 0, total: 0 },
  isLoading: false,
  isError: false,
}))
vi.mock('@/features/venues/hooks', () => ({
  useVenues: (options?: Record<string, unknown>) => mockUseVenues(options),
  useVenueShows: () => mockUseVenueShows(),
  // VenuePanel's confirm mutation (PSY-1542). Inert here — the panel's own
  // suite covers the confirm behaviour; this file's concern is the stack.
  useVenueConfirm: () => ({
    mutate: vi.fn(),
    isPending: false,
    data: undefined,
    error: null,
  }),
  formatVenueConfirmError: () => null,
}))

// The artist drill-in's own fetches (PSY-1541). AtlasGlobe statically imports
// ArtistPanel, so these must be stubbed for every test in the file, not just
// the drill-in ones. The panel's own suite covers what it does with the data;
// here the concern is the stack — which panel is on screen, and what the
// stepper is stepping through.
const mockUseArtistGraphCard = vi.fn<
  (args: { artistId: number | string | null }) => Record<string, unknown>
>(() => ({ data: undefined, isError: false }))
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (args: { artistId: number | string | null }) =>
    mockUseArtistGraphCard(args),
}))
vi.mock('@/features/artists/hooks/useArtists', () => ({
  useArtistShows: () => ({ data: { shows: [], artist_id: 0, total: 0 } }),
}))
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: () => <div data-testid="music-embed" />,
}))

// VenuePanel's confirm control is auth-gated (PSY-1542) and AuthContext has no
// provider in this file. Signed-in is the interesting default: it keeps the
// control live so the panel renders the same shape the real app does.
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true }),
}))

// AtlasSearch (rendered in the globe branch) reads the router (PSY-1310).
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  // VenuePanel's confirm control reads the pathname to build its auth
  // return-to (PSY-1542).
  usePathname: () => '/atlas',
}))

// Stub the WebGL canvas for the desktop-branch tests (PSY-1308 Drift): it
// fills the flyToRef seam with a spy so the drift handler's camera call is
// observable without three.js.
const flyToSpy = vi.fn()
// City view (PSY-1539): the stub also captures the canvas's camera-settle
// callback and the pin array, so a test can drive the camera the way the real
// map does and assert what the map would have drawn — the whole
// camera → city → fetch → filter → pins chain, without WebGL.
let lastCanvasProps: {
  onCameraSettle?: (c: { lng: number; lat: number; zoom: number }) => void
  onVenueSelect?: (venueId: number) => void
  venues?: readonly { id: number; name: string }[]
  cityLabel?: string | null
  width?: number
} = {}
vi.mock('./GlobeCanvas', () => ({
  default: (props: {
    flyToRef?: MutableRefObject<((scene: PlaceableScene) => void) | null>
    onCameraSettle?: (c: { lng: number; lat: number; zoom: number }) => void
    onVenueSelect?: (venueId: number) => void
    venues?: readonly { id: number; name: string }[]
    cityLabel?: string | null
    width?: number
  }) => {
    if (props.flyToRef) props.flyToRef.current = flyToSpy
    lastCanvasProps = props
    return <div data-testid="globe-canvas" />
  },
}))

import { AtlasGlobe } from './AtlasGlobe'

// ResizeObserver shim to drive the container width (same pattern as
// SceneGraph.test.tsx). Default to a narrow width → the mobile gate.
let mockContainerWidth = 500
function setMockContainerWidth(width: number) {
  mockContainerWidth = width
}

class ImmediateResizeObserver {
  private callback: ResizeObserverCallback
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element): void {
    this.callback(
      [
        {
          target,
          contentRect: {
            width: mockContainerWidth,
            height: 800,
          } as DOMRectReadOnly,
        } as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    )
  }
  unobserve(): void {}
  disconnect(): void {}
}

const sampleData: SceneListResponse = {
  scenes: [
    {
      city: 'Chicago',
      state: 'IL',
      slug: 'chicago-il',
      venue_count: 9,
      upcoming_show_count: 283,
      total_show_count: 337,
      shows_this_week: 0,
      shows_calendar_week: 0,
      latitude: 41.88,
      longitude: -87.63,
    },
    {
      // Unplaceable (no coords) — still listed on mobile, never plotted.
      city: 'Faketown',
      state: 'ZZ',
      slug: 'faketown-zz',
      venue_count: 2,
      upcoming_show_count: 3,
      total_show_count: 3,
      shows_this_week: 0,
      shows_calendar_week: 0,
    },
  ],
  count: 2,
}

describe('AtlasGlobe', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const originalResizeObserver = (window as any).ResizeObserver

  beforeEach(() => {
    setMockContainerWidth(500)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
    mockUseScenes.mockReset()
    // The geo-centering fetch is non-fatal; stub it to a no-op miss.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
  })

  afterEach(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = originalResizeObserver
    vi.unstubAllGlobals()
  })

  it('lists all scenes as expandable rows on small screens (mobile gate)', () => {
    mockUseScenes.mockReturnValue({
      data: sampleData,
      isLoading: false,
      isError: false,
    })
    renderWithProviders(<AtlasGlobe />)

    // Rows are collapsed accordion buttons (PSY-1311) — expansion behavior is
    // covered by MobileSceneList.test.tsx.
    expect(
      screen.getByRole('button', { name: /Chicago, IL/ }),
    ).toHaveAttribute('aria-expanded', 'false')
    // The unplaceable scene still appears in the mobile list.
    expect(
      screen.getByRole('button', { name: /Faketown, ZZ/ }),
    ).toBeInTheDocument()
  })

  it('shows an error state when the scenes query fails', () => {
    mockUseScenes.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    renderWithProviders(<AtlasGlobe />)
    expect(screen.getByText(/couldn’t load/i)).toBeInTheDocument()
  })

  it('shows a loading state while scenes load', () => {
    mockUseScenes.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })
    renderWithProviders(<AtlasGlobe />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  describe('Drift (desktop globe branch, PSY-1308)', () => {
    beforeEach(() => {
      setMockContainerWidth(800) // above the 640px mobile gate
      flyToSpy.mockReset()
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        isError: false,
      })
    })

    it('flies to a picked scene and opens its preview', async () => {
      renderWithProviders(<AtlasGlobe />)

      // The globe branch mounts after the pov resolves (the stubbed geo fetch
      // settles as a miss → default focus). Await the CANVAS stub, not just
      // the button: the button renders immediately while next/dynamic is
      // still resolving, and the flyTo seam is only filled once the canvas
      // renders (a pre-resolution click is a null-safe no-op by design).
      await screen.findByTestId('globe-canvas')
      const drift = screen.getByRole('button', {
        name: /drift to a random scene/i,
      })
      fireEvent.click(drift)

      // Chicago is the only placeable scene, so the weighted pick is
      // deterministic here: fly to it + open its preview panel.
      expect(flyToSpy).toHaveBeenCalledTimes(1)
      expect(flyToSpy.mock.calls[0][0]).toMatchObject({ slug: 'chicago-il' })
      expect(
        screen.getByRole('complementary', { name: /Chicago, IL scene/ }),
      ).toBeInTheDocument()
    })

    it('no-ops rather than re-flying when the only scene is already open', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      const drift = screen.getByRole('button', {
        name: /drift to a random scene/i,
      })

      fireEvent.click(drift)
      fireEvent.click(drift) // exclusion leaves zero candidates → no flight

      expect(flyToSpy).toHaveBeenCalledTimes(1)
      expect(
        screen.getByRole('complementary', { name: /Chicago, IL scene/ }),
      ).toBeInTheDocument()
    })
  })

  // ── City view (PSY-1539) ────────────────────────────────────────────────
  // The glue between camera settle, city resolution, the venue fetch and the
  // pin array. jsdom can't render the map, but the canvas stub captures the
  // props the map WOULD have drawn, so the chain is fully assertable.
  describe('city view', () => {
    const chicagoVenues = [
      {
        id: 1,
        name: 'Empty Bottle',
        city: 'Chicago',
        state: 'IL',
        verified: true,
        latitude: 41.88,
        longitude: -87.63,
        upcoming_show_count: 14,
        shows_this_week: 3,
        shows_calendar_week: 3,
        // Only this room carries the all-ages tag, so the chip has something
        // to keep AND something to drop (PSY-1573).
        hosts_all_ages: true,
        updated_at: '2026-07-25T00:00:00Z',
      },
      {
        id: 2,
        name: 'Hideout',
        city: 'Chicago',
        state: 'IL',
        verified: true,
        latitude: 41.88,
        longitude: -87.63,
        upcoming_show_count: 4,
        shows_this_week: 0,
        shows_calendar_week: 0,
        hosts_all_ages: false,
        updated_at: '2026-07-24T00:00:00Z',
      },
    ]

    /** Drive the camera the way the real canvas does: settle events only. */
    function settleCamera(lng: number, lat: number, zoom: number) {
      act(() => {
        lastCanvasProps.onCameraSettle?.({ lng, lat, zoom })
      })
    }

    beforeEach(() => {
      setMockContainerWidth(1400) // wide enough for the rail
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        isError: false,
      })
      mockUseVenues.mockReturnValue({
        data: { venues: chicagoVenues, total: 2 },
        isFetching: false,
        isPlaceholderData: false,
      })
      mockUseArtistGraphCard.mockReset()
      mockUseArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
    })

    // ── Artist drill-in (PSY-1541) ──────────────────────────────────────
    // The venue's week, as the shows endpoint serves it: two shows, four
    // distinct bands, one of them (Meat Wave) playing both nights.
    const venueWeek = [
      {
        id: 101,
        slug: 'show-101',
        title: 'Bottle Fest night one',
        event_date: '2026-07-28T01:00:00Z',
        city: 'Chicago',
        state: 'IL',
        price: null,
        age_requirement: null,
        artists: [
          { id: 10, slug: 'die-spitz', name: 'Die Spitz' },
          { id: 11, slug: 'meat-wave', name: 'Meat Wave' },
        ],
      },
      {
        id: 102,
        slug: 'show-102',
        title: 'Bottle Fest night two',
        event_date: '2026-07-29T01:00:00Z',
        city: 'Chicago',
        state: 'IL',
        price: null,
        age_requirement: null,
        artists: [
          { id: 11, slug: 'meat-wave', name: 'Meat Wave' },
          { id: 12, slug: 'gouge-away', name: 'Gouge Away' },
        ],
      },
    ]

    /** Camera → city → rail row → show row: the whole drill-in approach. */
    async function drillIntoFirstShow() {
      mockUseVenueShows.mockReturnValue({
        data: { shows: venueWeek, venue_id: 1, total: 2 },
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))
      fireEvent.click(
        screen.getByRole('button', { name: /Bottle Fest night one/ }),
      )
    }

    it('drills from a venue show row into that show’s first artist', async () => {
      await drillIntoFirstShow()

      expect(screen.getByTestId('atlas-artist-panel')).toBeInTheDocument()
      // The venue panel is REPLACED, not stacked over — that is what makes
      // Escape pop exactly one level.
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
      expect(
        screen.getByRole('heading', { name: 'Die Spitz' }),
      ).toBeInTheDocument()
    })

    // The locked decision (2026-07-25): the stepper walks THE LIST YOU DRILLED
    // IN FROM — this venue's week — not the one show you clicked, and not a
    // hardcoded venue scope.
    it('steps through the whole venue week in order, de-duplicated', async () => {
      await drillIntoFirstShow()
      expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
        'ARTIST · 1 OF 3 UPCOMING AT THIS VENUE',
      )

      fireEvent.click(screen.getByTestId('artist-panel-step-next'))
      expect(
        screen.getByRole('heading', { name: 'Meat Wave' }),
      ).toBeInTheDocument()
      expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
        'ARTIST · 2 OF 3 UPCOMING AT THIS VENUE',
      )

      // Meat Wave plays both nights but is ONE step — the third is night two's
      // other band, not a second Meat Wave.
      fireEvent.click(screen.getByTestId('artist-panel-step-next'))
      expect(
        screen.getByRole('heading', { name: 'Gouge Away' }),
      ).toBeInTheDocument()
      expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
        'ARTIST · 3 OF 3 UPCOMING AT THIS VENUE',
      )
    })

    // The stepper is the panel's ONLY forward affordance (the "NEXT UP … hear
    // them →" row that used to sit at the bottom is gone), so a keyboard user
    // has to be able to walk a whole bill on it: Enter must step AND leave
    // focus on `›`, or the second press lands nowhere. The panel deliberately
    // does not remount per step, and its focus effect is mount-only, which is
    // what makes this hold — this asserts it stays that way.
    it('walks the whole bill on repeated Enter, focus staying on ›', async () => {
      const user = userEvent.setup()
      await drillIntoFirstShow()

      const next = screen.getByTestId('artist-panel-step-next')
      next.focus()

      await user.keyboard('{Enter}')
      expect(
        screen.getByRole('heading', { name: 'Meat Wave' }),
      ).toBeInTheDocument()
      expect(screen.getByTestId('artist-panel-step-next')).toHaveFocus()

      await user.keyboard('{Enter}')
      expect(
        screen.getByRole('heading', { name: 'Gouge Away' }),
      ).toBeInTheDocument()
      expect(screen.getByTestId('artist-panel-step-next')).toHaveFocus()
    })

    it('steps backward to where it came from', async () => {
      await drillIntoFirstShow()
      fireEvent.click(screen.getByTestId('artist-panel-step-next'))
      fireEvent.click(screen.getByTestId('artist-panel-step-previous'))
      expect(
        screen.getByRole('heading', { name: 'Die Spitz' }),
      ).toBeInTheDocument()
    })

    it('drills in mid-list when a later show’s row is clicked', async () => {
      mockUseVenueShows.mockReturnValue({
        data: { shows: venueWeek, venue_id: 1, total: 2 },
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))

      fireEvent.click(
        screen.getByRole('button', { name: /Bottle Fest night two/ }),
      )

      // Night two's first artist is Meat Wave, which the de-duplicated list
      // already holds at index 1.
      expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
        'ARTIST · 2 OF 3 UPCOMING AT THIS VENUE',
      )
    })

    it('returns to the venue panel from the breadcrumb, rows intact', async () => {
      await drillIntoFirstShow()

      // Scoped to the panel: the rail also has an "Empty Bottle" row.
      fireEvent.click(
        within(screen.getByTestId('atlas-artist-panel')).getByRole('button', {
          name: /Empty Bottle/,
        }),
      )

      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /Bottle Fest night one/ }),
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /Bottle Fest night two/ }),
      ).toBeInTheDocument()
    })

    // The drill-in has no restore-focus-to-opener cleanup of its own (the show
    // row it opened from is already unmounted by then). The return path is
    // covered by the panel handed back to: VenuePanel remounts and focuses its
    // own close control, so a keyboard user lands INSIDE the venue panel
    // rather than back at the top of the document.
    it('lands keyboard focus in the venue panel on the way back', async () => {
      await drillIntoFirstShow()

      fireEvent.click(
        within(screen.getByTestId('atlas-artist-panel')).getByRole('button', {
          name: /Empty Bottle/,
        }),
      )

      const venuePanel = screen.getByTestId('atlas-venue-panel')
      expect(venuePanel).toContainElement(
        document.activeElement as HTMLElement | null,
      )
      expect(
        screen.getByRole('button', { name: 'Close Empty Bottle panel' }),
      ).toHaveFocus()
    })

    // Escape pops ONE level per keystroke: artist → venue → closed.
    it('pops one level per Escape', async () => {
      await drillIntoFirstShow()

      fireEvent.keyDown(document, { key: 'Escape' })
      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()

      fireEvent.keyDown(document, { key: 'Escape' })
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
      // Still in the city — Escape left the panel stack, not the city view.
      expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()
    })

    it('closes the whole stack from the artist panel’s ✕', async () => {
      await drillIntoFirstShow()

      fireEvent.click(
        screen.getByRole('button', { name: 'Close Die Spitz panel' }),
      )

      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
      expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()
    })

    // A drill-in must never outlive the panel its breadcrumb returns to.
    it('drops the drill-in when the camera leaves the city', async () => {
      await drillIntoFirstShow()
      settleCamera(-87.63, 41.88, 4)
      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
    })

    // The render-phase orphan guard, exercised through the filter path it was
    // written for: Hideout has nothing booked in the next 7 days, so applying
    // "Next 7 days" drops it from `filteredVenues` while its selection ID
    // survives. An artist panel whose "← Hideout" returns to nothing is the
    // dead end this prevents.
    it('drops the drill-in when a filter excludes its venue', async () => {
      mockUseVenueShows.mockReturnValue({
        data: { shows: venueWeek, venue_id: 2, total: 2 },
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      act(() => {
        lastCanvasProps.onVenueSelect?.(2) // Hideout: 0 in the next 7 days
      })
      fireEvent.click(
        screen.getByRole('button', { name: /Bottle Fest night one/ }),
      )
      expect(screen.getByTestId('atlas-artist-panel')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))

      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()

      // Clearing the filter restores the VENUE panel — the user's own
      // selection coming back — but NOT the drill-in, which was discarded.
      fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))
      expect(
        screen.getByRole('heading', { name: 'Hideout' }),
      ).toBeInTheDocument()
      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
    })

    it('drops the drill-in when another venue is selected', async () => {
      await drillIntoFirstShow()

      act(() => {
        lastCanvasProps.onVenueSelect?.(2)
      })

      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(
        screen.getByRole('heading', { name: 'Hideout' }),
      ).toBeInTheDocument()
    })

    it('does nothing when the clicked show has no steppable bill', async () => {
      mockUseVenueShows.mockReturnValue({
        data: {
          shows: [{ ...venueWeek[0], artists: [] }],
          venue_id: 1,
          total: 1,
        },
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))

      fireEvent.click(
        screen.getByRole('button', { name: /Bottle Fest night one/ }),
      )

      // No dead-end panel: the venue panel stays exactly where it was.
      expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()
    })

    it('stays clear of the map’s attribution control', async () => {
      await drillIntoFirstShow()
      // Same bounded height as the venue panel: a max, never a bottom anchor,
      // so the panel can never grow into the bottom-left OSM credit the ODbL
      // requires stay visible.
      expect(screen.getByTestId('atlas-artist-panel')).toHaveStyle({
        maxHeight: 'calc(100% - 0.75rem - 36px)',
      })
    })

    it('stays on the globe until the camera reaches street zoom', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')

      settleCamera(-87.63, 41.88, 6)

      expect(screen.queryByTestId('atlas-venue-rail')).not.toBeInTheDocument()
      expect(lastCanvasProps.cityLabel).toBeNull()
      expect(lastCanvasProps.venues).toEqual([])
    })

    it('opens the rail and pins the venues once the camera settles on a city', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')

      settleCamera(-87.63, 41.88, 13)

      expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()
      expect(lastCanvasProps.cityLabel).toBe('Chicago, IL')
      expect(lastCanvasProps.venues?.map((v) => v.name)).toEqual([
        'Empty Bottle',
        'Hideout',
      ])
    })

    // PSY-1574: the scene is keyed by CBSA metro, so the rail must ask for the
    // metro, not the principal city — otherwise it contradicts the scene page
    // that already counts a member-city venue.
    it('asks for the metro, not just the principal city', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')

      settleCamera(-87.63, 41.88, 13)

      expect(mockUseVenues).toHaveBeenLastCalledWith(
        expect.objectContaining({
          city: 'Chicago',
          state: 'IL',
          metroRollup: true,
        }),
      )
    })

    // The camera deliberately does NOT move to frame the metro: fitting a
    // 66-280 km metro into the pane lands below CITY_VIEW_MIN_ZOOM and would
    // close the rail it was fitting for.
    it('does not move the camera when the metro widens the rail', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      flyToSpy.mockClear()

      settleCamera(-87.63, 41.88, 13)

      expect(flyToSpy).not.toHaveBeenCalled()
    })

    it('leaves the camera space for the rail rather than letting it overlay', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      const fullWidth = lastCanvasProps.width

      settleCamera(-87.63, 41.88, 13)

      // The map pane shrinks by exactly the rail's width — that is what keeps
      // the rail off the map's bottom-left OSM attribution.
      expect(lastCanvasProps.width).toBe((fullWidth ?? 0) - 360)
    })

    it('narrows the pins with the rail when a filter is applied', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)

      fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))

      // One rail row, one pin — the same array feeds both.
      expect(screen.getAllByRole('button', { name: /Empty Bottle/ })).toHaveLength(1)
      expect(screen.queryByRole('button', { name: /Hideout/ })).not.toBeInTheDocument()
      expect(lastCanvasProps.venues?.map((v) => v.name)).toEqual(['Empty Bottle'])
    })

    it('narrows the pins with the rail for the all-ages chip too', async () => {
      // PSY-1573's acceptance criterion names the PINS explicitly, and the
      // sibling test above only covers "Next 7 days". Asserted end to end
      // rather than inferred from the shared code path, so a future all-ages
      // short-circuit that sourced pins from the unfiltered list would fail.
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)

      expect(lastCanvasProps.venues).toHaveLength(2)

      fireEvent.click(screen.getByRole('button', { name: 'All-ages shows' }))

      expect(screen.getAllByRole('button', { name: /Empty Bottle/ })).toHaveLength(1)
      expect(screen.queryByRole('button', { name: /Hideout/ })).not.toBeInTheDocument()
      expect(lastCanvasProps.venues?.map((v) => v.name)).toEqual(['Empty Bottle'])
    })

    // ── Venue panel (PSY-1540) ──────────────────────────────────────────
    // Both seams PSY-1539 left behind must reach the same panel.

    it('opens the venue panel from a rail row click', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))

      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()
      expect(
        screen.getByRole('heading', { name: 'Empty Bottle' }),
      ).toBeInTheDocument()
    })

    it('opens the venue panel from a map pin click', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)

      // The canvas reports a pin click by venue id — the same seam the real
      // MapLibre 'click' handler on the venue-pins layer calls.
      act(() => {
        lastCanvasProps.onVenueSelect?.(2)
      })

      expect(
        screen.getByRole('heading', { name: 'Hideout' }),
      ).toBeInTheDocument()
    })

    it('closes the venue panel on ✕ without clearing the city view', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))

      fireEvent.click(
        screen.getByRole('button', { name: 'Close Empty Bottle panel' }),
      )

      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
      // The rail and the pins are untouched — closing the panel is not
      // leaving the city.
      expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()
      expect(lastCanvasProps.venues).toHaveLength(2)
    })

    it('drops the venue panel when the camera leaves the city', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()

      settleCamera(-87.63, 41.88, 4)

      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
    })

    it('hides the venue panel while a filter excludes its venue', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      act(() => {
        lastCanvasProps.onVenueSelect?.(2) // Hideout: 0 in the next 7 days
      })
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()

      // Hideout loses its pin and its row, so its panel must go too rather
      // than describe a venue the user can no longer see beside it.
      fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()

      // The selection itself survives — clearing the filter restores it.
      fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))
      expect(
        screen.getByRole('heading', { name: 'Hideout' }),
      ).toBeInTheDocument()
    })

    it('hands the screen back to the globe when the camera pulls out', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      settleCamera(-87.63, 41.88, 13)
      expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()

      settleCamera(-87.63, 41.88, 4)

      expect(screen.queryByTestId('atlas-venue-rail')).not.toBeInTheDocument()
      expect(lastCanvasProps.venues).toEqual([])
      expect(
        screen.getByRole('button', { name: /drift to a random scene/i }),
      ).toBeInTheDocument()
    })

    // Regression: city view unmounts the globe chrome the scene preview lives
    // in WITHOUT calling its onClose, so a retained selection used to pop the
    // panel back open by itself the moment the camera zoomed out again.
    it('does not resurrect a scene preview after a trip through city view', async () => {
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      fireEvent.click(
        screen.getByRole('button', { name: /drift to a random scene/i }),
      )
      expect(
        screen.getByRole('complementary', { name: /Chicago, IL scene/ }),
      ).toBeInTheDocument()

      settleCamera(-87.63, 41.88, 13) // into city view
      settleCamera(-87.63, 41.88, 4) // back out

      expect(
        screen.queryByRole('complementary', { name: /Chicago, IL scene/ }),
      ).not.toBeInTheDocument()
    })

    // Regression: the venues query keeps previous data across a key change,
    // which across a CITY change would show the previous city's venues with
    // no loading state.
    it('shows no venues while the fetch for a new city is still carrying old data', async () => {
      mockUseVenues.mockReturnValue({
        data: { venues: chicagoVenues, total: 2 },
        isFetching: true,
        isPlaceholderData: true,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')

      settleCamera(-87.63, 41.88, 13)

      expect(lastCanvasProps.venues).toEqual([])
      expect(screen.getByText('Loading venues…')).toBeInTheDocument()
    })

    it('says so when the fetch cap truncated the city', async () => {
      mockUseVenues.mockReturnValue({
        data: { venues: chicagoVenues, total: 150 },
        isFetching: false,
        isPlaceholderData: false,
      })
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')

      settleCamera(-87.63, 41.88, 13)

      expect(screen.getByText('showing the 2 busiest of 150')).toBeInTheDocument()
    })
  })
})
