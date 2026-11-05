import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity, StrictMode, useEffect } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  SETTLE_INTRO_MS,
  SETTLE_INTRO_PLAYED_KEY,
  SETTLE_RELEASE_MS,
  ScryingGridWordmark,
} from './ScryingGridWordmark'
import { WORDMARK_ONE_LINE, WORDMARK_PRESETS } from './wordmarkPresets'

// The literal query, not an import of it: a drifted query must fail here.
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// jsdom cannot rasterise text, so sampling returns a fixed field of cells.
const sampleSpy = vi.hoisted(() =>
  vi.fn(() => [
    { x: 10, y: 10 },
    { x: 20, y: 10 },
  ]),
)
vi.mock('./sampleWordmark', async importOriginal => ({
  ...(await importOriginal<typeof import('./sampleWordmark')>()),
  sampleWordmark: sampleSpy,
}))

/** The alpha of the last cell drawn: the field's opacity in the last frame
 *  (about 0.86 at rest in light mode; at most 0.12 on the intro's first). */
function lastDrawnAlpha(): number {
  const match = /rgba\([^)]*,\s*([\d.]+)\)$/.exec(FAKE_CTX.fillStyle)
  if (!match) throw new Error(`no rgba fill drawn: ${FAKE_CTX.fillStyle}`)
  return Number(match[1])
}

const FAKE_CTX = {
  clearRect: vi.fn(),
  beginPath: vi.fn(),
  arc: vi.fn(),
  fill: vi.fn(),
  fillStyle: '',
  createImageData: (w: number, h: number) => ({
    data: new Uint8ClampedArray(w * h * 4),
  }),
  putImageData: vi.fn(),
}

describe('ScryingGridWordmark', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>
  let requestFrame: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      FAKE_CTX as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 400,
      height: 200,
      left: 0,
      top: 0,
    } as DOMRect)
    // Frames are counted, never run: a loop that is running asks for one.
    requestFrame = vi
      .spyOn(window, 'requestAnimationFrame')
      .mockImplementation(() => 1)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    matchMedia.restore()
    vi.restoreAllMocks()
  })

  it('draws the field once, with no animation loop, while the visitor asks for less motion', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    render(<ScryingGridWordmark />)
    expect(requestFrame).not.toHaveBeenCalled()
    expect(FAKE_CTX.arc).toHaveBeenCalled()
  })

  it('starts no animation loop while hydrating for a visitor who asks for less motion', async () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const { renderToString } = await import('react-dom/server')
    const container = document.createElement('div')
    container.innerHTML = renderToString(<ScryingGridWordmark />)
    document.body.appendChild(container)

    // The hydration pass reads the hook's server value (false); the field
    // must still come up static.
    render(<ScryingGridWordmark />, { container, hydrate: true })

    expect(requestFrame).not.toHaveBeenCalled()
    expect(FAKE_CTX.arc).toHaveBeenCalled()
  })

  it('goes static when reduced motion turns on mid-session, and animates again when it turns off', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    render(<ScryingGridWordmark />)
    expect(requestFrame).toHaveBeenCalledTimes(1)

    requestFrame.mockClear()
    FAKE_CTX.arc.mockClear()
    matchMedia.set(REDUCED_MOTION_QUERY, true)
    expect(requestFrame).not.toHaveBeenCalled()
    // The static field is drawn in place of the stopped loop.
    expect(FAKE_CTX.arc).toHaveBeenCalled()

    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(requestFrame).toHaveBeenCalledTimes(1)
  })
})

/**
 * A frame clock under the test's control: requestAnimationFrame queues, and
 * `advance` runs the queue in 16ms steps with the step's timestamp, the way a
 * display would.
 */
function installFrameClock() {
  let now = 1000
  let nextId = 1
  const queue = new Map<number, FrameRequestCallback>()
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    const id = nextId++
    queue.set(id, callback)
    return id
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => {
    queue.delete(id)
  })
  return {
    advance(ms: number) {
      const end = now + ms
      while (now < end) {
        now += 16
        const due = [...queue.values()]
        queue.clear()
        act(() => {
          for (const callback of due) callback(now)
        })
      }
    },
    get pending() {
      return queue.size
    },
  }
}

/** An IntersectionObserver whose answers the test gives. */
function installIntersection() {
  const callbacks: IntersectionObserverCallback[] = []
  const original = window.IntersectionObserver
  window.IntersectionObserver = class {
    constructor(callback: IntersectionObserverCallback) {
      callbacks.push(callback)
    }
    observe() {}
    unobserve() {}
    disconnect() {}
    takeRecords() {
      return []
    }
  } as unknown as typeof IntersectionObserver
  return {
    set(isIntersecting: boolean) {
      act(() => {
        for (const callback of callbacks) {
          callback(
            [{ isIntersecting } as IntersectionObserverEntry],
            {} as IntersectionObserver,
          )
        }
      })
    },
    restore() {
      window.IntersectionObserver = original
    },
  }
}

function setDocumentHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    get: () => hidden,
  })
  act(() => {
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

describe('ScryingGridWordmark props', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>

  beforeEach(() => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      FAKE_CTX as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 306,
      height: 30,
      left: 0,
      top: 0,
    } as DOMRect)
    sampleSpy.mockClear()
  })

  afterEach(() => {
    matchMedia.restore()
    vi.restoreAllMocks()
  })

  it('keeps the hero markup by default: an h1 named Psychic Homily over two lines', () => {
    const { container } = render(<ScryingGridWordmark headingId="hero" />)
    const heading = screen.getByRole('heading', { level: 1, name: 'Psychic Homily' })
    expect(heading).toHaveAttribute('id', 'hero')
    expect(container.querySelector('[data-hero-fallback]')?.innerHTML).toBe('PSYCHIC<br>HOMILY')
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(sampleSpy).toHaveBeenCalledWith(
      expect.any(Number),
      expect.any(Number),
      expect.objectContaining({
        lines: ['PSYCHIC', 'HOMILY'],
        padScale: 1,
        fillInk: false,
        fontWeight: undefined,
      }),
    )
  })

  it('renders no heading without `heading`, and names the field as an image', () => {
    render(<ScryingGridWordmark heading={false} />)
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Psychic Homily' })).toBeInTheDocument()
  })

  it('samples, and falls back to, the lines it is given at the preset layout', () => {
    const { container } = render(
      <ScryingGridWordmark heading={false} lines={WORDMARK_ONE_LINE} preset="bar" />,
    )
    expect(container.querySelector('[data-hero-fallback]')?.textContent).toBe('PSYCHIC HOMILY')
    // The container takes the preset's box.
    expect(screen.getByRole('img')).toHaveClass('h-[30px]', 'w-[306px]')
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    expect(sampleSpy).toHaveBeenCalledWith(
      expect.any(Number),
      expect.any(Number),
      expect.objectContaining({
        lines: ['PSYCHIC HOMILY'],
        gapDev: Math.round(WORDMARK_PRESETS.bar.gapFactor * dpr),
        fontWeight: 900,
        fillInk: true,
      }),
    )
  })

  it('waits for a box instead of failing when it mounts with no size (display:none)', () => {
    vi.mocked(HTMLElement.prototype.getBoundingClientRect).mockReturnValue({
      width: 0,
      height: 0,
      left: 0,
      top: 0,
    } as DOMRect)
    // With a heading, the fallback shows only on failure: waiting is not one.
    const { container } = render(<ScryingGridWordmark preset="nameplate" />)

    expect(sampleSpy).not.toHaveBeenCalled()
    expect(container.querySelector('[data-hero-fallback]')).toHaveClass('opacity-0')
  })

  it('paints the text fallback in the server HTML when there is no heading, and keeps it hidden with one', async () => {
    const { renderToString } = await import('react-dom/server')
    const host = document.createElement('div')

    host.innerHTML = renderToString(<ScryingGridWordmark heading={false} preset="bar" />)
    expect(host.querySelector('[data-hero-fallback]')).toHaveClass('opacity-100')
    // Sized to the box it is in, so it reads at the mark's cap height.
    expect(host.querySelector('[role="img"]')).toHaveClass('[container-type:size]')

    host.innerHTML = renderToString(<ScryingGridWordmark />)
    expect(host.querySelector('[data-hero-fallback]')).toHaveClass('opacity-0')
  })

  it('hands over from the text fallback to the canvas once the field is drawn', () => {
    const { container } = render(<ScryingGridWordmark heading={false} preset="bar" />)

    // Reduced motion here: begin() drew the static field and enhanced.
    expect(FAKE_CTX.arc).toHaveBeenCalled()
    expect(container.querySelector('[data-hero-fallback]')).toHaveClass('opacity-0')
    expect(container.querySelector('canvas')).toHaveClass('opacity-100')
  })

  it('does not rebuild the field when a re-render passes an equal lines array', () => {
    const { rerender } = render(<ScryingGridWordmark lines={['PSYCHIC HOMILY']} />)
    expect(sampleSpy).toHaveBeenCalledTimes(1)
    rerender(<ScryingGridWordmark lines={['PSYCHIC HOMILY']} />)
    expect(sampleSpy).toHaveBeenCalledTimes(1)
  })
})

describe("ScryingGridWordmark motion='settle'", () => {
  let matchMedia: ReturnType<typeof installMatchMedia>
  let clock: ReturnType<typeof installFrameClock>
  let intersection: ReturnType<typeof installIntersection>

  beforeEach(() => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    intersection = installIntersection()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      FAKE_CTX as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 1084,
      height: 100,
      left: 0,
      top: 0,
    } as DOMRect)
    clock = installFrameClock()
    FAKE_CTX.clearRect.mockClear()
    FAKE_CTX.arc.mockClear()
    // Each test is a fresh tab: no intro has played yet.
    window.sessionStorage.clear()
    FAKE_CTX.fillStyle = ''
  })

  afterEach(() => {
    matchMedia.restore()
    intersection.restore()
    setDocumentHidden(false)
    vi.restoreAllMocks()
  })

  it('plays one intro, then stops the loop on the resting frame', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    expect(clock.pending).toBe(1)

    clock.advance(SETTLE_INTRO_MS + 100)
    expect(clock.pending).toBe(0)

    // Nothing restarts on its own.
    const drawn = FAKE_CTX.clearRect.mock.calls.length
    clock.advance(5000)
    expect(FAKE_CTX.clearRect.mock.calls.length).toBe(drawn)
  })

  it('keeps the rest-mode loop running, as the hero always has', () => {
    render(<ScryingGridWordmark />)
    clock.advance(SETTLE_INTRO_MS * 3)
    expect(clock.pending).toBe(1)
  })

  it('ignites while the pointer is over the field and stops again within a second of it leaving', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(SETTLE_INTRO_MS + 100)
    expect(clock.pending).toBe(0)

    const field = screen.getByRole('img')
    fireEvent.pointerMove(field, { clientX: 40, clientY: 50 })
    expect(clock.pending).toBe(1)
    clock.advance(3000)
    // Still hovering: the loop stays up.
    expect(clock.pending).toBe(1)

    fireEvent.pointerLeave(field)
    clock.advance(SETTLE_RELEASE_MS + 50)
    expect(clock.pending).toBe(0)
  })

  it('ignites on a tap, which fires no move', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(SETTLE_INTRO_MS + 100)
    fireEvent.pointerDown(screen.getByRole('img'), { clientX: 40, clientY: 50 })
    expect(clock.pending).toBe(1)
  })

  it('pauses the intro off-screen and resumes it where it stopped', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(SETTLE_INTRO_MS / 2)
    intersection.set(false)
    expect(clock.pending).toBe(0)

    // Time passes with no frames: none of it is billed to the intro.
    clock.advance(10_000)
    intersection.set(true)
    expect(clock.pending).toBe(1)
    // Half the intro was left: neither none of it (billed) nor all of it
    // (restarted).
    clock.advance(SETTLE_INTRO_MS / 4)
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS / 4 + 100)
    expect(clock.pending).toBe(0)
  })

  it('pauses in a hidden tab and resumes where it stopped when the tab is shown', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(SETTLE_INTRO_MS / 2)
    setDocumentHidden(true)
    expect(clock.pending).toBe(0)

    clock.advance(10_000)
    setDocumentHidden(false)
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS / 4)
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS / 4 + 100)
    expect(clock.pending).toBe(0)
  })

  it('reveals the text when a field that mounted with no box samples to nothing', () => {
    const rect = vi.mocked(HTMLElement.prototype.getBoundingClientRect)
    rect.mockReturnValue({ width: 0, height: 0, left: 0, top: 0 } as DOMRect)
    const resize = installResizeObserver()
    try {
      // With a heading, so the fallback's reveal is the failure path alone.
      const { container } = render(<ScryingGridWordmark motion="settle" preset="nameplate" />)
      const fallback = container.querySelector('[data-hero-fallback]')
      expect(fallback).toHaveClass('opacity-0')

      // The breakpoint is crossed and the sample comes back empty.
      rect.mockReturnValue({ width: 1084, height: 100, left: 0, top: 0 } as DOMRect)
      sampleSpy.mockReturnValueOnce([])
      resize.report()
      clock.advance(16)

      expect(fallback).toHaveClass('opacity-100')
    } finally {
      resize.restore()
    }
  })

  it('plays the intro once per session: a later arrival paints the resting frame at once', () => {
    const first = render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(16)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).not.toBeNull()
    first.unmount()

    FAKE_CTX.arc.mockClear()
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    // No loop: the resting frame is already drawn, and nothing rises.
    expect(clock.pending).toBe(0)
    expect(FAKE_CTX.arc).toHaveBeenCalled()
  })

  it('starts on the resting frame when a mark first gets a box after its text was painted', () => {
    const rect = vi.mocked(HTMLElement.prototype.getBoundingClientRect)
    rect.mockReturnValue({ width: 0, height: 0, left: 0, top: 0 } as DOMRect)
    const resize = installResizeObserver()
    try {
      // The other breakpoint's mark: mounted with no box.
      render(<ScryingGridWordmark motion="settle" heading={false} preset="nameplate" />)
      expect(clock.pending).toBe(0)

      // The breakpoint is crossed: the text shows a frame before the field.
      rect.mockReturnValue({ width: 1084, height: 100, left: 0, top: 0 } as DOMRect)
      resize.report()
      clock.advance(16)

      expect(FAKE_CTX.arc).toHaveBeenCalled()
      expect(clock.pending).toBe(0)
      expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
      expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).toBeNull()
    } finally {
      resize.restore()
    }
  })

  it('still plays the intro after a mount that was torn down before its first frame', () => {
    // StrictMode mounts, tears down and mounts again before any frame runs.
    render(
      <StrictMode>
        <ScryingGridWordmark motion="settle" heading={false} />
      </StrictMode>,
    )
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).toBeNull()
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS / 2)
    expect(clock.pending).toBe(1)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).not.toBeNull()
  })

  it('keeps a running field when a resize leaves its size unchanged', () => {
    const resize = installResizeObserver()
    try {
      render(<ScryingGridWordmark motion="settle" heading={false} />)
      sampleSpy.mockClear()
      // The observer's first report, at the size the field was built for.
      resize.report()
      clock.advance(16)
      // Re-sampling would reassign the canvas size, which blanks its bitmap.
      expect(sampleSpy).not.toHaveBeenCalled()
      expect(clock.pending).toBe(1)
    } finally {
      resize.restore()
    }
  })

  it('draws the resting frame once, with no intro and no pointer reaction, under reduced motion', () => {
    matchMedia.set(REDUCED_MOTION_QUERY, true)
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    expect(clock.pending).toBe(0)
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    fireEvent.pointerMove(screen.getByRole('img'), { clientX: 40, clientY: 50 })
    expect(clock.pending).toBe(0)
  })

  it('does not dim a shown field into the intro when reduced motion turns off', async () => {
    matchMedia.set(REDUCED_MOTION_QUERY, true)
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    // The field has been on screen a while.
    await act(async () => {})

    // The effect re-runs in place: the canvas is not newly appearing.
    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    expect(clock.pending).toBe(0)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).toBeNull()
  })
})

/** A FontFaceSet whose readiness the test gives: `check` answers `loaded`;
 *  `ready` settles on `load`, and a `load()` request on `loadFace` (or
 *  `load`), so a test can tell which one the code waited on. */
function installFonts({ loaded }: { loaded: boolean }) {
  let settleReady: () => void = () => {}
  const ready = new Promise<void>(resolve => {
    settleReady = resolve
  })
  let settleFace: () => void = () => {}
  const face = new Promise<void>(resolve => {
    settleFace = resolve
  })
  const check = vi.fn(() => loaded)
  const loadFace = vi.fn(() => face)
  Object.defineProperty(document, 'fonts', {
    configurable: true,
    value: { check, ready, load: loadFace },
  })
  return {
    check,
    loadFace,
    /** Settles the face request alone; `ready` stays pending. */
    async faceLoaded() {
      await act(async () => {
        settleFace()
        await face
      })
    },
    async load() {
      await act(async () => {
        settleFace()
        settleReady()
        await ready
      })
    },
    restore() {
      delete (document as { fonts?: unknown }).fonts
    },
  }
}

/** Calls `onRun` from a passive effect. Rendered BEFORE the mark: React runs
 *  a commit's passive effects in tree order after all its layout effects, so
 *  this one runs after the mark's layout effect and before the mark's own
 *  passive effect. A draw not done by then is a passive-effect draw, painted
 *  a frame after the mount. */
function PassiveEffect({ onRun }: { onRun: () => void }) {
  useEffect(onRun, [onRun])
  return null
}

/** Whether the field had drawn when the first passive effect ran. */
function drawnByFirstPassiveEffect() {
  const seen: { drawn?: boolean } = {}
  const onRun = () => {
    seen.drawn ??= FAKE_CTX.arc.mock.calls.length > 0
  }
  return { seen, onRun }
}

/** A ResizeObserver whose reports the test sends, to the observers that are
 *  observing something, as a real one reports only observed targets. */
function installResizeObserver() {
  const observing = new Set<ResizeObserverCallback>()
  const original = window.ResizeObserver
  window.ResizeObserver = class {
    constructor(private readonly callback: ResizeObserverCallback) {}
    observe() {
      observing.add(this.callback)
    }
    unobserve() {
      observing.delete(this.callback)
    }
    disconnect() {
      observing.delete(this.callback)
    }
  } as unknown as typeof ResizeObserver
  return {
    report() {
      act(() => {
        for (const callback of observing) callback([], {} as ResizeObserver)
      })
    },
    restore() {
      window.ResizeObserver = original
    },
  }
}

describe('ScryingGridWordmark first paint', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>
  let clock: ReturnType<typeof installFrameClock>
  let fonts: ReturnType<typeof installFonts> | null = null

  beforeEach(() => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      FAKE_CTX as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 306,
      height: 30,
      left: 0,
      top: 0,
    } as DOMRect)
    clock = installFrameClock()
    FAKE_CTX.arc.mockClear()
    sampleSpy.mockClear()
    window.sessionStorage.clear()
    FAKE_CTX.fillStyle = ''
  })

  afterEach(() => {
    fonts?.restore()
    fonts = null
    matchMedia.restore()
    vi.restoreAllMocks()
  })

  it('draws the dots before the first paint and never shows the text when the font is already loaded', () => {
    fonts = installFonts({ loaded: true })
    const { seen, onRun } = drawnByFirstPassiveEffect()
    const { container } = render(
      <>
        <PassiveEffect onRun={onRun} />
        <ScryingGridWordmark preset="bar" lines={WORDMARK_ONE_LINE} heading={false} motion="settle" />
      </>,
    )

    expect(seen.drawn).toBe(true)
    // The mount task ends handed over (the cold-load test below ends it
    // still on the text): the browser's first paint is the dots.
    const fallback = container.querySelector('[data-hero-fallback]')
    const canvas = container.querySelector('canvas')
    expect(fallback).toHaveClass('opacity-0')
    expect(canvas).toHaveClass('opacity-100')
    // No cross-fade: a transition would animate from the text it replaced.
    expect(fallback).not.toHaveClass('transition-opacity')
    expect(canvas).not.toHaveClass('transition-opacity')
    // The face checked is the one sampled: the preset's weight, the mark's text.
    expect(fonts.check).toHaveBeenCalledWith(expect.stringMatching(/^900 100px /), 'PSYCHIC HOMILY')
    // The intro still runs on this first arrival of the session.
    expect(clock.pending).toBe(1)
  })

  it('keeps the text until the font is ready on a cold load, then cross-fades', async () => {
    fonts = installFonts({ loaded: false })
    const resize = installResizeObserver()
    try {
      const { container } = render(<ScryingGridWordmark preset="bar" heading={false} motion="settle" />)
      const fallback = container.querySelector('[data-hero-fallback]')
      // A resize before the font is ready samples nothing: it would rasterise
      // fallback glyphs.
      resize.report()
      clock.advance(16)
      expect(sampleSpy).not.toHaveBeenCalled()
      expect(fallback).toHaveClass('opacity-100')

      // It waits on the sampled face itself, not on fonts.ready.
      expect(fonts.loadFace).toHaveBeenCalledWith(expect.stringMatching(/^900 100px /), 'PSYCHIC HOMILY')
      await fonts.faceLoaded()
      expect(sampleSpy).toHaveBeenCalled()
      expect(fallback).toHaveClass('opacity-0', 'transition-opacity')
      expect(container.querySelector('canvas')).toHaveClass('opacity-100', 'transition-opacity')
      // The text was painted first: the field cross-fades in at rest.
      expect(clock.pending).toBe(0)
      expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    } finally {
      resize.restore()
    }
  })

  it('hides the server-painted text at hydration when the font is already loaded', async () => {
    fonts = installFonts({ loaded: true })
    const { renderToString } = await import('react-dom/server')
    const container = document.createElement('div')
    container.innerHTML = renderToString(
      <>
        <PassiveEffect onRun={() => {}} />
        <ScryingGridWordmark preset="bar" heading={false} motion="settle" />
      </>,
    )
    document.body.appendChild(container)
    expect(container.querySelector('[data-hero-fallback]')).toHaveClass('opacity-100')

    const { seen, onRun } = drawnByFirstPassiveEffect()
    render(
      <>
        <PassiveEffect onRun={onRun} />
        <ScryingGridWordmark preset="bar" heading={false} motion="settle" />
      </>,
      { container, hydrate: true },
    )

    expect(seen.drawn).toBe(true)
    const fallback = container.querySelector('[data-hero-fallback]')
    expect(fallback).toHaveClass('opacity-0')
    expect(fallback).not.toHaveClass('transition-opacity')
  })

  it('replaces server-painted text with the resting frame, and leaves the intro for the first client-side arrival', async () => {
    fonts = installFonts({ loaded: true })
    const { renderToString } = await import('react-dom/server')
    const container = document.createElement('div')
    container.innerHTML = renderToString(<ScryingGridWordmark preset="bar" heading={false} motion="settle" />)
    document.body.appendChild(container)

    const hydrated = render(<ScryingGridWordmark preset="bar" heading={false} motion="settle" />, {
      container,
      hydrate: true,
    })
    // No rise from faint cells over the text it replaced: the resting frame,
    // no loop, and the session's intro still unplayed.
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    expect(clock.pending).toBe(0)
    clock.advance(SETTLE_INTRO_MS)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).toBeNull()
    hydrated.unmount()

    // Home -> Shows -> Home: the canvas is the first thing the cell shows.
    render(<ScryingGridWordmark preset="bar" heading={false} motion="settle" />)
    expect(lastDrawnAlpha()).toBeLessThanOrEqual(0.12)
    expect(clock.pending).toBe(1)
    clock.advance(16)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).not.toBeNull()
  })

  it('keeps the resting frame over server-painted text under StrictMode\'s double effect', async () => {
    fonts = installFonts({ loaded: true })
    const { renderToString } = await import('react-dom/server')
    const ui = (
      <StrictMode>
        <ScryingGridWordmark preset="bar" heading={false} motion="settle" />
      </StrictMode>
    )
    const container = document.createElement('div')
    container.innerHTML = renderToString(ui)
    document.body.appendChild(container)

    render(ui, { container, hydrate: true })
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    expect(clock.pending).toBe(0)
  })

  it('plays the intro when a hydrated mark is shown again after being hidden (an Activity re-show)', async () => {
    fonts = installFonts({ loaded: true })
    const { renderToString } = await import('react-dom/server')
    const ui = (mode: 'visible' | 'hidden') => (
      <Activity mode={mode}>
        <ScryingGridWordmark preset="nameplate" heading={false} motion="settle" />
      </Activity>
    )
    const container = document.createElement('div')
    container.innerHTML = renderToString(ui('visible'))
    document.body.appendChild(container)

    const view = render(ui('visible'), { container, hydrate: true })
    expect(lastDrawnAlpha()).toBeGreaterThan(0.5)
    expect(clock.pending).toBe(0)
    await act(async () => {})

    // Home -> Shows: the page is kept, hidden. Shows -> Home: shown again,
    // the canvas first in the cell.
    view.rerender(ui('hidden'))
    await act(async () => {})
    view.rerender(ui('visible'))

    expect(lastDrawnAlpha()).toBeLessThanOrEqual(0.12)
    expect(clock.pending).toBe(1)
    clock.advance(16)
    expect(window.sessionStorage.getItem(SETTLE_INTRO_PLAYED_KEY)).not.toBeNull()
  })

  it('keeps the hero\'s early field: a resize before the font builds it, and fonts.ready resamples it', async () => {
    fonts = installFonts({ loaded: false })
    const resize = installResizeObserver()
    try {
      const { container } = render(<ScryingGridWordmark />)
      resize.report()
      clock.advance(16)
      expect(sampleSpy).toHaveBeenCalledTimes(1)
      expect(container.querySelector('canvas')).toHaveClass('opacity-100', 'transition-opacity')

      await fonts.load()
      expect(sampleSpy).toHaveBeenCalledTimes(2)
    } finally {
      resize.restore()
    }
  })

  it('leaves the hero as it was: it waits for fonts.ready and fades in, even with the font loaded', async () => {
    fonts = installFonts({ loaded: true })
    const { container } = render(<ScryingGridWordmark />)
    expect(sampleSpy).not.toHaveBeenCalled()
    expect(container.querySelector('canvas')).toHaveClass('opacity-0')

    await fonts.load()
    expect(sampleSpy).toHaveBeenCalled()
    expect(container.querySelector('canvas')).toHaveClass('opacity-100', 'transition-opacity')
    expect(container.querySelector('[data-hero-fallback]')).toHaveClass('opacity-0')
  })
})
