import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  ArtistBase,
  artistHasMusic,
  showHasArtistMusic,
  ShowArtistMusicPanel,
} from './ShowArtistMusic'
import type { ArtistResponse } from '../types'

// The embed's own resolution is tested where it lives; what this file pins is
// which acts get one and what surrounds it.
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: ({ artistName }: { artistName: string }) => (
    <div data-testid={`embed-${artistName}`} />
  ),
}))

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    name: 'Sunn Amps',
    slug: 'sunn-amps',
    ...overrides,
  } as ArtistResponse
}

const WITH_BANDCAMP = makeArtist({
  socials: { bandcamp: 'https://sunnamps.bandcamp.com' },
} as never)

describe('artistHasMusic', () => {
  // A stored Bandcamp URL no longer implies a renderable embed on its own, so
  // this asks the shared predicate rather than testing the column. Otherwise
  // the expand control opens onto nothing.
  it('is true for an act with a renderable source', () => {
    expect(artistHasMusic(WITH_BANDCAMP)).toBe(true)
  })

  it('is false for an act with no sources at all', () => {
    expect(artistHasMusic(makeArtist())).toBe(false)
  })
})

describe('showHasArtistMusic', () => {
  it('is true when any act on the bill has music', () => {
    expect(showHasArtistMusic([makeArtist(), WITH_BANDCAMP])).toBe(true)
  })

  it('is false for an empty bill', () => {
    expect(showHasArtistMusic([])).toBe(false)
  })

  it('is false when no act has music', () => {
    expect(showHasArtistMusic([makeArtist(), makeArtist({ id: 2 })])).toBe(false)
  })
})

describe('ShowArtistMusicPanel', () => {
  it('renders a player for each act that has one', () => {
    render(
      <ShowArtistMusicPanel
        artists={[
          WITH_BANDCAMP,
          makeArtist({ id: 2, name: 'Low Ceiling', slug: 'low-ceiling' }),
        ]}
      />
    )

    expect(screen.getByTestId('embed-Sunn Amps')).toBeInTheDocument()
    expect(screen.queryByTestId('embed-Low Ceiling')).toBeNull()
  })

  it('links an act that has a page', () => {
    render(<ShowArtistMusicPanel artists={[WITH_BANDCAMP]} />)

    expect(screen.getByRole('link', { name: 'Sunn Amps' })).toHaveAttribute(
      'href',
      '/artists/sunn-amps'
    )
  })

  it('names an act with no page as plain text', () => {
    render(
      <ShowArtistMusicPanel
        artists={[{ ...WITH_BANDCAMP, slug: undefined } as never]}
      />
    )

    expect(screen.queryByRole('link', { name: 'Sunn Amps' })).toBeNull()
    expect(screen.getByText('Sunn Amps')).toBeInTheDocument()
  })

  // The caller gates on `showHasArtistMusic`, but the panel must not render an
  // empty bordered block if that ever drifts.
  it('renders nothing when no act has music', () => {
    const { container } = render(
      <ShowArtistMusicPanel artists={[makeArtist()]} />
    )

    expect(container.firstChild).toBeNull()
  })
})

describe('ArtistBase', () => {
  it('states where a placeable act is based', () => {
    render(
      <ArtistBase artist={makeArtist({ city: 'Tempe', state: 'AZ' } as never)} />
    )

    expect(screen.getByText(/Tempe, AZ/)).toBeInTheDocument()
  })

  it('renders nothing for an act with no placeable location', () => {
    const { container } = render(<ArtistBase artist={makeArtist()} />)

    expect(container.firstChild).toBeNull()
  })
})
