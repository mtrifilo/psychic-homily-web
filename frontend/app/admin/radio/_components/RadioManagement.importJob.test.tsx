import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { RadioImportJob } from '@/lib/hooks/admin/useAdminRadio'

// ImportJobRow's only hook dependency is useCancelImportJob (a useMutation). We
// stub it so the row can be rendered standalone without a QueryClientProvider.
// The point of these tests is the rendered OUTPUT for the PSY-1120 fixes:
//   1. a completed job whose error_log carries the PSY-1119 "completed with
//      errors" header renders an amber warning, NOT a plain green "completed";
//   2. a completed/running job that imported zero plays shows "no plays found",
//      never "0% matched" (which reads as a matching failure);
//   3. a clean completed job with plays still shows the "% matched" summary.
vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useCancelImportJob: () => ({ mutate: vi.fn(), isPending: false }),
  }
})

import { ImportJobRow } from './RadioManagement'

// Minimal complete RadioImportJob. Overrides are spread last.
function makeJob(overrides: Partial<RadioImportJob>): RadioImportJob {
  return {
    id: 1,
    show_id: 10,
    show_name: 'Morning Show',
    station_id: 100,
    station_name: 'KEXP',
    since: '2025-04-01',
    until: '2025-04-30',
    status: 'completed',
    episodes_found: 4,
    episodes_imported: 4,
    plays_imported: 0,
    plays_matched: 0,
    current_episode_date: null,
    error_log: null,
    started_at: '2025-05-01T10:00:00Z',
    completed_at: '2025-05-01T10:05:00Z',
    created_at: '2025-05-01T09:59:00Z',
    updated_at: '2025-05-01T10:05:00Z',
    ...overrides,
  }
}

describe('ImportJobRow — completed-with-errors warning (PSY-1119/1120)', () => {
  it('renders an amber warning (not plain green) when error_log carries the PSY-1119 header', () => {
    const job = makeJob({
      status: 'completed',
      episodes_found: 64,
      episodes_imported: 64,
      plays_imported: 50,
      plays_matched: 40,
      error_log:
        'completed with errors: 3 episodes failed to fetch, 2 play matches failed to persist\n' +
        'fetch failed for episode abc: 404\n',
    })
    render(<ImportJobRow job={job} />)

    // The status badge reads "completed with errors", not "completed".
    expect(screen.getByText('completed with errors')).toBeInTheDocument()
    // The badge label is replaced — there is no plain "completed" badge.
    expect(screen.queryByText(/^completed$/)).toBeNull()

    // The warning banner spells out the failure counts.
    const warning = screen.getByText('Completed with errors')
    expect(warning).toBeInTheDocument()
    expect(screen.getByText(/3 failed to fetch playlists/)).toBeInTheDocument()
    expect(
      screen.getByText(/2 play matches failed to persist/)
    ).toBeInTheDocument()
  })

  it('does NOT render the warning for a clean completed job (no error_log)', () => {
    const job = makeJob({
      status: 'completed',
      plays_imported: 50,
      plays_matched: 40,
      error_log: null,
    })
    render(<ImportJobRow job={job} />)

    expect(screen.queryByText('Completed with errors')).toBeNull()
    expect(screen.queryByText('completed with errors')).toBeNull()
    // The plain "completed" badge is shown.
    expect(screen.getByText('completed')).toBeInTheDocument()
  })

  it('does NOT treat a 0/0 header as a warning (clean import)', () => {
    const job = makeJob({
      status: 'completed',
      plays_imported: 10,
      plays_matched: 10,
      error_log:
        'completed with errors: 0 episodes failed to fetch, 0 play matches failed to persist\n',
    })
    render(<ImportJobRow job={job} />)

    expect(screen.queryByText('Completed with errors')).toBeNull()
    expect(screen.getByText('completed')).toBeInTheDocument()
  })

  it('does NOT render the warning for a failed job even if error_log is present', () => {
    const job = makeJob({
      status: 'failed',
      error_log: 'fatal: provider unreachable',
    })
    render(<ImportJobRow job={job} />)

    // failed jobs render their own error_log block, not the completed-with-errors banner.
    expect(screen.queryByText('Completed with errors')).toBeNull()
    expect(screen.getByText('fatal: provider unreachable')).toBeInTheDocument()
  })
})

describe('ImportJobRow — 0-plays match display (PSY-1120)', () => {
  it('shows "no plays found" instead of "0% matched" for a completed 0-play job', () => {
    const job = makeJob({
      status: 'completed',
      episodes_found: 4,
      episodes_imported: 4,
      plays_imported: 0,
      plays_matched: 0,
      error_log: null,
    })
    render(<ImportJobRow job={job} />)

    // No misleading "0% matched" anywhere.
    expect(screen.queryByText(/0% matched/)).toBeNull()
    expect(screen.queryByText(/% matched/)).toBeNull()
    // The completed summary and the progress row both say "no plays found".
    expect(screen.getAllByText(/no plays found/).length).toBeGreaterThan(0)
  })

  it('shows "% matched" for a completed job that imported plays', () => {
    const job = makeJob({
      status: 'completed',
      episodes_found: 4,
      episodes_imported: 4,
      plays_imported: 50,
      plays_matched: 40,
      error_log: null,
    })
    render(<ImportJobRow job={job} />)

    // 40 / 50 = 80%.
    expect(screen.getByText(/80% matched/)).toBeInTheDocument()
    expect(screen.queryByText(/no plays found/)).toBeNull()
  })

  it('shows "no plays found" in the progress row for a running 0-play job', () => {
    const job = makeJob({
      status: 'running',
      episodes_found: 4,
      episodes_imported: 1,
      plays_imported: 0,
      plays_matched: 0,
      completed_at: null,
    })
    render(<ImportJobRow job={job} />)

    expect(screen.queryByText(/0% matched/)).toBeNull()
    expect(screen.getByText(/no plays found/)).toBeInTheDocument()
  })
})
