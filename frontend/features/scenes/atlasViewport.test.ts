import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  ATLAS_PHONE_PANE_BELOW_PX,
  ATLAS_REDUCED_MOTION_LIST_BELOW_PX,
  atlasMapRetryCanSucceed,
  atlasSceneListReason,
  isAtlasPhonePane,
  probeWebGL2,
} from './atlasViewport'

describe('isAtlasPhonePane', () => {
  it('is the phone Atlas exactly below 900px of pane, where the rail stops fitting', () => {
    expect(ATLAS_PHONE_PANE_BELOW_PX).toBe(900)
    expect(isAtlasPhonePane(390)).toBe(true)
    expect(isAtlasPhonePane(899)).toBe(true)
    expect(isAtlasPhonePane(899.5)).toBe(true)
    expect(isAtlasPhonePane(900)).toBe(false)
  })

  it('is the desktop Atlas on 1023 and 1024px panes alike', () => {
    expect(isAtlasPhonePane(1023)).toBe(false)
    expect(isAtlasPhonePane(1024)).toBe(false)
  })

  it('stays separate from the reduced-motion list threshold, which sits below it', () => {
    expect(ATLAS_REDUCED_MOTION_LIST_BELOW_PX).toBeLessThan(ATLAS_PHONE_PANE_BELOW_PX)
  })
})

describe('atlasSceneListReason', () => {
  const below = ATLAS_REDUCED_MOTION_LIST_BELOW_PX - 1
  const at = ATLAS_REDUCED_MOTION_LIST_BELOW_PX

  it('gives everyone with WebGL2 and no motion preference the map, phones included', () => {
    for (const paneWidthPx of [320, 360, 390, below, at, 1440]) {
      expect(
        atlasSceneListReason({ paneWidthPx, supportsWebGL2: true, prefersReducedMotion: false, mapFailed: false }),
      ).toBeNull()
    }
  })

  it('lists the scenes without WebGL2 at every width', () => {
    for (const paneWidthPx of [390, below, at, 1440]) {
      for (const prefersReducedMotion of [false, true]) {
        expect(
          atlasSceneListReason({ paneWidthPx, supportsWebGL2: false, prefersReducedMotion, mapFailed: false }),
        ).toBe('no-webgl2')
      }
    }
  })

  it.each([
    [390, 'reduced-motion'],
    [below, 'reduced-motion'],
    [at, null],
    [1440, null],
  ])('for reduced motion on a %ipx pane, gives %s', (paneWidthPx, reason) => {
    expect(
      atlasSceneListReason({
        paneWidthPx,
        supportsWebGL2: true,
        prefersReducedMotion: true,
        mapFailed: false,
      }),
    ).toBe(reason)
  })
})

describe('atlasSceneListReason over every combination of its inputs', () => {
  const below = ATLAS_REDUCED_MOTION_LIST_BELOW_PX - 1
  const at = ATLAS_REDUCED_MOTION_LIST_BELOW_PX

  // Written out row by row rather than derived, so the table states the rule
  // instead of restating the implementation.
  it.each([
    // supportsWebGL2, prefersReducedMotion, mapFailed, paneWidthPx, reason
    [true, false, false, below, null],
    [true, false, false, at, null],
    [true, true, false, below, 'reduced-motion'],
    [true, true, false, at, null],
    [true, false, true, below, 'map-failed'],
    [true, false, true, at, 'map-failed'],
    [true, true, true, below, 'reduced-motion'],
    [true, true, true, at, 'map-failed'],
    [false, false, false, below, 'no-webgl2'],
    [false, false, false, at, 'no-webgl2'],
    [false, true, false, below, 'no-webgl2'],
    [false, true, false, at, 'no-webgl2'],
    [false, false, true, below, 'no-webgl2'],
    [false, false, true, at, 'no-webgl2'],
    [false, true, true, below, 'no-webgl2'],
    [false, true, true, at, 'no-webgl2'],
  ])(
    'WebGL2 %s, reduced motion %s, failed map %s, %ipx pane: %s',
    (supportsWebGL2, prefersReducedMotion, mapFailed, paneWidthPx, reason) => {
      expect(
        atlasSceneListReason({ paneWidthPx, supportsWebGL2, prefersReducedMotion, mapFailed }),
      ).toBe(reason)
    },
  )
})

describe('a failed map', () => {
  it('lists the scenes at every width when it is the only reason', () => {
    for (const paneWidthPx of [390, 1440]) {
      expect(
        atlasSceneListReason({
          paneWidthPx,
          supportsWebGL2: true,
          prefersReducedMotion: false,
          mapFailed: true,
        }),
      ).toBe('map-failed')
    }
  })

  it('latches a failure and its class for the page load, until a retry clears it', async () => {
    vi.resetModules()
    const { atlasMapFailureThisPage, clearAtlasMapFailure, markAtlasMapFailed } =
      await import('./atlasViewport')
    expect(atlasMapFailureThisPage()).toBeNull()
    markAtlasMapFailed('context-lost')
    expect(atlasMapFailureThisPage()).toBe('context-lost')
    clearAtlasMapFailure()
    expect(atlasMapFailureThisPage()).toBeNull()
  })
})

describe('atlasMapRetryCanSucceed', () => {
  it.each([
    ['context-lost', true],
    ['context-lost-during-restore', true],
    ['context-lost-before-style', true],
    ['context-restore-refused', true],
    ['context-refused', false],
    ['style-load-failed', false],
    ['first-render-timeout', true],
  ] as const)('after %s: %s', (failureClass, retry) => {
    expect(atlasMapRetryCanSucceed(failureClass)).toBe(retry)
  })

  it('offers no retry for a failure that carries no class', () => {
    expect(atlasMapRetryCanSucceed(null)).toBe(false)
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
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('answers false without a document, and keeps no answer for the client', async () => {
    vi.resetModules()
    const { atlasSupportsWebGL2 } = await import('./atlasViewport')
    const realDocument = document
    vi.stubGlobal('document', undefined)
    expect(atlasSupportsWebGL2()).toBe(false)
    vi.stubGlobal('document', realDocument)
    const loseContext = vi.fn()
    const getContext = vi.fn(() => ({ getExtension: () => ({ loseContext }) }))
    const realCreateElement = realDocument.createElement.bind(realDocument)
    vi.spyOn(realDocument, 'createElement').mockImplementation((tag: string) =>
      tag === 'canvas'
        ? ({ getContext } as unknown as HTMLCanvasElement)
        : realCreateElement(tag),
    )
    expect(atlasSupportsWebGL2()).toBe(true)
    expect(getContext).toHaveBeenCalledTimes(1)
  })

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

describe('reprobeAtlasWebGL2', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  function stubCanvasContexts(...answers: (object | null)[]) {
    const getContext = vi.fn()
    for (const answer of answers) getContext.mockReturnValueOnce(answer)
    const realCreateElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) =>
      tag === 'canvas'
        ? ({ getContext } as unknown as HTMLCanvasElement)
        : realCreateElement(tag),
    )
    return getContext
  }

  it('probes a fresh canvas on every call', async () => {
    vi.resetModules()
    const { reprobeAtlasWebGL2 } = await import('./atlasViewport')
    const context = { getExtension: () => null }
    const getContext = stubCanvasContexts(null, context)
    expect(reprobeAtlasWebGL2()).toBe(false)
    expect(reprobeAtlasWebGL2()).toBe(true)
    expect(getContext).toHaveBeenCalledTimes(2)
  })

  it('leaves the answer kept for the page load alone', async () => {
    vi.resetModules()
    const { atlasSupportsWebGL2, reprobeAtlasWebGL2 } = await import('./atlasViewport')
    const context = { getExtension: () => null }
    stubCanvasContexts(context, null)
    expect(atlasSupportsWebGL2()).toBe(true)
    expect(reprobeAtlasWebGL2()).toBe(false)
    expect(atlasSupportsWebGL2()).toBe(true)
  })
})
