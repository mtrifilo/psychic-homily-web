import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
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
vi.mock('../hooks', () => ({
  useScenes: () => mockUseScenes(),
  useSceneArtists: () => ({ data: undefined, isLoading: false }),
  // The preview panel (opened by the drift tests) reads the scene's this-week
  // shows (PSY-1309); a quiet week is the neutral default here.
  useSceneShows: () => ({ data: { shows: [] }, isLoading: false }),
}))

// AtlasSearch (rendered in the globe branch) reads the router (PSY-1310).
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

// Stub the WebGL canvas for the desktop-branch tests (PSY-1308 Drift): it
// fills the flyToRef seam with a spy so the drift handler's camera call is
// observable without three.js.
const flyToSpy = vi.fn()
vi.mock('./GlobeCanvas', () => ({
  default: ({
    flyToRef,
  }: {
    flyToRef?: MutableRefObject<((scene: PlaceableScene) => void) | null>
  }) => {
    if (flyToRef) flyToRef.current = flyToSpy
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

  it('lists all scenes as links on small screens (mobile gate)', () => {
    mockUseScenes.mockReturnValue({
      data: sampleData,
      isLoading: false,
      isError: false,
    })
    renderWithProviders(<AtlasGlobe />)

    expect(
      screen.getByRole('link', { name: /Chicago, IL/ }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
    // The unplaceable scene still appears in the mobile list.
    expect(
      screen.getByRole('link', { name: /Faketown, ZZ/ }),
    ).toHaveAttribute('href', '/scenes/faketown-zz')
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
})
