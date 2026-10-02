import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { useRef } from 'react'
import { FirstSaveHint } from './FirstSaveHint'

// Placement against the fixed mobile tab bar. jsdom has no layout, so the
// viewport, the anchor and the bar are given rects here; floating-ui does the
// rest for real.

vi.mock('@/features/shows/hooks/useFirstSaveHint', async importOriginal => ({
  ...(await importOriginal<
    typeof import('@/features/shows/hooks/useFirstSaveHint')
  >()),
  useStampFirstSaveHint: () => ({ mutate: vi.fn() }),
}))

const VIEWPORT = { width: 400, height: 800 }

function rect(x: number, y: number, width: number, height: number): DOMRect {
  return {
    x,
    y,
    width,
    height,
    top: y,
    left: x,
    right: x + width,
    bottom: y + height,
    toJSON() {},
  } as DOMRect
}

let barHeight = 0
let anchorTop = 0

/** Rendered closed first and then re-rendered open, so the hint mounts after
 *  the anchor's ref is attached, as it does in SaveButton (it opens only after
 *  a click). */
function Harness({ withBar, open }: { withBar: boolean; open: boolean }) {
  const anchorRef = useRef<HTMLSpanElement>(null)
  return (
    <>
      <span data-test-anchor="" ref={anchorRef}>
        <button type="button">Save</button>
        {open ? (
          <FirstSaveHint anchorRef={anchorRef} align="start" onClose={vi.fn()} />
        ) : null}
      </span>
      {withBar ? <nav data-bottom-tab-bar="" /> : null}
    </>
  )
}

describe('FirstSaveHint placement against the mobile tab bar', () => {
  const originalRect = Element.prototype.getBoundingClientRect

  beforeEach(() => {
    Object.defineProperty(document.documentElement, 'clientHeight', {
      configurable: true,
      value: VIEWPORT.height,
    })
    Object.defineProperty(document.documentElement, 'clientWidth', {
      configurable: true,
      value: VIEWPORT.width,
    })
    Element.prototype.getBoundingClientRect = function (this: Element) {
      if (this.hasAttribute('data-bottom-tab-bar')) {
        return rect(0, VIEWPORT.height - barHeight, VIEWPORT.width, barHeight)
      }
      if (this.hasAttribute('data-test-anchor')) {
        return rect(20, anchorTop, 80, 30)
      }
      return rect(0, 0, 0, 0)
    }
  })

  afterEach(() => {
    Element.prototype.getBoundingClientRect = originalRect
    delete (document.documentElement as { clientHeight?: number }).clientHeight
    delete (document.documentElement as { clientWidth?: number }).clientWidth
  })

  it.each([
    // No room below once the rendered bar is counted, so it opens above.
    ['opens above a control just above a rendered bar', 57, true, 720, 'top-start'],
    // The bar is display:none from xl and measures 0: nothing is reserved.
    ['stays below when the bar is hidden (measures 0)', 0, true, 720, 'bottom-start'],
    ['stays below when the page has no bar', 0, false, 720, 'bottom-start'],
    // Room below even with the bar counted.
    ['stays below a control well above a rendered bar', 57, true, 600, 'bottom-start'],
  ] as const)('%s', async (_label, height, present, top, want) => {
    barHeight = height
    anchorTop = top
    const { rerender } = render(<Harness withBar={present} open={false} />)
    rerender(<Harness withBar={present} open />)

    await waitFor(() =>
      expect(screen.getByTestId('first-save-hint')).toHaveAttribute(
        'data-placement',
        want
      )
    )
  })
})
