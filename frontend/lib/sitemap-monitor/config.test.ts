import { describe, expect, it } from 'vitest'
import { resolveConfig } from './config'

describe('resolveConfig', () => {
  it('defaults to production with conservative thresholds', () => {
    const config = resolveConfig({})
    expect(config.target).toBe('https://psychichomily.com')
    expect(config.apiBase).toBe('https://api.psychichomily.com')
    expect(config.entryPath).toBe('/sitemap-index')
    expect(config.driftRatio).toBe(0.2)
    expect(config.driftFloor).toBe(10)
    expect(config.minFutureShows).toBe(10)
    expect(config.notifyOnSuccess).toBe(false)
  })

  it('overrides every threshold from the environment', () => {
    const config = resolveConfig({
      SITEMAP_MONITOR_TARGET: 'https://stage.psychichomily.com',
      SITEMAP_MONITOR_API_BASE: 'https://stage.api.psychichomily.com',
      SITEMAP_MONITOR_DRIFT_RATIO: '0.05',
      SITEMAP_MONITOR_DRIFT_FLOOR: '3',
      SITEMAP_MONITOR_MIN_FUTURE_SHOWS: '50',
      SITEMAP_MONITOR_SAMPLE_SIZE: '25',
      SITEMAP_MONITOR_NOTIFY_ON_SUCCESS: 'true',
    })
    expect(config.target).toBe('https://stage.psychichomily.com')
    expect(config.apiBase).toBe('https://stage.api.psychichomily.com')
    expect(config.driftRatio).toBe(0.05)
    expect(config.driftFloor).toBe(3)
    expect(config.minFutureShows).toBe(50)
    expect(config.sampleSize).toBe(25)
    expect(config.notifyOnSuccess).toBe(true)
  })

  it('strips a trailing slash so URLs never double up', () => {
    expect(resolveConfig({ SITEMAP_MONITOR_TARGET: 'https://psychichomily.com/' }).target).toBe(
      'https://psychichomily.com'
    )
  })

  it('treats an empty string as unset', () => {
    expect(resolveConfig({ SITEMAP_MONITOR_DRIFT_RATIO: '  ' }).driftRatio).toBe(0.2)
  })

  // The load-bearing case: a threshold that silently became NaN would make
  // every comparison pass, turning the monitor into a green light that checks
  // nothing.
  it('rejects a non-numeric threshold instead of coercing it', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_RATIO: '0.2x' })).toThrow(
      /SITEMAP_MONITOR_DRIFT_RATIO must be a number/
    )
  })

  it('rejects an out-of-range threshold', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_RATIO: '2' })).toThrow(/between 0 and 0.5/)
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_FLOOR: '-1' })).toThrow(/between 0 and/)
  })

  /**
   * Each of these would leave the monitor reporting a cheerful pass while
   * asserting nothing — the shape of the incident it exists to catch. Widening
   * a knob to silence a noisy alarm must not be able to disable the check.
   */
  it('refuses settings that would make the check vacuous', () => {
    // ratio 1 would make the budget equal the expected count, so observed 0
    // would pass for every family.
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_RATIO: '1' })).toThrow(/between 0 and 0.5/)
    // 0 upcoming shows required is satisfied by an entirely empty sitemap.
    expect(() => resolveConfig({ SITEMAP_MONITOR_MIN_FUTURE_SHOWS: '0' })).toThrow(/between 1 and/)
    // 0 samples renders as "0/0 reachable", which reads as a pass.
    expect(() => resolveConfig({ SITEMAP_MONITOR_SAMPLE_SIZE: '0' })).toThrow(/between 1 and/)
  })

  // The bypass token rides on these requests as a header.
  it('requires https off localhost', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_TARGET: 'http://psychichomily.com' })).toThrow(
      /must use https off localhost/
    )
    expect(resolveConfig({ SITEMAP_MONITOR_TARGET: 'http://127.0.0.1:8787' }).target).toBe(
      'http://127.0.0.1:8787'
    )
  })

  // walkSitemap would append the entry path to it while rebaseOnTarget builds
  // shard URLs from the origin alone — the two halves would disagree.
  it('rejects a target carrying a path', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_TARGET: 'https://example.com/base' })).toThrow(
      /must be a bare origin/
    )
  })

  it('rejects a non-boolean flag', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_NOTIFY_ON_SUCCESS: 'yes' })).toThrow(
      /must be true or false/
    )
  })

  it('rejects a target that is not an absolute http(s) URL', () => {
    expect(() => resolveConfig({ SITEMAP_MONITOR_TARGET: 'psychichomily.com' })).toThrow(
      /must be an absolute URL/
    )
    expect(() => resolveConfig({ SITEMAP_MONITOR_TARGET: 'ftp://psychichomily.com' })).toThrow(
      /must be an http\(s\) URL/
    )
  })
})
