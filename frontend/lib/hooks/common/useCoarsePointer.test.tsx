import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { useCoarsePointer } from './useCoarsePointer'

// The literal query, not an import of it: a drifted query must fail here.
const COARSE_POINTER_QUERY = '(pointer: coarse)'

describe('useCoarsePointer', () => {
  let matchMedia: ReturnType<typeof installMatchMedia> | null = null
  afterEach(() => {
    matchMedia?.restore()
    matchMedia = null
  })

  it('asks whether the primary pointer is coarse', () => {
    matchMedia = installMatchMedia()
    renderHook(() => useCoarsePointer())
    expect(matchMedia.queries).toContain(COARSE_POINTER_QUERY)
  })

  it('is false for a fine pointer and true for a coarse one', () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: false })
    expect(renderHook(() => useCoarsePointer()).result.current).toBe(false)
    matchMedia.restore()

    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: true })
    expect(renderHook(() => useCoarsePointer()).result.current).toBe(true)
  })

  it('follows the pointer when it changes mid-session', () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: false })
    const { result } = renderHook(() => useCoarsePointer())

    matchMedia.set(COARSE_POINTER_QUERY, true)
    expect(result.current).toBe(true)

    matchMedia.set(COARSE_POINTER_QUERY, false)
    expect(result.current).toBe(false)
  })

  it('is false in the server HTML, whatever the browser would answer', async () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: true })
    const { renderToString } = await import('react-dom/server')
    function Probe() {
      return <span>{String(useCoarsePointer())}</span>
    }
    expect(renderToString(<Probe />)).toContain('false')
  })
})
