import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

// CreateStationForm calls useCreateRadioStation; stub just that hook so the form
// renders without a real mutation/QueryClient. Other module exports load as-is.
vi.mock('@/lib/hooks/admin/useAdminRadio', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminRadio')>()
  return {
    ...actual,
    useCreateRadioStation: () => ({ mutate: vi.fn(), isPending: false }),
  }
})

import { CreateStationForm } from './RadioManagement'

const noop = () => {}

function open(rerender: (ui: React.ReactElement) => void, isOpen: boolean) {
  rerender(
    <CreateStationForm open={isOpen} onOpenChange={noop} onSuccess={noop} />
  )
}

describe('CreateStationForm reset-on-open (PSY-911)', () => {
  it('clears entered field values when the Sheet is closed and reopened', () => {
    const { rerender } = render(
      <CreateStationForm open={false} onOpenChange={noop} onSuccess={noop} />
    )

    // Open, type a value.
    open(rerender, true)
    const name = () => screen.getByLabelText('Name *') as HTMLInputElement
    fireEvent.change(name(), { target: { value: 'KEXP' } })
    expect(name().value).toBe('KEXP') // persists while open (no clobber)

    // Close, then reopen — the form must be blank again, not stale 'KEXP'.
    open(rerender, false)
    open(rerender, true)
    expect(name().value).toBe('')
    // Defaults restored (not blanked).
    expect((screen.getByLabelText('Country') as HTMLInputElement).value).toBe(
      'US'
    )
  })
})
