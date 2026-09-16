import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { useRef } from 'react'

import {
  KEYBOARD_INSET_VAR,
  KEYBOARD_VISIBLE_HEIGHT_VAR,
  usePinAboveSoftKeyboard,
} from './usePinAboveSoftKeyboard'

type Listener = () => void

const originalVisualViewport = Object.getOwnPropertyDescriptor(
  window,
  'visualViewport'
)
const originalInnerHeight = window.innerHeight
const originalRaf = window.requestAnimationFrame
const originalCancelRaf = window.cancelAnimationFrame

afterEach(() => {
  if (originalVisualViewport) {
    Object.defineProperty(window, 'visualViewport', originalVisualViewport)
  }
  window.innerHeight = originalInnerHeight
  window.requestAnimationFrame = originalRaf
  window.cancelAnimationFrame = originalCancelRaf
})

/**
 * jsdom has no visual viewport and no frame loop. Both are replaced by hand so
 * the hook's arithmetic and its listener bookkeeping are the only things under
 * test.
 */
function mockViewport(height: number, offsetTop = 0) {
  const listeners: Record<string, Listener[]> = { resize: [], scroll: [] }
  const viewport = {
    height,
    offsetTop,
    addEventListener: (type: string, fn: Listener) =>
      listeners[type]?.push(fn),
    removeEventListener: (type: string, fn: Listener) => {
      const list = listeners[type]
      const i = list?.indexOf(fn) ?? -1
      if (i >= 0) list.splice(i, 1)
    },
  }
  Object.defineProperty(window, 'visualViewport', {
    configurable: true,
    get: () => viewport,
  })
  window.requestAnimationFrame = (cb: FrameRequestCallback) => {
    cb(0)
    return 1
  }
  window.cancelAnimationFrame = () => {}
  return { viewport, listeners }
}

function Probe({ enabled }: { enabled: boolean }) {
  const ref = useRef<HTMLDivElement | null>(null)
  usePinAboveSoftKeyboard(enabled, ref)
  return <div ref={ref} data-testid="surface" />
}

function readVars() {
  const node = screen.getByTestId('surface')
  return {
    inset: node.style.getPropertyValue(KEYBOARD_INSET_VAR),
    visible: node.style.getPropertyValue(KEYBOARD_VISIBLE_HEIGHT_VAR),
  }
}

describe('usePinAboveSoftKeyboard', () => {
  it('writes a zero inset when the two viewports agree', () => {
    mockViewport(844)
    window.innerHeight = 844

    render(<Probe enabled />)

    expect(readVars()).toEqual({ inset: '0px', visible: '844px' })
  })

  it('writes the keyboard height as the inset', () => {
    const { viewport, listeners } = mockViewport(844)
    window.innerHeight = 844

    render(<Probe enabled />)

    viewport.height = 302
    act(() => listeners.resize.forEach(fn => fn()))

    expect(readVars()).toEqual({ inset: '542px', visible: '302px' })
  })

  it('takes the scrolled offset off the inset', () => {
    const { viewport, listeners } = mockViewport(302)
    window.innerHeight = 844

    render(<Probe enabled />)

    viewport.offsetTop = 100
    act(() => listeners.scroll.forEach(fn => fn()))

    expect(readVars().inset).toBe('442px')
  })

  // A surface taller than the layout viewport would otherwise produce a
  // negative offset and pull itself off the bottom of the screen.
  it('never writes a negative inset', () => {
    mockViewport(900)
    window.innerHeight = 844

    render(<Probe enabled />)

    expect(readVars().inset).toBe('0px')
  })

  it('writes nothing and listens to nothing while disabled', () => {
    const { listeners } = mockViewport(302)
    window.innerHeight = 844

    render(<Probe enabled={false} />)

    expect(readVars()).toEqual({ inset: '', visible: '' })
    expect(listeners.resize).toHaveLength(0)
    expect(listeners.scroll).toHaveLength(0)
  })

  it('drops its listeners when the surface closes', () => {
    const { listeners } = mockViewport(844)
    window.innerHeight = 844

    const view = render(<Probe enabled />)
    expect(listeners.resize).toHaveLength(1)
    expect(listeners.scroll).toHaveLength(1)

    view.rerender(<Probe enabled={false} />)

    expect(listeners.resize).toHaveLength(0)
    expect(listeners.scroll).toHaveLength(0)
  })

  it('leaves the surface alone where there is no visual viewport', () => {
    Object.defineProperty(window, 'visualViewport', {
      configurable: true,
      get: () => null,
    })

    render(<Probe enabled />)

    expect(readVars()).toEqual({ inset: '', visible: '' })
  })
})
