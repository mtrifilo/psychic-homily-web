import { describe, expect, it, vi } from 'vitest'

const { mockNotFound } = vi.hoisted(() => ({
  mockNotFound: vi.fn(() => {
    throw new Error('not found')
  }),
}))

vi.mock('next/navigation', () => ({ notFound: mockNotFound }))
vi.mock('@/features/charts/components/ChartDrilldownPage', () => ({
  ChartDrilldownPage: () => null,
}))
vi.mock('@/features/charts', () => ({
  ChartsPage: () => null,
}))
vi.mock('@/components/shared', () => ({ LoadingSpinner: () => null }))

import ChartModuleOrArchiveRoute, { generateMetadata } from './page'

/**
 * Canonicals are asserted, not assumed. The drilldown branch shipped without
 * one until PSY-1767, and a missing `alternates` key fails silently: the page
 * renders, the tests pass, and only a crawler notices. See the pagination
 * indexing policy on `listRootCanonical`.
 */
describe('charts/[module] canonical (pagination indexing policy)', () => {
  it('points a module drilldown at its own list root', async () => {
    const metadata = await generateMetadata({
      params: Promise.resolve({ module: 'most-active-artists' }),
    })

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/charts/most-active-artists'
    )
  })

  it('points a calendar year archive at its own path', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    const metadata = await generateMetadata({
      params: Promise.resolve({ module: '2026' }),
    })

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/charts/2026'
    )
    vi.useRealTimers()
  })

  it('declares no canonical for a segment neither branch vouches for', async () => {
    const metadata = await generateMetadata({
      params: Promise.resolve({ module: 'unknown' }),
    })

    expect(metadata.alternates).toBeUndefined()
  })
})

describe('charts/[module] route', () => {
  it('accepts an existing module slug', async () => {
    const result = await ChartModuleOrArchiveRoute({
      params: Promise.resolve({ module: 'most-active-artists' }),
    })

    expect(result).toBeTruthy()
    expect(mockNotFound).not.toHaveBeenCalled()
  })

  it('accepts a valid calendar year archive', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    const result = await ChartModuleOrArchiveRoute({
      params: Promise.resolve({ module: '2026' }),
    })
    expect(result).toBeTruthy()
    expect(mockNotFound).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('calls notFound for an unknown slug (proxy allowlist produces the real HTTP 404)', async () => {
    await expect(
      ChartModuleOrArchiveRoute({
        params: Promise.resolve({ module: 'unknown' }),
      })
    ).rejects.toThrow('not found')
    expect(mockNotFound).toHaveBeenCalledOnce()
  })

  it('calls notFound for a pre-launch year', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    await expect(
      ChartModuleOrArchiveRoute({
        params: Promise.resolve({ module: '2025' }),
      })
    ).rejects.toThrow('not found')
    expect(mockNotFound).toHaveBeenCalled()
    vi.useRealTimers()
  })
})
