import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Mock apiRequest
const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    TAGS: {
      SEARCH: '/api/tags/search',
    },
  },
}))

// Mock use-debounce to return the value immediately for testing
vi.mock('use-debounce', () => ({
  useDebounce: (value: string) => [value],
}))

import { useEntitySearch } from './useEntitySearch'

describe('useEntitySearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: all endpoints return empty
    mockApiRequest.mockResolvedValue({ artists: [], venues: [], releases: [], labels: [], festivals: [], tags: [], count: 0 })
  })

  it('should not fetch when query is less than 2 characters', () => {
    const { result } = renderHook(
      () => useEntitySearch({ query: 'a' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.totalResults).toBe(0)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('should not fetch when query is empty', () => {
    const { result } = renderHook(
      () => useEntitySearch({ query: '' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.totalResults).toBe(0)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('should not fetch when disabled', () => {
    renderHook(
      () => useEntitySearch({ query: 'test', enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('should fetch all 6 entity types when query is 2+ characters', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/artists/search')) {
        return Promise.resolve({
          artists: [{ id: 1, slug: 'the-growlers', name: 'The Growlers', city: 'Dana Point', state: 'CA' }],
          count: 1,
        })
      }
      if (url.includes('/venues/search')) {
        return Promise.resolve({ venues: [], count: 0 })
      }
      if (url.includes('/releases/search')) {
        return Promise.resolve({ releases: [], count: 0 })
      }
      if (url.includes('/labels/search')) {
        return Promise.resolve({ labels: [], count: 0 })
      }
      if (url.includes('/festivals/search')) {
        return Promise.resolve({ festivals: [], count: 0 })
      }
      if (url.includes('/tags/search')) {
        return Promise.resolve({ tags: [] })
      }
      return Promise.resolve({ count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'growlers' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.totalResults).toBe(1)
    })

    // All 6 endpoints should be called
    expect(mockApiRequest).toHaveBeenCalledTimes(6)
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/artists/search?q=growlers'))
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/venues/search?q=growlers'))
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/releases/search?q=growlers'))
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/labels/search?q=growlers'))
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/festivals/search?q=growlers'))
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('/tags/search?q=growlers'))
  })

  it('should map tag results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/tags/search')) {
        return Promise.resolve({
          tags: [
            { id: 1, slug: 'post-punk', name: 'Post-Punk', category: 'genre', usage_count: 42 },
            { id: 2, slug: 'phoenix', name: 'Phoenix', category: 'locale', usage_count: 15 },
          ],
        })
      }
      return Promise.resolve({ artists: [], venues: [], releases: [], labels: [], festivals: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'post' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.tags.length).toBe(2)
    })

    const tags = result.current.data!.tags
    expect(tags[0]).toEqual({
      id: 1,
      slug: 'post-punk',
      name: 'Post-Punk',
      subtitle: 'Genre',
      entityType: 'tag',
      href: '/tags/post-punk',
    })
    expect(tags[1]).toEqual({
      id: 2,
      slug: 'phoenix',
      name: 'Phoenix',
      subtitle: 'Locale',
      entityType: 'tag',
      href: '/tags/phoenix',
    })
  })

  it('should map artist results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/artists/search')) {
        return Promise.resolve({
          artists: [
            { id: 1, slug: 'the-growlers', name: 'The Growlers', city: 'Dana Point', state: 'CA' },
            { id: 2, slug: 'eraser', name: 'Eraser', city: null, state: null },
          ],
          count: 2,
        })
      }
      return Promise.resolve({ venues: [], releases: [], labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.artists.length).toBe(2)
    })

    const artists = result.current.data!.artists
    expect(artists[0]).toEqual({
      id: 1,
      slug: 'the-growlers',
      name: 'The Growlers',
      subtitle: 'Dana Point, CA',
      entityType: 'artist',
      href: '/artists/the-growlers',
    })
    expect(artists[1]).toEqual({
      id: 2,
      slug: 'eraser',
      name: 'Eraser',
      subtitle: null,
      entityType: 'artist',
      href: '/artists/eraser',
    })
  })

  it('should map venue results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/venues/search')) {
        return Promise.resolve({
          venues: [{ id: 10, slug: 'crescent-ballroom', name: 'Crescent Ballroom', city: 'Phoenix', state: 'AZ' }],
          count: 1,
        })
      }
      return Promise.resolve({ artists: [], releases: [], labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'crescent' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.venues.length).toBe(1)
    })

    expect(result.current.data!.venues[0]).toEqual({
      id: 10,
      slug: 'crescent-ballroom',
      name: 'Crescent Ballroom',
      subtitle: 'Phoenix, AZ',
      entityType: 'venue',
      href: '/venues/crescent-ballroom',
    })
  })

  it('should map release results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/releases/search')) {
        return Promise.resolve({
          releases: [{ id: 5, slug: 'nevermind', title: 'Nevermind', release_type: 'album', release_year: 1991 }],
          count: 1,
        })
      }
      return Promise.resolve({ artists: [], venues: [], labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'nevermind' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.releases.length).toBe(1)
    })

    expect(result.current.data!.releases[0]).toEqual({
      id: 5,
      slug: 'nevermind',
      name: 'Nevermind',
      subtitle: 'album · 1991',
      entityType: 'release',
      href: '/releases/nevermind',
    })
  })

  it('should map label results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/labels/search')) {
        return Promise.resolve({
          labels: [{ id: 3, slug: 'sub-pop', name: 'Sub Pop', city: 'Seattle', state: 'WA' }],
          count: 1,
        })
      }
      return Promise.resolve({ artists: [], venues: [], releases: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'sub pop' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.labels.length).toBe(1)
    })

    expect(result.current.data!.labels[0]).toEqual({
      id: 3,
      slug: 'sub-pop',
      name: 'Sub Pop',
      subtitle: 'Seattle, WA',
      entityType: 'label',
      href: '/labels/sub-pop',
    })
  })

  it('should map festival results correctly', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/festivals/search')) {
        return Promise.resolve({
          festivals: [{ id: 7, slug: 'm3f-2026', name: 'M3F 2026', city: 'Phoenix', state: 'AZ', edition_year: 2026 }],
          count: 1,
        })
      }
      return Promise.resolve({ artists: [], venues: [], releases: [], labels: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'm3f' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.festivals.length).toBe(1)
    })

    expect(result.current.data!.festivals[0]).toEqual({
      id: 7,
      slug: 'm3f-2026',
      name: 'M3F 2026',
      subtitle: 'Phoenix, AZ · 2026',
      entityType: 'festival',
      href: '/festivals/m3f-2026',
    })
  })

  it('should limit results to 5 per type', async () => {
    const manyArtists = Array.from({ length: 10 }, (_, i) => ({
      id: i + 1,
      slug: `artist-${i + 1}`,
      name: `Artist ${i + 1}`,
      city: null,
      state: null,
    }))

    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/artists/search')) {
        return Promise.resolve({ artists: manyArtists, count: 10 })
      }
      return Promise.resolve({ venues: [], releases: [], labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'artist' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data?.artists.length).toBe(5)
    })
  })

  it('should handle individual endpoint failures gracefully', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/artists/search')) {
        return Promise.reject(new Error('Network error'))
      }
      if (url.includes('/venues/search')) {
        return Promise.resolve({
          venues: [{ id: 1, slug: 'test-venue', name: 'Test Venue', city: 'Phoenix', state: 'AZ' }],
          count: 1,
        })
      }
      return Promise.resolve({ releases: [], labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.totalResults).toBe(1)
    })

    // Artists should be empty (failed), venues should have results
    expect(result.current.data!.artists).toEqual([])
    expect(result.current.data!.venues.length).toBe(1)
  })

  it('should calculate totalResults across all entity types', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/artists/search')) {
        return Promise.resolve({
          artists: [{ id: 1, slug: 'a1', name: 'Artist 1' }],
          count: 1,
        })
      }
      if (url.includes('/venues/search')) {
        return Promise.resolve({
          venues: [{ id: 2, slug: 'v1', name: 'Venue 1', city: 'Phoenix', state: 'AZ' }],
          count: 1,
        })
      }
      if (url.includes('/releases/search')) {
        return Promise.resolve({
          releases: [
            { id: 3, slug: 'r1', title: 'Release 1' },
            { id: 4, slug: 'r2', title: 'Release 2' },
          ],
          count: 2,
        })
      }
      return Promise.resolve({ labels: [], festivals: [], tags: [], count: 0 })
    })

    const { result } = renderHook(
      () => useEntitySearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.totalResults).toBe(4) // 1 artist + 1 venue + 2 releases
    })
  })

  it('should trim whitespace from query', async () => {
    mockApiRequest.mockResolvedValue({ artists: [], venues: [], releases: [], labels: [], festivals: [], tags: [], count: 0 })

    renderHook(
      () => useEntitySearch({ query: '  ab  ' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalled()
    })

    // Should search for "ab" (trimmed)
    expect(mockApiRequest).toHaveBeenCalledWith(expect.stringContaining('q=ab'))
  })

  it('should not fetch when trimmed query is less than 2 characters', () => {
    renderHook(
      () => useEntitySearch({ query: '  a  ' }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})
