import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ArtistCard } from './ArtistCard'
import type { Artist } from '@/lib/types/artist'

function makeArtist(overrides: Partial<Artist> = {}): Artist {
  return {
    id: 1,
    slug: 'test-artist',
    name: 'Test Artist',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: null,
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtistCard', () => {
  it('renders artist name as a link', () => {
    renderWithProviders(<ArtistCard artist={makeArtist()} />)

    const link = screen.getByRole('link', { name: 'Test Artist' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/artists/test-artist')
  })

  it('renders location with city and state', () => {
    renderWithProviders(<ArtistCard artist={makeArtist()} />)

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('does not render location when city and state are null', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ city: null, state: null })} />
    )

    expect(screen.queryByText('Location Unknown')).not.toBeInTheDocument()
  })

  it('renders social links when present', () => {
    renderWithProviders(
      <ArtistCard
        artist={makeArtist({
          social: {
            instagram: '@testartist',
            facebook: null,
            twitter: null,
            youtube: null,
            spotify: 'https://open.spotify.com/artist/123',
            soundcloud: null,
            bandcamp: null,
            website: 'https://testartist.com',
          },
        })}
      />
    )

    expect(screen.getByTitle('Instagram')).toBeInTheDocument()
    expect(screen.getByTitle('Spotify')).toBeInTheDocument()
    expect(screen.getByTitle('Website')).toBeInTheDocument()
  })

  it('uses correct slug in link', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ slug: 'the-national', name: 'The National' })} />
    )

    const link = screen.getByRole('link', { name: 'The National' })
    expect(link).toHaveAttribute('href', '/artists/the-national')
  })
})
