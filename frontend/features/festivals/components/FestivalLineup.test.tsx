import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FestivalLineup } from './FestivalLineup'
import type { FestivalArtist } from '../types'

function makeArtist(overrides: Partial<FestivalArtist> = {}): FestivalArtist {
  return {
    id: 1,
    artist_id: 1,
    artist_slug: 'artist-one',
    artist_name: 'Artist One',
    billing_tier: 'mid_card',
    position: 1,
    day_date: null,
    stage: null,
    set_time: null,
    venue_id: null,
    ...overrides,
  }
}

describe('FestivalLineup', () => {
  it('renders nothing when there are no artists', () => {
    const { container } = render(<FestivalLineup artists={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders artist names linking to artist pages', () => {
    render(
      <FestivalLineup
        artists={[
          makeArtist({ id: 1, artist_name: 'Headliner', artist_slug: 'headliner', billing_tier: 'headliner' }),
        ]}
      />
    )
    const link = screen.getByRole('link', { name: 'Headliner' })
    expect(link).toHaveAttribute('href', '/artists/headliner')
  })

  it('renders artists without a slug as plain text (href #)', () => {
    render(
      <FestivalLineup
        artists={[makeArtist({ artist_name: 'No Slug', artist_slug: '' })]}
      />
    )
    const link = screen.getByText('No Slug').closest('a')
    expect(link).toHaveAttribute('href', '#')
  })

  it('groups artists under their billing-tier headings in display order', () => {
    render(
      <FestivalLineup
        artists={[
          makeArtist({ id: 1, artist_name: 'Big', billing_tier: 'headliner' }),
          makeArtist({ id: 2, artist_name: 'Small', billing_tier: 'undercard' }),
        ]}
      />
    )
    expect(screen.getByText('Headliner')).toBeInTheDocument()
    expect(screen.getByText('Undercard')).toBeInTheDocument()
    expect(screen.getByText('Big')).toBeInTheDocument()
    expect(screen.getByText('Small')).toBeInTheDocument()
  })

  it('defaults a missing billing_tier to mid_card', () => {
    render(
      <FestivalLineup
        artists={[makeArtist({ artist_name: 'Untiered', billing_tier: '' })]}
      />
    )
    expect(screen.getByText('Mid Card')).toBeInTheDocument()
    expect(screen.getByText('Untiered')).toBeInTheDocument()
  })

  describe('multiDay grouping', () => {
    it('renders a day header per distinct day_date', () => {
      render(
        <FestivalLineup
          multiDay
          artists={[
            makeArtist({ id: 1, artist_name: 'Day1 Act', day_date: '2025-05-09' }),
            makeArtist({ id: 2, artist_name: 'Day2 Act', day_date: '2025-05-10' }),
          ]}
        />
      )
      // Friday May 9 / Saturday May 10, 2025 — assert on the weekday+month text.
      expect(screen.getByText(/May 9/)).toBeInTheDocument()
      expect(screen.getByText(/May 10/)).toBeInTheDocument()
      expect(screen.getByText('Day1 Act')).toBeInTheDocument()
      expect(screen.getByText('Day2 Act')).toBeInTheDocument()
    })

    it('buckets day-less artists under an Additional Artists heading', () => {
      render(
        <FestivalLineup
          multiDay
          artists={[
            makeArtist({ id: 1, artist_name: 'Scheduled', day_date: '2025-05-09' }),
            makeArtist({ id: 2, artist_name: 'Unscheduled', day_date: null }),
          ]}
        />
      )
      expect(screen.getByText('Additional Artists')).toBeInTheDocument()
      expect(screen.getByText('Unscheduled')).toBeInTheDocument()
    })
  })
})
