import { describe, expect, it, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      CHART_DEFAULTS: '/auth/preferences/chart-defaults',
    },
  },
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    auth: { profile: ['auth', 'profile'] },
  },
}))

import { useSetChartDefaults } from './useChartDefaults'

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('useSetChartDefaults', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('PUTs chart defaults to the preferences endpoint', async () => {
    mockApiRequest.mockResolvedValue({
      success: true,
      message: 'Chart defaults updated',
      defaults: { window: 'month', scene: '38060' },
    })

    const { result } = renderHook(() => useSetChartDefaults(), { wrapper })

    result.current.mutate({ window: 'month', scene: '38060' })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/chart-defaults',
      {
        method: 'PUT',
        body: JSON.stringify({
          defaults: { window: 'month', scene: '38060' },
        }),
      }
    )
  })

  it('PUTs null defaults to clear', async () => {
    mockApiRequest.mockResolvedValue({
      success: true,
      message: 'Chart defaults updated',
      defaults: null,
    })

    const { result } = renderHook(() => useSetChartDefaults(), { wrapper })

    result.current.mutate(null)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/chart-defaults',
      {
        method: 'PUT',
        body: JSON.stringify({ defaults: null }),
      }
    )
  })
})
