import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SCENES: {
      LIST: '/scenes',
      DETAIL: (slug: string) => `/scenes/${slug}`,
      ARTISTS: (slug: string) => `/scenes/${slug}/artists`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    scenes: {
      list: ['scenes', 'list'],
      detail: (slug: string) => ['scenes', 'detail', slug],
      artists: (slug: string, period?: number) => ['scenes', 'artists', slug, period],
    },
  },
}))

import { useScenes, useSceneDetail, useSceneArtists } from './useScenes'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('useScenes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches scene list', async () => {
    mockApiRequest.mockResolvedValueOnce({ scenes: [{ slug: 'phoenix-az', label: 'Phoenix, AZ' }] })

    const { result } = renderHook(() => useScenes(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/scenes', { method: 'GET' })
    expect(result.current.data?.scenes).toHaveLength(1)
  })

  it('handles empty scenes', async () => {
    mockApiRequest.mockResolvedValueOnce({ scenes: [] })

    const { result } = renderHook(() => useScenes(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.scenes).toEqual([])
  })

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useScenes(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useSceneDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a scene by slug', async () => {
    mockApiRequest.mockResolvedValueOnce({
      slug: 'phoenix-az',
      label: 'Phoenix, AZ',
      show_count: 50,
      artist_count: 30,
    })

    const { result } = renderHook(() => useSceneDetail('phoenix-az'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/scenes/phoenix-az', { method: 'GET' })
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useSceneDetail(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})

describe('useSceneArtists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches scene artists with default params', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

    const { result } = renderHook(
      () => useSceneArtists({ slug: 'phoenix-az' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/scenes/phoenix-az/artists')
    expect(url).toContain('period=90')
    expect(url).toContain('limit=20')
  })

  it('includes custom period and limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

    const { result } = renderHook(
      () => useSceneArtists({ slug: 'phoenix-az', period: 30, limit: 50, offset: 10 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('period=30')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=10')
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(
      () => useSceneArtists({ slug: '' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
