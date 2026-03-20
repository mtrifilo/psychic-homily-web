import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      DATA_QUALITY: {
        SUMMARY: '/admin/data-quality',
        CATEGORY: (category: string) => `/admin/data-quality/${category}`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      dataQuality: {
        summary: ['admin', 'dataQuality', 'summary'],
        category: (category: string, limit: number, offset: number) =>
          ['admin', 'dataQuality', 'category', category, { limit, offset }],
      },
    },
  },
}))

import { useDataQualitySummary, useDataQualityCategory } from './useDataQuality'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('useDataQualitySummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches data quality summary', async () => {
    const mockSummary = {
      categories: [
        { key: 'missing_social', label: 'Missing Social Links', entity_type: 'artist', count: 10, description: '' },
        { key: 'no_shows', label: 'Venues with No Shows', entity_type: 'venue', count: 5, description: '' },
      ],
      total_items: 15,
    }
    mockApiRequest.mockResolvedValueOnce(mockSummary)

    const { result } = renderHook(() => useDataQualitySummary(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/admin/data-quality', { method: 'GET' })
    expect(result.current.data?.categories).toHaveLength(2)
    expect(result.current.data?.total_items).toBe(15)
  })

  it('handles API errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useDataQualitySummary(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useDataQualityCategory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches items for a category with defaults', async () => {
    const mockItems = {
      items: [
        { entity_type: 'artist', entity_id: 1, name: 'Test Artist', slug: 'test-artist', reason: 'missing social', show_count: 5 },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockItems)

    const { result } = renderHook(
      () => useDataQualityCategory('missing_social'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/data-quality/missing_social')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
  })

  it('uses custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ items: [], total: 0 })

    const { result } = renderHook(
      () => useDataQualityCategory('missing_social', 10, 20),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=20')
  })

  it('does not fetch when category is empty', () => {
    const { result } = renderHook(
      () => useDataQualityCategory(''),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useDataQualityCategory('missing_social', 50, 0, { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
