import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { RadioSyncRun } from '@/lib/hooks/admin/useAdminRadio'

// SyncRunRow's only hook dependency is useCancelSyncRun (a useMutation). We stub
// it so the row renders standalone without a QueryClientProvider. These tests
// cover the rendered OUTPUT for the PSY-1120 fixes, carried onto the PSY-1136
// sync-run shape:
//   1. a `partial` run (imported data but hit per-episode/match errors — the old
//      "completed with errors") renders an amber warning + its categorized
//      errors[], NOT a plain green "success";
//   2. a run that imported zero plays shows "no plays found", never "0% matched";
//   3. a clean `success` run with plays still shows the "% matched" summary.
vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useCancelSyncRun: () => ({ mutate: vi.fn(), isPending: false }),
  }
})

import { SyncRunRow } from './RadioManagement'

// Minimal complete RadioSyncRun. Overrides are spread last.
function makeRun(overrides: Partial<RadioSyncRun>): RadioSyncRun {
  return {
    id: 1,
    station_id: 100,
    station_name: 'KEXP',
    show_id: 10,
    show_name: 'Morning Show',
    run_type: 'backfill',
    trigger: 'manual',
    status: 'success',
    window_start: '2025-04-01',
    window_end: '2025-04-30',
    episodes_found: 4,
    episodes_imported: 4,
    plays_imported: 0,
    plays_matched: 0,
    plays_unmatched: 0,
    current_episode_date: null,
    breaker_skipped: false,
    errors: [],
    started_at: '2025-05-01T10:00:00Z',
    finished_at: '2025-05-01T10:05:00Z',
    created_at: '2025-05-01T09:59:00Z',
    updated_at: '2025-05-01T10:05:00Z',
    ...overrides,
  }
}

describe('SyncRunRow — partial (completed-with-errors) run (PSY-1119/1120/1136)', () => {
  it('renders an amber warning (not plain green) for a partial run, with its categorized errors', () => {
    const run = makeRun({
      status: 'partial',
      episodes_found: 64,
      episodes_imported: 64,
      plays_imported: 50,
      plays_matched: 40,
      plays_unmatched: 10,
      errors: [
        { category: 'provider_unreachable', detail: 'fetch failed for episode abc: 404' },
        { category: 'match_persist_error', detail: 'could not persist match' },
      ],
    })
    render(<SyncRunRow run={run} />)

    // The status badge reads "completed with errors", not "success".
    expect(screen.getByText('completed with errors')).toBeInTheDocument()
    expect(screen.queryByText(/^success$/)).toBeNull()

    // The warning banner (error count, not plays_unmatched) + the categorized list.
    expect(screen.getByText('Completed with errors')).toBeInTheDocument()
    expect(screen.getByText(/64 episodes imported/)).toBeInTheDocument()
    expect(screen.getByText(/2 errors/)).toBeInTheDocument()
    expect(screen.getByText('provider_unreachable')).toBeInTheDocument()
    expect(screen.getByText('match_persist_error')).toBeInTheDocument()
  })

  it('does NOT render the warning for a clean success run', () => {
    const run = makeRun({
      status: 'success',
      plays_imported: 50,
      plays_matched: 40,
      errors: [],
    })
    render(<SyncRunRow run={run} />)

    expect(screen.queryByText('Completed with errors')).toBeNull()
    expect(screen.queryByText('completed with errors')).toBeNull()
    expect(screen.getByText('success')).toBeInTheDocument()
  })

  it('renders the categorized error list for a failed run (not the partial banner)', () => {
    const run = makeRun({
      status: 'failed',
      errors: [{ category: 'provider_unreachable', detail: 'provider 500' }],
    })
    render(<SyncRunRow run={run} />)

    // A failed run shows its error list, not the partial "Completed with errors" banner.
    expect(screen.queryByText('Completed with errors')).toBeNull()
    expect(screen.getByText('provider_unreachable')).toBeInTheDocument()
    expect(screen.getByText(/provider 500/)).toBeInTheDocument()
  })

  it('renders a skipped (breaker) note', () => {
    const run = makeRun({ status: 'skipped', breaker_skipped: true })
    render(<SyncRunRow run={run} />)

    expect(screen.getByText('skipped (breaker)')).toBeInTheDocument()
    expect(screen.getByText(/circuit breaker was open/)).toBeInTheDocument()
  })
})

describe('SyncRunRow — 0-plays match display (PSY-1120/1136)', () => {
  it('shows "no plays found" instead of "0% matched" for a success 0-play run', () => {
    const run = makeRun({
      status: 'success',
      episodes_found: 4,
      episodes_imported: 4,
      plays_imported: 0,
      plays_matched: 0,
    })
    render(<SyncRunRow run={run} />)

    expect(screen.queryByText(/0% matched/)).toBeNull()
    expect(screen.queryByText(/% matched/)).toBeNull()
    // Both the progress row and the completed summary say "no plays found".
    expect(screen.getAllByText(/no plays found/).length).toBeGreaterThan(0)
  })

  it('shows "% matched" for a success run that imported plays', () => {
    const run = makeRun({
      status: 'success',
      episodes_found: 4,
      episodes_imported: 4,
      plays_imported: 50,
      plays_matched: 40,
    })
    render(<SyncRunRow run={run} />)

    // 40 / 50 = 80%.
    expect(screen.getByText(/80% matched/)).toBeInTheDocument()
    expect(screen.queryByText(/no plays found/)).toBeNull()
  })

  it('shows "no plays found" in the progress row for a running 0-play run', () => {
    const run = makeRun({
      status: 'running',
      episodes_found: 4,
      episodes_imported: 1,
      plays_imported: 0,
      plays_matched: 0,
      finished_at: null,
    })
    render(<SyncRunRow run={run} />)

    expect(screen.queryByText(/0% matched/)).toBeNull()
    expect(screen.getByText(/no plays found/)).toBeInTheDocument()
  })

  it('renders window dates for a backfill run', () => {
    const run = makeRun({ status: 'running', finished_at: null })
    render(<SyncRunRow run={run} />)
    expect(screen.getByText(/2025-04-01 to 2025-04-30/)).toBeInTheDocument()
  })
})
