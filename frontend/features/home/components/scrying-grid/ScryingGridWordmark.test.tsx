import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  SETTLE_INTRO_MS,
  SETTLE_RELEASE_MS,
  ScryingGridWordmark,
  WORDMARK_ONE_LINE,
  WORDMARK_PRESETS,
} from './ScryingGridWordmark'

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
    const { container } = render(<ScryingGridWordmark heading={false} preset="nameplate" />)

    expect(sampleSpy).not.toHaveBeenCalled()
    expect(container.querySelector('[data-hero-fallback]')).toHaveClass('opacity-0')
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

    clock.advance(10_000)
    intersection.set(true)
    expect(clock.pending).toBe(1)
    // Half the intro was left, not none of it.
    clock.advance(SETTLE_INTRO_MS / 4)
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS)
    expect(clock.pending).toBe(0)
  })

  it('pauses in a hidden tab and resumes when the tab is shown', () => {
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    clock.advance(SETTLE_INTRO_MS / 2)
    setDocumentHidden(true)
    expect(clock.pending).toBe(0)

    setDocumentHidden(false)
    expect(clock.pending).toBe(1)
    clock.advance(SETTLE_INTRO_MS)
    expect(clock.pending).toBe(0)
  })

  it('draws the resting frame once, with no intro and no pointer reaction, under reduced motion', () => {
    matchMedia.set(REDUCED_MOTION_QUERY, true)
    render(<ScryingGridWordmark motion="settle" heading={false} />)
    expect(clock.pending).toBe(0)
    expect(FAKE_CTX.arc).toHaveBeenCalled()
    fireEvent.pointerMove(screen.getByRole('img'), { clientX: 40, clientY: 50 })
    expect(clock.pending).toBe(0)
  })
})
