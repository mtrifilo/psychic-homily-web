import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioShowCard } from './RadioShowCard'
import type { RadioShowListItem } from '../types'

// next/link is mocked per-test (no global mock in test/setup.ts). EntityCardTitle
// renders through this same Link, so title-link assertions stay real.
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makeShow(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 10,
    station_name: 'WFMU',
    name: 'The Drummer Show',
    slug: 'drummer',
    host_name: 'The Drummer',
    genre_tags: ['punk', 'garage', 'soul'],
    image_url: null,
    is_active: true,
    episode_count: 12,
    ...overrides,
  }
}

describe('RadioShowCard', () => {
  it('renders the show name as a link to the show detail page', () => {
    render(<RadioShowCard show={makeShow()} stationSlug="wfmu" />)
    const link = screen.getByText('The Drummer Show').closest('a')
    expect(link).toHaveAttribute('href', '/radio/wfmu/drummer')
  })

  it('renders the host name', () => {
    render(<RadioShowCard show={makeShow()} stationSlug="wfmu" />)
    expect(screen.getByText('Hosted by The Drummer')).toBeInTheDocument()
  })

  it('omits the host line when host_name is null', () => {
    render(<RadioShowCard show={makeShow({ host_name: null })} stationSlug="wfmu" />)
    expect(screen.queryByText(/Hosted by/)).not.toBeInTheDocument()
  })

  it('renders at most three genre tags', () => {
    render(
      <RadioShowCard
        show={makeShow({ genre_tags: ['a', 'b', 'c', 'd', 'e'] })}
        stationSlug="wfmu"
      />
    )
    expect(screen.getByText('a')).toBeInTheDocument()
    expect(screen.getByText('b')).toBeInTheDocument()
    expect(screen.getByText('c')).toBeInTheDocument()
    expect(screen.queryByText('d')).not.toBeInTheDocument()
  })

  it('handles a null genre_tags array without crashing', () => {
    render(<RadioShowCard show={makeShow({ genre_tags: null })} stationSlug="wfmu" />)
    expect(screen.getByText('The Drummer Show')).toBeInTheDocument()
  })

  it('pluralizes the episode count', () => {
    render(<RadioShowCard show={makeShow({ episode_count: 12 })} stationSlug="wfmu" />)
    expect(screen.getByText('12 episodes')).toBeInTheDocument()
  })

  it('singularizes a single episode', () => {
    render(<RadioShowCard show={makeShow({ episode_count: 1 })} stationSlug="wfmu" />)
    expect(screen.getByText('1 episode')).toBeInTheDocument()
  })

  it('hides the episode count when there are no episodes', () => {
    render(<RadioShowCard show={makeShow({ episode_count: 0 })} stationSlug="wfmu" />)
    expect(screen.queryByText(/episode/)).not.toBeInTheDocument()
  })

  it('renders the show image with alt text when image_url is set', () => {
    render(
      <RadioShowCard
        show={makeShow({ image_url: 'https://example.com/show.jpg' })}
        stationSlug="wfmu"
      />
    )
    const img = screen.getByAltText('The Drummer Show')
    expect(img).toHaveAttribute('src', 'https://example.com/show.jpg')
  })
})
