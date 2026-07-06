import { describe, it, expect, vi } from 'vitest'
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
// stop sibling listeners on the same target/phase — one keypress must close
// exactly ONE panel (the /graph-Observatory coexistence contract). PSY-1360
// sharpens that into innermost-first: the coordinated useGraphPanelEscape stack
// closes the most-recently-mounted panel first, deterministically (before, the
// FIRST-mounted / outermost panel won by registration order).
describe('graph panel Esc layering (innermost-first, PSY-1360)', () => {
  // Real open/close state so a closed panel actually UNMOUNTS — popping its
  // useGraphPanelEscape token off the shared stack. A mock onClose would leave both
  // panels mounted and couldn't distinguish innermost-first from a dead listener.
  function StackedPanels() {
    const [artistOpen, setArtistOpen] = useState(true)
    const [connectionOpen, setConnectionOpen] = useState(true)
    return (
      <div>
        {artistOpen && (
          <ArtistContextPanel
            artistName="Lightning Bolt"
            artistSlug="lightning-bolt"
            card={undefined}
            onClose={() => setArtistOpen(false)}
          />
        )}
        {/* Mounted last → innermost → dismissed first. */}
        {connectionOpen && (
          <ConnectionPanel
            source={{ name: 'Dehd' }}
            target={{ name: 'Lifeguard' }}
            connections={[{ type: 'shared_bills' }]}
            onClose={() => setConnectionOpen(false)}
          />
        )}
      </div>
    )
  }

  it('one Escape closes only the innermost panel; the next closes the outer', () => {
    render(<StackedPanels />)
    expect(screen.getByRole('region', { name: /connected/i })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /about lightning bolt/i })).toBeInTheDocument()

    // First Escape: only the innermost (ConnectionPanel) closes.
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('region', { name: /connected/i })).not.toBeInTheDocument()
    expect(screen.getByRole('region', { name: /about lightning bolt/i })).toBeInTheDocument()

    // Second Escape reaches the now-topmost panel (defaultPrevented state must
    // not leak across keypresses).
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('region', { name: /about lightning bolt/i })).not.toBeInTheDocument()
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

// PSY-1355: on a scene/station/venue graph the ConnectionPanel floats over the
// canvas (NOT inside a modal). If the user opens the ⌘K command palette (a Radix
// Dialog) on top and presses Escape, the palette must close FIRST — the panel had
// been swallowing it (its custom capture listener registered before the palette's
// and won by order). Converting the panels to Radix DismissableLayer fixes it for
// free: the palette mounts last → it's the highest layer → Radix dismisses only it.
describe('ConnectionPanel with a ⌘K palette stacked on top (PSY-1355)', () => {
  function PanelThenPalette() {
    const [panelOpen, setPanelOpen] = useState(true)
    const [paletteOpen, setPaletteOpen] = useState(true)
    return (
      <div>
        {panelOpen && (
          <ConnectionPanel
            source={{ name: 'Dehd' }}
            target={{ name: 'Lifeguard' }}
            connections={[{ type: 'shared_bills' }]}
            onClose={() => setPanelOpen(false)}
          />
        )}
        {/* Opened AFTER the panel → topmost Radix layer, as in the real app. */}
        <Dialog open={paletteOpen} onOpenChange={setPaletteOpen}>
          <DialogContent>
            <DialogTitle>Command palette</DialogTitle>
            <DialogDescription className="sr-only">Search</DialogDescription>
            <input aria-label="command input" />
          </DialogContent>
        </Dialog>
      </div>
    )
  }

  it('Escape closes the palette first, leaving the ConnectionPanel open; a second closes the panel', () => {
    render(<PanelThenPalette />)
    // The modal palette aria-hides the panel behind it (as the real ⌘K does), so
    // query it with { hidden: true } while the palette is open.
    expect(screen.getByRole('region', { name: /connected/i, hidden: true })).toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: /command palette/i })).toBeInTheDocument()

    // First Escape: only the topmost layer (the palette) dismisses.
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: /command palette/i })).not.toBeInTheDocument()
    // Palette gone → the panel is no longer aria-hidden → back in the a11y tree,
    // proving the first Escape spared it (the PSY-1355 bug closed it instead).
    expect(screen.getByRole('region', { name: /connected/i })).toBeInTheDocument()

    // With the palette gone the panel is topmost, so the next Escape closes it.
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('region', { name: /connected/i })).not.toBeInTheDocument()
  })
})
