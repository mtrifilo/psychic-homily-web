import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioShowListItem,
} from '@/lib/hooks/admin/useAdminRadio'

// PSY-1120 consolidated the two parallel import systems (one-shot
// ShowImportControls + async job-based ShowImportSection) down to the single
// job-based ShowImportSection. This test asserts that exactly ONE import
// affordance renders on an expanded show row — the job-based one — and that the
// deleted one-shot "Since"/"Until" controls are gone.

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
    useDiscoverShows: noopMutation,
    useDeleteRadioShow: noopMutation,
    // Job-based import (kept): no jobs yet, no active job.
    useShowImportJobs: () => ({ data: { jobs: [], count: 0 }, isLoading: false }),
    useCreateImportJob: noopMutation,
    useImportJob: () => ({ data: undefined }),
    useCancelImportJob: noopMutation,
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
  // Expand the show's import panel.
  fireEvent.click(
    screen.getByRole('button', { name: /Import episodes for Morning Show/i })
  )
}

describe('Import consolidation — one affordance per show (PSY-1120)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders exactly one "Import Episodes" affordance (the job-based one)', () => {
    openShowImport()

    // Exactly one "Import Episodes" button — the job-based ShowImportSection's
    // collapsed trigger. The deleted one-shot ShowImportControls also rendered
    // an "Import Episodes" button; if it were still mounted there would be two.
    const importButtons = screen.getAllByRole('button', {
      name: /^Import Episodes$/,
    })
    expect(importButtons).toHaveLength(1)
  })

  it('does not render the deleted one-shot Since/Until controls', () => {
    openShowImport()

    // The job-based create form uses "From"/"To" labels and only appears after
    // clicking the trigger; the deleted one-shot path rendered "Since"/"Until"
    // inputs inline immediately on expand.
    expect(screen.queryByText('Since')).toBeNull()
    expect(screen.queryByText('Until')).toBeNull()
  })

  it('opens the job-based create form with From/To when triggered', () => {
    openShowImport()

    fireEvent.click(
      screen.getByRole('button', { name: /^Import Episodes$/ })
    )
    // The job-based form fields.
    expect(screen.getByText('From')).toBeInTheDocument()
    expect(screen.getByText('To')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Start Import/i })
    ).toBeInTheDocument()
  })
})
