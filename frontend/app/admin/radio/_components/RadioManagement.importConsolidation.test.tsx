import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioShowListItem,
} from '@/lib/hooks/admin/useAdminRadio'

// PSY-1120 consolidated the two parallel import systems down to one
// ShowImportSection; PSY-1136 then repointed it at the unified sync-run backend
// (the per-show "Backfill Episodes" affordance). This test asserts that exactly
// ONE backfill affordance renders on an expanded show row and that the deleted
// one-shot "Since"/"Until" controls are gone.

const listItem: RadioStationListItem = {
  id: 1,
  name: 'KEXP',
  slug: 'kexp',
  city: 'Seattle',
  state: 'WA',
  country: 'US',
  broadcast_type: 'terrestrial',
  frequency_mhz: 90.3,
  logo_url: null,
  is_active: true,
  show_count: 1,
}

const show: RadioShowListItem = {
  id: 50,
  station_id: 1,
  station_name: 'KEXP',
  name: 'Morning Show',
  slug: 'morning-show',
  host_name: 'DJ Cool',
  genre_tags: null,
  image_url: null,
  is_active: true,
  schedule_locked: false,
  lifecycle_state: 'active',
  latest_air_date: null,
  episode_count: 3,
}

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useAdminRadioStations: () => ({
      data: { stations: [listItem], count: 1 },
      isLoading: false,
    }),
    useRadioStationDetail: () => ({ data: undefined }),
    useRadioShows: () => ({ data: { shows: [show], count: 1 }, isLoading: false }),
    useDeleteRadioStation: noopMutation,
    useTriggerStationSync: noopMutation,
    useDeleteRadioShow: noopMutation,
    // Unified sync triggers (PSY-1136): no active run.
    useTriggerShowBackfill: noopMutation,
    useSyncRun: () => ({ data: undefined }),
    useCancelSyncRun: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function openShowImport() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
  // Open the station detail.
  fireEvent.click(screen.getByRole('button', { name: /Station: KEXP/i }))
  // Expand the show's backfill panel.
  fireEvent.click(
    screen.getByRole('button', { name: /Backfill episodes for Morning Show/i })
  )
}

describe('Backfill affordance — one per show (PSY-1120/1136)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders exactly one "Backfill Episodes" affordance', () => {
    openShowImport()

    const backfillButtons = screen.getAllByRole('button', {
      name: /^Backfill Episodes$/,
    })
    expect(backfillButtons).toHaveLength(1)
  })

  it('does not render the deleted one-shot Since/Until controls', () => {
    openShowImport()

    // The backfill form uses "From"/"To" labels and only appears after clicking
    // the trigger; the deleted one-shot path rendered "Since"/"Until" inputs
    // inline immediately on expand.
    expect(screen.queryByText('Since')).toBeNull()
    expect(screen.queryByText('Until')).toBeNull()
  })

  it('opens the backfill form with From/To when triggered', () => {
    openShowImport()

    fireEvent.click(screen.getByRole('button', { name: /^Backfill Episodes$/ }))
    expect(screen.getByText('From')).toBeInTheDocument()
    expect(screen.getByText('To')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Start Backfill/i })
    ).toBeInTheDocument()
  })
})
