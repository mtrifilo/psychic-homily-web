import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { UnmatchedPlayGroup } from '@/lib/hooks/admin/useAdminRadio'

// PSY-1664: linking an unmatched artist shows a per-artist "Linked N plays"
// confirmation that dismisses itself after 4s. Those dismiss timers live in a
// keyed ref map so two artists linked in quick succession each get a full
// window; the map is cleared on unmount. Untracked, they still fired after the
// panel was gone and called `setState` into a torn-down React DOM, which under
// vitest lands after jsdom teardown and fails the whole run.

const group: UnmatchedPlayGroup = {
  artist_name: 'Amyl & The Sniffers',
  play_count: 7,
  station_names: ['WFMU'],
  suggested_matches: [
    { artist_id: 31, artist_name: 'Amyl and the Sniffers', artist_slug: 'amyl-and-the-sniffers' },
  ],
}

const bulkLinkMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useRadioStats: () => ({
      data: {
        total_stations: 1,
        total_shows: 1,
        total_episodes: 1,
        total_plays: 100,
        matched_plays: 93,
        unique_artists: 20,
      },
      isLoading: false,
    }),
    useAdminRadioStations: () => ({ data: { stations: [], count: 0 }, isLoading: false }),
    useUnmatchedPlays: () => ({
      data: { groups: [group], total: 1 },
      isLoading: false,
      isFetching: false,
    }),
    useBulkLinkPlays: () => ({ mutate: bulkLinkMutate, isPending: false }),
    useListStationHealth: () => ({ data: { stations: [], count: 0 } }),
    useRecentFailedRuns: () => ({ runs: [], isLoading: false, isError: false }),
    useDeleteRadioStation: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function renderMatchingTab() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
  // Radix Tabs switches on mousedown, not click.
  fireEvent.mouseDown(screen.getByRole('tab', { name: /Matching/i }))
  return view
}

describe('RadioMatchingTab success-message timer cleanup (PSY-1664)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('leaves no pending success-dismiss timer behind on unmount', () => {
    vi.useFakeTimers()
    try {
      // A successful bulk link is what arms the keyed 4000ms dismiss timer.
      bulkLinkMutate.mockImplementation(
        (_vars: unknown, opts?: { onSuccess?: (data: { updated: number }) => void }) => {
          opts?.onSuccess?.({ updated: 7 })
        }
      )

      const { unmount } = renderMatchingTab()

      // Baseline-delta rather than an absolute zero: the surrounding admin
      // shell (react-query, Radix Tabs) keeps timers of its own alive.
      const baseline = vi.getTimerCount()

      // Pick the suggested artist, then link: the Link button stays disabled
      // until a suggestion is selected.
      fireEvent.click(screen.getByRole('button', { name: 'Amyl and the Sniffers' }))
      fireEvent.click(screen.getByRole('button', { name: /Link/i }))

      expect(bulkLinkMutate).toHaveBeenCalledTimes(1)
      expect(screen.getByText('Linked 7 plays')).toBeInTheDocument()
      expect(vi.getTimerCount()).toBeGreaterThan(baseline)

      unmount()
      expect(vi.getTimerCount()).toBeLessThanOrEqual(baseline)
    } finally {
      vi.useRealTimers()
    }
  })
})
