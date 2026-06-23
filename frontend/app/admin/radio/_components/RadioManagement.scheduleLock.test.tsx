import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  RadioStationListItem,
  RadioShowListItem,
} from '@/lib/hooks/admin/useAdminRadio'

// PSY-1193: the admin show editor surfaces + toggles schedule_locked (provenance pin
// from PSY-1186). These tests assert the lock state is reflected from the show, that
// toggling sends schedule_locked on save, and that locked shows are badged in the list.

const station: RadioStationListItem = {
  id: 1,
  name: 'WFMU',
  slug: 'wfmu',
  city: 'Jersey City',
  state: 'NJ',
  country: 'US',
  broadcast_type: 'terrestrial',
  frequency_mhz: 91.1,
  logo_url: null,
  is_active: true,
  show_count: 1,
}

function makeShow(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 50,
    station_id: 1,
    station_name: 'WFMU',
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
    ...overrides,
  }
}

// Stable mutate spy so we can assert the exact payload the edit form sends.
const updateMutate = vi.fn()
let currentShow: RadioShowListItem = makeShow()

vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    ...actual,
    useAdminRadioStations: () => ({
      data: { stations: [station], count: 1 },
      isLoading: false,
    }),
    useRadioStationDetail: () => ({ data: undefined }),
    useRadioShows: () => ({
      data: { shows: [currentShow], count: 1 },
      isLoading: false,
    }),
    useUpdateRadioShow: () => ({ mutate: updateMutate, isPending: false }),
    useDeleteRadioStation: noopMutation,
    useTriggerStationSync: noopMutation,
    useDeleteRadioShow: noopMutation,
    useTriggerShowBackfill: noopMutation,
    useSyncRun: () => ({ data: undefined }),
    useCancelSyncRun: noopMutation,
  }
})

import { RadioManagement } from './RadioManagement'

function renderAndOpenStation() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RadioManagement />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: /Station: WFMU/i }))
}

describe('Admin schedule_locked toggle + badge (PSY-1193)', () => {
  beforeEach(() => {
    updateMutate.mockClear()
    currentShow = makeShow()
  })

  it('badges a locked show in the list and shows no badge for an unlocked one', () => {
    currentShow = makeShow({ schedule_locked: true })
    renderAndOpenStation()
    expect(screen.getByText('Schedule locked')).toBeInTheDocument()
  })

  it('does not badge an unlocked show', () => {
    currentShow = makeShow({ schedule_locked: false })
    renderAndOpenStation()
    expect(screen.queryByText('Schedule locked')).toBeNull()
  })

  it('reflects the current lock state in the edit form and sends schedule_locked=true when toggled on', () => {
    currentShow = makeShow({ schedule_locked: false })
    renderAndOpenStation()
    fireEvent.click(screen.getByRole('button', { name: /Edit Morning Show/i }))

    const lockSwitch = screen.getByLabelText('Lock schedule')
    expect(lockSwitch).toHaveAttribute('aria-checked', 'false')

    fireEvent.click(lockSwitch)
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    expect(updateMutate).toHaveBeenCalledTimes(1)
    const [payload] = updateMutate.mock.calls[0]
    expect(payload).toMatchObject({ showId: 50, schedule_locked: true })
  })

  it('sends schedule_locked=false when an already-locked show is unlocked', () => {
    currentShow = makeShow({ schedule_locked: true })
    renderAndOpenStation()
    fireEvent.click(screen.getByRole('button', { name: /Edit Morning Show/i }))

    const lockSwitch = screen.getByLabelText('Lock schedule')
    expect(lockSwitch).toHaveAttribute('aria-checked', 'true')

    fireEvent.click(lockSwitch)
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    expect(updateMutate).toHaveBeenCalledTimes(1)
    const [payload] = updateMutate.mock.calls[0]
    expect(payload).toMatchObject({ showId: 50, schedule_locked: false })
  })
})
