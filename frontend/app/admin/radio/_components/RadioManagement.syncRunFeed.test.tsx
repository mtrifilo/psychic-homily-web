import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioSyncRun,
  SyncRunListResult,
} from '@/lib/hooks/admin/useAdminRadio'

// PSY-1130: admin sync-run feed — per-station history + global recent-failures view.

const station: RadioStationListItem = {
  id: 7,
  name: 'WFMU',
  slug: 'wfmu',
  city: 'Jersey City',
  state: 'NJ',
  country: 'US',
  broadcast_type: 'terrestrial',
  frequency_mhz: 91.1,
  logo_url: null,
  is_active: true,
  show_count: 0,
}

function makeRun(overrides: Partial<RadioSyncRun> = {}): RadioSyncRun {
  return {
    id: 1,
    station_id: 7,
    station_name: 'WFMU',
    run_type: 'fetch',
    trigger: 'scheduled',
    status: 'success',
    episodes_found: 0,
    episodes_imported: 0,
    plays_imported: 0,
    plays_matched: 0,
    plays_unmatched: 0,
    breaker_skipped: false,
    started_at: '2026-06-23T05:00:00Z',
    created_at: '2026-06-23T05:00:00Z',
    updated_at: '2026-06-23T05:00:00Z',
    ...overrides,
  }
}

// Mutable per-test state the mock reads.
let stationRuns: SyncRunListResult = { sync_runs: [], total: 0, count: 0 }
let stationRunsLoading = false
let stationRunsError = false
let recentFailures: { runs: RadioSyncRun[]; isLoading: boolean; isError: boolean } = {
  runs: [],
  isLoading: false,
  isError: false,
}

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useAdminRadioStations: () => ({ data: { stations: [station], count: 1 }, isLoading: false }),
    useRadioStationDetail: () => ({ data: undefined }),
    useRadioShows: () => ({ data: { shows: [], count: 0 }, isLoading: false }),
    useStationSyncRuns: () => ({
      data: stationRuns,
      isLoading: stationRunsLoading,
      isError: stationRunsError,
    }),
    useRecentFailedRuns: () => recentFailures,
    useDeleteRadioStation: noopMutation,
    useTriggerStationSync: noopMutation,
    useDeleteRadioShow: noopMutation,
    useTriggerShowBackfill: noopMutation,
    useSyncRun: () => ({ data: undefined }),
    useCancelSyncRun: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function renderMgmt() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
}

describe('Sync-run feed (PSY-1130)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stationRuns = { sync_runs: [], total: 0, count: 0 }
    stationRunsLoading = false
    stationRunsError = false
    recentFailures = { runs: [], isLoading: false, isError: false }
  })

  it('shows a positive empty state when there are no recent failures', () => {
    renderMgmt()
    expect(screen.getByText('No recent sync failures.')).toBeInTheDocument()
  })

  it('renders the global recent-failures panel with station + error summary', () => {
    recentFailures = {
      runs: [
        makeRun({
          id: 11,
          status: 'partial',
          errors: [{ category: 'empty_unexpected', detail: 'fetched 12% of trailing mean' }],
        }),
      ],
      isLoading: false,
      isError: false,
    }
    renderMgmt()
    expect(screen.getByText('Recent sync failures')).toBeInTheDocument()
    // Anomaly surfaced via the error summary.
    expect(screen.getByText(/empty_unexpected/)).toBeInTheDocument()
  })

  it('clicking a global failure opens that station detail with its sync-run feed', () => {
    recentFailures = {
      runs: [makeRun({ id: 12, status: 'failed', errors: [{ category: 'fetch_failed' }] })],
      isLoading: false,
      isError: false,
    }
    stationRuns = {
      sync_runs: [makeRun({ id: 12, status: 'failed', errors: [{ category: 'fetch_failed' }] })],
      total: 1,
      count: 1,
    }
    renderMgmt()
    // Target the compact failure row by its unique error text (the station table row
    // also contains "WFMU"); its accessible name includes the error category.
    fireEvent.click(screen.getByRole('button', { name: /fetch_failed/i }))
    // Station detail renders the per-station feed.
    expect(screen.getByText('Recent sync runs')).toBeInTheDocument()
  })

  it('per-station feed: empty state when the station has no runs', () => {
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    expect(screen.getByText('No sync runs recorded for this station yet.')).toBeInTheDocument()
  })

  it('per-station feed: a failed run is visually distinct and shows its error', () => {
    stationRuns = {
      sync_runs: [
        makeRun({ id: 5, status: 'failed', errors: [{ category: 'scrape_error', detail: 'HTTP 503' }] }),
      ],
      total: 1,
      count: 1,
    }
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    const feedHeading = screen.getByText('Recent sync runs')
    expect(feedHeading).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText(/scrape_error/)).toBeInTheDocument()
  })

  it('per-station feed: error state handled', () => {
    stationRunsError = true
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    expect(screen.getByText(/Couldn.t load sync runs/)).toBeInTheDocument()
  })
})
