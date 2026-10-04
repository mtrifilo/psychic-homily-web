import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  ATLAS_COMPACT_VIEWPORT_QUERY,
  isAtlasCompactViewport,
  useAtlasCompactViewport,
} from './atlasViewport'

describe('useAtlasCompactViewport', () => {
  let restore: () => void = () => {}
  afterEach(() => restore())

  it('asks for the exact complement of the Tailwind lg breakpoint', () => {
    // `lg:` is `min-width: 64rem`; a branch on this query and an `lg:`
    // utility must flip at the same width.
    expect(ATLAS_COMPACT_VIEWPORT_QUERY).toBe('not all and (min-width: 64rem)')
    const mm = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
    restore = mm.restore
    const { result } = renderHook(() => useAtlasCompactViewport())
    expect(mm.queries).toContain(ATLAS_COMPACT_VIEWPORT_QUERY)
    expect(result.current).toBe(true)
  })

  it('answers the one-shot read from the same query', () => {
    const mm = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: false })
    restore = mm.restore
    expect(isAtlasCompactViewport()).toBe(false)
    mm.set(ATLAS_COMPACT_VIEWPORT_QUERY, true)
    expect(isAtlasCompactViewport()).toBe(true)
  })
})
