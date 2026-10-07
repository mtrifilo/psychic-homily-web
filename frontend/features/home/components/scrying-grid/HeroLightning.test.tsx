import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { HeroLightning } from './HeroLightning'

// The literal query, not an import of it: a drifted query must fail here.
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

/** The bolt layer is live exactly while its spawn timer is pending. */
function boltsScheduled(): boolean {
  return vi.getTimerCount() > 0
}

describe('HeroLightning', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>
  const clearRect = vi.fn()

  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      clearRect,
    } as unknown as CanvasRenderingContext2D)
  })

  afterEach(() => {
    matchMedia.restore()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('schedules no bolts while the visitor asks for less motion', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    render(<HeroLightning />)
    expect(boltsScheduled()).toBe(false)
  })

  it('builds no bolt layer while hydrating for a visitor who asks for less motion', async () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const { renderToString } = await import('react-dom/server')
    const container = document.createElement('div')
    container.innerHTML = renderToString(<HeroLightning />)
    document.body.appendChild(container)

    // The hydration pass reads the hook's server value (false); the layer
    // must still never reach its canvas.
    render(<HeroLightning />, { container, hydrate: true })

    expect(HTMLCanvasElement.prototype.getContext).not.toHaveBeenCalled()
    expect(boltsScheduled()).toBe(false)
  })

  it('stops when reduced motion turns on mid-session, and resumes when it turns off', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    render(<HeroLightning />)
    expect(boltsScheduled()).toBe(true)
    clearRect.mockClear()

    matchMedia.set(REDUCED_MOTION_QUERY, true)
    expect(boltsScheduled()).toBe(false)
    // Whatever bolt was on the canvas is wiped rather than left frozen.
    expect(clearRect).toHaveBeenCalled()

    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(boltsScheduled()).toBe(true)
  })
})
