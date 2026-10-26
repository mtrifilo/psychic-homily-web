import { describe, expect, it } from 'vitest'
import { createPositiveVerdictCache } from './positive-verdict-cache'

function clock(start = 1_000) {
  let t = start
  return {
    now: () => t,
    advance: (ms: number) => {
      t += ms
    },
  }
}

describe('createPositiveVerdictCache', () => {
  it('remembers a key until its time-to-live has passed', () => {
    const c = clock()
    const cache = createPositiveVerdictCache({ ttlMs: 300, maxEntries: 10, now: c.now })

    expect(cache.has('a')).toBe(false)
    cache.remember('a')
    expect(cache.has('a')).toBe(true)
    c.advance(299)
    expect(cache.has('a')).toBe(true)
    c.advance(1)
    expect(cache.has('a')).toBe(false)
  })

  it('restarts the time-to-live when a key is remembered again', () => {
    const c = clock()
    const cache = createPositiveVerdictCache({ ttlMs: 300, maxEntries: 10, now: c.now })

    cache.remember('a')
    c.advance(200)
    cache.remember('a')
    c.advance(200)
    expect(cache.has('a')).toBe(true)
  })

  it('drops the entry nearest expiry to stay within its bound', () => {
    const c = clock()
    const cache = createPositiveVerdictCache({ ttlMs: 300, maxEntries: 2, now: c.now })

    cache.remember('a')
    c.advance(1)
    cache.remember('b')
    c.advance(1)
    cache.remember('c')

    expect(cache.has('a')).toBe(false)
    expect(cache.has('b')).toBe(true)
    expect(cache.has('c')).toBe(true)
  })

  it('keeps a refreshed key over an older one when full', () => {
    const c = clock()
    const cache = createPositiveVerdictCache({ ttlMs: 300, maxEntries: 2, now: c.now })

    cache.remember('a')
    cache.remember('b')
    cache.remember('a')
    cache.remember('c')

    expect(cache.has('a')).toBe(true)
    expect(cache.has('b')).toBe(false)
    expect(cache.has('c')).toBe(true)
  })

  it('forgets everything on clear', () => {
    const cache = createPositiveVerdictCache({ ttlMs: 300, maxEntries: 10 })
    cache.remember('a')
    cache.clear()
    expect(cache.has('a')).toBe(false)
  })
})
