import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListResponse } from '../types'

// A canvas module that fails to load (a deploy rotated its hashed chunk, a
// network drop) must leave the visitor on the scene list, not on the map's
// skeleton or the app's error page.
//
// Vitest resolves `next/dynamic` to the Pages Router loader, which hands a
// failed import to `loading` instead of throwing. The bundlers resolve it to
// the App Router loader for app code, so this file mocks `next/dynamic` with
// that loader, and the canvas module's import rejects.
vi.mock('next/dynamic', async () => ({
  default: (await import('next/dist/shared/lib/app-dynamic')).default,
}))
vi.mock('./GlobeCanvas', () =>
  Promise.reject(new TypeError('Failed to fetch dynamically imported module')),
)

const mockUseScenes = vi.fn()
vi.mock('../hooks', () => ({
  useScenes: () => mockUseScenes(),
  useSceneDetail: () => ({ data: undefined }),
}))
vi.mock('@/features/venues/hooks', () => ({
  useVenues: () => ({ data: undefined, isFetching: false, isPlaceholderData: false }),
}))
vi.mock('@/lib/hooks/common/useFollow', () => ({
  useMyFollowing: () => ({ data: undefined }),
}))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/atlas',
  useSearchParams: () => new URLSearchParams(),
}))
vi.mock('./atlasMapPreload', () => ({ preloadAtlasMap: () => {} }))

// jsdom has no WebGL, so the real probe would send every case to the list
// before any canvas is tried.
let mockMapFailedThisPage = false
vi.mock('../atlasViewport', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../atlasViewport')>()),
  atlasSupportsWebGL2: () => true,
  atlasMapFailedThisPage: () => mockMapFailedThisPage,
  markAtlasMapFailed: () => {
    mockMapFailedThisPage = true
  },
}))

import { AtlasGlobe } from './AtlasGlobe'

const PANE_WIDTH_PX = 390

class ImmediateResizeObserver {
  constructor(private callback: ResizeObserverCallback) {}
  observe(target: Element): void {
    this.callback(
      [
        {
          target,
          contentRect: { width: PANE_WIDTH_PX, height: 800 } as DOMRectReadOnly,
        } as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    )
  }
  unobserve(): void {}
  disconnect(): void {}
}

const scenes: SceneListResponse = {
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
  ],
  count: 1,
}

describe('AtlasGlobe when the canvas module fails to load', () => {
  const originalResizeObserver = window.ResizeObserver

  beforeEach(() => {
    mockMapFailedThisPage = false
    window.ResizeObserver = ImmediateResizeObserver as unknown as typeof ResizeObserver
    // The geo-centering fetch is non-fatal; a miss opens on the default focus.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
    mockUseScenes.mockReturnValue({ data: scenes, isLoading: false, isError: false })
  })

  afterEach(() => {
    window.ResizeObserver = originalResizeObserver
    vi.unstubAllGlobals()
  })

  it('swaps in the scene list for this mount, without latching the page', async () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    renderWithProviders(<AtlasGlobe />)
    await waitFor(() =>
      expect(screen.getByTestId('atlas-scene-list')).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: /Chicago, IL/ })).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    // A chunk failure is not a refused WebGL2 context: a later mount may try
    // the map again.
    expect(mockMapFailedThisPage).toBe(false)
    quiet.mockRestore()
  })
})
