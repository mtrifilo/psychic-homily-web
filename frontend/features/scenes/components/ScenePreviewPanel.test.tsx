import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
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
// Default: a quiet week — tests for the "This week" section override this.
const mockUseSceneShows = vi.fn()
vi.mock('../hooks', () => ({
  useSceneArtists: (opts: unknown) => mockUseSceneArtists(opts),
  useSceneShows: (slug: string) => mockUseSceneShows(slug),
}))

// Stub MusicEmbed so the test asserts WHICH artist/track is chosen without its
// on-mount /api/bandcamp/album-id fetch + iframe resolution (covered by its own
// tests). It echoes the props the panel passes.
const mockMusicEmbed = vi.fn()
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: (props: { bandcampAlbumUrl?: string | null; artistName: string }) => {
    mockMusicEmbed(props)
    return <div data-testid="music-embed">{props.artistName}</div>
  },
}))

import { ScenePreviewPanel } from './ScenePreviewPanel'
import { EMBED_SEARCH_LIMIT } from './ScenePreviewContent'

const scene: SceneListItem = {
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  venue_count: 9,
  upcoming_show_count: 283,
  total_show_count: 337,
  shows_this_week: 0,
  latitude: 41.88,
  longitude: -87.63,
}

describe('ScenePreviewPanel', () => {
  beforeEach(() => {
    mockUseSceneArtists.mockReset()
    mockMusicEmbed.mockReset()
    mockUseSceneShows.mockReset()
    mockUseSceneShows.mockReturnValue({ data: { shows: [] }, isLoading: false })
  })

  it('renders the city, counts, active artists, and a link into the scene', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [{ id: 1, slug: 'band-a', name: 'Band A' }], total: 1 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(screen.getByText('Chicago, IL')).toBeInTheDocument()
    expect(screen.getByText(/283 upcoming · 9 venues/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Band A' }),
    ).toHaveAttribute('href', '/artists/band-a')
    expect(
      screen.getByRole('link', { name: /open scene/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
    // Pins the roster fetch contract: this slug, and the WIDER embed-search
    // limit (not the displayed count) so the player isn't capped to the shown
    // few (PSY-1224 review).
    expect(mockUseSceneArtists).toHaveBeenCalledWith({
      slug: 'chicago-il',
      limit: EMBED_SEARCH_LIMIT,
    })
  })

  it('flags active roster members with an accessible "(active)" marker', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [
          { id: 1, slug: 'band-a', name: 'Band A', is_active: true },
          { id: 2, slug: 'band-b', name: 'Band B', is_active: false },
        ],
        total: 2,
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    const bandA = screen.getByText('Band A').closest('li')!
    expect(within(bandA).getByText('(active)')).toBeInTheDocument()
    const bandB = screen.getByText('Band B').closest('li')!
    expect(within(bandB).queryByText('(active)')).not.toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    const onClose = vi.fn()
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={onClose} />)

    fireEvent.click(screen.getByRole('button', { name: /close scene preview/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('shows a loading state while artists load', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: true })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('handles a scene with an empty roster', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [], total: 0 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)
    expect(screen.getByText(/no artists based here yet/i)).toBeInTheDocument()
  })

  // PSY-1224: the "instant payoff" — play the first active local band that has an
  // embeddable Bandcamp track.
  it('plays the first active artist that has a Bandcamp embed', () => {
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
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(screen.getByRole('heading', { name: 'Listen' })).toBeInTheDocument()
    expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    expect(mockMusicEmbed).toHaveBeenCalledWith(
      expect.objectContaining({
        bandcampAlbumUrl: 'https://band-a.bandcamp.com/album/x',
        artistName: 'Band A',
      }),
    )
  })

  it('skips inactive bands and active bands without an embed when choosing the track', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [
          { id: 1, slug: 'a', name: 'Active No Embed', is_active: true },
          {
            id: 2,
            slug: 'b',
            name: 'Inactive With Embed',
            is_active: false,
            bandcamp_embed_url: 'https://b.bandcamp.com/album/y',
          },
          {
            id: 3,
            slug: 'c',
            name: 'Active With Embed',
            is_active: true,
            bandcamp_embed_url: 'https://c.bandcamp.com/album/z',
          },
        ],
        total: 3,
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(mockMusicEmbed).toHaveBeenCalledTimes(1)
    expect(mockMusicEmbed).toHaveBeenCalledWith(
      expect.objectContaining({
        artistName: 'Active With Embed',
        bandcampAlbumUrl: 'https://c.bandcamp.com/album/z',
      }),
    )
  })

  it('embeds a band ranked below the displayed list (wider fetch than display)', () => {
    // Six active bands without embeds fill the visible list; a 7th active band
    // with an embed must still be chosen for the player even though it's not
    // shown — the whole point of fetching wider than we display.
    const visible = Array.from({ length: 6 }, (_, i) => ({
      id: i + 1,
      slug: `band-${i}`,
      name: `Band ${i}`,
      is_active: true,
    }))
    const deepCut = {
      id: 99,
      slug: 'deep-cut',
      name: 'Deep Cut',
      is_active: true,
      bandcamp_embed_url: 'https://deep-cut.bandcamp.com/album/x',
    }
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [...visible, deepCut], total: 7 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    // The player uses the deep-roster band (its name shows in the embed)...
    expect(mockMusicEmbed).toHaveBeenCalledWith(
      expect.objectContaining({
        artistName: 'Deep Cut',
        bandcampAlbumUrl: 'https://deep-cut.bandcamp.com/album/x',
      }),
    )
    expect(screen.getByTestId('music-embed')).toHaveTextContent('Deep Cut')
    // ...but the displayed roster is capped — Deep Cut isn't a list entry.
    expect(
      screen.queryByRole('link', { name: 'Deep Cut' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Band 0' })).toBeInTheDocument()
  })

  it('shows no player when no active artist has an embed (graceful empty)', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [
          { id: 1, slug: 'a', name: 'Active No Embed', is_active: true },
          {
            id: 2,
            slug: 'b',
            name: 'Inactive With Embed',
            is_active: false,
            bandcamp_embed_url: 'https://b.bandcamp.com/album/y',
          },
        ],
        total: 2,
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(
      screen.queryByRole('heading', { name: 'Listen' }),
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()
  })

  it('lists this-week shows with date, link, and venue (PSY-1309)', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [], total: 0 },
      isLoading: false,
    })
    mockUseSceneShows.mockReturnValue({
      data: {
        shows: [
          {
            id: 42,
            slug: 'big-show',
            title: 'Big Show',
            event_date: '2026-07-04',
            venue_name: 'Valley Bar',
          },
          // No slug → the link falls back to the id.
          { id: 43, title: 'Slugless Show', event_date: '2026-07-05' },
        ],
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(screen.getByRole('heading', { name: 'This week' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Big Show' })).toHaveAttribute(
      'href',
      '/shows/big-show',
    )
    expect(screen.getByRole('link', { name: 'Slugless Show' })).toHaveAttribute(
      'href',
      '/shows/43',
    )
    // UTC-pinned date formatting: 2026-07-04 renders as July 4, never shifted
    // to July 3 by a negative-offset local zone.
    expect(screen.getByText(/Jul 4/)).toBeInTheDocument()
    expect(screen.getByText(/Valley Bar/)).toBeInTheDocument()
  })

  it('renders no "This week" section on a quiet week', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [], total: 0 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(
      screen.queryByRole('heading', { name: 'This week' }),
    ).not.toBeInTheDocument()
  })
})
