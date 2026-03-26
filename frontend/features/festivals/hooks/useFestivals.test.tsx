import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/features/festivals/api', () => ({
  festivalEndpoints: {
    LIST: '/festivals',
    GET: (id: string | number) => `/festivals/${id}`,
    ARTISTS: (id: string | number) => `/festivals/${id}/artists`,
    VENUES: (id: string | number) => `/festivals/${id}/venues`,
    ARTIST_FESTIVALS: (id: string | number) => `/artists/${id}/festivals`,
    SIMILAR: (id: string | number) => `/festivals/${id}/similar`,
    BREAKOUTS: (id: string | number) => `/festivals/${id}/breakouts`,
    ARTIST_TRAJECTORY: (id: string | number) => `/artists/${id}/festival-trajectory`,
    SERIES_COMPARE: (slug: string) => `/festivals/series/${slug}/compare`,
  },
  festivalQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['festivals', 'list', filters],
    detail: (id: string | number) => ['festivals', 'detail', String(id)],
    artists: (id: string | number, dayDate?: string) => ['festivals', 'artists', String(id), dayDate],
    venues: (id: string | number) => ['festivals', 'venues', String(id)],
    artistFestivals: (id: string | number) => ['festivals', 'artist', String(id)],
    similar: (id: string | number) => ['festivals', 'similar', String(id)],
    breakouts: (id: string | number) => ['festivals', 'breakouts', String(id)],
    artistTrajectory: (id: string | number) => ['festivals', 'trajectory', String(id)],
    seriesCompare: (slug: string, years: number[]) => ['festivals', 'series', slug, years.join(',')],
  },
}))

import {
  useFestivals,
  useFestival,
  useFestivalArtists,
  useFestivalLineup,
  useFestivalVenues,
  useArtistFestivals,
  useSimilarFestivals,
  useFestivalBreakouts,
  useArtistFestivalTrajectory,
  useSeriesComparison,
} from './useFestivals'


describe('useFestivals', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches festivals without filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ festivals: [], count: 0 })

    const { result } = renderHook(() => useFestivals(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals', { method: 'GET' })
  })

  it('includes year filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ festivals: [], count: 0 })

    const { result } = renderHook(() => useFestivals({ year: 2025 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('year=2025')
  })

  it('includes seriesSlug filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ festivals: [], count: 0 })

    const { result } = renderHook(() => useFestivals({ seriesSlug: 'coachella' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('series_slug=coachella')
  })
})

describe('useFestival', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a festival by slug', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'FORM Arcosanti', slug: 'form-arcosanti' })

    const { result } = renderHook(() => useFestival({ idOrSlug: 'form-arcosanti' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals/form-arcosanti', { method: 'GET' })
  })

  it('does not fetch when idOrSlug is 0', () => {
    const { result } = renderHook(() => useFestival({ idOrSlug: 0 }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useFestival({ idOrSlug: 'test', enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useFestivalArtists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches festival artists', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

    const { result } = renderHook(
      () => useFestivalArtists({ festivalIdOrSlug: 'form' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals/form/artists', { method: 'GET' })
  })

  it('includes dayDate filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

    const { result } = renderHook(
      () => useFestivalArtists({ festivalIdOrSlug: 'form', dayDate: '2025-05-09' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('day_date=2025-05-09')
  })
})

describe('useFestivalLineup (alias)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('delegates to useFestivalArtists', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

    const { result } = renderHook(
      () => useFestivalLineup({ festivalId: 'form' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals/form/artists', { method: 'GET' })
  })
})

describe('useFestivalVenues', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches venues for a festival', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [] })

    const { result } = renderHook(
      () => useFestivalVenues({ festivalIdOrSlug: 'form' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals/form/venues', { method: 'GET' })
  })
})

describe('useArtistFestivals', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches festivals for an artist', async () => {
    mockApiRequest.mockResolvedValueOnce({ festivals: [] })

    const { result } = renderHook(
      () => useArtistFestivals({ artistIdOrSlug: 'radiohead' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/radiohead/festivals', { method: 'GET' })
  })
})

describe('useSimilarFestivals', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches similar festivals with default limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ festivals: [] })

    const { result } = renderHook(
      () => useSimilarFestivals({ festivalIdOrSlug: 'form' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('limit=10')
  })
})

describe('useFestivalBreakouts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches breakout artists', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [] })

    const { result } = renderHook(
      () => useFestivalBreakouts({ festivalIdOrSlug: 'form' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/festivals/form/breakouts', { method: 'GET' })
  })
})

describe('useArtistFestivalTrajectory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches artist trajectory', async () => {
    mockApiRequest.mockResolvedValueOnce({ entries: [] })

    const { result } = renderHook(
      () => useArtistFestivalTrajectory({ artistIdOrSlug: 'radiohead' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/radiohead/festival-trajectory', { method: 'GET' })
  })
})

describe('useSeriesComparison', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches series comparison', async () => {
    mockApiRequest.mockResolvedValueOnce({ years: [] })

    const { result } = renderHook(
      () => useSeriesComparison({ seriesSlug: 'coachella', years: [2024, 2025] }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('years=2024%2C2025')
  })

  it('does not fetch when fewer than 2 years', () => {
    const { result } = renderHook(
      () => useSeriesComparison({ seriesSlug: 'coachella', years: [2024] }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when seriesSlug is empty', () => {
    const { result } = renderHook(
      () => useSeriesComparison({ seriesSlug: '', years: [2024, 2025] }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
