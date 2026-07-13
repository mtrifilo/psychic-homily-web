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
// FollowButton pulls AuthContext (unavailable here) — mock at the module
// boundary, same idiom as VenueDetail/LabelDetail tests.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number | string }) => (
    <button data-testid="follow-button">
      Follow {entityType} {String(entityId)}
    </button>
  ),
}))

// SceneNotifyModeToggle also pulls AuthContext and has focused coverage in its
// own suite; keep this list composition test isolated from that auth concern.
vi.mock('./SceneNotifyModeToggle', () => ({
  SceneNotifyModeToggle: () => null,
}))

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
import { PREVIEW_ARTIST_LIMIT } from './ScenePreviewContent'

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

  it('stars followed scenes and leaves the rest unmarked (PSY-1340)', () => {
    renderWithProviders(
      <MobileSceneList
        scenes={scenes}
        loading={false}
        followedSlugs={new Set([scenes[0].slug])}
      />,
    )
    const stars = screen.getAllByRole('img', { name: 'Followed scene' })
    expect(stars).toHaveLength(1)
    expect(stars[0].closest('button')).toHaveTextContent(
      `${scenes[0].city}, ${scenes[0].state}`,
    )
  })

  it('fetches nothing while all rows are collapsed', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    expect(mockUseSceneArtists).not.toHaveBeenCalled()
    expect(mockUseSceneShows).not.toHaveBeenCalled()
  })

  it('expands a row into the scene preview, fetching only that scene', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Phoenix, AZ/ }))

    // Same roster-fetch contract as the desktop panel: the display-sized limit.
    // The player comes from the backend's representative embed (PSY-1294), so we
    // no longer over-fetch a wider embed-search window.
    expect(mockUseSceneArtists).toHaveBeenCalledWith({
      slug: 'phoenix-az',
      limit: PREVIEW_ARTIST_LIMIT,
    })
    expect(mockUseSceneShows).toHaveBeenCalledWith('phoenix-az')
    const row = screen.getByRole('button', { name: /Phoenix, AZ/ })
    expect(row).toHaveAttribute('aria-expanded', 'true')
    // aria-controls must point at the MOUNTED detail region (a dangling id
    // fails aria-valid-attr-value — the reason it's conditional).
    const controls = row.getAttribute('aria-controls')
    expect(controls).toBeTruthy()
    expect(document.getElementById(controls!)).toBeInTheDocument()
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
    // No dangling aria-controls while the detail region is unmounted.
    expect(row).not.toHaveAttribute('aria-controls')
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
        artists: [{ id: 1, slug: 'band-a', name: 'Band A', is_active: true }],
        total: 1,
        // The preview plays the backend's representative embed (PSY-1294), not a
        // per-artist field scanned from the roster above.
        representative_embed: {
          embed_url: 'https://band-a.bandcamp.com/album/x',
          artist_name: 'Band A',
          artist_slug: 'band-a',
        },
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

  it('shows both headline counts on the row (payoff parity with the panel)', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    expect(
      screen.getByRole('button', { name: /Chicago, IL/ }),
    ).toHaveTextContent('283 upcoming · 9 venues')
  })

  it('expands an unplaceable scene into a working preview (globe can never show these)', () => {
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Faketown, ZZ/ }))

    expect(mockUseSceneArtists).toHaveBeenCalledWith({
      slug: 'faketown-zz',
      limit: PREVIEW_ARTIST_LIMIT,
    })
    expect(
      screen.getByRole('link', { name: /open scene/i }),
    ).toHaveAttribute('href', '/scenes/faketown-zz')
  })

  it('distinguishes a failed roster fetch from an empty scene', () => {
    mockUseSceneArtists.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    renderWithProviders(<MobileSceneList scenes={scenes} loading={false} />)

    fireEvent.click(screen.getByRole('button', { name: /Chicago, IL/ }))

    expect(
      screen.getByText(/couldn’t load this scene’s artists/i),
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/no artists based here yet/i),
    ).not.toBeInTheDocument()
  })
})
