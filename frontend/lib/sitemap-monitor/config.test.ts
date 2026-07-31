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
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_RATIO: '2' })).toThrow(/between 0 and 1/)
    expect(() => resolveConfig({ SITEMAP_MONITOR_DRIFT_FLOOR: '-1' })).toThrow(/between 0 and/)
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
