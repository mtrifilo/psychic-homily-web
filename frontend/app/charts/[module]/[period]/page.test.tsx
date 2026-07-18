import { describe, expect, it, vi } from 'vitest'

const { mockNotFound } = vi.hoisted(() => ({
  mockNotFound: vi.fn(() => {
    throw new Error('not found')
  }),
}))

vi.mock('next/navigation', () => ({ notFound: mockNotFound }))
vi.mock('@/features/charts', () => ({
  ChartsPage: () => null,
}))
vi.mock('@/components/shared', () => ({ LoadingSpinner: () => null }))

import ChartQuarterArchiveRoute from './page'

describe('charts/[module]/[period] archive route', () => {
  it('accepts a valid quarter archive', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    const result = await ChartQuarterArchiveRoute({
      params: Promise.resolve({ module: '2026', period: 'q2' }),
    })
    expect(result).toBeTruthy()
    expect(mockNotFound).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('calls notFound for a future quarter', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    await expect(
      ChartQuarterArchiveRoute({
        params: Promise.resolve({ module: '2026', period: 'q4' }),
      })
    ).rejects.toThrow('not found')
    expect(mockNotFound).toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('calls notFound when the first segment is a module slug', async () => {
    await expect(
      ChartQuarterArchiveRoute({
        params: Promise.resolve({
          module: 'most-active-artists',
          period: 'q1',
        }),
      })
    ).rejects.toThrow('not found')
    expect(mockNotFound).toHaveBeenCalled()
  })
})
