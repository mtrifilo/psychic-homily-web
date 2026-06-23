import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioShowListItem,
} from '@/lib/hooks/admin/useAdminRadio'

// PSY-1122: navigable per-station show list — search / filter / sort / paginate + bulk
// lifecycle. Default view is active-first (retired hidden); 0-episode shows are
// de-emphasised; bulk sets lifecycle via the PSY-1172 write path.

const station: RadioStationListItem = {
  id: 2,
  name: 'WFMU',
  slug: 'wfmu',
  city: 'Jersey City',
  state: 'NJ',
  country: 'US',
  broadcast_type: 'terrestrial',
  frequency_mhz: 91.1,
  logo_url: null,
  is_active: true,
  show_count: 4,
}

function makeShow(o: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 2,
    station_name: 'WFMU',
    name: 'A Show',
    slug: 'a-show',
    host_name: 'Some Host',
    genre_tags: null,
    image_url: null,
    is_active: true,
    schedule_locked: false,
    lifecycle_state: 'active',
    latest_air_date: '2026-06-20',
    episode_count: 5,
    ...o,
  }
}

let shows: RadioShowListItem[] = []
const bulkMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useAdminRadioStations: () => ({ data: { stations: [station], count: 1 }, isLoading: false }),
    useRadioStationDetail: () => ({ data: undefined }),
    useRadioShows: () => ({ data: { shows, count: shows.length }, isLoading: false }),
    useStationSyncRuns: () => ({ data: { sync_runs: [], total: 0, count: 0 }, isLoading: false, isError: false }),
    useRecentFailedRuns: () => ({ runs: [], isLoading: false, isError: false }),
    useStationHealth: () => ({ data: undefined, isLoading: false, isError: false }),
    useListStationHealth: () => ({ data: { stations: [], count: 0 } }),
    useBulkSetShowLifecycle: () => ({ mutate: bulkMutate, isPending: false }),
    useDeleteRadioStation: noopMutation,
    useTriggerStationSync: noopMutation,
    useDeleteRadioShow: noopMutation,
    useTriggerShowBackfill: noopMutation,
    useSyncRun: () => ({ data: undefined }),
    useCancelSyncRun: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function openStation() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
}

describe('Navigable show list (PSY-1122)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    shows = [
      makeShow({ id: 1, name: 'Morning Drive', lifecycle_state: 'active', episode_count: 10 }),
      makeShow({ id: 2, name: 'Late Dormant', lifecycle_state: 'dormant', episode_count: 2 }),
      makeShow({ id: 3, name: 'Old Retired', lifecycle_state: 'retired', episode_count: 40 }),
      makeShow({ id: 4, name: 'Empty Newbie', lifecycle_state: 'active', episode_count: 0, latest_air_date: null }),
    ]
  })

  it('default view hides retired shows (active-first)', () => {
    openStation()
    expect(screen.getByText('Morning Drive')).toBeInTheDocument()
    expect(screen.getByText('Late Dormant')).toBeInTheDocument()
    expect(screen.queryByText('Old Retired')).toBeNull() // retired hidden by default
  })

  it('de-emphasises 0-episode shows with a "no episodes" note', () => {
    openStation()
    expect(screen.getByText('Empty Newbie')).toBeInTheDocument()
    expect(screen.getByText('no episodes')).toBeInTheDocument()
  })

  it('search filters by show name', () => {
    openStation()
    fireEvent.change(screen.getByLabelText('Search shows'), { target: { value: 'morning' } })
    expect(screen.getByText('Morning Drive')).toBeInTheDocument()
    expect(screen.queryByText('Late Dormant')).toBeNull()
  })

  it('search filters by host name', () => {
    shows = [
      makeShow({ id: 1, name: 'Show One', host_name: 'DJ Alpha' }),
      makeShow({ id: 2, name: 'Show Two', host_name: 'DJ Beta' }),
    ]
    openStation()
    fireEvent.change(screen.getByLabelText('Search shows'), { target: { value: 'alpha' } })
    expect(screen.getByText('Show One')).toBeInTheDocument()
    expect(screen.queryByText('Show Two')).toBeNull()
  })

  it('shows an empty state when nothing matches the search', () => {
    openStation()
    fireEvent.change(screen.getByLabelText('Search shows'), { target: { value: 'zzzz-nothing' } })
    expect(screen.getByText('No shows match your search / filter.')).toBeInTheDocument()
  })

  it('bulk-sets lifecycle on selected shows via the write path', () => {
    openStation()
    // Select one show, then apply a bulk "Retired".
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select Morning Drive' }))
    expect(screen.getByText('1 selected')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retired' }))
    expect(bulkMutate).toHaveBeenCalledTimes(1)
    const [payload] = bulkMutate.mock.calls[0]
    expect(payload).toMatchObject({ showIds: [1], stationId: 2, lifecycleState: 'retired' })
  })

  it('surfaces a bulk partial-failure error', () => {
    // mutate invokes the caller's onError (a partial Promise.allSettled failure).
    bulkMutate.mockImplementationOnce((_vars, opts) =>
      opts?.onError?.(new Error('Updated 1 of 2 show(s); 1 failed.'))
    )
    openStation()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select Morning Drive' }))
    fireEvent.click(screen.getByRole('button', { name: 'Retired' }))
    expect(screen.getByText('Updated 1 of 2 show(s); 1 failed.')).toBeInTheDocument()
    // Selection is kept on failure so the operator can retry.
    expect(screen.getByText('1 selected')).toBeInTheDocument()
  })
})
