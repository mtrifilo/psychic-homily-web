import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { RadioSyncRun } from '@/lib/hooks/admin/useAdminRadio'

// useTrackedSyncRun is the load-bearing abstraction PSY-1136 added: it tracks a
// triggered run, polls it, and invalidates the shows/stations/stats lists exactly
// ONCE per run when it settles. The guard keys on the run ID (not the status
// string) so two same-session runs that both end on the same terminal status each
// invalidate — the regression these tests pin. We mock useSyncRun (same module)
// to drive the polled run deterministically without timers/real HTTP.
let syncRunData: RadioSyncRun | undefined
let syncRunIsError = false

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useSyncRun: (runId: number, enabled = true) => ({
      data: enabled && runId > 0 ? syncRunData : undefined,
      isError: syncRunIsError,
    }),
  }
})

import { useTrackedSyncRun } from './RadioManagement'

function makeRun(id: number, status: RadioSyncRun['status']): RadioSyncRun {
  return {
    id,
    station_id: 1,
    station_name: 'KEXP',
    run_type: 'backfill',
    trigger: 'manual',
    status,
    episodes_found: 0,
    episodes_imported: 0,
    plays_imported: 0,
    plays_matched: 0,
    plays_unmatched: 0,
    breaker_skipped: false,
    started_at: '2026-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  const setDataSpy = vi.spyOn(queryClient, 'setQueryData')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const view = renderHook(() => useTrackedSyncRun(1), { wrapper })
  return { ...view, invalidateSpy, setDataSpy }
}

describe('useTrackedSyncRun (PSY-1136)', () => {
  beforeEach(() => {
    syncRunData = undefined
    syncRunIsError = false
    vi.clearAllMocks()
  })

  it('seeds the run into the cache and flips isRunning immediately on trackRun', () => {
    const { result, rerender, setDataSpy } = setup()
    const run = makeRun(7, 'running')
    syncRunData = run
    act(() => {
      result.current.trackRun(run)
    })
    rerender()

    expect(setDataSpy).toHaveBeenCalledWith(['radio', 'sync-runs', 7], run)
    expect(result.current.isRunning).toBe(true)
  })

  it('invalidates the lists once when a run settles, and not while running', () => {
    const { result, rerender, invalidateSpy } = setup()
    syncRunData = makeRun(7, 'running')
    act(() => {
      result.current.trackRun(syncRunData!)
    })
    rerender()
    expect(invalidateSpy).not.toHaveBeenCalled() // running → no invalidation

    syncRunData = makeRun(7, 'success')
    rerender()
    const afterSettle = invalidateSpy.mock.calls.length
    expect(afterSettle).toBeGreaterThanOrEqual(3) // shows + stations + stats

    rerender() // still success
    expect(invalidateSpy.mock.calls.length).toBe(afterSettle) // guard: no double-invalidate
  })

  it('invalidates AGAIN for a second run that ends on the same terminal status', () => {
    const { result, rerender, invalidateSpy } = setup()
    syncRunData = makeRun(7, 'success')
    act(() => {
      result.current.trackRun(syncRunData!)
    })
    rerender()
    const afterFirst = invalidateSpy.mock.calls.length
    expect(afterFirst).toBeGreaterThanOrEqual(3)

    // Second run, SAME terminal status string. A boolean once-guard would skip
    // this; the per-runId guard must invalidate again.
    syncRunData = makeRun(8, 'success')
    act(() => {
      result.current.trackRun(syncRunData!)
    })
    rerender()
    expect(invalidateSpy.mock.calls.length).toBeGreaterThan(afterFirst)
  })

  it('surfaces a poll error via isError and clears isRunning even with a retained running run', () => {
    const { result, rerender } = setup()
    // react-query retains the seeded `running` data on a fetch error; isRunning
    // must still go false so the trigger button re-enables and the consumer shows
    // the error instead of a frozen "running" row.
    syncRunData = makeRun(7, 'running')
    syncRunIsError = true
    act(() => {
      result.current.trackRun(makeRun(7, 'running'))
    })
    rerender()
    expect(result.current.isError).toBe(true)
    expect(result.current.isRunning).toBe(false)
  })
})
