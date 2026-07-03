import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ArtistContextPanel } from './ArtistContextPanel'
import type { ArtistGraphCard } from '@/features/artists/types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const CARD: ArtistGraphCard = {
  id: 7,
  name: 'Lightning Bolt',
  slug: 'lightning-bolt',
  city: 'Providence',
  state: 'RI',
  next_show: {
    id: 99,
    event_date: '2026-06-12T20:00:00Z',
    venue_name: 'Trunk Space',
    venue_city: 'Phoenix',
    venue_state: 'AZ',
  },
  labels: [{ name: 'Thrill Jockey', slug: 'thrill-jockey' }],
  radio: { stations: ['WFMU', 'KEXP'], play_count: 31 },
  connections: { bills: 7, similar: 4, members: 2, radio: 5, shared_labels: 3 },
}

const onClose = vi.fn()

beforeEach(() => {
  onClose.mockReset()
})

describe('ArtistContextPanel', () => {
  it('renders the full card: identity, next show, label link, radio, connections, open-page link', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={CARD} onClose={onClose} />,
    )
    expect(screen.getByRole('heading', { name: 'Lightning Bolt' })).toBeInTheDocument()
    expect(screen.getByText('Providence, RI')).toBeInTheDocument()
    expect(screen.getByText(/Trunk Space, Phoenix/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Thrill Jockey' })).toHaveAttribute(
      'href',
      '/labels/thrill-jockey',
    )
    expect(screen.getByText(/WFMU · KEXP · 31 plays/)).toBeInTheDocument()
    expect(screen.getByText('7 bills · 4 similar · 2 members · 5 radio')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/lightning-bolt',
    )
  })

  it('renders skeleton rows (no field labels) while the card loads', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={undefined} onClose={onClose} />,
    )
    expect(screen.getByLabelText('Loading artist details')).toBeInTheDocument()
    expect(screen.queryByText('Next show')).toBeNull()
    expect(screen.queryByRole('link', { name: /open page/i })).toBeNull()
  })

  it('degrades to name + open-page link (via the node slug) when the fetch errors', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={undefined} isError onClose={onClose} />,
    )
    expect(screen.getByText(/details couldn’t load/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/lightning-bolt',
    )
  })

  it('omits empty sections (no show, no labels, no radio, zero connections)', () => {
    render(
      <ArtistContextPanel
        artistName="Sparse Artist"
        artistSlug="sparse-artist"
        card={{
          ...CARD,
          name: 'Sparse Artist',
          slug: 'sparse-artist',
          city: null,
          state: null,
          next_show: null,
          labels: [],
          radio: null,
          connections: { bills: 0, similar: 0, members: 0, radio: 0, shared_labels: 0 },
        }}
        onClose={onClose}
      />,
    )
    expect(screen.queryByText('Next show')).toBeNull()
    expect(screen.queryByText(/label/i)).toBeNull()
    expect(screen.queryByText('As heard on')).toBeNull()
    expect(screen.queryByText('Connections')).toBeNull()
    expect(screen.getByRole('link', { name: /open page/i })).toBeInTheDocument()
  })

  it('closes on the X button and on capture-phase Escape', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={CARD} onClose={onClose} />,
    )
    fireEvent.click(screen.getByRole('button', { name: /close details/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('pins hostile slugs to one path segment', () => {
    render(
      <ArtistContextPanel
        artistName="Evil"
        artistSlug="../admin"
        card={undefined}
        isError
        onClose={onClose}
      />,
    )
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/..%2Fadmin',
    )
  })
})
