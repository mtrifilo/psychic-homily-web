import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  describeCollectionMutationError,
  useAutoDismissError,
} from './collectionDetailShared'

// PSY-957: the reactive wrapper consumed by the like/unlike/reorder call
// sites. Its signature and observable behavior predate PSY-957 (PSY-609);
// these tests pin that rebuilding it on the shared useAutoDismissBanner
// (now at @/lib/hooks/common) didn't change the contract.
describe('useAutoDismissError (PSY-609 contract)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  const formatter = (e: unknown) =>
    e instanceof Error ? e.message : 'something failed'

  it('returns null while there is no error', () => {
    const { result } = renderHook(() =>
      useAutoDismissError(null, false, formatter)
    )
    expect(result.current).toBeNull()
  })

  it('shows the formatted message when the mutation errors, then auto-dismisses', () => {
    const { result, rerender } = renderHook(
      ({ e, isErr }: { e: unknown; isErr: boolean }) =>
        useAutoDismissError(e, isErr, formatter),
      { initialProps: { e: null as unknown, isErr: false } }
    )

    rerender({ e: new Error('boom'), isErr: true })
    expect(result.current).toBe('boom')

    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current).toBeNull()
  })

  it('shows immediately when the first render already has an error', () => {
    // NOTE: `err` must be referentially stable across renders (it is in real
    // usage — TanStack keeps mutation.error stable between state changes).
    // Constructing it inside the renderHook callback would re-trigger the
    // error-signal guard every render.
    const err = new Error('immediate')
    const { result } = renderHook(() =>
      useAutoDismissError(err, true, formatter)
    )
    expect(result.current).toBe('immediate')
  })

  it('does NOT re-show after auto-dismiss while the mutation stays in its error state', () => {
    // The subtle half of the previous-value-guard contract: once a banner
    // auto-dismisses, an unrelated re-render (parent state change) while
    // isError/err are unchanged must NOT revive the dismissed banner. A
    // guard keyed on render count instead of err identity would regress this.
    const err = new Error('persistent 4xx')
    const { result, rerender } = renderHook(() =>
      useAutoDismissError(err, true, formatter)
    )
    expect(result.current).toBe('persistent 4xx')

    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current).toBeNull()

    // Same err + isError; just a re-render. Banner stays dismissed.
    rerender()
    expect(result.current).toBeNull()
  })

  it('re-shows when a new error fires after the previous one dismissed', () => {
    const { result, rerender } = renderHook(
      ({ e, isErr }: { e: unknown; isErr: boolean }) =>
        useAutoDismissError(e, isErr, formatter),
      { initialProps: { e: null as unknown, isErr: false } }
    )

    rerender({ e: new Error('first'), isErr: true })
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current).toBeNull()

    // The mutation resets, then errors again with a new error object.
    rerender({ e: null, isErr: false })
    rerender({ e: new Error('second'), isErr: true })
    expect(result.current).toBe('second')
  })

  it('respects a custom delay', () => {
    const err = new Error('slow burn')
    const { result } = renderHook(() =>
      useAutoDismissError(err, true, formatter, 5000)
    )

    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current).toBe('slow burn')

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current).toBeNull()
  })
})

// Direct coverage for the copy helper now that this module has its own test
// file (previously only exercised through component tests). CollectionCard
// delegates to it as of PSY-957, so its copy is load-bearing on three
// surfaces (detail like/unlike, reorder, card like/unlike).
describe('describeCollectionMutationError (PSY-609)', () => {
  it('renders dedicated copy for 403 (private target)', () => {
    const err = Object.assign(new Error('forbidden'), { status: 403 })
    expect(describeCollectionMutationError(err, 'fallback')).toBe(
      'This collection is private.'
    )
  })

  it('renders unlike-specific copy for 403 with unlikePrivate', () => {
    const err = Object.assign(new Error('forbidden'), { status: 403 })
    expect(
      describeCollectionMutationError(err, 'fallback', { unlikePrivate: true })
    ).toBe('This collection is private — your like was removed.')
  })

  it('falls back to the error message for non-403 errors', () => {
    expect(
      describeCollectionMutationError(new Error('network blew up'), 'fallback')
    ).toBe('network blew up')
  })

  it('falls back to the provided copy when the error has no message', () => {
    expect(
      describeCollectionMutationError({}, 'Failed to like collection.')
    ).toBe('Failed to like collection.')
  })
})
