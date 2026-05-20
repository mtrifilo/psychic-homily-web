import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@/test/utils'
import { TrendingShowsList } from './TrendingShowsList'
import type { TrendingShow } from '../types'

function makeShow(overrides: Partial<TrendingShow> = {}): TrendingShow {
  return {
    show_id: 1,
    title: 'Night of Echoes',
    slug: 'night-of-echoes',
    date: '2026-04-15T20:00:00Z',
    venue_name: 'Valley Bar',
    venue_slug: 'valley-bar',
    city: 'Phoenix',
    artist_names: ['Moonlight Parade'],
    going_count: 42,
    interested_count: 88,
    total_attendance: 130,
    ...overrides,
  }
}

describe('TrendingShowsList', () => {
  it('renders ranked shows in the order provided', () => {
    const shows = [
      makeShow({ show_id: 1, title: 'First Show', slug: 'first-show' }),
      makeShow({ show_id: 2, title: 'Second Show', slug: 'second-show' }),
      makeShow({ show_id: 3, title: 'Third Show', slug: 'third-show' }),
    ]

    render(<TrendingShowsList shows={shows} />)

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)

    // Rank badges are 1-based and follow array order.
    expect(within(items[0]).getByText('1')).toBeInTheDocument()
    expect(within(items[0]).getByText('First Show')).toBeInTheDocument()
    expect(within(items[1]).getByText('2')).toBeInTheDocument()
    expect(within(items[1]).getByText('Second Show')).toBeInTheDocument()
    expect(within(items[2]).getByText('3')).toBeInTheDocument()
    expect(within(items[2]).getByText('Third Show')).toBeInTheDocument()
  })

  it('links each show to its detail page', () => {
    render(<TrendingShowsList shows={[makeShow()]} />)

    const link = screen.getByRole('link', { name: /Night of Echoes/ })
    expect(link).toHaveAttribute('href', '/shows/night-of-echoes')
  })

  it('renders attendance counts', () => {
    render(<TrendingShowsList shows={[makeShow({ going_count: 42, interested_count: 88 })]} />)

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('88')).toBeInTheDocument()
  })

  it('renders venue and city metadata in full mode', () => {
    render(<TrendingShowsList shows={[makeShow({ venue_name: 'Valley Bar', city: 'Phoenix' })]} />)

    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
    expect(screen.getByText('Phoenix')).toBeInTheDocument()
  })

  it('hides venue/city metadata in compact mode', () => {
    render(<TrendingShowsList shows={[makeShow({ venue_name: 'Valley Bar', city: 'Phoenix' })]} compact />)

    expect(screen.queryByText('Valley Bar')).not.toBeInTheDocument()
    expect(screen.queryByText('Phoenix')).not.toBeInTheDocument()
    // Attendance counts stay visible even when compact.
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  describe('display title fallback', () => {
    it('uses the explicit title when present', () => {
      render(<TrendingShowsList shows={[makeShow({ title: 'Explicit Title' })]} />)
      expect(screen.getByText('Explicit Title')).toBeInTheDocument()
    })

    it('falls back to "artists @ venue" when title is empty', () => {
      render(
        <TrendingShowsList
          shows={[makeShow({ title: '', artist_names: ['Band A', 'Band B'], venue_name: 'The Venue' })]}
        />
      )
      expect(screen.getByText('Band A, Band B @ The Venue')).toBeInTheDocument()
    })

    it('falls back to artists only when venue is missing', () => {
      render(
        <TrendingShowsList
          shows={[makeShow({ title: '', artist_names: ['Solo Act'], venue_name: '' })]}
        />
      )
      expect(screen.getByText('Solo Act')).toBeInTheDocument()
    })

    it('falls back to "Show @ venue" when only the venue is known', () => {
      render(
        <TrendingShowsList
          shows={[makeShow({ title: '', artist_names: [], venue_name: 'Lonely Venue' })]}
        />
      )
      expect(screen.getByText('Show @ Lonely Venue')).toBeInTheDocument()
    })

    it('falls back to "Untitled Show" when nothing is known', () => {
      render(
        <TrendingShowsList
          shows={[makeShow({ title: '', artist_names: [], venue_name: '' })]}
        />
      )
      expect(screen.getByText('Untitled Show')).toBeInTheDocument()
    })
  })

  it('renders the empty state when there are no shows', () => {
    render(<TrendingShowsList shows={[]} />)

    expect(screen.getByText('No upcoming shows right now.')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})
