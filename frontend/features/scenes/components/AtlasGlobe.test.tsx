import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListResponse } from '../types'

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
})
