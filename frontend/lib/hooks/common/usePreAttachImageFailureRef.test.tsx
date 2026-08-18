import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { usePreAttachImageFailureRef } from './usePreAttachImageFailureRef'
import { stubAllImagesLoadState } from '@/test/stubAllImagesLoadState'

const LIVE_SRC = 'https://example.com/image.jpg'

/**
 * The predicate lives here, so its truth table is pinned here rather than
 * inferred from the three components that compose it.
 *
 * `src` has no default on purpose: a default would swallow the `undefined`
 * case, which is one of the two the empty-src guard exists for.
 */
function Subject({
  onFailure,
  src,
}: {
  onFailure: () => void
  src?: string
}) {
  const ref = usePreAttachImageFailureRef(onFailure)
  // Keyed on `src` so a URL change genuinely REMOUNTS the element, which is
  // what the ordering test below needs. The hook never requires a caller to
  // key; this is the harness making a remount happen on demand.
  // eslint-disable-next-line @next/next/no-img-element
  return <img key={src} ref={ref} src={src} alt="" />
}

describe('usePreAttachImageFailureRef', () => {
  // "Finished, and decoded nothing" — the dead hotlink the hook exists for.
  it('reports a node that finished having decoded nothing', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })
    const onFailure = vi.fn()

    render(<Subject onFailure={onFailure} src={LIVE_SRC} />)

    expect(onFailure).toHaveBeenCalledTimes(1)
  })

  // The `complete` half. Without it every decoded image would be condemned.
  it('stays silent for a node that finished and decoded', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 600 })
    const onFailure = vi.fn()

    render(<Subject onFailure={onFailure} src={LIVE_SRC} />)

    expect(onFailure).not.toHaveBeenCalled()
  })

  // The `naturalWidth` half. jsdom's own defaults are this case, so it is
  // also what keeps the hook quiet in every other test in the suite.
  it('stays silent for a node still in flight', () => {
    const onFailure = vi.fn()

    render(<Subject onFailure={onFailure} src={LIVE_SRC} />)

    expect(onFailure).not.toHaveBeenCalled()
  })

  // React calls a callback ref with null on detach. The hook must not treat
  // "no node" as "the node failed".
  it('stays silent when the node detaches', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 600 })
    const onFailure = vi.fn()

    const { unmount } = render(<Subject onFailure={onFailure} src={LIVE_SRC} />)
    unmount()

    expect(onFailure).not.toHaveBeenCalled()
  })

  // `complete` is spec-TRUE for an absent or empty `src`, so without the
  // guard an image that was never even requested reports as failed and its
  // owner shows a fallback forever.
  it.each([
    ['an empty src', ''],
    ['no src at all', undefined],
  ])('stays silent for %s', (_label, src) => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })
    const onFailure = vi.fn()

    render(<Subject onFailure={onFailure} src={src} />)

    expect(onFailure).not.toHaveBeenCalled()
  })

  // The ref is identity-stable, so it attaches once per node however often
  // the owner re-renders — including with a fresh inline callback each time.
  // Without that, React detaches and re-attaches on every render, and the
  // re-read against a reused node is what condemns a good replacement image
  // (and, if the owner sets fresh state each call, loops until React throws).
  it('attaches once per node even when the caller passes an inline callback', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })
    const onFailure = vi.fn()

    const { rerender } = render(
      <Subject onFailure={() => onFailure()} src={LIVE_SRC} />
    )
    rerender(<Subject onFailure={() => onFailure()} src={LIVE_SRC} />)
    rerender(<Subject onFailure={() => onFailure()} src={LIVE_SRC} />)

    expect(onFailure).toHaveBeenCalledTimes(1)
  })

  // The latest-ref must be written before the ref attaches, on REMOUNT as
  // well as mount, or a remounted node would call a callback from the
  // previous render. `useInsertionEffect` is what guarantees that ordering;
  // a passive effect would be one render stale here.
  it('calls the newest callback when the node remounts', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })
    const first = vi.fn()
    const second = vi.fn()

    const { rerender } = render(
      <Subject onFailure={first} src="https://example.com/a.jpg" />
    )
    rerender(<Subject onFailure={second} src="https://example.com/b.jpg" />)

    expect(second).toHaveBeenCalledTimes(1)
    expect(first).toHaveBeenCalledTimes(1) // its own mount only
  })
})
