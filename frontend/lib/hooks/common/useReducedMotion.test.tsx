import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { useReducedMotion } from './useReducedMotion'

// The literal query, not an import of it: a drifted query must fail here.
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

describe('useReducedMotion', () => {
  let matchMedia: ReturnType<typeof installMatchMedia> | null = null
  afterEach(() => {
    matchMedia?.restore()
    matchMedia = null
  })

  it('asks for the reduced-motion preference', () => {
    matchMedia = installMatchMedia()
    renderHook(() => useReducedMotion())
    expect(matchMedia.queries).toContain(REDUCED_MOTION_QUERY)
  })

  it('is false when the visitor has not asked for less motion', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const { result } = renderHook(() => useReducedMotion())
    expect(result.current).toBe(false)
  })

  it('is true when the visitor has asked for less motion', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const { result } = renderHook(() => useReducedMotion())
    expect(result.current).toBe(true)
  })

  it('follows the preference when it changes mid-session', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const { result } = renderHook(() => useReducedMotion())

    matchMedia.set(REDUCED_MOTION_QUERY, true)
    expect(result.current).toBe(true)

    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(result.current).toBe(false)
  })

  it('is false in the server HTML, whatever the browser would answer', async () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const { renderToString } = await import('react-dom/server')
    function Probe() {
      return <span>{String(useReducedMotion())}</span>
    }
    expect(renderToString(<Probe />)).toContain('false')
  })
})
