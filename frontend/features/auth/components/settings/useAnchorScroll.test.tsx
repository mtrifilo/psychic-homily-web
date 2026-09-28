import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { useAnchorScroll } from './useAnchorScroll'

// The subscribed hash, as the render saw it. Real renders read it from the
// address bar; the stale case below needs the two to differ.
let mockRenderedHash: string | null = null
vi.mock('@/lib/hooks/common/useUrlHash', async importOriginal => {
  const actual = await importOriginal<
    typeof import('@/lib/hooks/common/useUrlHash')
  >()
  return {
    ...actual,
    useUrlHash: () => mockRenderedHash ?? actual.useUrlHash(),
  }
})

function Card() {
  const ref = useAnchorScroll('alerts')
  return <div ref={ref} id="alerts" tabIndex={-1} />
}

describe('useAnchorScroll', () => {
  const scrollIntoView = vi.fn()

  beforeEach(() => {
    scrollIntoView.mockReset()
    Element.prototype.scrollIntoView = scrollIntoView
    mockRenderedHash = null
  })

  afterEach(() => {
    window.history.replaceState(null, '', '/')
  })

  it('reveals and focuses its card when its own route carries the fragment', () => {
    window.history.replaceState(null, '', '/profile?tab=settings#alerts')
    const { container } = render(<Card />)
    const card = container.querySelector('#alerts')
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView.mock.contexts[0]).toBe(card)
    expect(card).toHaveFocus()
  })

  // A client navigation renders the new route while the address bar still
  // shows the page being left; by the time refs attach it has been replaced.
  it("ignores the page being left's fragment that the render still saw", () => {
    mockRenderedHash = '#alerts'
    window.history.replaceState(null, '', '/profile?tab=settings')
    const { container } = render(<Card />)
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(container.querySelector('#alerts')).not.toHaveFocus()
  })

  it('does nothing without its fragment', () => {
    window.history.replaceState(null, '', '/profile?tab=settings')
    render(<Card />)
    expect(scrollIntoView).not.toHaveBeenCalled()
  })
})
