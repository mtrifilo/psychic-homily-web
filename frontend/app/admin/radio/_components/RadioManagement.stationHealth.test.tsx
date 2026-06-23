import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioStationHealth,
} from '@/lib/hooks/admin/useAdminRadio'
import { RadioManagement, deriveStationHealthLevel } from './RadioManagement'

// PSY-1200: station health cards + the traffic-light breach derivation.

function makeHealth(overrides: Partial<RadioStationHealth> = {}): RadioStationHealth {
  return {
    station_id: 2,
    station_name: 'WFMU',
    station_slug: 'wfmu',
    last_success_at: '2026-06-23T05:00:00Z',
    last_run_at: '2026-06-23T05:00:00Z',
    consecutive_failures: 0,
    breaker_state: 'closed',
    breaker_tripped_at: null,
    recent_success_rate: 0.95,
    play_match_rate: 0.8,
    zero_play_episode_rate: 0.05,
    updated_at: '2026-06-23T05:00:00Z',
    ...overrides,
  }
}

describe('deriveStationHealthLevel (PSY-1200)', () => {
  it('is unknown for a station that has never run', () => {
    expect(deriveStationHealthLevel(makeHealth({ last_run_at: null }))).toBe('unknown')
  })

  it('is healthy with recent success, good rates, no failures', () => {
    expect(deriveStationHealthLevel(makeHealth())).toBe('healthy')
  })

  it('breaches when the breaker is open', () => {
    expect(deriveStationHealthLevel(makeHealth({ breaker_state: 'open' }))).toBe('breach')
  })

  it('breaches at 3+ consecutive failures', () => {
    expect(deriveStationHealthLevel(makeHealth({ consecutive_failures: 3 }))).toBe('breach')
  })

  it('breaches on a low success rate', () => {
    expect(deriveStationHealthLevel(makeHealth({ recent_success_rate: 0.4 }))).toBe('breach')
  })

  it('breaches when chronically empty (high zero-play-episode rate)', () => {
    // The KEXP day-one case: syncs "succeed" but return nothing.
    expect(
      deriveStationHealthLevel(
        makeHealth({ recent_success_rate: 1, zero_play_episode_rate: 0.9 })
      )
    ).toBe('breach')
  })

  it('warns at 1-2 consecutive failures', () => {
    expect(deriveStationHealthLevel(makeHealth({ consecutive_failures: 1 }))).toBe('warning')
  })

  it('warns on a low play-match rate', () => {
    expect(deriveStationHealthLevel(makeHealth({ play_match_rate: 0.2 }))).toBe('warning')
  })

  it('warns on a moderately empty episode rate', () => {
    expect(deriveStationHealthLevel(makeHealth({ zero_play_episode_rate: 0.6 }))).toBe('warning')
  })

  it('does not let nil (never-computed) rates trigger a level on their own', () => {
    expect(
      deriveStationHealthLevel(
        makeHealth({
          recent_success_rate: null,
          play_match_rate: null,
          zero_play_episode_rate: null,
        })
      )
    ).toBe('healthy')
  })
})

// ---- Component rendering ----

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
  show_count: 0,
}

let singleHealth: RadioStationHealth = makeHealth()
let bulkHealth: RadioStationHealth[] = [makeHealth()]

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useAdminRadioStations: () => ({ data: { stations: [station], count: 1 }, isLoading: false }),
    useRadioStationDetail: () => ({ data: undefined }),
    useRadioShows: () => ({ data: { shows: [], count: 0 }, isLoading: false }),
    useStationSyncRuns: () => ({ data: { sync_runs: [], total: 0, count: 0 }, isLoading: false, isError: false }),
    useRecentFailedRuns: () => ({ runs: [], isLoading: false, isError: false }),
    useStationHealth: () => ({ data: singleHealth, isLoading: false, isError: false }),
    useListStationHealth: () => ({ data: { stations: bulkHealth, count: bulkHealth.length } }),
    useDeleteRadioStation: noopMutation,
    useTriggerStationSync: noopMutation,
    useDeleteRadioShow: noopMutation,
    useTriggerShowBackfill: noopMutation,
    useSyncRun: () => ({ data: undefined }),
    useCancelSyncRun: noopMutation,
  }
})

function renderMgmt() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
}

describe('Station health UI (PSY-1200)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    singleHealth = makeHealth()
    bulkHealth = [makeHealth()]
  })

  it('shows the health badge in the stations list', () => {
    bulkHealth = [makeHealth({ breaker_state: 'open' })]
    renderMgmt()
    expect(screen.getByText('Needs attention')).toBeInTheDocument()
  })

  it('renders the health card with metrics in the station detail', () => {
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    expect(screen.getByText('Station health')).toBeInTheDocument()
    expect(screen.getByText('Success rate')).toBeInTheDocument()
    expect(screen.getByText('95%')).toBeInTheDocument() // recent_success_rate 0.95
    expect(screen.getByText('Play-match rate')).toBeInTheDocument()
  })

  it('renders a nil rate as an em-dash, not 0%', () => {
    singleHealth = makeHealth({ play_match_rate: null })
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('emphasises a breach in the detail card', () => {
    singleHealth = makeHealth({ consecutive_failures: 5, breaker_state: 'open' })
    renderMgmt()
    fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
    // The card header badge shows the breach label.
    expect(screen.getByText('Needs attention')).toBeInTheDocument()
  })
})
