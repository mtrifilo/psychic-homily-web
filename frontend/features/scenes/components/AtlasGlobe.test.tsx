import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { MutableRefObject, ReactNode } from 'react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListResponse } from '../types'
import type { PlaceableScene } from './globeTypes'

// AtlasGlobe statically imports only `globeTypes` (no react-globe.gl) and
// dynamic-imports GlobeCanvas (ssr:false). In jsdom we exercise the testable
// surface: data wiring, the loading/error states, and the scene-list fallback
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
// `?city=` is the globe's one URL entry point (PSY-2079); tests that exercise
// it set this before rendering.
let searchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  // VenuePanel's confirm control reads the pathname to build its auth
  // return-to (PSY-1542).
  usePathname: () => '/atlas',
  useSearchParams: () => searchParams,
}))

// Stub the WebGL canvas for the desktop-branch tests (PSY-1308 Drift): it
// fills the flyToRef seam with a spy so the drift handler's camera call is
// observable without three.js.
const flyToSpy = vi.fn()
// City view (PSY-1539): the stub also captures the canvas's camera-settle
// callback and the pin array, so a test can drive the camera the way the real
// map does and assert what the map would have drawn — the whole
// camera → city → fetch → filter → pins chain, without WebGL.
type CanvasSettle = {
  lng: number
  lat: number
  zoom: number
  bounds?: { west: number; south: number; east: number; north: number }
}
let lastCanvasProps: {
  pov?: { lat: number; lng: number; altitude: number }
  onCameraSettle?: (c: CanvasSettle) => void
  onVenueSelect?: (venueId: number) => void
  venues?: readonly { id: number; name: string }[]
  cityLabel?: string | null
  width?: number
  attributionPosition?: 'bottom-left' | 'top-left'
  onBackToGlobe?: () => void
  venueStacks?: readonly { key: string; venueIds: readonly number[]; label: string }[]
  onVenueStackSelect?: (key: string) => void
  scenes?: readonly PlaceableScene[]
  onSelect?: (scene: PlaceableScene) => void
} = {}
// Set by a case to make the canvas throw from its mount effect, as the real
// GlobeCanvas does when MapLibre gets no WebGL2 context.
let mockCanvasThrowsOnStart: false | 'context' | 'other' = false
vi.mock('./GlobeCanvas', async () => {
  const { useEffect } = await import('react')
  const { AtlasMapContextError } = await import('../atlasViewport')
  return {
    default: function MockGlobeCanvas(
      props: typeof lastCanvasProps & {
        flyToRef?: MutableRefObject<((scene: PlaceableScene) => void) | null>
      },
    ) {
      if (props.flyToRef) props.flyToRef.current = flyToSpy
      lastCanvasProps = props
      useEffect(() => {
        if (mockCanvasThrowsOnStart === 'context') throw new AtlasMapContextError()
        if (mockCanvasThrowsOnStart === 'other') throw new Error('a later effect threw')
      }, [])
      return <div data-testid="globe-canvas" />
    },
  }
})

const preloadAtlasMap = vi.fn()
vi.mock('./atlasMapPreload', () => ({
  preloadAtlasMap: () => preloadAtlasMap(),
}))

// jsdom has no WebGL, so the real probe would answer false and every case
// would get the scene list. The probe itself is unit-tested in
// atlasViewport.test.ts; here a case says whether the browser has WebGL2.
let mockSupportsWebGL2 = true
// The page-load failure latch, reset per case so one case's failed map does
// not send every later case to the list.
let mockMapFailedThisPage = false
vi.mock('../atlasViewport', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../atlasViewport')>()),
  atlasSupportsWebGL2: () => mockSupportsWebGL2,
  atlasMapFailedThisPage: () => mockMapFailedThisPage,
  markAtlasMapFailed: () => {
    mockMapFailedThisPage = true
  },
}))

import { AtlasGlobe } from './AtlasGlobe'
import { clearAtlasCamera, readAtlasCamera, saveAtlasCamera } from './atlasCamera'
import { ATLAS_SHEET_TOP_INSET_PX, CITY_VIEW_MIN_ZOOM } from '../cityView'
import {
  ATLAS_COMPACT_VIEWPORT_QUERY,
  ATLAS_REDUCED_MOTION_LIST_BELOW_PX,
} from '../atlasViewport'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { altitudeForZoom } from './globeScale'

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// ResizeObserver shim to drive the container width (same pattern as
// SceneGraph.test.tsx). Defaults to a phone-sized pane.
let mockContainerWidth = 500
function setMockContainerWidth(width: number) {
  mockContainerWidth = width
}

// Every live observer's re-report, so a test can announce a new container
// width to all of them (AtlasGlobe's container and any sheet hosts alike).
const liveResizeReports = new Set<() => void>()
function reportResize() {
  for (const report of liveResizeReports) report()
}
class ImmediateResizeObserver {
  private callback: ResizeObserverCallback
  private reports: (() => void)[] = []
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element): void {
    const report = () => this.report(target)
    this.reports.push(report)
    liveResizeReports.add(report)
    this.report(target)
  }
  private report(target: Element): void {
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
  disconnect(): void {
    for (const report of this.reports) liveResizeReports.delete(report)
    this.reports = []
  }
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
    mockSupportsWebGL2 = true
    mockCanvasThrowsOnStart = false
    mockMapFailedThisPage = false
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = ImmediateResizeObserver
    mockUseScenes.mockReset()
    searchParams = new URLSearchParams()
    // The geo-centering fetch is non-fatal; stub it to a no-op miss.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
  })

  afterEach(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(window as any).ResizeObserver = originalResizeObserver
    vi.unstubAllGlobals()
  })

  describe('map or scene list', () => {
    let matchMedia: ReturnType<typeof installMatchMedia> | null = null
    afterEach(() => {
      matchMedia?.restore()
      matchMedia = null
    })

    function renderWithScenes() {
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
    }

    async function expectMap() {
      expect(await screen.findByTestId('globe-canvas')).toBeInTheDocument()
      expect(screen.queryByTestId('atlas-scene-list')).not.toBeInTheDocument()
    }

    /** The fallback lists every scene, unplaceable ones too, with its links. */
    async function expectFullSceneList() {
      expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument()
      expect(screen.queryByTestId('globe-canvas')).not.toBeInTheDocument()
      const chicago = screen.getByRole('button', { name: /Chicago, IL/ })
      expect(chicago).toHaveAttribute('aria-expanded', 'false')
      expect(screen.getByRole('button', { name: /Faketown, ZZ/ })).toBeInTheDocument()
      await userEvent.click(chicago)
      expect(screen.getByRole('link', { name: /Open scene/ })).toHaveAttribute(
        'href',
        '/scenes/chicago-il',
      )
    }

    it.each([390, 360, 500])('renders the map, not the list, on a %ipx pane', async (width) => {
      setMockContainerWidth(width)
      renderWithScenes()
      await expectMap()
      // Phone panes take the sheet layout: credit top-left, back control.
      expect(lastCanvasProps.attributionPosition).toBe('top-left')
      expect(lastCanvasProps.onBackToGlobe).toBeTypeOf('function')
    })

    it.each([390, 1400])(
      'lists every scene instead of the map without WebGL2, on a %ipx pane',
      async (width) => {
        setMockContainerWidth(width)
        mockSupportsWebGL2 = false
        renderWithScenes()
        await expectFullSceneList()
        expect(preloadAtlasMap).not.toHaveBeenCalled()
      },
    )

    it('lists every scene instead of the map for reduced motion below the threshold', async () => {
      matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
      setMockContainerWidth(ATLAS_REDUCED_MOTION_LIST_BELOW_PX - 1)
      renderWithScenes()
      await expectFullSceneList()
      expect(preloadAtlasMap).not.toHaveBeenCalled()
    })

    it.each([ATLAS_REDUCED_MOTION_LIST_BELOW_PX, 1400])(
      'keeps the map for reduced motion on a %ipx pane',
      async (width) => {
        matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
        setMockContainerWidth(width)
        renderWithScenes()
        await expectMap()
        // The preference was asked for, so the map is a decision, not a miss.
        expect(matchMedia.queries).toContain(REDUCED_MOTION_QUERY)
      },
    )

    it.each([390, 1400])(
      'falls back to the list when the map throws while starting, on a %ipx pane',
      async (width) => {
        const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
        mockCanvasThrowsOnStart = 'context'
        setMockContainerWidth(width)
        renderWithScenes()
        await waitFor(() =>
          expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument(),
        )
        await expectFullSceneList()
        quiet.mockRestore()
      },
    )

    it('goes straight to the list on a later mount once the map was refused a context', async () => {
      const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockCanvasThrowsOnStart = 'context'
      setMockContainerWidth(390)
      mockUseScenes.mockReturnValue({ data: sampleData, isLoading: false, isError: false })
      const first = renderWithProviders(<AtlasGlobe />)
      await waitFor(() => expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument())
      expect(mockMapFailedThisPage).toBe(true)
      first.unmount()

      mockCanvasThrowsOnStart = false
      renderWithScenes()
      expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument()
      expect(screen.queryByTestId('globe-canvas')).not.toBeInTheDocument()
      quiet.mockRestore()
    })

    it('falls back for this mount only when the map throws something else', async () => {
      const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockCanvasThrowsOnStart = 'other'
      setMockContainerWidth(390)
      mockUseScenes.mockReturnValue({ data: sampleData, isLoading: false, isError: false })
      const first = renderWithProviders(<AtlasGlobe />)
      await waitFor(() => expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument())
      expect(mockMapFailedThisPage).toBe(false)
      first.unmount()

      mockCanvasThrowsOnStart = false
      renderWithScenes()
      expect(await screen.findByTestId('globe-canvas')).toBeInTheDocument()
      quiet.mockRestore()
    })

    it('shows the error state, not the list, when scenes fail without WebGL2', () => {
      mockSupportsWebGL2 = false
      setMockContainerWidth(1400)
      mockUseScenes.mockReturnValue({ data: undefined, isLoading: false, isError: true })
      renderWithProviders(<AtlasGlobe />)
      expect(screen.getByText(/couldn’t load/i)).toBeInTheDocument()
      expect(screen.queryByTestId('atlas-scene-list')).not.toBeInTheDocument()
    })

    it('swaps to the list when reduced motion is turned on mid-session', async () => {
      matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
      setMockContainerWidth(390)
      renderWithScenes()
      await expectMap()
      matchMedia.set(REDUCED_MOTION_QUERY, true)
      expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument()
      expect(screen.queryByTestId('globe-canvas')).not.toBeInTheDocument()
    })
  })

  describe('early canvas fetch', () => {
    it('starts loading the canvas module while scenes are still loading, on a map-wide container', () => {
      setMockContainerWidth(1200)
      mockUseScenes.mockReturnValue({ data: undefined, isLoading: true, isError: false })
      renderWithProviders(<AtlasGlobe />)
      expect(screen.queryByTestId('globe-canvas')).not.toBeInTheDocument()
      expect(preloadAtlasMap).toHaveBeenCalledTimes(1)
    })

    it('starts loading it while the camera focus waits on visitor geo, scenes already loaded', () => {
      setMockContainerWidth(1200)
      // The geo lookup never answers, so the focus stays pending (within
      // the geo timeout) and the canvas cannot render yet.
      vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
      mockUseScenes.mockReturnValue({ data: sampleData, isLoading: false, isError: false })
      renderWithProviders(<AtlasGlobe />)
      expect(screen.queryByTestId('globe-canvas')).not.toBeInTheDocument()
      expect(preloadAtlasMap).toHaveBeenCalledTimes(1)
    })

    it('does not fetch it once the scenes arrive with nothing to place', () => {
      setMockContainerWidth(1200)
      mockUseScenes.mockReturnValue({
        data: { scenes: [], count: 0 },
        isLoading: false,
        isError: false,
      })
      renderWithProviders(<AtlasGlobe />)
      expect(preloadAtlasMap).not.toHaveBeenCalled()
    })

    it('does not fetch it when the scenes query has failed', () => {
      setMockContainerWidth(1200)
      mockUseScenes.mockReturnValue({ data: undefined, isLoading: false, isError: true })
      renderWithProviders(<AtlasGlobe />)
      expect(preloadAtlasMap).not.toHaveBeenCalled()
    })
  })

  // A tripwire on the frame's class contract, not a layout measurement (jsdom
  // has none; atlas-credit.spec.ts measures it). The banner publishes
  // --cookie-banner-height while it is up (absent otherwise, hence the 0px
  // fallbacks); the frame's height and its minimum both give it up, and at
  // `xl` the banner's height stands in for the home-indicator inset it
  // already contains.
  it('gives up the published cookie banner height in its height and minimum', () => {
    setMockContainerWidth(1200)
    mockUseScenes.mockReturnValue({
      data: sampleData,
      isLoading: false,
      isError: false,
    })
    renderWithProviders(<AtlasGlobe />)
    const frame = screen.getByTestId('atlas-pane-frame')
    expect(frame).toHaveClass(
      'h-[calc(100dvh-4rem-var(--bottom-tab-bar-height)-env(safe-area-inset-bottom)-var(--cookie-banner-height,0px))]',
      'xl:h-[calc(100dvh-4rem-max(env(safe-area-inset-bottom),var(--cookie-banner-height,0px)))]',
      'min-h-[calc(480px-var(--cookie-banner-height,0px))]',
    )
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

  it('shows a loading state in the scene list while scenes load', () => {
    mockSupportsWebGL2 = false
    mockUseScenes.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })
    renderWithProviders(<AtlasGlobe />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  describe('?city= entry (PSY-2079)', () => {
    // Module state by design (it outlives a canvas teardown), so each case
    // starts and ends from a known camera.
    afterEach(() => clearAtlasCamera())

    // The shared sample places only Chicago; these cases need a second city to
    // move BETWEEN.
    const twoCities: SceneListResponse = {
      scenes: [
        ...sampleData.scenes,
        {
          city: 'Phoenix',
          state: 'AZ',
          slug: 'phoenix-az',
          venue_count: 8,
          upcoming_show_count: 40,
          total_show_count: 90,
          shows_this_week: 2,
          shows_calendar_week: 2,
          latitude: 33.4484,
          longitude: -112.074,
        },
      ],
      count: 3,
    }

    beforeEach(() => {
      clearAtlasCamera()
      setMockContainerWidth(800)
      mockUseScenes.mockReturnValue({
        data: twoCities,
        isLoading: false,
        isError: false,
      })
    })

    it('opens on the named city, close enough that city view engages', async () => {
      searchParams = new URLSearchParams('city=Chicago,IL')

      renderWithProviders(<AtlasGlobe />)

      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.pov?.lat).toBeCloseTo(41.88)
      expect(lastCanvasProps.pov?.lng).toBeCloseTo(-87.63)
      expect(lastCanvasProps.pov!.altitude).toBeLessThan(
        altitudeForZoom(CITY_VIEW_MIN_ZOOM),
      )
    })

    it('drops the camera the session left behind, or the link would do nothing', async () => {
      saveAtlasCamera({ center: [2, 3], zoom: 14 })
      searchParams = new URLSearchParams('city=Chicago,IL')

      renderWithProviders(<AtlasGlobe />)

      await screen.findByTestId('globe-canvas')
      expect(readAtlasCamera()).toBeNull()
    })

    it('leaves a mounted globe where it is for a second city, camera intact', async () => {
      searchParams = new URLSearchParams('city=Chicago,IL')
      const { rerender } = renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.pov?.lat).toBeCloseTo(41.88)

      // Where the visitor has moved to since arriving.
      saveAtlasCamera({ center: [2, 3], zoom: 14 })

      // A second entry followed WITHOUT a remount. The focus resolves once,
      // because GlobeCanvas documents a teardown hazard for a pov whose
      // identity changes under a mounted canvas. What must NOT happen is the
      // camera being discarded by a link that cannot move the map.
      searchParams = new URLSearchParams('city=Phoenix,AZ')
      rerender(<AtlasGlobe />)
      await Promise.resolve()

      expect(lastCanvasProps.pov?.lat).toBeCloseTo(41.88)
      expect(readAtlasCamera()).not.toBeNull()
    })

    it('does not re-aim or drop the camera when the same URL re-renders', async () => {
      searchParams = new URLSearchParams('city=Chicago,IL')
      const { rerender } = renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      const aimed = lastCanvasProps.pov

      saveAtlasCamera({ center: [2, 3], zoom: 14 })
      rerender(<AtlasGlobe />)

      expect(lastCanvasProps.pov).toBe(aimed)
      expect(readAtlasCamera()).not.toBeNull()
    })

    it('falls back to the geo focus for a city no scene knows', async () => {
      searchParams = new URLSearchParams('city=Atlantis,ZZ')

      renderWithProviders(<AtlasGlobe />)

      await screen.findByTestId('globe-canvas')
      // The stubbed /api/geo answers a miss, so this is the default focus.
      expect(lastCanvasProps.pov?.lat).toBeCloseTo(39.5)
      expect(lastCanvasProps.pov?.lng).toBeCloseTo(-98.35)
    })

    it('leaves a saved camera alone when no city is named', async () => {
      saveAtlasCamera({ center: [2, 3], zoom: 14 })

      renderWithProviders(<AtlasGlobe />)

      await screen.findByTestId('globe-canvas')
      expect(readAtlasCamera()).not.toBeNull()
    })
  })

  describe('Drift (desktop globe branch, PSY-1308)', () => {
    beforeEach(() => {
      setMockContainerWidth(1400) // the side-panel layout
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

  describe('genre legend and Drift', () => {
    let matchMedia: ReturnType<typeof installMatchMedia>
    beforeEach(() => {
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        isError: false,
      })
    })
    afterEach(() => matchMedia.restore())

    const legendToggle = () => screen.getByRole('button', { name: 'Genres' })

    const drift = () => screen.getByRole('button', { name: /drift to a random scene/i })
    const notOnMapLink = () => screen.getByRole('link', { name: /not on the map/i })

    it('groups the bottom chrome bottom-left in the sheet layout, the link above Drift and the key', async () => {
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
      setMockContainerWidth(820)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.attributionPosition).toBe('top-left')
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
      // One column; at its foot the "not on the map" link (Faketown has no
      // coords) above a row of Drift and the genre key, so none can overlap.
      const row = drift().parentElement!
      const bottomGroup = row.parentElement!
      const column = bottomGroup.parentElement!
      expect(row).toContainElement(legendToggle())
      expect(bottomGroup.firstElementChild).toBe(notOnMapLink())
      expect(column).toHaveClass('absolute', 'bottom-4', 'left-4', 'flex', 'flex-col')
      // The column's gaps stay the map's; only the controls take taps.
      expect(column).toHaveClass('pointer-events-none')
      expect(drift()).toHaveClass('pointer-events-auto')
      expect(notOnMapLink()).toHaveClass('pointer-events-auto')
      expect(legendToggle().parentElement).toHaveClass('pointer-events-auto')
      expect(drift()).not.toHaveClass('absolute')
      expect(notOnMapLink()).not.toHaveClass('absolute')
    })

    it('bounds the sheet layout’s column to the band below the credit strip', async () => {
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
      setMockContainerWidth(820)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      const row = drift().parentElement!
      const column = row.parentElement!.parentElement!
      // The pane publishes the strip no sheet may cover; the column starts
      // below it and stacks from the bottom, so an open key on a short pane
      // shrinks into the column (and scrolls) instead of rising over the
      // top-left credit.
      expect(screen.getByTestId('globe-canvas').parentElement).toHaveStyle({
        '--atlas-sheet-top-inset': `${ATLAS_SHEET_TOP_INSET_PX}px`,
      })
      expect(column).toHaveClass('top-[var(--atlas-sheet-top-inset)]', 'flex-col')
      expect(row).toHaveClass('min-h-0')
      // The bottom group sits at the column's foot and can shrink.
      expect(row.parentElement).toHaveClass('mt-auto', 'min-h-0')
      expect(row.parentElement!.parentElement).toBe(column)
      expect(legendToggle().parentElement).toHaveClass('max-h-full', 'min-h-0')
      expect(document.getElementById('atlas-genre-legend')).toHaveClass(
        'min-h-0',
        'overflow-y-auto',
      )
      expect(notOnMapLink()).toHaveClass('shrink-0')
    })

    it('heads the sheet layout’s column with My Scenes, so an open key cannot rise under it', async () => {
      mockUseMyFollowing.mockReturnValue({
        data: { following: [{ slug: 'chicago-il', name: 'Chicago' }], total: 1 },
      } as never)
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
      setMockContainerWidth(820)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      const strip = screen.getByRole('navigation', { name: 'My scenes' })
      const column = drift().parentElement!.parentElement!.parentElement!
      expect(column.firstElementChild).toBe(strip)
      expect(strip).toHaveClass('shrink-0')
      expect(strip).not.toHaveClass('absolute')
      mockUseMyFollowing.mockReturnValue({ data: undefined })
    })

    it('keeps My Scenes under the search in the panel layout', async () => {
      mockUseMyFollowing.mockReturnValue({
        data: { following: [{ slug: 'chicago-il', name: 'Chicago' }], total: 1 },
      } as never)
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false })
      setMockContainerWidth(1400)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(screen.getByRole('navigation', { name: 'My scenes' })).toHaveClass(
        'absolute',
        'left-4',
        'top-16',
      )
      mockUseMyFollowing.mockReturnValue({ data: undefined })
    })

    it('keeps the panel layout’s own placements below lg, clear of the bottom-left credit', async () => {
      // 900 to 1023px: a compact viewport, but a pane wide enough for the
      // panel layout, whose credit docks bottom-left.
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
      setMockContainerWidth(950)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.attributionPosition).toBe('bottom-left')
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
      expect(drift()).toHaveClass('absolute', 'bottom-4', 'left-1/2', '-translate-x-1/2')
      expect(legendToggle().parentElement).toHaveClass('absolute', 'bottom-4', 'right-4')
      expect(notOnMapLink()).toHaveClass('absolute', 'bottom-11', 'left-4')
      // Each docks on the map pane itself, in no shared row.
      expect(legendToggle().parentElement!.parentElement).toBe(drift().parentElement)
      expect(notOnMapLink().parentElement).toBe(drift().parentElement)
    })

    it('starts open on a wide viewport, docked bottom-right', async () => {
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false })
      setMockContainerWidth(1400)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'true')
      expect(legendToggle().parentElement).toHaveClass('absolute', 'bottom-4', 'right-4')
      expect(drift()).toHaveClass('absolute', 'left-1/2')
    })

    it('follows the viewport until toggled, then keeps the user’s choice', async () => {
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
      setMockContainerWidth(820)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')

      // Widening past lg before any toggle opens it.
      matchMedia.set(ATLAS_COMPACT_VIEWPORT_QUERY, false)
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'true')

      // A toggle is the user's choice and survives crossing back.
      fireEvent.click(legendToggle())
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
      matchMedia.set(ATLAS_COMPACT_VIEWPORT_QUERY, true)
      matchMedia.set(ATLAS_COMPACT_VIEWPORT_QUERY, false)
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
    })

    it('keeps a collapse the user chose across a scene preview', async () => {
      matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false })
      setMockContainerWidth(1400)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      fireEvent.click(legendToggle())
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
      fireEvent.click(screen.getByRole('button', { name: /drift to a random scene/i }))
      expect(screen.queryByRole('button', { name: 'Genres' })).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /close scene preview/i }))
      expect(legendToggle()).toHaveAttribute('aria-expanded', 'false')
    })
  })

  describe('scene search list offset', () => {
    beforeEach(() => {
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        isError: false,
      })
    })

    /** Draws a top-left credit with text inside the map pane, as MapLibre would. */
    function drawTopCredit() {
      screen.getByTestId('globe-canvas').innerHTML =
        '<div data-atlas-credit="top"><details class="maplibregl-ctrl maplibregl-ctrl-attrib">OpenStreetMap</details></div>'
    }

    /**
     * The list's vertical translate. jsdom lays the trigger out at 0, so this
     * is the popover's side offset: 4 is its default gap.
     */
    async function listTranslateY(): Promise<string> {
      fireEvent.click(screen.getByRole('combobox', { name: 'Search scenes' }))
      let transform = ''
      await waitFor(() => {
        const wrapper = document.querySelector<HTMLElement>(
          '[data-radix-popper-content-wrapper]',
        )
        transform = wrapper?.style.transform ?? ''
        // Radix parks the wrapper off screen until floating-ui has placed it.
        expect(transform).toMatch(/^translate\(0px, -?\d+px\)$/)
      })
      return transform
    }

    it('opens below a drawn top-left credit in the sheet layout', async () => {
      setMockContainerWidth(800)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.attributionPosition).toBe('top-left')
      drawTopCredit()
      expect(await listTranslateY()).toBe('translate(0px, 112px)')
    })

    it('opens directly under its trigger when the sheet layout draws no credit', async () => {
      setMockContainerWidth(800)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(await listTranslateY()).toBe('translate(0px, 4px)')
    })

    it('keeps the default gap in the panel layout, whose credit is bottom-left', async () => {
      setMockContainerWidth(1400)
      renderWithProviders(<AtlasGlobe />)
      await screen.findByTestId('globe-canvas')
      expect(lastCanvasProps.attributionPosition).toBe('bottom-left')
      screen.getByTestId('globe-canvas').innerHTML =
        '<details class="maplibregl-ctrl maplibregl-ctrl-attrib">OpenStreetMap</details>'
      expect(await listTranslateY()).toBe('translate(0px, 4px)')
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
    // ── Sheet layout (panes under 900px) ────────────────────────────────
    // Too narrow for the rail beside the map, so the venue list and every
    // panel become bottom sheets over a full-width map. Exercised at 800px,
    // below the rail's 900; phone widths take the same layout (see "map or
    // scene list").
    describe('sheet layout', () => {
      beforeEach(() => {
        setMockContainerWidth(800)
      })

      /** Settle with a viewport that holds both Chicago pins. */
      function settleOnChicago() {
        act(() => {
          lastCanvasProps.onCameraSettle?.({
            lng: -87.63,
            lat: 41.88,
            zoom: 13,
            bounds: { west: -87.7, south: 41.8, east: -87.5, north: 41.95 },
          })
        })
      }

      it('replaces the rail with a venue sheet at Peek, map full width', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        expect(screen.queryByTestId('atlas-venue-rail')).not.toBeInTheDocument()
        expect(lastCanvasProps.width).toBe(800)
        const sheet = screen.getByTestId('atlas-venue-sheet')
        expect(sheet).toHaveAttribute('data-detent', 'peek')
        expect(
          within(sheet).getByRole('heading', { name: 'Chicago · 2 venues' }),
        ).toBeInTheDocument()
        // Both rooms pin at the city centroid, so they share one point.
        expect(screen.getByTestId('venue-sheet-peek-line')).toHaveTextContent(
          '2 in view · 2 share the city centre point',
        )
        // No close control: the list is the city view's standing surface.
        expect(
          within(sheet).queryByRole('button', { name: /close/i }),
        ).not.toBeInTheDocument()
      })

      it('lists every venue as a row from Half, including stacked ones', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        fireEvent.click(screen.getByRole('button', { name: 'Expand Chicago venues' }))
        const sheet = screen.getByTestId('atlas-venue-sheet')
        expect(sheet).toHaveAttribute('data-detent', 'half')
        expect(within(sheet).getByRole('button', { name: /Empty Bottle/ })).toBeInTheDocument()
        expect(within(sheet).getByRole('button', { name: /Hideout/ })).toBeInTheDocument()
        // The rail's filters come along, as 24px chips.
        const chip = within(sheet).getByRole('button', { name: 'Next 7 days' })
        expect(chip.className).toContain('min-h-6')
      })

      it('hands the canvas the phone chrome: top credit, back control, stacks', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        expect(lastCanvasProps.attributionPosition).toBe('top-left')
        expect(lastCanvasProps.onBackToGlobe).toBeTypeOf('function')
        expect(lastCanvasProps.venueStacks).toEqual([
          expect.objectContaining({
            venueIds: [1, 2],
            label: '2 venues · city centre',
          }),
        ])

        flyToSpy.mockReset()
        act(() => lastCanvasProps.onBackToGlobe?.())
        expect(flyToSpy).toHaveBeenCalledWith(
          expect.objectContaining({ slug: 'chicago-il' }),
        )
      })

      it('opens the list at Peek scoped to a tapped stack, Half one pull up, and widens it back', async () => {
        mockUseVenues.mockReturnValue({
          data: {
            venues: [
              ...chicagoVenues,
              {
                ...chicagoVenues[1],
                id: 3,
                name: 'Thalia Hall',
                street_latitude: 41.857,
                street_longitude: -87.657,
              },
            ],
            total: 3,
          },
          isFetching: false,
          isPlaceholderData: false,
        })
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        const stack = lastCanvasProps.venueStacks![0]
        act(() => lastCanvasProps.onVenueStackSelect?.(stack.key))

        const sheet = screen.getByTestId('atlas-venue-sheet')
        expect(sheet).toHaveAttribute('data-detent', 'peek')
        // Peek names the scope in place of the city-wide count line.
        expect(screen.getByTestId('venue-sheet-peek-line')).toHaveTextContent(
          '2 venues at the city centre point',
        )

        fireEvent.click(within(sheet).getByRole('button', { name: 'Expand Chicago venues' }))
        expect(sheet).toHaveAttribute('data-detent', 'half')
        expect(screen.getByTestId('venue-sheet-scope-line')).toHaveTextContent(
          '2 venues at the city centre point',
        )
        expect(within(sheet).getByRole('button', { name: /Empty Bottle/ })).toBeInTheDocument()
        expect(within(sheet).queryByRole('button', { name: /Thalia Hall/ })).not.toBeInTheDocument()

        within(sheet).getByRole('button', { name: 'Show all' }).focus()
        fireEvent.click(within(sheet).getByRole('button', { name: 'Show all' }))
        // The button is gone; focus stays in the sheet, on the rows.
        expect(sheet).toContainElement(document.activeElement as HTMLElement)
        expect(within(sheet).getByRole('button', { name: /Thalia Hall/ })).toBeInTheDocument()
        expect(screen.queryByTestId('venue-sheet-scope-line')).not.toBeInTheDocument()
      })

      it('brings the list back to Peek when a stack is tapped from Half, Full or over a venue sheet', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        const sheet = screen.getByTestId('atlas-venue-sheet')
        const stack = lastCanvasProps.venueStacks![0]

        fireEvent.click(within(sheet).getByRole('button', { name: 'Expand Chicago venues' }))
        expect(sheet).toHaveAttribute('data-detent', 'half')
        act(() => lastCanvasProps.onVenueStackSelect?.(stack.key))
        expect(sheet).toHaveAttribute('data-detent', 'peek')

        fireEvent.click(within(sheet).getByRole('button', { name: 'Expand Chicago venues' }))
        fireEvent.click(within(sheet).getByRole('button', { name: 'Expand Chicago venues' }))
        expect(sheet).toHaveAttribute('data-detent', 'full')
        act(() => lastCanvasProps.onVenueStackSelect?.(stack.key))
        expect(sheet).toHaveAttribute('data-detent', 'peek')

        // Back to Half, then a venue sheet over it: the hidden list keeps Half.
        fireEvent.click(within(sheet).getByRole('button', { name: 'Expand Chicago venues' }))
        expect(sheet).toHaveAttribute('data-detent', 'half')
        act(() => lastCanvasProps.onVenueSelect?.(2))
        expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()
        expect(sheet).toHaveClass('hidden')
        expect(sheet).toHaveAttribute('data-detent', 'half')
        act(() => lastCanvasProps.onVenueStackSelect?.(stack.key))
        expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
        expect(sheet).not.toHaveClass('hidden')
        expect(sheet).toHaveAttribute('data-detent', 'peek')
      })

      it('opens a single pin as a venue sheet at Half, hiding the list', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        act(() => lastCanvasProps.onVenueSelect?.(2))
        const venueSheet = screen.getByTestId('atlas-venue-panel')
        expect(venueSheet).toHaveAttribute('data-slot', 'bottom-sheet')
        expect(venueSheet).toHaveAttribute('data-detent', 'half')
        expect(
          within(venueSheet).getByRole('heading', { name: 'Hideout' }),
        ).toBeInTheDocument()
        expect(screen.getByTestId('atlas-venue-sheet')).toHaveClass('hidden')

        fireEvent.click(screen.getByRole('button', { name: 'Close Hideout panel' }))
        expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
        expect(screen.getByTestId('atlas-venue-sheet')).not.toHaveClass('hidden')
      })

      it('walks list row, venue sheet, artist sheet at Full, and back by Escape', async () => {
        mockUseVenueShows.mockReturnValue({
          data: { shows: venueWeek, venue_id: 1, total: 2 },
          isLoading: false,
          isError: false,
        })
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        fireEvent.click(screen.getByRole('button', { name: 'Expand Chicago venues' }))
        fireEvent.click(screen.getByRole('button', { name: /Empty Bottle/ }))
        fireEvent.click(screen.getByRole('button', { name: /Bottle Fest night one/ }))

        const artistSheet = screen.getByTestId('atlas-artist-panel')
        expect(artistSheet).toHaveAttribute('data-slot', 'bottom-sheet')
        expect(artistSheet).toHaveAttribute('data-detent', 'full')
        expect(
          within(artistSheet).getByRole('heading', { name: 'Die Spitz' }),
        ).toBeInTheDocument()
        // The stepper's touch targets are 28px.
        expect(screen.getByTestId('artist-panel-step-next').className).toContain('size-7')

        await userEvent.keyboard('{Escape}')
        expect(screen.queryByTestId('atlas-artist-panel')).not.toBeInTheDocument()
        expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()

        await userEvent.keyboard('{Escape}')
        expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
        const list = screen.getByTestId('atlas-venue-sheet')
        expect(list).not.toHaveClass('hidden')
        expect(list).toHaveAttribute('data-detent', 'half')

        // Escape on the list itself collapses it rather than removing it.
        await userEvent.keyboard('{Escape}')
        expect(list).toHaveAttribute('data-detent', 'peek')
      })

      it('lifts the zoom control by the list sheet’s height', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        const pane = screen.getByTestId('globe-canvas').parentElement!
        expect(pane).toHaveAttribute('data-atlas-layout', 'sheet')
        expect(pane.style.getPropertyValue('--atlas-sheet-offset')).toBe(
          'min(120px, calc(100% - 112px))',
        )
        act(() => lastCanvasProps.onVenueSelect?.(1))
        expect(pane.style.getPropertyValue('--atlas-sheet-offset')).toBe('0px')
      })

      it('opens a tapped scene dot’s preview as a sheet at Half', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        const dot = lastCanvasProps.scenes![0]
        act(() => lastCanvasProps.onSelect?.(dot))

        const preview = screen.getByRole('region', { name: `${dot.city}, ${dot.state} scene` })
        expect(preview).toHaveAttribute('data-slot', 'bottom-sheet')
        expect(preview).toHaveAttribute('data-detent', 'half')
      })

      it('keeps the preview’s detent when another scene is picked while it is open', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        const dot = lastCanvasProps.scenes![0]
        act(() => lastCanvasProps.onSelect?.(dot))
        const preview = screen.getByTestId('atlas-scene-preview-sheet')
        fireEvent.click(
          within(preview).getByRole('button', { name: `Expand ${dot.city}, ${dot.state} scene` }),
        )
        expect(preview).toHaveAttribute('data-detent', 'full')

        // The sheet stays mounted across a swap, so the height the viewer
        // chose holds; only a fresh open starts at Half.
        const other = { ...dot, city: 'Austin', state: 'TX', slug: 'austin-tx' }
        act(() => lastCanvasProps.onSelect?.(other))
        const swapped = screen.getByRole('region', { name: 'Austin, TX scene' })
        expect(swapped).toBe(preview)
        expect(swapped).toHaveAttribute('data-detent', 'full')
      })

      it('opens Drift’s scene preview as a sheet at Half', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        fireEvent.click(screen.getByRole('button', { name: /drift to a random scene/i }))

        const preview = screen.getByRole('region', { name: /Chicago, IL scene/ })
        expect(preview).toHaveAttribute('data-slot', 'bottom-sheet')
        expect(preview).toHaveAttribute('data-detent', 'half')
        expect(
          screen.queryByRole('complementary', { name: /Chicago, IL scene/ }),
        ).not.toBeInTheDocument()

        await userEvent.keyboard('{Escape}')
        expect(
          screen.queryByRole('region', { name: /Chicago, IL scene/ }),
        ).not.toBeInTheDocument()
      })

      it('states no venue count at Peek while the list loads or after it fails', async () => {
        mockUseVenues.mockReturnValue({
          data: undefined,
          isFetching: true,
          isPlaceholderData: false,
        })
        const { unmount } = renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        let sheet = screen.getByTestId('atlas-venue-sheet')
        expect(within(sheet).getByRole('heading', { name: 'Chicago' })).toBeInTheDocument()
        expect(screen.getByTestId('venue-sheet-peek-line')).toHaveTextContent('Loading venues…')
        unmount()

        mockUseVenues.mockReturnValue({
          data: undefined,
          isFetching: false,
          isPlaceholderData: false,
          isError: true,
        })
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        sheet = screen.getByTestId('atlas-venue-sheet')
        expect(within(sheet).getByRole('heading', { name: 'Chicago' })).toBeInTheDocument()
        expect(screen.getByTestId('venue-sheet-peek-line')).toHaveTextContent(
          'Couldn’t load venues here.',
        )
      })

      it('drops a stack scope a filter thins out, for good', async () => {
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        act(() => lastCanvasProps.onVenueStackSelect?.(lastCanvasProps.venueStacks![0].key))
        expect(screen.getByTestId('venue-sheet-scope-line')).toBeInTheDocument()

        // Only Empty Bottle has shows this week, so the stack falls to one pin.
        const sheet = screen.getByTestId('atlas-venue-sheet')
        fireEvent.click(within(sheet).getByRole('button', { name: 'Next 7 days' }))
        expect(screen.queryByTestId('venue-sheet-scope-line')).not.toBeInTheDocument()
        fireEvent.click(within(sheet).getByRole('button', { name: 'Next 7 days' }))
        expect(screen.queryByTestId('venue-sheet-scope-line')).not.toBeInTheDocument()
        expect(within(sheet).getByRole('button', { name: /Hideout/ })).toBeInTheDocument()
      })

      it('lets Escape close the venue sheet after the pane narrows past 900px', async () => {
        setMockContainerWidth(1000)
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()
        act(() => lastCanvasProps.onVenueSelect?.(1))
        expect(screen.getByTestId('atlas-venue-panel')).not.toHaveAttribute('data-slot')

        // The list sheet and the re-keyed venue sheet mount in one commit.
        setMockContainerWidth(800)
        act(() => reportResize())
        expect(screen.getByTestId('atlas-venue-panel')).toHaveAttribute(
          'data-slot',
          'bottom-sheet',
        )
        await userEvent.keyboard('{Escape}')
        expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()
        expect(screen.getByTestId('atlas-venue-sheet')).toHaveAttribute('data-detent', 'peek')
      })

      it('keeps the side-panel layout, unchanged, at 900px and up', async () => {
        setMockContainerWidth(900)
        renderWithProviders(<AtlasGlobe />)
        await screen.findByTestId('globe-canvas')
        settleOnChicago()

        expect(screen.getByTestId('atlas-venue-rail')).toBeInTheDocument()
        expect(screen.queryByTestId('atlas-venue-sheet')).not.toBeInTheDocument()
        expect(lastCanvasProps.attributionPosition).toBe('bottom-left')
        expect(lastCanvasProps.onBackToGlobe).toBeUndefined()
        expect(lastCanvasProps.venueStacks).toBeUndefined()
        act(() => lastCanvasProps.onVenueSelect?.(1))
        expect(screen.getByTestId('atlas-venue-panel')).not.toHaveAttribute(
          'data-slot',
          'bottom-sheet',
        )
      })
    })
  })
})
