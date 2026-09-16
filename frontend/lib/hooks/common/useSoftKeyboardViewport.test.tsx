import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'

import {
  SOFT_KEYBOARD_VIEWPORT_QUERY,
  useSoftKeyboardViewport,
} from './useSoftKeyboardViewport'

const originalMatchMedia = window.matchMedia

afterEach(() => {
  window.matchMedia = originalMatchMedia
})

type Listener = () => void

function mockMatchMedia(matching: boolean) {
  const listeners: Listener[] = []
  const seen: string[] = []
  window.matchMedia = vi.fn((query: string) => {
    seen.push(query)
    return {
      matches: matching && query === SOFT_KEYBOARD_VIEWPORT_QUERY,
      media: query,
      addEventListener: (_: string, fn: Listener) => listeners.push(fn),
      removeEventListener: (_: string, fn: Listener) => {
        const i = listeners.indexOf(fn)
        if (i >= 0) listeners.splice(i, 1)
      },
    } as unknown as MediaQueryList
  })
  return { listeners, seen }
}

function Probe() {
  return <span data-testid="value">{String(useSoftKeyboardViewport())}</span>
}

describe('useSoftKeyboardViewport', () => {
  it('reports the query result and asks for the shared query', () => {
    const { seen } = mockMatchMedia(true)

    render(<Probe />)

    expect(screen.getByTestId('value')).toHaveTextContent('true')
    expect(seen).toContain(SOFT_KEYBOARD_VIEWPORT_QUERY)
  })

  it('is false on a wide fine-pointer viewport', () => {
    mockMatchMedia(false)

    render(<Probe />)

    expect(screen.getByTestId('value')).toHaveTextContent('false')
  })

  // The value decides the trigger's ARIA contract, so a viewport that changes
  // mid-session has to move it rather than leave a stale promise on screen.
  it('re-reads when the viewport changes mid-session', () => {
    const { listeners } = mockMatchMedia(false)

    render(<Probe />)
    expect(screen.getByTestId('value')).toHaveTextContent('false')

    mockMatchMedia(true)
    act(() => listeners.forEach(fn => fn()))

    expect(screen.getByTestId('value')).toHaveTextContent('true')
  })

  it('is false where matchMedia is unavailable', () => {
    // @ts-expect-error deliberately removing the API the guard exists for
    window.matchMedia = undefined

    render(<Probe />)

    expect(screen.getByTestId('value')).toHaveTextContent('false')
  })

  it('drops its listener on unmount', () => {
    const { listeners } = mockMatchMedia(true)

    const view = render(<Probe />)
    expect(listeners).toHaveLength(1)

    view.unmount()
    expect(listeners).toHaveLength(0)
  })
})
