import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
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
const mockUseVenues = vi.fn<() => Record<string, unknown>>(() => ({
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
  useVenues: () => mockUseVenues(),
  useVenueShows: () => mockUseVenueShows(),
}))

// AtlasSearch (rendered in the globe branch) reads the router (PSY-1310).
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
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

      fireEvent.click(screen.getByRole('button', { name: 'This week' }))

      // One rail row, one pin — the same array feeds both.
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
        lastCanvasProps.onVenueSelect?.(2) // Hideout: 0 shows this week
      })
      expect(screen.getByTestId('atlas-venue-panel')).toBeInTheDocument()

      // Hideout loses its pin and its row, so its panel must go too rather
      // than describe a venue the user can no longer see beside it.
      fireEvent.click(screen.getByRole('button', { name: 'This week' }))
      expect(screen.queryByTestId('atlas-venue-panel')).not.toBeInTheDocument()

      // The selection itself survives — clearing the filter restores it.
      fireEvent.click(screen.getByRole('button', { name: 'This week' }))
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
