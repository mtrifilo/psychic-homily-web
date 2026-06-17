import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { RadioStationDetail } from '@/lib/hooks/admin/useAdminRadio'

// EditStationFormFields calls useUpdateRadioStation. Stub it so we can drive an
// onError path synchronously (the mutate spy invokes options.onError), exercising
// the error banner without a real mutation/QueryClient. Other module exports load
// as-is so AdminFormLayout, InlineErrorBanner (role="alert"), the Sheet, etc. are
// the real ones.
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

// Pins the PSY-1121 fix: a failed edit's error banner must NOT persist into the
// next open of the SAME station's edit Sheet. AdminFormLayout keeps the fields
// component mounted across close (for the close animation), and re-opening the
// same station does NOT remount it (the key={station.id} reset only fires on a
// station switch) — so the error has to be cleared on (re)open, mirroring
// CreateStationForm's reset-on-open.
describe('EditStationFormFields error reset-on-open (PSY-1121)', () => {
  beforeEach(() => updateMutate.mockClear())

  it('clears a stale submit error when the Sheet is closed and reopened', () => {
    // mutate immediately invokes the caller's onError -> setError(message).
    updateMutate.mockImplementation((_input, opts) => {
      opts?.onError?.(new Error('Server exploded'))
    })

    const { rerender } = render(
      <EditStationFormFields
        key={1}
        station={makeStation()}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )

    // Trigger a failed submit -> error banner appears.
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))
    expect(screen.getByRole('alert')).toHaveTextContent(/server exploded/i)

    // Close (open=false) without unmounting (no key change).
    rerender(
      <EditStationFormFields
        key={1}
        station={makeStation()}
        open={false}
        onOpenChange={noop}
        onSuccess={noop}
      />
    )

    // Reopen the SAME station's edit Sheet — the stale error must be gone.
    rerender(
      <EditStationFormFields
        key={1}
        station={makeStation()}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps field edits across a close+reopen of the same station (only error resets)', () => {
    updateMutate.mockImplementation(() => {})

    const { rerender } = render(
      <EditStationFormFields
        key={1}
        station={makeStation({ name: 'KEXP' })}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )

    const name = () => screen.getByLabelText('Name *') as HTMLInputElement
    fireEvent.change(name(), { target: { value: 'KEXP-EDITED' } })
    expect(name().value).toBe('KEXP-EDITED')

    // Close + reopen the same station (no key change) — dirty edit is preserved.
    rerender(
      <EditStationFormFields
        key={1}
        station={makeStation({ name: 'KEXP' })}
        open={false}
        onOpenChange={noop}
        onSuccess={noop}
      />
    )
    rerender(
      <EditStationFormFields
        key={1}
        station={makeStation({ name: 'KEXP' })}
        open
        onOpenChange={noop}
        onSuccess={noop}
      />
    )
    expect(name().value).toBe('KEXP-EDITED')
  })
})
