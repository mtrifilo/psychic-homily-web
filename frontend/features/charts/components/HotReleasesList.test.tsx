import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@/test/utils'
import { HotReleasesList } from './HotReleasesList'
import type { HotRelease } from '../types'

function makeRelease(overrides: Partial<HotRelease> = {}): HotRelease {
  return {
    release_id: 1,
    title: 'Eternal Drift',
    slug: 'eternal-drift',
    release_date: '2026-03-01',
    artist_names: ['Moonlight Parade'],
    bookmark_count: 34,
    ...overrides,
  }
}

describe('HotReleasesList', () => {
  it('renders ranked releases in the order provided', () => {
    const releases = [
      makeRelease({ release_id: 1, title: 'First Release', slug: 'first-release' }),
      makeRelease({ release_id: 2, title: 'Second Release', slug: 'second-release' }),
      makeRelease({ release_id: 3, title: 'Third Release', slug: 'third-release' }),
    ]

    render(<HotReleasesList releases={releases} />)

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)

    expect(within(items[0]).getByText('1')).toBeInTheDocument()
    expect(within(items[0]).getByText('First Release')).toBeInTheDocument()
    expect(within(items[1]).getByText('2')).toBeInTheDocument()
    expect(within(items[1]).getByText('Second Release')).toBeInTheDocument()
    expect(within(items[2]).getByText('3')).toBeInTheDocument()
    expect(within(items[2]).getByText('Third Release')).toBeInTheDocument()
  })

  it('links each release to its detail page', () => {
    render(<HotReleasesList releases={[makeRelease()]} />)

    const link = screen.getByRole('link', { name: /Eternal Drift/ })
    expect(link).toHaveAttribute('href', '/releases/eternal-drift')
  })

  it('renders bookmark count', () => {
    render(<HotReleasesList releases={[makeRelease({ bookmark_count: 34 })]} />)

    expect(screen.getByText('34')).toBeInTheDocument()
  })

  it('renders joined artist names in full mode', () => {
    render(<HotReleasesList releases={[makeRelease({ artist_names: ['Band A', 'Band B'] })]} />)

    expect(screen.getByText('Band A, Band B')).toBeInTheDocument()
  })

  it('handles a missing release date without rendering a date', () => {
    render(<HotReleasesList releases={[makeRelease({ release_date: null, artist_names: ['Solo'] })]} />)

    // Title and artist still render; absence of a date must not throw.
    expect(screen.getByText('Eternal Drift')).toBeInTheDocument()
    expect(screen.getByText('Solo')).toBeInTheDocument()
  })

  it('omits the artist line when there are no artist names', () => {
    render(<HotReleasesList releases={[makeRelease({ artist_names: [], release_date: null })]} />)

    expect(screen.getByText('Eternal Drift')).toBeInTheDocument()
    expect(screen.queryByText('Moonlight Parade')).not.toBeInTheDocument()
  })

  it('hides artist metadata but keeps bookmark count in compact mode', () => {
    render(<HotReleasesList releases={[makeRelease({ artist_names: ['Band A'], bookmark_count: 34 })]} compact />)

    expect(screen.queryByText('Band A')).not.toBeInTheDocument()
    // Bookmark count is outside the !compact block, so it stays visible.
    expect(screen.getByText('34')).toBeInTheDocument()
    expect(screen.getByText('Eternal Drift')).toBeInTheDocument()
  })

  it('renders the empty state when there are no releases', () => {
    render(<HotReleasesList releases={[]} />)

    expect(screen.getByText('No recent releases yet.')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})
