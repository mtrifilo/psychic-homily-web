import { describe, it, expect, vi } from 'vitest'
import { renderWithProviders } from '@/test/utils'
import { fireEvent, screen } from '@testing-library/react'
import { ArtistNodeTooltip } from './ArtistGraph'

// PSY-361: the tooltip is the user's escape hatch to the full artist
// detail page (Option A — user-decided). Lock the link's href format
// so future refactors don't silently break shareability.

// Next.js Link in jsdom can't resolve route prefetching, but renders fine
// as an <a>. No special mock needed.
vi.mock('next/link', () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}))

describe('ArtistNodeTooltip (PSY-361)', () => {
  const baseNode = {
    name: 'Frozen Soul',
    slug: 'frozen-soul',
    upcoming_show_count: 0,
  }

  // PSY-1218: onMouseEnter/onMouseLeave are REQUIRED props — the pointer-events-auto
  // wrapper captures the pointer and depends on them for dismissal. Presentational
  // tests that don't exercise the dismiss wiring pass these no-ops to satisfy the
  // contract; the interaction tests override with vi.fn / the parent-timer suite.
  const handlers = { onMouseEnter: () => {}, onMouseLeave: () => {} }

  it('renders the View artist page link with the correct href', () => {
    renderWithProviders(
      <ArtistNodeTooltip node={baseNode} position={{ x: 100, y: 200 }} {...handlers} />
    )
    const link = screen.getByRole('link', { name: /View artist page/i })
    expect(link).toBeInTheDocument()
    expect(link.getAttribute('href')).toBe('/artists/frozen-soul')
  })

  it('uses the node slug — not the name — to build the href', () => {
    // Slug is the canonical URL identifier. If the name and slug ever
    // diverge (e.g. an artist edits their display name), this test
    // protects the URL contract.
    renderWithProviders(
      <ArtistNodeTooltip
        node={{ name: 'Frozen Soul (Texas)', slug: 'frozen-soul', upcoming_show_count: 0 }}
        position={{ x: 0, y: 0 }}
        {...handlers}
      />
    )
    const link = screen.getByRole('link', { name: /View artist page/i })
    expect(link.getAttribute('href')).toBe('/artists/frozen-soul')
  })

  it('renders the artist name and location when present', () => {
    renderWithProviders(
      <ArtistNodeTooltip
        node={{
          ...baseNode,
          city: 'Fort Worth',
          state: 'TX',
        }}
        position={{ x: 0, y: 0 }}
        {...handlers}
      />
    )
    expect(screen.getByText('Frozen Soul')).toBeInTheDocument()
    expect(screen.getByText('Fort Worth, TX')).toBeInTheDocument()
  })

  it('renders upcoming show count when > 0', () => {
    renderWithProviders(
      <ArtistNodeTooltip
        node={{ ...baseNode, upcoming_show_count: 1 }}
        position={{ x: 0, y: 0 }}
        {...handlers}
      />
    )
    expect(screen.getByText(/1 upcoming show$/)).toBeInTheDocument()
  })

  it('pluralizes upcoming show count when > 1', () => {
    renderWithProviders(
      <ArtistNodeTooltip
        node={{ ...baseNode, upcoming_show_count: 3 }}
        position={{ x: 0, y: 0 }}
        {...handlers}
      />
    )
    expect(screen.getByText(/3 upcoming shows/)).toBeInTheDocument()
  })

  it('is hoverable (wrapper pointer-events-auto) so the cursor can reach the link', () => {
    // PSY-1218: the wrapper is pointer-events-AUTO so the tooltip captures the
    // pointer when the cursor travels onto it (the parent then keeps it open via
    // onMouseEnter), making the link reachable. The link keeps pointer-events-auto
    // explicitly as defense-in-depth. A future refactor dropping either class would
    // re-break the link's clickability (the PSY-1218 bug).
    renderWithProviders(
      <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} {...handlers} />
    )
    const wrapper = screen.getByTestId('artist-node-tooltip')
    expect(wrapper.className).toMatch(/pointer-events-auto/)
    const link = screen.getByRole('link', { name: /View artist page/i })
    expect(link.className).toMatch(/pointer-events-auto/)
  })

  it('fires onMouseEnter (keep open) and onMouseLeave (reschedule dismiss) — PSY-1218', () => {
    // The parent wires these to cancel/reschedule the dismiss timer; without them the
    // tooltip would vanish the instant the cursor left the node, before the link is
    // reachable. fireEvent (not userEvent) to avoid focus/timer races.
    const onMouseEnter = vi.fn()
    const onMouseLeave = vi.fn()
    renderWithProviders(
      <ArtistNodeTooltip
        node={baseNode}
        position={{ x: 0, y: 0 }}
        onMouseEnter={onMouseEnter}
        onMouseLeave={onMouseLeave}
      />
    )
    const wrapper = screen.getByTestId('artist-node-tooltip')
    fireEvent.mouseEnter(wrapper)
    expect(onMouseEnter).toHaveBeenCalledTimes(1)
    fireEvent.mouseLeave(wrapper)
    expect(onMouseLeave).toHaveBeenCalledTimes(1)
  })

  // PSY-1215: the tooltip is anchored at the node (left/top) and offset via a
  // transform; near the right/bottom container edge it flips toward the interior
  // so it doesn't run off the dialog.
  it('anchors down-right of the node by default (no flip)', () => {
    renderWithProviders(
      <ArtistNodeTooltip node={baseNode} position={{ x: 100, y: 200 }} {...handlers} />
    )
    const style = screen.getByTestId('artist-node-tooltip').getAttribute('style') ?? ''
    expect(style).toContain('left: 100px')
    expect(style).toContain('top: 200px')
    expect(style).toContain('translateX(8px)')
    expect(style).toContain('translateY(8px)')
  })

  it('flips toward the node top-left when flipX/flipY are set', () => {
    renderWithProviders(
      <ArtistNodeTooltip
        node={baseNode}
        position={{ x: 100, y: 200, flipX: true, flipY: true }}
        {...handlers}
      />
    )
    const style = screen.getByTestId('artist-node-tooltip').getAttribute('style') ?? ''
    expect(style).toContain('translateX(calc(-100% - 8px))')
    expect(style).toContain('translateY(calc(-100% - 8px))')
  })

  // PSY-1259: the tooltip is the DOM-accessible home of the expand/collapse + re-center
  // actions (the canvas node click does expand on the visual layer; these are the
  // keyboard/click-reachable twins, and re-center has no canvas gesture anymore).
  describe('expand / re-center actions (PSY-1259)', () => {
    it('omits both action buttons when no callbacks are wired (e.g. bill-composition graph)', () => {
      renderWithProviders(
        <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} {...handlers} />
      )
      expect(screen.queryByRole('button', { name: /expand neighbors|collapse neighbors/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /center on this artist/i })).not.toBeInTheDocument()
      // The nav escape is unaffected.
      expect(screen.getByRole('link', { name: /View artist page/i })).toBeInTheDocument()
    })

    it('renders an Expand button and fires onExpand on click', () => {
      const onExpand = vi.fn()
      renderWithProviders(
        <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} {...handlers} onExpand={onExpand} />
      )
      const btn = screen.getByRole('button', { name: /\+ Expand neighbors/i })
      fireEvent.click(btn)
      expect(onExpand).toHaveBeenCalledTimes(1)
    })

    it('flips the label to Collapse when isExpanded', () => {
      const onExpand = vi.fn()
      renderWithProviders(
        <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} {...handlers} onExpand={onExpand} isExpanded />
      )
      expect(screen.getByRole('button', { name: /− Collapse neighbors/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /Expand neighbors/i })).not.toBeInTheDocument()
    })

    it('renders a Center-on-this-artist button and fires onRecenter on click', () => {
      const onRecenter = vi.fn()
      renderWithProviders(
        <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} {...handlers} onRecenter={onRecenter} />
      )
      const btn = screen.getByRole('button', { name: /center on this artist/i })
      fireEvent.click(btn)
      expect(onRecenter).toHaveBeenCalledTimes(1)
    })
  })
})
