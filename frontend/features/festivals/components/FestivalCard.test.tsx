import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FestivalCard } from './FestivalCard'
import type { FestivalListItem } from '../types'

function makeFestival(overrides: Partial<FestivalListItem> = {}): FestivalListItem {
  return {
    id: 1,
    name: 'FORM Arcosanti',
    slug: 'form-arcosanti-2025',
    series_slug: 'form-arcosanti',
    edition_year: 2025,
    city: 'Mayer',
    state: 'AZ',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    status: 'confirmed',
    artist_count: 12,
    venue_count: 1,
    ...overrides,
  }
}

describe('FestivalCard', () => {
  it('renders as an article with the festival name linking to its page', () => {
    render(<FestivalCard festival={makeFestival()} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'FORM Arcosanti' })
    expect(link).toHaveAttribute('href', '/festivals/form-arcosanti-2025')
  })

  it('renders the status badge label', () => {
    render(<FestivalCard festival={makeFestival({ status: 'announced' })} />)
    expect(screen.getByText('Announced')).toBeInTheDocument()
  })

  it('renders the city/state location', () => {
    render(<FestivalCard festival={makeFestival()} />)
    expect(screen.getByText('Mayer, AZ')).toBeInTheDocument()
  })

  it('renders the formatted date range and edition year', () => {
    render(<FestivalCard festival={makeFestival()} />)
    expect(screen.getByText('May 9–11, 2025')).toBeInTheDocument()
    expect(screen.getByText('2025')).toBeInTheDocument()
  })

  it('singularizes the artist count', () => {
    render(<FestivalCard festival={makeFestival({ artist_count: 1 })} />)
    expect(screen.getByText('1 artist')).toBeInTheDocument()
  })

  it('pluralizes the artist count', () => {
    render(<FestivalCard festival={makeFestival({ artist_count: 12 })} />)
    expect(screen.getByText('12 artists')).toBeInTheDocument()
  })

  it('omits location when neither city nor state is set', () => {
    render(
      <FestivalCard festival={makeFestival({ city: null, state: null })} />
    )
    expect(screen.queryByText(/, AZ/)).not.toBeInTheDocument()
  })

  describe('compact density', () => {
    it('renders the name as a link and the artist count', () => {
      render(<FestivalCard festival={makeFestival()} density="compact" />)
      const link = screen.getByRole('link', { name: 'FORM Arcosanti' })
      expect(link).toHaveAttribute('href', '/festivals/form-arcosanti-2025')
      expect(screen.getByText('12 artists')).toBeInTheDocument()
    })
  })

  describe('expanded density', () => {
    it('shows the venue count when present', () => {
      render(
        <FestivalCard
          festival={makeFestival({ venue_count: 3 })}
          density="expanded"
        />
      )
      expect(screen.getByText('3 venues')).toBeInTheDocument()
    })

    it('hides the venue count when zero', () => {
      render(
        <FestivalCard
          festival={makeFestival({ venue_count: 0 })}
          density="expanded"
        />
      )
      expect(screen.queryByText(/venue/)).not.toBeInTheDocument()
    })
  })
})
