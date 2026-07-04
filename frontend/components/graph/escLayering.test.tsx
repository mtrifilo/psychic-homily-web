import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useEffect, useRef, useState } from 'react'
import { render, fireEvent, screen } from '@testing-library/react'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import {
  dismissConnectionPanelOnEscape,
  type ConnectionPanelDismissHandle,
} from '@/features/artists/components/ArtistGraph'
import { ArtistContextPanel } from './ArtistContextPanel'
import { ConnectionPanel } from './ConnectionPanel'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// PSY-1345 adversarial finding (2 lenses): both graph panels listen for
// Escape on document in the CAPTURE phase, and stopPropagation does NOT
// stop sibling listeners on the same target/phase. The defaultPrevented
// guard + stopImmediatePropagation pair must make one keypress close
// exactly ONE panel — this is the /graph-Observatory coexistence contract.
describe('graph panel Esc layering', () => {
  const closeArtist = vi.fn()
  const closeConnection = vi.fn()

  beforeEach(() => {
    closeArtist.mockReset()
    closeConnection.mockReset()
  })

  it('one Escape closes exactly one panel when both are mounted', () => {
    render(
      <div>
        <ArtistContextPanel
          artistName="Lightning Bolt"
          artistSlug="lightning-bolt"
          card={undefined}
          onClose={closeArtist}
        />
        <ConnectionPanel
          source={{ name: 'Dehd' }}
          target={{ name: 'Lifeguard' }}
          connections={[{ type: 'shared_bills' }]}
          onClose={closeConnection}
        />
      </div>,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(closeArtist.mock.calls.length + closeConnection.mock.calls.length).toBe(1)

    // A second Escape reaches the surviving panel (the consumed event's
    // defaultPrevented state must not leak across keypresses).
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(closeArtist.mock.calls.length + closeConnection.mock.calls.length).toBe(2)
  })
})

// PSY-1351: the ego graph mounts the ConnectionPanel inside a Radix <Dialog>.
// Radix's DismissableLayer listens for Escape on document in the CAPTURE phase
// and registers when the dialog OPENS — before the panel's own document/capture
// listener registers on edge-click. Same phase + same target → registration
// order decides, so the panel loses and one Escape would close BOTH the panel
// and the dialog. ArtistGraphDialog fixes this at the Dialog boundary: its
// onEscapeKeyDown closes an open panel (via a shared dismiss handle) and
// preventDefaults, so Radix skips the dialog dismiss until the panel is gone.
//
// This harness mirrors ArtistGraphDialog's exact wiring (onEscapeKeyDown reads
// a handle a child keeps current) and — crucially — dispatches Escape INSIDE
// the focus-trapped [role="dialog"], as the real app does. A prior version of
// this test fired on document.body, which could not distinguish the shipped fix
// from a broken one that skips in-dialog targets.
describe('ConnectionPanel inside a Radix Dialog (ego graph, PSY-1351)', () => {
  function EgoDialogHarness({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
    const dismissRef = useRef<ConnectionPanelDismissHandle | null>(null)
    const [panelOpen, setPanelOpen] = useState(true)

    // Mirror ArtistGraphVisualization keeping the dialog's dismiss handle current.
    useEffect(() => {
      dismissRef.current = { isOpen: panelOpen, close: () => setPanelOpen(false) }
    }, [panelOpen])

    return (
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent
          // The REAL handler ArtistGraphDialog uses — shared so the test can't
          // drift from the component (adversarial-review fix).
          onEscapeKeyDown={e => dismissConnectionPanelOnEscape(dismissRef, e)}
        >
          <DialogTitle>Similar artists</DialogTitle>
          <DialogDescription className="sr-only">Artist relationship graph</DialogDescription>
          {panelOpen && (
            <ConnectionPanel
              source={{ name: 'Dehd' }}
              target={{ name: 'Lifeguard' }}
              connections={[{ type: 'shared_bills' }]}
              onClose={() => setPanelOpen(false)}
            />
          )}
        </DialogContent>
      </Dialog>
    )
  }

  it('first Escape closes only the panel; the dialog survives, a later Escape closes it', () => {
    const onOpenChange = vi.fn()
    render(<EgoDialogHarness onOpenChange={onOpenChange} />)

    // Escape targeted INSIDE the dialog, as Radix's focus trap makes it in the app.
    const dialog = screen.getByRole('dialog')
    fireEvent.keyDown(dialog, { key: 'Escape' })

    // Panel gone (its "Why … connected" region unmounts), dialog still open.
    expect(screen.queryByRole('region', { name: /connected/i })).not.toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalled()

    // With the panel gone, the next Escape falls through to Radix and closes
    // the dialog — proving the panel (not a dead listener) was what blocked it.
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onOpenChange).toHaveBeenCalled()
  })
})
