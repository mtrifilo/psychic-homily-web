import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioStationDetail,
} from '@/lib/hooks/admin/useAdminRadio'

// Drive the station-detail header from mocked hooks. The point of this test is
// that after an edit, the detail panel header reflects the FRESH server values
// returned by useRadioStationDetail (which useUpdateRadioStation invalidates),
// not the stale list-row snapshot captured at row-click. We simulate the post-
// invalidation state by having useRadioStationDetail return values that differ
// from the list item the row was rendered from. (PSY-1121)

const listItem: RadioStationListItem = {
  id: 1,
  name: 'KEXP (stale)',
  slug: 'kexp',
  city: 'Seattle',
  state: 'WA',
  country: 'US',
  broadcast_type: 'terrestrial',
  frequency_mhz: 90.3,
  logo_url: null,
  is_active: false,
  show_count: 2,
}

// Fresh detail — differs from the list snapshot in every header field.
const freshDetail: RadioStationDetail = {
  id: 1,
  name: 'KEXP (fresh)',
  slug: 'kexp',
  description: null,
  city: 'Tacoma',
  state: 'WA',
  country: 'US',
  timezone: 'America/Los_Angeles',
  stream_url: null,
  stream_urls: null,
  website: null,
  donation_url: null,
  donation_embed_url: null,
  logo_url: null,
  social: null,
  broadcast_type: 'internet',
  frequency_mhz: 88.5,
  playlist_source: null,
  playlist_config: null,
  last_playlist_fetch_at: null,
  is_active: true,
  show_count: 7,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
}

// `detailReturn` is reassigned per-test so we can flip the header between
// "loading (no detail)" and "fresh detail". vi.hoisted runs before the hoisted
// vi.mock factory, so the factory can safely close over this mutable holder.
const hoisted = vi.hoisted(() => ({
  detailReturn: { data: undefined as RadioStationDetail | undefined },
}))

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
    useRadioStationDetail: () => hoisted.detailReturn,
    useRadioShows: () => ({ data: { shows: [], count: 0 }, isLoading: false }),
    useDeleteRadioStation: noopMutation,
    useDiscoverShows: noopMutation,
    useDeleteRadioShow: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function openDetail() {
  // RadioManagement keeps CreateStationForm mounted (useCreateRadioStation ->
  // useQueryClient), so a QueryClientProvider is required even though every radio
  // hook this test exercises is mocked.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
  // The stations table renders one clickable row per station.
  fireEvent.click(screen.getByRole('button', { name: /Station: KEXP \(stale\)/i }))
}

describe('StationDetailPanel header refresh (PSY-1121)', () => {
  beforeEach(() => {
    hoisted.detailReturn = { data: undefined }
  })

  it('falls back to the list-row snapshot while the detail is still loading', () => {
    hoisted.detailReturn = { data: undefined }
    openDetail()

    const heading = screen.getByRole('heading', { name: 'KEXP (stale)' })
    expect(heading).toBeInTheDocument()
  })

  it('renders the FRESH detail values once useRadioStationDetail resolves', () => {
    hoisted.detailReturn = { data: freshDetail }
    openDetail()

    // Header heading + meta line reflect the fresh detail, not the stale list row.
    expect(
      screen.getByRole('heading', { name: 'KEXP (fresh)' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'KEXP (stale)' })).toBeNull()

    // Meta line: city/broadcast/frequency/show_count come from the fresh detail.
    expect(screen.getByText(/Tacoma/)).toBeInTheDocument()
    expect(screen.getByText(/internet/)).toBeInTheDocument()
    expect(screen.getByText(/88\.5 MHz/)).toBeInTheDocument()
    expect(screen.getByText(/7 show\(s\)/)).toBeInTheDocument()

    // is_active flips to Active (fresh) vs Inactive (stale list snapshot).
    // The header Badge is the first "Active" badge in the panel.
    const activeBadges = screen.getAllByText('Active')
    expect(activeBadges.length).toBeGreaterThan(0)
    expect(screen.queryByText('Inactive')).toBeNull()
  })
})
