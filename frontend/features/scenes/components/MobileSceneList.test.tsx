import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListItem } from '../types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseSceneArtists = vi.fn()
const mockUseSceneShows = vi.fn()
vi.mock('../hooks', () => ({
  useSceneArtists: (opts: unknown) => mockUseSceneArtists(opts),
  useSceneShows: (slug: string) => mockUseSceneShows(slug),
}))

// Stub MusicEmbed so expansion tests don't trigger its on-mount album-id fetch
// (covered by its own tests).
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: (props: { artistName: string }) => (
    <div data-testid="music-embed">{props.artistName}</div>
  ),
}))

import { MobileSceneList } from './MobileSceneList'
import { EMBED_SEARCH_LIMIT } from './ScenePreviewContent'

function makeScene(overrides: Partial<SceneListItem>): SceneListItem {
  return {
    city: 'Chicago',
    state: 'IL',
    slug: 'chicago-il',
    venue_count: 9,
    upcoming_show_count: 283,
    total_show_count: 337,
    shows_this_week: 0,
    latitude: 41.88,
    longitude: -87.63,
    ...overrides,
  }
}

// Deliberately NOT in liveliest-first order — the list must sort, not trust
// API order.
const scenes: SceneListItem[] = [
  makeScene({
    city: 'Faketown',
    state: 'ZZ',
    slug: 'faketown-zz',
    upcoming_show_count: 3,
    latitude: null,
    longitude: null,
  }),
  makeScene({}), // Chicago, 283 upcoming
  makeScene({
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    upcoming_show_count: 41,
  }),
]

describe('MobileSceneList', () => {
  beforeEach(() => {
    mockUseSceneArtists.mockReset()
    mockUseSceneShows.mockReset()
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [{ id: 1, slug: 'band-a', name: 'Band A', is_active: true }],
        total: 1,
      },
      isLoading: false,
    })
    mockUseSceneShows.mockReturnValue({ data: { shows: [] }, isLoading: false })
  })

  it('lists every scene (incl. unplaceable) sorted most-active-first', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    const rows = screen.getAllByRole('button', { expanded: false })
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining('Chicago, IL'),
      expect.stringContaining('Phoenix, AZ'),
      expect.stringContaining('Faketown, ZZ'),
    ])
  })

  it('fetches nothing while all rows are collapsed', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    expect(mockUseSceneArtists).not.toHaveBeenCalled()
    expect(mockUseSceneShows).not.toHaveBeenCalled()
  })

  it('expands a row into the scene preview, fetching only that scene', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Phoenix, AZ/ }))

    // Same roster-fetch contract as the desktop panel: the WIDER embed-search
    // limit, not the displayed count (PSY-1224).
    expect(mockUseSceneArtists).toHaveBeenCalledWith({
      slug: 'phoenix-az',
      limit: EMBED_SEARCH_LIMIT,
    })
    expect(mockUseSceneShows).toHaveBeenCalledWith('phoenix-az')
    expect(
      screen.getByRole('button', { name: /Phoenix, AZ/ }),
    ).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('link', { name: 'Band A' })).toHaveAttribute(
      'href',
      '/artists/band-a',
    )
    expect(
      screen.getByRole('link', { name: /open scene/i }),
    ).toHaveAttribute('href', '/scenes/phoenix-az')
  })

  it('collapses an expanded row on a second tap', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    const row = screen.getByRole('button', { name: /Phoenix, AZ/ })
    fireEvent.click(row)
    expect(row).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(row)
    expect(row).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('link', { name: /open scene/i }),
    ).not.toBeInTheDocument()
  })

  it('keeps one row open at a time (expanding B closes A)', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Phoenix, AZ/ }))
    fireEvent.click(screen.getByRole('button', { name: /Chicago, IL/ }))

    expect(
      screen.getByRole('button', { name: /Chicago, IL/ }),
    ).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.getByRole('button', { name: /Phoenix, AZ/ }),
    ).toHaveAttribute('aria-expanded', 'false')
    const openScene = screen.getByRole('link', { name: /open scene/i })
    expect(openScene).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('renders the playable embed inside an expanded row', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [
          {
            id: 1,
            slug: 'band-a',
            name: 'Band A',
            is_active: true,
            bandcamp_embed_url: 'https://band-a.bandcamp.com/album/x',
          },
        ],
        total: 1,
      },
      isLoading: false,
    })
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Chicago, IL/ }))

    expect(screen.getByTestId('music-embed')).toHaveTextContent('Band A')
  })

  it('shows the loading state', () => {
    renderWithProviders(<MobileSceneList scenes={[]} loading={true} />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })
})
