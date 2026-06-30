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
vi.mock('../hooks', () => ({
  useSceneArtists: (opts: unknown) => mockUseSceneArtists(opts),
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

const scene: SceneListItem = {
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  venue_count: 9,
  upcoming_show_count: 283,
  total_show_count: 337,
  latitude: 41.88,
  longitude: -87.63,
}

describe('ScenePreviewPanel', () => {
  beforeEach(() => {
    mockUseSceneArtists.mockReset()
    mockMusicEmbed.mockReset()
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
})
