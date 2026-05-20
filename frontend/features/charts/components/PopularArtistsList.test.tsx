import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@/test/utils'
import { PopularArtistsList } from './PopularArtistsList'
import type { PopularArtist } from '../types'

function makeArtist(overrides: Partial<PopularArtist> = {}): PopularArtist {
  return {
    artist_id: 1,
    name: 'Moonlight Parade',
    slug: 'moonlight-parade',
    image_url: '',
    follow_count: 120,
    upcoming_show_count: 5,
    score: 125,
    ...overrides,
  }
}

describe('PopularArtistsList', () => {
  it('renders ranked artists in the order provided', () => {
    const artists = [
      makeArtist({ artist_id: 1, name: 'First Artist', slug: 'first-artist' }),
      makeArtist({ artist_id: 2, name: 'Second Artist', slug: 'second-artist' }),
      makeArtist({ artist_id: 3, name: 'Third Artist', slug: 'third-artist' }),
    ]

    render(<PopularArtistsList artists={artists} />)

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)

    expect(within(items[0]).getByText('1')).toBeInTheDocument()
    expect(within(items[0]).getByText('First Artist')).toBeInTheDocument()
    expect(within(items[1]).getByText('2')).toBeInTheDocument()
    expect(within(items[1]).getByText('Second Artist')).toBeInTheDocument()
    expect(within(items[2]).getByText('3')).toBeInTheDocument()
    expect(within(items[2]).getByText('Third Artist')).toBeInTheDocument()
  })

  it('links each artist to its detail page', () => {
    render(<PopularArtistsList artists={[makeArtist()]} />)

    const link = screen.getByRole('link', { name: /Moonlight Parade/ })
    expect(link).toHaveAttribute('href', '/artists/moonlight-parade')
  })

  it('renders follower and upcoming-show counts in full mode', () => {
    render(<PopularArtistsList artists={[makeArtist({ follow_count: 120, upcoming_show_count: 5 })]} />)

    expect(screen.getByText('120')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('hides counts in compact mode', () => {
    render(<PopularArtistsList artists={[makeArtist({ follow_count: 120, upcoming_show_count: 5 })]} compact />)

    expect(screen.queryByText('120')).not.toBeInTheDocument()
    expect(screen.queryByText('5')).not.toBeInTheDocument()
    // Name still renders when compact.
    expect(screen.getByText('Moonlight Parade')).toBeInTheDocument()
  })

  it('renders the empty state when there are no artists', () => {
    render(<PopularArtistsList artists={[]} />)

    expect(screen.getByText('No popular artists right now.')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})
