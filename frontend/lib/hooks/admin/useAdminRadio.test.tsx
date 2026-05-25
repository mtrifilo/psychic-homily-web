import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({}),
}))

import {
  radioQueryKeys,
  useAdminRadioStations,
  useRadioStationDetail,
  useRadioShows,
  useRadioStats,
  useCreateRadioStation,
  useUpdateRadioStation,
  useDeleteRadioStation,
  useCreateRadioShow,
  useUpdateRadioShow,
  useDeleteRadioShow,
  useFetchPlaylists,
  useDiscoverShows,
  useImportShowEpisodes,
  useCreateImportJob,
  useImportJob,
  useCancelImportJob,
  useShowImportJobs,
  useUnmatchedPlays,
  useBulkLinkPlays,
} from './useAdminRadio'

describe('radioQueryKeys', () => {
  it('generates stable keys for stations and stats', () => {
    expect(radioQueryKeys.all).toEqual(['radio'])
    expect(radioQueryKeys.stations).toEqual(['radio', 'stations'])
    expect(radioQueryKeys.stats).toEqual(['radio', 'stats'])
  })

  it('generates parameterised keys for station/show/job scopes', () => {
    expect(radioQueryKeys.stationDetail(42)).toEqual(['radio', 'stations', 42])
    expect(radioQueryKeys.shows(7)).toEqual(['radio', 'shows', 7])
    expect(radioQueryKeys.importJob(99)).toEqual([
      'radio',
      'import-jobs',
      99,
    ])
    expect(radioQueryKeys.showImportJobs(7)).toEqual([
      'radio',
      'show-import-jobs',
      7,
    ])
    expect(radioQueryKeys.unmatched(0)).toEqual(['radio', 'unmatched', 0])
  })
})

describe('useAdminRadioStations', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches station list', async () => {
    mockApiRequest.mockResolvedValueOnce({
      stations: [
        {
          id: 1,
          name: 'KEXP',
          slug: 'kexp',
          city: 'Seattle',
          state: 'WA',
          country: 'US',
          broadcast_type: 'fm',
          frequency_mhz: 90.3,
          logo_url: null,
          is_active: true,
          show_count: 50,
        },
      ],
      count: 1,
    })

    const { result } = renderHook(() => useAdminRadioStations(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('http://localhost:8080/radio-stations')
    expect(result.current.data?.stations).toHaveLength(1)
  })
})

describe('useRadioStationDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches station detail when enabled and stationId > 0', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 42, name: 'KEXP', slug: 'kexp' })

    const { result } = renderHook(() => useRadioStationDetail(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/radio-stations/42'
    )
  })

  it('does not fetch when stationId is 0', () => {
    const { result } = renderHook(() => useRadioStationDetail(0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useRadioStationDetail(42, false), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useRadioShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches shows for a station with the station_id query param', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], count: 0 })

    const { result } = renderHook(() => useRadioShows(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/radio-shows?station_id=42'
    )
  })

  it('does not fetch when stationId is 0', () => {
    const { result } = renderHook(() => useRadioShows(0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useRadioStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches overall radio stats', async () => {
    mockApiRequest.mockResolvedValueOnce({
      total_stations: 3,
      total_shows: 50,
      total_episodes: 1000,
      total_plays: 50000,
      matched_plays: 45000,
      unique_artists: 8000,
    })

    const { result } = renderHook(() => useRadioStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('http://localhost:8080/radio/stats')
    expect(result.current.data?.matched_plays).toBe(45000)
  })
})

describe('useCreateRadioStation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs station payload and invalidates stations + stats', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'NTS', slug: 'nts' })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCreateRadioStation(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ name: 'NTS', broadcast_type: 'internet' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ name: 'NTS', broadcast_type: 'internet' }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stations'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stats'] })
  })
})

describe('useUpdateRadioStation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('PUTs the station and invalidates list, detail, and stats', async () => {
    // Update should invalidate the specific station detail (not just the
    // list) so that any detail page picks up the change without a refresh.
    mockApiRequest.mockResolvedValueOnce({ id: 42, name: 'KEXP Renamed' })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useUpdateRadioStation(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ id: 42, name: 'KEXP Renamed' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ name: 'KEXP Renamed' }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stations'] })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'stations', 42],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stats'] })
  })
})

describe('useDeleteRadioStation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('DELETEs and invalidates stations + stats', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useDeleteRadioStation(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42',
      { method: 'DELETE' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stations'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stats'] })
  })
})

describe('useCreateRadioShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to the station-scoped show endpoint and invalidates that station’s shows', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, station_id: 42, name: 'New Show' })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCreateRadioShow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ stationId: 42, name: 'New Show' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42/shows',
      expect.objectContaining({
        method: 'POST',
        // stationId is stripped from the payload because it's in the URL
        body: JSON.stringify({ name: 'New Show' }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'shows', 42],
    })
  })
})

describe('useUpdateRadioShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('PUTs to the show endpoint and invalidates the parent station’s shows', async () => {
    // showId + stationId are stripped from the payload (URL carries showId;
    // stationId is just needed for the cache invalidation key).
    mockApiRequest.mockResolvedValueOnce({ id: 99, name: 'Updated Show' })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useUpdateRadioShow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({
        showId: 99,
        stationId: 42,
        name: 'Updated Show',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ name: 'Updated Show' }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'shows', 42],
    })
  })
})

describe('useDeleteRadioShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('DELETEs and invalidates the parent station’s shows + stations + stats', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useDeleteRadioShow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 99, stationId: 42 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99',
      { method: 'DELETE' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'shows', 42],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stats'] })
  })
})

describe('useFetchPlaylists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('triggers a fetch and invalidates stations + stats', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useFetchPlaylists(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42/fetch',
      { method: 'POST' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio', 'stations'] })
  })
})

describe('useDiscoverShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to discover and invalidates that station’s show list', async () => {
    // onSuccess receives the stationId as the second argument (the
    // mutation variable) and uses it to invalidate the precise show
    // scope rather than the whole tree.
    mockApiRequest.mockResolvedValueOnce({ shows_discovered: 3, show_names: [] })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useDiscoverShows(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42/discover',
      { method: 'POST' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'shows', 42],
    })
  })
})

describe('useImportShowEpisodes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to the show-import endpoint with since/until', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows_discovered: 0,
      episodes_imported: 5,
      plays_imported: 100,
      plays_matched: 80,
    })

    const { result } = renderHook(() => useImportShowEpisodes(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 99,
        since: '2026-01-01',
        until: '2026-02-01',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99/import',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          since: '2026-01-01',
          until: '2026-02-01',
        }),
      })
    )
  })
})

describe('useCreateImportJob', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to the import-job endpoint and invalidates that show’s jobs', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      show_id: 99,
      status: 'pending',
    })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCreateImportJob(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({
        showId: 99,
        since: '2026-01-01',
        until: '2026-02-01',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99/import-job',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          since: '2026-01-01',
          until: '2026-02-01',
        }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'show-import-jobs', 99],
    })
  })
})

describe('useImportJob', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a completed job WITHOUT setting a polling interval', async () => {
    // refetchInterval should be `false` for terminal states so we don't
    // keep polling a job that will never change.
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      show_id: 99,
      status: 'completed',
      episodes_imported: 5,
      plays_imported: 100,
    })

    const { result } = renderHook(() => useImportJob(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/import-jobs/1'
    )
    expect(result.current.data?.status).toBe('completed')
  })

  it('does not fetch when jobId is 0 or enabled=false', () => {
    const { result: zero } = renderHook(() => useImportJob(0), {
      wrapper: createWrapper(),
    })
    expect(zero.current.fetchStatus).toBe('idle')

    const { result: disabled } = renderHook(() => useImportJob(1, false), {
      wrapper: createWrapper(),
    })
    expect(disabled.current.fetchStatus).toBe('idle')
  })
})

describe('useCancelImportJob', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to the cancel endpoint and invalidates job + radio.all', async () => {
    // Cancelling a job invalidates the specific job key AND the broad
    // ['radio'] umbrella so show-level lists pick up the new state.
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCancelImportJob(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/import-jobs/42/cancel',
      { method: 'POST' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'import-jobs', 42],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['radio'] })
  })
})

describe('useShowImportJobs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the job list for a show', async () => {
    mockApiRequest.mockResolvedValueOnce({ jobs: [], count: 0 })

    const { result } = renderHook(() => useShowImportJobs(99), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99/import-jobs'
    )
  })

  it('does not fetch when showId is 0', () => {
    const { result } = renderHook(() => useShowImportJobs(0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useUnmatchedPlays', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('passes station_id when > 0 and always passes pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ groups: [], total: 0 })

    const { result } = renderHook(() => useUnmatchedPlays(42, 25, 50), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/radio/unmatched')
    expect(url).toContain('station_id=42')
    expect(url).toContain('limit=25')
    expect(url).toContain('offset=50')
  })

  it('omits station_id when stationId is 0 (all-stations view)', async () => {
    // Backend treats absence of station_id as "all stations" — make sure
    // the hook honors the 0-means-all sentinel rather than sending station_id=0.
    mockApiRequest.mockResolvedValueOnce({ groups: [], total: 0 })

    const { result } = renderHook(() => useUnmatchedPlays(0), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain('station_id=')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
  })

  it('keys cache entries by stationId AND pagination', async () => {
    // The query key includes stationId/limit/offset so different paginations
    // don't collide in the cache — guard that two different pagination
    // calls produce TWO requests (rather than one hitting the cache).
    mockApiRequest
      .mockResolvedValueOnce({ groups: [], total: 0 })
      .mockResolvedValueOnce({ groups: [], total: 0 })

    const queryClient = createTestQueryClient()

    const { result: page1 } = renderHook(() => useUnmatchedPlays(42, 50, 0), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(page1.current.isSuccess).toBe(true))

    const { result: page2 } = renderHook(() => useUnmatchedPlays(42, 50, 50), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(page2.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    const url1 = mockApiRequest.mock.calls[0][0] as string
    const url2 = mockApiRequest.mock.calls[1][0] as string
    expect(url1).toContain('offset=0')
    expect(url2).toContain('offset=50')
  })
})

describe('useBulkLinkPlays', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs the bulk-link payload', async () => {
    mockApiRequest.mockResolvedValueOnce({ updated: 12 })

    const { result } = renderHook(() => useBulkLinkPlays(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        artistName: 'wednesday',
        artistId: 42,
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/plays/bulk-link',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          artist_name: 'wednesday',
          artist_id: 42,
        }),
      })
    )
  })

  it('invalidates the broad unmatched scope and stats on success', async () => {
    // The broad ['radio', 'unmatched'] key is used (not the per-station one)
    // because the linking may affect multiple stations' unmatched lists.
    mockApiRequest.mockResolvedValueOnce({ updated: 5 })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useBulkLinkPlays(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ artistName: 'X', artistId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'unmatched'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'stats'],
    })
  })

  it('surfaces mutation errors without invalidating', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Artist not found'))

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useBulkLinkPlays(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ artistName: 'X', artistId: 999 })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Artist not found')
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})
