import { describe, it, expect, afterEach, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  ATLAS_COMPACT_VIEWPORT_QUERY,
  ATLAS_REDUCED_MOTION_LIST_BELOW_PX,
  atlasRendersSceneList,
  isAtlasCompactViewport,
  probeWebGL2,
  useAtlasCompactViewport,
} from './atlasViewport'

describe('useAtlasCompactViewport', () => {
  let restore: () => void = () => {}
  afterEach(() => restore())

  it('asks for the exact complement of the Tailwind lg breakpoint', () => {
    // `lg:` is `min-width: 64rem`; a branch on this query and an `lg:`
    // utility must flip at the same width.
    expect(ATLAS_COMPACT_VIEWPORT_QUERY).toBe('not all and (min-width: 64rem)')
    const mm = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
    restore = mm.restore
    const { result } = renderHook(() => useAtlasCompactViewport())
    expect(mm.queries).toContain(ATLAS_COMPACT_VIEWPORT_QUERY)
    expect(result.current).toBe(true)
  })

  it('answers the one-shot read from the same query', () => {
    const mm = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false })
    restore = mm.restore
    expect(isAtlasCompactViewport()).toBe(false)
    mm.set(ATLAS_COMPACT_VIEWPORT_QUERY, true)
    expect(isAtlasCompactViewport()).toBe(true)
  })
})

describe('atlasRendersSceneList', () => {
  const below = ATLAS_REDUCED_MOTION_LIST_BELOW_PX - 1
  const at = ATLAS_REDUCED_MOTION_LIST_BELOW_PX

  it('gives everyone with WebGL2 and no motion preference the map, phones included', () => {
    for (const paneWidthPx of [320, 360, 390, below, at, 1440]) {
      expect(
        atlasRendersSceneList({ paneWidthPx, supportsWebGL2: true, prefersReducedMotion: false }),
      ).toBe(false)
    }
  })

  it('lists the scenes without WebGL2 at every width', () => {
    for (const paneWidthPx of [390, below, at, 1440]) {
      for (const prefersReducedMotion of [false, true]) {
        expect(
          atlasRendersSceneList({ paneWidthPx, supportsWebGL2: false, prefersReducedMotion }),
        ).toBe(true)
      }
    }
  })

  it.each([
    [390, true],
    [below, true],
    [at, false],
    [1440, false],
  ])('for reduced motion on a %ipx pane, lists the scenes: %s', (paneWidthPx, list) => {
    expect(
      atlasRendersSceneList({ paneWidthPx, supportsWebGL2: true, prefersReducedMotion: true }),
    ).toBe(list)
  })
})

describe('probeWebGL2', () => {
  function canvasReturning(getContext: (type: string) => unknown) {
    return () => ({ getContext }) as unknown as HTMLCanvasElement
  }

  it('asks for a webgl2 context and releases the one it gets', () => {
    const loseContext = vi.fn()
    const getExtension = vi.fn(() => ({ loseContext }))
    const getContext = vi.fn(() => ({ getExtension }))
    expect(probeWebGL2(canvasReturning(getContext))).toBe(true)
    expect(getContext).toHaveBeenCalledWith('webgl2', expect.objectContaining({ antialias: false }))
    expect(getExtension).toHaveBeenCalledWith('WEBGL_lose_context')
    expect(loseContext).toHaveBeenCalledTimes(1)
  })

  it('still reports support when the context cannot be released early', () => {
    expect(probeWebGL2(canvasReturning(() => ({ getExtension: () => null })))).toBe(true)
  })

  it('reports no support when the browser returns no webgl2 context', () => {
    expect(probeWebGL2(canvasReturning(() => null))).toBe(false)
  })

  it('reports no support when asking throws', () => {
    expect(
      probeWebGL2(
        canvasReturning(() => {
          throw new Error('blocked')
        }),
      ),
    ).toBe(false)
  })
})

describe('atlasSupportsWebGL2', () => {
  afterEach(() => vi.restoreAllMocks())

  it('probes one fresh canvas per page load and keeps the answer', async () => {
    vi.resetModules()
    const { atlasSupportsWebGL2 } = await import('./atlasViewport')
    const getContext = vi.fn(() => null)
    const realCreateElement = document.createElement.bind(document)
    const createElement = vi
      .spyOn(document, 'createElement')
      .mockImplementation((tag: string) =>
        tag === 'canvas'
          ? ({ getContext } as unknown as HTMLCanvasElement)
          : realCreateElement(tag),
      )
    expect(atlasSupportsWebGL2()).toBe(false)
    expect(atlasSupportsWebGL2()).toBe(false)
    expect(createElement).toHaveBeenCalledTimes(1)
    expect(getContext).toHaveBeenCalledTimes(1)
  })
})
