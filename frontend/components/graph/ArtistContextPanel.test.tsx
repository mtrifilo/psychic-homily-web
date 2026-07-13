import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ArtistContextPanel } from './ArtistContextPanel'
import type { ArtistGraphCard } from '@/features/artists/types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// MusicEmbed owns its own async embed resolution + network (covered by its own
// tests). Here we only care that the panel mounts it with the right props when
// an audio URL is present — stub it to expose the props it received.
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: ({
    bandcampAlbumUrl,
    spotifyUrl,
    artistName,
  }: {
    bandcampAlbumUrl?: string | null
    spotifyUrl?: string | null
    artistName: string
  }) => (
    <div
      data-testid="music-embed"
      data-bandcamp={bandcampAlbumUrl ?? ''}
      data-spotify={spotifyUrl ?? ''}
      data-artist={artistName}
    />
  ),
}))

const CARD: ArtistGraphCard = {
  id: 7,
  name: 'Lightning Bolt',
  slug: 'lightning-bolt',
  city: 'Providence',
  state: 'RI',
  bandcamp_embed_url: null,
  spotify: null,
  next_show: {
    id: 99,
    event_date: '2026-06-12T20:00:00Z',
    venue_name: 'Trunk Space',
    venue_city: 'Phoenix',
    venue_state: 'AZ',
    venue_timezone: 'America/Phoenix',
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
    expect(screen.getByText('7 bills · 4 similar · 2 members · 5 radio · 3 label ties')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/lightning-bolt',
    )
  })

  it('mounts a playable embed with the artist name when the card has a Bandcamp embed URL (PSY-1302)', () => {
    render(
      <ArtistContextPanel
        artistName="Lightning Bolt"
        artistSlug="lightning-bolt"
        card={{ ...CARD, bandcamp_embed_url: 'https://lightningbolt.bandcamp.com/album/wonderful-rainbow' }}
        onClose={onClose}
      />,
    )
    expect(screen.getByText('Listen')).toBeInTheDocument()
    const embed = screen.getByTestId('music-embed')
    expect(embed).toHaveAttribute('data-bandcamp', 'https://lightningbolt.bandcamp.com/album/wonderful-rainbow')
    expect(embed).toHaveAttribute('data-artist', 'Lightning Bolt')
    expect(
      screen
        .getByText('Listen')
        .compareDocumentPosition(screen.getByText('Next show')) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('falls back to the Spotify URL when there is no Bandcamp embed (PSY-1302)', () => {
    render(
      <ArtistContextPanel
        artistName="Lightning Bolt"
        artistSlug="lightning-bolt"
        card={{ ...CARD, bandcamp_embed_url: null, spotify: 'https://open.spotify.com/artist/2wY6Ju4nsyAmd4jVZ8Ovzm' }}
        onClose={onClose}
      />,
    )
    const embed = screen.getByTestId('music-embed')
    expect(embed).toHaveAttribute('data-spotify', 'https://open.spotify.com/artist/2wY6Ju4nsyAmd4jVZ8Ovzm')
  })

  it('renders no player (and no Listen label) when the artist has neither audio URL', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={CARD} onClose={onClose} />,
    )
    expect(screen.queryByTestId('music-embed')).toBeNull()
    expect(screen.queryByText('Listen')).toBeNull()
  })

  it('shows no dead "Listen" row when the only audio URL is an unparseable Spotify link (PSY-1302)', () => {
    // MusicEmbed would render nothing for a Spotify link with no embeddable id,
    // so the headed row must not appear at all (no dead affordance).
    render(
      <ArtistContextPanel
        artistName="Lightning Bolt"
        artistSlug="lightning-bolt"
        card={{ ...CARD, bandcamp_embed_url: null, spotify: 'https://open.spotify.com/playlist/xyz' }}
        onClose={onClose}
      />,
    )
    expect(screen.queryByTestId('music-embed')).toBeNull()
    expect(screen.queryByText('Listen')).toBeNull()
  })

  it('renders skeleton rows (no field labels) while the card loads', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={undefined} onClose={onClose} />,
    )
    expect(screen.getByLabelText('Loading artist details')).toBeInTheDocument()
    expect(screen.queryByText('Next show')).toBeNull()
    // Navigation must never be stranded — the link renders from the node
    // slug even before the card arrives.
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/lightning-bolt',
    )
  })

  it('shows the cached card, not the error apology, when a background refetch fails (isError + card)', () => {
    render(
      <ArtistContextPanel artistName="Lightning Bolt" artistSlug="lightning-bolt" card={CARD} isError onClose={onClose} />,
    )
    expect(screen.queryByText(/details couldn’t load/i)).toBeNull()
    expect(screen.getByText(/Trunk Space, Phoenix/)).toBeInTheDocument()
  })

  it('marks station truncation so the play total can’t misread as the sum of the named three', () => {
    render(
      <ArtistContextPanel
        artistName="Lightning Bolt"
        artistSlug="lightning-bolt"
        card={{ ...CARD, radio: { stations: ['A', 'B', 'C', 'D'], play_count: 57 } }}
        onClose={onClose}
      />,
    )
    expect(screen.getByText(/A · B · C\s*\+1\s*· 57 plays/)).toBeInTheDocument()
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

  it('renders label ties when shared_labels is the only connection type', () => {
    render(
      <ArtistContextPanel
        artistName="Labelmate"
        artistSlug="labelmate"
        card={{
          ...CARD,
          connections: { bills: 0, similar: 0, members: 0, radio: 0, shared_labels: 3 },
        }}
        onClose={onClose}
      />,
    )
    expect(screen.getByText('3 label ties')).toBeInTheDocument()
  })

  // Layered dismiss (a ⌘K palette / dialog stacked over the panel wins Escape)
  // is now enforced by Radix's DismissableLayer stack, not input-target sniffing,
  // so it's covered against a REAL Radix layer in escLayering.test.tsx (PSY-1355)
  // rather than a plain-input simulation here.

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
