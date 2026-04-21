import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ArtistCard } from './ArtistCard'
import type { ArtistListItem } from '../types'

function makeArtist(overrides: Partial<ArtistListItem> = {}): ArtistListItem {
  return {
    id: 1,
    slug: 'test-artist',
    name: 'Test Artist',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    upcoming_show_count: 3,
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

  it('renders artist name as a heading', () => {
    renderWithProviders(<ArtistCard artist={makeArtist()} />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Test Artist' })
    expect(heading).toBeInTheDocument()
  })

  it('renders upcoming show count', () => {
    renderWithProviders(<ArtistCard artist={makeArtist({ upcoming_show_count: 5 })} />)

    expect(screen.getByText('5 upcoming')).toBeInTheDocument()
  })

  it('renders "No upcoming shows" hint when count is zero and no last-show date (PSY-495)', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ upcoming_show_count: 0 })} />
    )

    expect(screen.getByText('No upcoming shows')).toBeInTheDocument()
  })

  it('renders "No upcoming shows · last show Mon YYYY" when last_show_date is known (PSY-495)', () => {
    renderWithProviders(
      <ArtistCard
        artist={makeArtist({
          upcoming_show_count: 0,
          last_show_date: '2024-03-15T00:00:00Z',
        })}
      />
    )

    expect(
      screen.getByText('No upcoming shows · last show Mar 2024')
    ).toBeInTheDocument()
  })

  it('falls back to plain "No upcoming shows" when last_show_date is unparseable', () => {
    renderWithProviders(
      <ArtistCard
        artist={makeArtist({
          upcoming_show_count: 0,
          last_show_date: 'not-a-date',
        })}
      />
    )

    expect(screen.getByText('No upcoming shows')).toBeInTheDocument()
  })

  it('renders location with city and state', () => {
    renderWithProviders(<ArtistCard artist={makeArtist()} />)

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
  })

  it('renders location with only city', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ city: 'Chicago', state: null })} />
    )

    expect(screen.getByText('Chicago')).toBeInTheDocument()
  })

  it('renders location with only state', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ city: null, state: 'IL' })} />
    )

    expect(screen.getByText('IL')).toBeInTheDocument()
  })

  it('does not render location when city and state are null', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ city: null, state: null })} />
    )

    expect(screen.queryByText('Phoenix, AZ')).not.toBeInTheDocument()
  })

  it('uses correct slug in link', () => {
    renderWithProviders(
      <ArtistCard artist={makeArtist({ slug: 'the-national', name: 'The National' })} />
    )

    const link = screen.getByRole('link', { name: 'The National' })
    expect(link).toHaveAttribute('href', '/artists/the-national')
  })

  it('renders as an article element', () => {
    renderWithProviders(<ArtistCard artist={makeArtist()} />)

    expect(screen.getByRole('article')).toBeInTheDocument()
  })
})
