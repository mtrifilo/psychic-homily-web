import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'

import { useCaptureEscape, type UseCaptureEscapeOptions } from './useCaptureEscape'

function Listener({ onEscape, options }: { onEscape: () => void; options?: UseCaptureEscapeOptions }) {
  useCaptureEscape(onEscape, options)
  return null
}

describe('useCaptureEscape', () => {
  it('fires onEscape on a document capture-phase Escape', () => {
    const onEscape = vi.fn()
    render(<Listener onEscape={onEscape} />)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).toHaveBeenCalledTimes(1)
  })

  it('ignores non-Escape keys', () => {
    const onEscape = vi.fn()
    render(<Listener onEscape={onEscape} />)
    fireEvent.keyDown(document, { key: 'Enter' })
    expect(onEscape).not.toHaveBeenCalled()
  })

  it('defers to an earlier listener that already consumed the key (defaultPrevented)', () => {
    const onEscape = vi.fn()
    // Registered before the hook's listener (capture phase, same target) → it
    // runs first and marks the event consumed; the hook must then bail.
    const consume = (e: KeyboardEvent) => e.preventDefault()
    document.addEventListener('keydown', consume, { capture: true })
    try {
      render(<Listener onEscape={onEscape} />)
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(onEscape).not.toHaveBeenCalled()
    } finally {
      document.removeEventListener('keydown', consume, { capture: true })
    }
  })

  it('only the innermost (last-mounted) listener fires; unmounting it restores the outer', () => {
    const outer = vi.fn()
    const inner = vi.fn()
    const { rerender } = render(
      <>
        <Listener onEscape={outer} />
        <Listener onEscape={inner} />
      </>,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(inner).toHaveBeenCalledTimes(1)
    expect(outer).not.toHaveBeenCalled()

    // Inner unmounts → its token pops off the stack → outer becomes innermost.
    rerender(
      <>
        <Listener onEscape={outer} />
      </>,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(outer).toHaveBeenCalledTimes(1)
    expect(inner).toHaveBeenCalledTimes(1)
  })

  it('does not register a listener when enabled is false', () => {
    const onEscape = vi.fn()
    render(<Listener onEscape={onEscape} options={{ enabled: false }} />)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).not.toHaveBeenCalled()
  })

  it('ignoreFromInput leaves an Escape targeted at a text control for that control', () => {
    const onEscape = vi.fn()
    const { getByLabelText } = render(
      <>
        <input aria-label="search" />
        <Listener onEscape={onEscape} options={{ ignoreFromInput: true }} />
      </>,
    )
    fireEvent.keyDown(getByLabelText('search'), { key: 'Escape' })
    expect(onEscape).not.toHaveBeenCalled()

    // Same key from a non-input target is still handled.
    fireEvent.keyDown(document.body, { key: 'Escape' })
    expect(onEscape).toHaveBeenCalledTimes(1)
  })
})
