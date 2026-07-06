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
// FollowButton pulls AuthContext (unavailable here) — mock at the module
// boundary, same idiom as VenueDetail/LabelDetail tests.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number | string }) => (
    <button data-testid="follow-button">
      Follow {entityType} {String(entityId)}
    </button>
  ),
}))

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
import { PREVIEW_ARTIST_LIMIT } from './ScenePreviewContent'

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
    // Pins the roster fetch contract: this slug and the display-sized limit. The
    // player no longer comes from this roster (the backend picks it over the full
    // roster — PSY-1294), so we no longer over-fetch beyond the shown few.
    expect(mockUseSceneArtists).toHaveBeenCalledWith({
      slug: 'chicago-il',
      limit: PREVIEW_ARTIST_LIMIT,
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

  it('focuses close on open and returns focus to the return target on close (PSY-1313)', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    // The keyboard path: AtlasGlobe passes the "Search scenes" trigger as the
    // panel's explicit focus-return target.
    const opener = document.createElement('button')
    opener.textContent = 'Search scenes'
    document.body.appendChild(opener)

    const { unmount } = renderWithProviders(
      <ScenePreviewPanel
        scene={scene}
        onClose={() => {}}
        returnFocusTo={{ current: opener }}
      />,
    )
    expect(
      screen.getByRole('button', { name: /close scene preview/i }),
    ).toHaveFocus()

    unmount()
    expect(opener).toHaveFocus()
    opener.remove()
  })

  it('leaves focus alone on close when the user has tabbed elsewhere (non-modal)', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    const { unmount } = renderWithProviders(
      <ScenePreviewPanel
        scene={scene}
        onClose={() => {}}
        returnFocusTo={{ current: opener }}
      />,
    )
    // User tabs out of the non-modal panel into some other page control…
    const elsewhere = document.createElement('a')
    elsewhere.href = '#'
    document.body.appendChild(elsewhere)
    elsewhere.focus()
    // …then the panel closes (document-level Esc fires regardless of focus).
    unmount()
    expect(elsewhere).toHaveFocus()
    opener.remove()
    elsewhere.remove()
  })

  it('ignores Escape while typing in a field (the reopened search owns that Esc)', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    const onClose = vi.fn()
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={onClose} />)
    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
    input.remove()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not return focus to a target that left the DOM (no crash, no stray focus)', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    const opener = document.createElement('button')
    document.body.appendChild(opener)

    const { unmount } = renderWithProviders(
      <ScenePreviewPanel
        scene={scene}
        onClose={() => {}}
        returnFocusTo={{ current: opener }}
      />,
    )
    opener.remove()
    expect(() => unmount()).not.toThrow()
    expect(document.activeElement).toBe(document.body)
  })

  it('carries the follow-a-scene affordance, slug-addressed (PSY-1340)', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)
    expect(screen.getByTestId('follow-button')).toHaveTextContent(
      'Follow scenes chicago-il',
    )
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

  // PSY-1224/PSY-1294: the "instant payoff" — the preview plays the scene's
  // representative embed, chosen by the BACKEND over the full metro roster. The
  // component just renders whatever `representative_embed` the response carries;
  // the selection logic (active-first, below-window coverage, dormant fallback)
  // lives in the scene service and is covered by its integration tests.
  it('plays the scene representative embed from the response', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [{ id: 1, slug: 'band-a', name: 'Band A', is_active: true }],
        total: 1,
        representative_embed: {
          embed_url: 'https://band-a.bandcamp.com/album/x',
          artist_name: 'Band A',
          artist_slug: 'band-a',
        },
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(screen.getByRole('heading', { name: 'Listen' })).toBeInTheDocument()
    expect(screen.getByTestId('music-embed')).toBeInTheDocument()
    expect(mockMusicEmbed).toHaveBeenCalledTimes(1)
    expect(mockMusicEmbed).toHaveBeenCalledWith(
      expect.objectContaining({
        bandcampAlbumUrl: 'https://band-a.bandcamp.com/album/x',
        artistName: 'Band A',
      }),
    )
  })

  it('plays the representative embed even when its band is not in the shown roster', () => {
    // The whole point of PSY-1294: the embed source is decoupled from the fetched
    // roster. Six bands fill the shown list; the representative embed is a band
    // that isn't among them, and it still plays.
    const visible = Array.from({ length: 6 }, (_, i) => ({
      id: i + 1,
      slug: `band-${i}`,
      name: `Band ${i}`,
      is_active: true,
    }))
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: visible,
        total: 7,
        representative_embed: {
          embed_url: 'https://deep-cut.bandcamp.com/album/x',
          artist_name: 'Deep Cut',
          artist_slug: 'deep-cut',
        },
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    // The player uses the representative band (its name shows in the embed)...
    expect(mockMusicEmbed).toHaveBeenCalledWith(
      expect.objectContaining({
        artistName: 'Deep Cut',
        bandcampAlbumUrl: 'https://deep-cut.bandcamp.com/album/x',
      }),
    )
    expect(screen.getByTestId('music-embed')).toHaveTextContent('Deep Cut')
    // ...but it's not one of the shown roster rows.
    expect(
      screen.queryByRole('link', { name: 'Deep Cut' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Band 0' })).toBeInTheDocument()
  })

  it('shows no player when the scene has no representative embed (graceful empty)', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [{ id: 1, slug: 'a', name: 'Band A', is_active: true }],
        total: 1,
        representative_embed: null,
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
            // A real title wins over the bill names (PSY-1325 fallback order).
            artist_names: ['Some Band'],
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

  // PSY-1325: most shows have NO title — the bill is the display name. An
  // empty title must never render an invisible, unclickable link.
  it('falls back to bill names (capped) for untitled this-week shows', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [], total: 0 },
      isLoading: false,
    })
    mockUseSceneShows.mockReturnValue({
      data: {
        shows: [
          {
            id: 50,
            slug: 'untitled-bill',
            title: '',
            event_date: '2026-07-04',
            venue_name: 'Empty Bottle',
            artist_names: ['Fruit Bats', 'Hannah Frey'],
          },
          {
            id: 51,
            title: '',
            event_date: '2026-07-05',
            artist_names: ['A', 'B', 'C', 'D', 'E'],
          },
          // Degenerate payload: no title AND no artists — still a clickable link.
          { id: 52, title: '', event_date: '2026-07-06' },
          // Whitespace-only title is as invisible as an empty one — bill wins.
          {
            id: 53,
            title: '   ',
            event_date: '2026-07-07',
            artist_names: ['Trim Check'],
          },
          // Blank bill entries get the same trim gate: [' ', 'Real Band']
          // must render 'Real Band', not join a space into the link.
          {
            id: 54,
            title: '',
            event_date: '2026-07-08',
            artist_names: ['  ', 'Real Band'],
          },
          // Exactly at the cap: no dangling "+0 more".
          {
            id: 55,
            title: '',
            event_date: '2026-07-09',
            artist_names: ['X', 'Y', 'Z'],
          },
        ],
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(
      screen.getByRole('link', { name: 'Fruit Bats, Hannah Frey' }),
    ).toHaveAttribute('href', '/shows/untitled-bill')
    // Festival-sized bills cap at 3 names.
    expect(
      screen.getByRole('link', { name: 'A, B, C +2 more' }),
    ).toHaveAttribute('href', '/shows/51')
    expect(
      screen.getByRole('link', { name: 'Untitled Show' }),
    ).toHaveAttribute('href', '/shows/52')
    expect(
      screen.getByRole('link', { name: 'Trim Check' }),
    ).toHaveAttribute('href', '/shows/53')
    expect(
      screen.getByRole('link', { name: 'Real Band' }),
    ).toHaveAttribute('href', '/shows/54')
    expect(screen.getByRole('link', { name: 'X, Y, Z' })).toHaveAttribute(
      'href',
      '/shows/55',
    )
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
