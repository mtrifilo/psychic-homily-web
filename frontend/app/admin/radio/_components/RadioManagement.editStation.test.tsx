import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { RadioStationDetail } from '@/lib/hooks/admin/useAdminRadio'

// EditStationFormFields calls useUpdateRadioStation; stub just that hook so the
// fields render without a real mutation/QueryClient. Other module exports load
// as-is so AdminFormLayout, the Sheet primitives, etc. are the real ones.
const updateMutate = vi.fn()
vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useUpdateRadioStation: () => ({ mutate: updateMutate, isPending: false }),
  }
})

import { EditStationFormFields } from './RadioManagement'

const noop = () => {}

function makeStation(
  overrides: Partial<RadioStationDetail> = {}
): RadioStationDetail {
  return {
    id: 1,
    name: 'KEXP',
    slug: 'kexp',
    description: null,
    city: 'Seattle',
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
    broadcast_type: 'both',
    frequency_mhz: 90.3,
    playlist_source: null,
    playlist_config: null,
    last_playlist_fetch_at: null,
    is_active: true,
    show_count: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

// Pins the PSY-930 Radio Edit Station migration: the AdminFormLayout (Sheet)
// initializes its fields from the loaded station detail, and a station switch
// re-initializes the fields when callers pass `key={station.id}` (the
// init-from-props-inline + key-reset contract, no useEffect ratchet).
describe('EditStationFormFields (PSY-930 Edit Station -> AdminFormLayout Sheet)', () => {
  beforeEach(() => updateMutate.mockClear())

  it('initializes the fields from the loaded station detail', () => {
    render(
      <EditStationFormFields
        key={1}
        station={makeStation()}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )

    expect(screen.getByLabelText('Name *')).toHaveValue('KEXP')
    expect(screen.getByLabelText('City')).toHaveValue('Seattle')
    expect(screen.getByLabelText('State')).toHaveValue('WA')
    expect(screen.getByLabelText('Frequency (MHz)')).toHaveValue(90.3)
    // The Sheet renders as a dialog with the Edit Station title + Save action.
    expect(
      screen.getByRole('heading', { name: 'Edit Station' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /save changes/i })
    ).toBeInTheDocument()
  })

  it('re-initializes the fields when the station switches (via key prop)', () => {
    const { rerender } = render(
      <EditStationFormFields
        key={1}
        station={makeStation({ id: 1, name: 'KEXP', city: 'Seattle' })}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )
    expect(screen.getByLabelText('Name *')).toHaveValue('KEXP')

    rerender(
      <EditStationFormFields
        key={2}
        station={makeStation({ id: 2, name: 'WFMU', city: 'Jersey City' })}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )
    expect(screen.getByLabelText('Name *')).toHaveValue('WFMU')
    expect(screen.getByLabelText('City')).toHaveValue('Jersey City')
  })
})
