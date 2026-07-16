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
vi.mock('@/components/shared', () => ({ LoadingSpinner: () => null }))

import ChartModuleRoute from './page'

describe('charts/[module] route', () => {
  it('accepts an existing module slug', async () => {
    const result = await ChartModuleRoute({
      params: Promise.resolve({ module: 'most-active-artists' }),
    })

    expect(result).toBeTruthy()
    expect(mockNotFound).not.toHaveBeenCalled()
  })

  it('uses the app hard-404 convention for an unknown slug', async () => {
    await expect(
      ChartModuleRoute({ params: Promise.resolve({ module: 'unknown' }) })
    ).rejects.toThrow('not found')
    expect(mockNotFound).toHaveBeenCalledOnce()
  })
})
