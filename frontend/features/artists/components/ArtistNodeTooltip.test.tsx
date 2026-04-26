import { describe, it, expect, vi } from 'vitest'
import { renderWithProviders } from '@/test/utils'
import { screen } from '@testing-library/react'
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

  it('renders the View artist page link with the correct href', () => {
    renderWithProviders(
      <ArtistNodeTooltip node={baseNode} position={{ x: 100, y: 200 }} />
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
      />
    )
    expect(screen.getByText(/1 upcoming show$/)).toBeInTheDocument()
  })

  it('pluralizes upcoming show count when > 1', () => {
    renderWithProviders(
      <ArtistNodeTooltip
        node={{ ...baseNode, upcoming_show_count: 3 }}
        position={{ x: 0, y: 0 }}
      />
    )
    expect(screen.getByText(/3 upcoming shows/)).toBeInTheDocument()
  })

  it('keeps the link interactive (pointer-events-auto) inside the non-interactive tooltip body', () => {
    // The whole tooltip is pointer-events-none so it doesn't steal
    // hover/click events from the canvas. Only the link is re-enabled.
    // Without this test a future refactor could quietly drop the
    // `pointer-events-auto` class and the link would become unclickable.
    renderWithProviders(
      <ArtistNodeTooltip node={baseNode} position={{ x: 0, y: 0 }} />
    )
    const link = screen.getByRole('link', { name: /View artist page/i })
    expect(link.className).toMatch(/pointer-events-auto/)
  })
})
