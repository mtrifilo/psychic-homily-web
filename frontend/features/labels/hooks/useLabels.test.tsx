import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('../api', () => ({
  labelEndpoints: {
    LIST: '/labels',
    GET: (idOrSlug: string | number) => `/labels/${idOrSlug}`,
    ARTISTS: (idOrSlug: string | number) => `/labels/${idOrSlug}/artists`,
    RELEASES: (idOrSlug: string | number) => `/labels/${idOrSlug}/releases`,
  },
  labelQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['labels', 'list', filters],
    detail: (id: string | number) => ['labels', 'detail', String(id)],
    roster: (id: string | number) => ['labels', 'roster', String(id)],
    catalog: (id: string | number) => ['labels', 'catalog', String(id)],
  },
}))

vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    LABELS: (artistIdOrSlug: string | number) => `/artists/${artistIdOrSlug}/labels`,
  },
  artistQueryKeys: {
    labels: (artistId: string | number) => ['artists', 'labels', String(artistId)],
  },
}))

import { useLabels, useLabel, useArtistLabels, useLabelRoster, useLabelCatalog } from './useLabels'


describe('useLabels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches labels without filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ labels: [{ id: 1, name: 'Sub Pop' }], count: 1 })

    const { result } = renderHook(() => useLabels(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels', { method: 'GET' })
  })

  it('includes status filter in query params', async () => {
    mockApiRequest.mockResolvedValueOnce({ labels: [], count: 0 })

    const { result } = renderHook(() => useLabels({ status: 'active' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels?status=active', { method: 'GET' })
  })

  it('includes city and state in query params', async () => {
    mockApiRequest.mockResolvedValueOnce({ labels: [], count: 0 })

    const { result } = renderHook(() => useLabels({ city: 'Phoenix', state: 'AZ' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const calledUrl = mockApiRequest.mock.calls[0][0] as string
    expect(calledUrl).toContain('city=Phoenix')
    expect(calledUrl).toContain('state=AZ')
  })
})

describe('useLabel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a label by ID', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'Sub Pop', slug: 'sub-pop' })

    const { result } = renderHook(() => useLabel({ idOrSlug: 1 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/1', { method: 'GET' })
  })

  it('fetches a label by slug', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'Sub Pop', slug: 'sub-pop' })

    const { result } = renderHook(() => useLabel({ idOrSlug: 'sub-pop' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop', { method: 'GET' })
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useLabel({ idOrSlug: 1, enabled: false }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch when idOrSlug is 0', () => {
    const { result } = renderHook(() => useLabel({ idOrSlug: 0 }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when idOrSlug is empty string', () => {
    const { result } = renderHook(() => useLabel({ idOrSlug: '' }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useArtistLabels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches labels for an artist', async () => {
    mockApiRequest.mockResolvedValueOnce({ labels: [{ id: 1, name: 'Sub Pop' }] })

    const { result } = renderHook(
      () => useArtistLabels({ artistIdOrSlug: 'the-shins' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/the-shins/labels', { method: 'GET' })
  })

  it('does not fetch when artistId is 0', () => {
    const { result } = renderHook(
      () => useArtistLabels({ artistIdOrSlug: 0 }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useLabelRoster', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches roster for a label', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [{ id: 1, name: 'Artist A' }] })

    const { result } = renderHook(() => useLabelRoster({ labelIdOrSlug: 'sub-pop' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop/artists', { method: 'GET' })
  })
})

describe('useLabelCatalog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches catalog for a label', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [{ id: 1, title: 'Album A' }] })

    const { result } = renderHook(() => useLabelCatalog({ labelIdOrSlug: 'sub-pop' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop/releases', { method: 'GET' })
  })
})
