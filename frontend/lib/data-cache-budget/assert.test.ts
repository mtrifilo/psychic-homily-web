import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureMessage } = vi.hoisted(() => ({ captureMessage: vi.fn() }))
vi.mock('@sentry/nextjs', () => ({ captureMessage }))

import {
  assertFetchFitsDataCache,
  DataCacheBudgetError,
  readJsonWithinDataCacheBudget,
} from './assert'
import {
  DATA_CACHE_RAW_BUDGET_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
} from './budget'

// The measured production sizes from PSY-1674, raw bytes.
const ARTISTS_FULL_2026_08_08 = 3_233_345
const ARTISTS_PROJECTION_2026_08_08 = 311_240

const originalPhase = process.env.NEXT_PHASE

function inProductionBuild(yes: boolean) {
  if (yes) process.env.NEXT_PHASE = 'phase-production-build'
  else delete process.env.NEXT_PHASE
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  if (originalPhase === undefined) delete process.env.NEXT_PHASE
  else process.env.NEXT_PHASE = originalPhase
})

describe('readJsonWithinDataCacheBudget', () => {
  const respondWith = (text: string) => new Response(text)

  it('parses and returns the body', async () => {
    await expect(
      readJsonWithinDataCacheBudget('/x', respondWith('{"artists":[{"slug":"a"}]}'))
    ).resolves.toEqual({ artists: [{ slug: 'a' }] })
  })

  // The cap is on BYTES, not characters. A body of multi-byte characters must
  // not be under-counted into looking safe by the cheap length-based prefilter.
  it('weighs multi-byte bodies by their byte length, not character count', async () => {
    inProductionBuild(true)
    // Each 🎸 is 4 UTF-8 bytes but only 2 UTF-16 code units, so `.length` is
    // HALF the real byte size — under the budget where the bytes are over.
    const emoji = '🎸'.repeat(DATA_CACHE_RAW_LIMIT_BYTES / 4 + 10)
    expect(emoji.length).toBeLessThan(DATA_CACHE_RAW_LIMIT_BYTES)

    await expect(
      readJsonWithinDataCacheBudget('/x', respondWith(JSON.stringify({ s: emoji })))
    ).rejects.toThrow(DataCacheBudgetError)
  })

  it('lets a small body through without complaint', async () => {
    inProductionBuild(true)
    await expect(
      readJsonWithinDataCacheBudget('/x', respondWith('{"ok":true}'))
    ).resolves.toEqual({ ok: true })
    expect(captureMessage).not.toHaveBeenCalled()
  })
})

describe('assertFetchFitsDataCache during a production build', () => {
  beforeEach(() => inProductionBuild(true))

  // The regression this gate exists to prevent: repointing the artists ItemList
  // back at the full list endpoint.
  it('throws on the full /artists payload', () => {
    expect(() => assertFetchFitsDataCache('/artists', ARTISTS_FULL_2026_08_08)).toThrow(
      DataCacheBudgetError
    )
  })

  it('explains that the payload will not be cached at all', () => {
    let message = ''
    try {
      assertFetchFitsDataCache('/artists', ARTISTS_FULL_2026_08_08)
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('/artists')
    expect(message).toContain('re-pull')
    expect(message).toContain('3.08 MiB')
  })

  it('passes the projection the fix introduced', () => {
    expect(() =>
      assertFetchFitsDataCache('/artists/listing', ARTISTS_PROJECTION_2026_08_08)
    ).not.toThrow()
    expect(captureMessage).not.toHaveBeenCalled()
  })

  // The warn band: still cacheable today, so it must fail the build rather than
  // wait for the deploy that has already broken.
  it('throws while still under the hard cap but inside the warn band', () => {
    const approaching = DATA_CACHE_RAW_BUDGET_BYTES + 1
    expect(approaching).toBeLessThan(DATA_CACHE_RAW_LIMIT_BYTES)
    expect(() => assertFetchFitsDataCache('/x', approaching)).toThrow(DataCacheBudgetError)
  })

  it('passes just under the warn line', () => {
    expect(() =>
      assertFetchFitsDataCache('/x', DATA_CACHE_RAW_BUDGET_BYTES - 1)
    ).not.toThrow()
  })
})

describe('assertFetchFitsDataCache at request time', () => {
  beforeEach(() => inProductionBuild(false))

  // A page that renders is worth more than a cache entry, and the deploy has
  // already happened — so this reports rather than 500s the route.
  it('reports to Sentry instead of throwing', () => {
    expect(() => assertFetchFitsDataCache('/artists', ARTISTS_FULL_2026_08_08)).not.toThrow()
    expect(captureMessage).toHaveBeenCalledTimes(1)
    expect(captureMessage.mock.calls[0][1]).toMatchObject({ level: 'error' })
  })

  // Over the cap is a live defect; merely approaching it is not yet.
  it('reports the warn band at a lower level than the breach', () => {
    assertFetchFitsDataCache('/x', DATA_CACHE_RAW_BUDGET_BYTES + 1)
    expect(captureMessage.mock.calls[0][1]).toMatchObject({ level: 'warning' })
  })

  it('stays silent for a payload within budget', () => {
    assertFetchFitsDataCache('/artists/listing', ARTISTS_PROJECTION_2026_08_08)
    expect(captureMessage).not.toHaveBeenCalled()
  })
})
