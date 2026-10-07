import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { ScryingGridWordmark } from './ScryingGridWordmark'

// The literal query, not an import of it: a drifted query must fail here.
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// jsdom cannot rasterise text, so sampling returns a fixed field of cells.
vi.mock('./sampleWordmark', async importOriginal => ({
  ...(await importOriginal<typeof import('./sampleWordmark')>()),
  sampleWordmark: () => [
    { x: 10, y: 10 },
    { x: 20, y: 10 },
  ],
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
