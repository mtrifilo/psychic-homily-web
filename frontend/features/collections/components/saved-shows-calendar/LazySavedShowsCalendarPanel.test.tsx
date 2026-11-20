import { describe, expect, it, vi } from 'vitest'
import { useRef, useState } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'

// Whether the panel's code fails to load, as a chunk does after a deploy or on
// a dropped connection.
let failLoad = true
vi.mock('./SavedShowsCalendarPanel', () => {
  if (failLoad) throw new Error('Loading chunk failed')
  return {
    SavedShowsCalendarPanel: ({ open }: { open: boolean }) =>
      open ? <div role="dialog" aria-label="Saved shows calendar" /> : null,
  }
})

import { LazySavedShowsCalendarPanel } from './LazySavedShowsCalendarPanel'

function Opener() {
  const anchorRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  return (
    <>
      <div ref={anchorRef}>
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          Calendar
        </button>
      </div>
      <LazySavedShowsCalendarPanel
        open={open}
        onOpenChange={setOpen}
        anchorRef={anchorRef}
      />
    </>
  )
}

describe('LazySavedShowsCalendarPanel', () => {
  it('refuses the open when the code fails to load, and loads again on the next open', async () => {
    render(<Opener />)
    const control = screen.getByRole('button', { name: 'Calendar' })

    await act(async () => control.click())

    // Not left announcing a panel that is not there.
    await waitFor(() => expect(control).toHaveAttribute('aria-expanded', 'false'))
    expect(screen.queryByRole('dialog')).toBeNull()

    failLoad = false
    await act(async () => control.click())

    expect(
      await screen.findByRole('dialog', { name: 'Saved shows calendar' })
    ).toBeInTheDocument()
    expect(control).toHaveAttribute('aria-expanded', 'true')
  })
})
