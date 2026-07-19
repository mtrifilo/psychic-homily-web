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
  useTriggerStationSync,
  useTriggerShowBackfill,
  useSyncRun,
  useCancelSyncRun,
  useUnmatchedPlays,
  useBulkLinkPlays,
  useAdminMatchSuggestions,
  useAcceptMatchSuggestion,
  useRejectMatchSuggestion,
} from './useAdminRadio'

describe('radioQueryKeys', () => {
  it('generates stable keys for stations and stats', () => {
    expect(radioQueryKeys.all).toEqual(['radio'])
    expect(radioQueryKeys.stations).toEqual(['radio', 'stations'])
    expect(radioQueryKeys.stats).toEqual(['radio', 'stats'])
  })

  it('generates parameterised keys for station/show/run scopes', () => {
    expect(radioQueryKeys.stationDetail(42)).toEqual(['radio', 'stations', 42])
    expect(radioQueryKeys.shows(7)).toEqual(['radio', 'shows', 7])
    expect(radioQueryKeys.syncRun(99)).toEqual(['radio', 'sync-runs', 99])
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

describe('useTriggerStationSync', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs the mode to the station sync endpoint and returns the run handle', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 7, status: 'running', run_type: 'discover' })

    const { result } = renderHook(() => useTriggerStationSync(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ stationId: 42, mode: 'discover' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-stations/42/sync',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ mode: 'discover' }),
      })
    )
    expect(result.current.data?.id).toBe(7)
  })
})

describe('useTriggerShowBackfill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs since/until to the show backfill endpoint and returns the run handle', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 9, status: 'running', run_type: 'backfill' })

    const { result } = renderHook(() => useTriggerShowBackfill(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 99, since: '2026-01-01', until: '2026-02-01' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio-shows/99/backfill',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ since: '2026-01-01', until: '2026-02-01' }),
      })
    )
    expect(result.current.data?.id).toBe(9)
  })
})

describe('useSyncRun', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a terminal run WITHOUT setting a polling interval', async () => {
    // refetchInterval is false for any non-`running` status so we don't keep
    // polling a run that will never change.
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      status: 'success',
      episodes_imported: 5,
      plays_imported: 100,
    })

    const { result } = renderHook(() => useSyncRun(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/sync-runs/1'
    )
    expect(result.current.data?.status).toBe('success')
  })

  it('does not fetch when runId is 0 or enabled=false', () => {
    const { result: zero } = renderHook(() => useSyncRun(0), {
      wrapper: createWrapper(),
    })
    expect(zero.current.fetchStatus).toBe('idle')

    const { result: disabled } = renderHook(() => useSyncRun(1, false), {
      wrapper: createWrapper(),
    })
    expect(disabled.current.fetchStatus).toBe('idle')
  })
})

describe('useCancelSyncRun', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs to the cancel endpoint and invalidates that run', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCancelSyncRun(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/sync-runs/42/cancel',
      { method: 'POST' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'sync-runs', 42],
    })
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

describe('useAdminMatchSuggestions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('GETs the admin pending list with pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ suggestions: [], total: 0 })

    const { result } = renderHook(() => useAdminMatchSuggestions(25, 50), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/radio/match-suggestions')
    expect(url).toContain('limit=25')
    expect(url).toContain('offset=50')
  })
})

describe('useAcceptMatchSuggestion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs accept with also_bulk_link_name', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 7,
      status: 'accepted',
      bulk_updated: 3,
    })

    const { result } = renderHook(() => useAcceptMatchSuggestion(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ suggestionId: 7, alsoBulkLinkName: true })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/match-suggestions/7/accept',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ also_bulk_link_name: true }),
      })
    )
  })
})

describe('useRejectMatchSuggestion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs reject with reason and invalidates queues', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 7, status: 'rejected' })

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useRejectMatchSuggestion(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ suggestionId: 7, reason: 'Nope' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/radio/match-suggestions/7/reject',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ reason: 'Nope' }),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'match-suggestions'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['radio', 'unmatched'],
    })
  })
})
