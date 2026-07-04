import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
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
