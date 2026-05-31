import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

// Stable mutate spy (module-level) so we can assert the exact payload the form
// builds. The mock returns the SAME fn across renders — a fresh `vi.fn()` per
// call (as in the reset test) would lose the recorded args. Other module exports
// load as-is.
const mutate = vi.fn()
vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useCreateRadioStation: () => ({ mutate, isPending: false }),
  }
})

import { CreateStationForm } from './RadioManagement'

const noop = () => {}

function fill(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

// Guards the field-wiring parity claim of the Dialog->Sheet migration (PSY-911):
// the AdminFormLayout move must not change which keys reach the create mutation,
// the camelCase->snake_case mapping, the trims, the parseFloat on frequency, or
// the omit-empty-optionals behaviour.
describe('CreateStationForm submit payload (PSY-911 field-wiring parity)', () => {
  beforeEach(() => mutate.mockClear())

  it('maps fields to the snake_case API payload, trims, and omits empty optionals', () => {
    render(<CreateStationForm open onOpenChange={noop} onSuccess={noop} />)

    fill('Name *', '  KEXP  ') // leading/trailing space -> trimmed
    fill('Slug (auto if empty)', 'kexp')
    fill('City', 'Seattle')
    fill('State', 'WA')
    // Country is left at its 'US' default (still included — non-empty).
    fill('Frequency (MHz)', '90.3')
    fill('Stream URL', 'https://kexp.org/stream')

    fireEvent.click(screen.getByRole('button', { name: /create station/i }))

    expect(mutate).toHaveBeenCalledTimes(1)
    const [payload] = mutate.mock.calls[0]
    expect(payload).toEqual({
      name: 'KEXP', // trimmed
      broadcast_type: 'both', // Select default
      slug: 'kexp',
      city: 'Seattle',
      state: 'WA',
      country: 'US', // default retained
      stream_url: 'https://kexp.org/stream',
      frequency_mhz: 90.3, // parseFloat -> number, NOT the string '90.3'
    })
    expect(typeof payload.frequency_mhz).toBe('number')
    // Empty optionals are omitted entirely, not sent as ''.
    expect(payload).not.toHaveProperty('description')
    expect(payload).not.toHaveProperty('timezone')
    expect(payload).not.toHaveProperty('website')
    expect(payload).not.toHaveProperty('donation_url')
    expect(payload).not.toHaveProperty('logo_url')
    expect(payload).not.toHaveProperty('playlist_source')
    expect(payload).not.toHaveProperty('playlist_config')
  })

  it('blocks submit with a role=alert error when Name is empty (mutate not called)', () => {
    render(<CreateStationForm open onOpenChange={noop} onSuccess={noop} />)

    fireEvent.click(screen.getByRole('button', { name: /create station/i }))

    expect(mutate).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent(/name is required/i)
  })
})
