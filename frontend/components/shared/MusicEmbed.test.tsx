import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { QueryClientProvider } from '@tanstack/react-query'
// MusicEmbed resolves its Bandcamp embed via TanStack Query (PSY-1102), so it
// must render inside a QueryClientProvider. `renderWithProviders` (re-exported
// as `render`) wraps each render in a fresh client with retries disabled, which
// keeps the `mockRejectedValueOnce` error-path tests deterministic.
import { createTestQueryClient, render } from '../../test/utils'
import { BANDCAMP_SLIM_EMBED_HEIGHT_PX, MusicEmbed } from './MusicEmbed'
import { COARSE_POINTER_HIT_AREA_CLASS } from './touchTarget'
import { hasRenderableMusic } from '@/lib/musicAvailability'

// The page theme, as next-themes reports it. `undefined` is what next-themes
// reports before it has read storage, and what a tree with no provider sees.
const theme = vi.hoisted(() => ({ resolved: undefined as string | undefined }))
vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: theme.resolved }),
}))


describe('MusicEmbed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
    theme.resolved = undefined
  })

  it('renders loading state initially when bandcamp URL is provided', () => {
    vi.spyOn(global, 'fetch').mockImplementation(
      () => new Promise(() => {}) // never resolves
    )
    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
      />
    )
    // Loading section should be visible
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders "Music" heading when not compact and loading', () => {
    vi.spyOn(global, 'fetch').mockImplementation(
      () => new Promise(() => {})
    )
    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
        compact={false}
      />
    )
    expect(screen.getByText('Music')).toBeInTheDocument()
  })

  it('does not render "Music" heading when compact and loading', () => {
    vi.spyOn(global, 'fetch').mockImplementation(
      () => new Promise(() => {})
    )
    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
        compact={true}
      />
    )
    expect(screen.queryByText('Music')).not.toBeInTheDocument()
  })

  it('renders bandcamp iframe when album ID is fetched successfully', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      json: async () => ({ kind: 'album', id: '12345' }),
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Bandcamp')
      expect(iframe).toBeInTheDocument()
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('album=12345')
      )
    })
  })

  it('renders a track embed when the resolver returns a track', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      json: async () => ({ kind: 'track', id: '2445352951' }),
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/track/test"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Bandcamp')
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('track=2445352951')
      )
    })
  })

  it('renders spotify iframe when spotify URL is provided', async () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toBeInTheDocument()
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('embed/artist/4Z8W4fKeB5YxbusRsdQVPb')
      )
    })
  })

  it('parses spotify URI format', async () => {
    render(
      <MusicEmbed
        spotifyUrl="spotify:artist:0TnOYISbd1XYRBk9myaseg"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('embed/artist/0TnOYISbd1XYRBk9myaseg')
      )
    })
  })

  // PSY-1195: release pages pass an album/track Spotify URL (not an artist URL).
  it('renders a spotify album embed when an album URL is provided', async () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/album/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('embed/album/4Z8W4fKeB5YxbusRsdQVPb')
      )
    })
  })

  it('renders a spotify track embed when a track URL is provided', async () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/track/0TnOYISbd1XYRBk9myaseg"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('embed/track/0TnOYISbd1XYRBk9myaseg')
      )
    })
  })

  it('renders no spotify embed for a non-embeddable Spotify URL (playlist)', async () => {
    const { container } = render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/playlist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      // No embeddable URL of any kind → MusicEmbed renders nothing.
      expect(container.querySelector('section')).not.toBeInTheDocument()
    })
  })

  it('prefers a bandcamp album embed over a spotify album URL (PSY-1187 precedence)', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      json: async () => ({ kind: 'album', id: '77777' }),
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        spotifyUrl="https://open.spotify.com/album/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      expect(screen.getByTitle('Test Artist on Bandcamp')).toBeInTheDocument()
      expect(screen.queryByTitle('Test Artist on Spotify')).not.toBeInTheDocument()
    })
  })

  it('renders fallback link when bandcamp fetch fails', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: false,
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const link = screen.getByText('Listen to Test Artist on Bandcamp')
      expect(link).toBeInTheDocument()
      expect(link).toHaveAttribute('href', 'https://band.bandcamp.com/album/test')
      expect(link).toHaveAttribute('target', '_blank')
    })
  })

  // The link's text line is 20px tall. The 24px touch target comes from the
  // invisible hit area alone, so the link takes the same room in layout on
  // every pointer and the blocks around it do not move.
  it('grows the fallback link\'s tap target on coarse pointers without resizing it', async () => {
    render(
      <MusicEmbed
        bandcampProfileUrl="https://band.bandcamp.com"
        artistName="Test Artist"
      />
    )

    const link = await screen.findByRole('link', {
      name: 'Listen to Test Artist on Bandcamp',
    })
    const hitArea = COARSE_POINTER_HIT_AREA_CLASS.split(' ')
    expect(link).toHaveClass(...hitArea)
    const coarseOnly = [...link.classList].filter(token =>
      token.startsWith('pointer-coarse:')
    )
    expect(coarseOnly.sort()).toEqual([...hitArea].sort())
  })

  // PSY-1102 adversarial review: a transient 5xx from the scraper route must
  // NOT cache as a durable null "success" (which would freeze the embed on the
  // fallback link for the whole staleTime). resolveBandcampEmbed throws on 5xx
  // so the query errors instead of caching; this mount still falls through to
  // Spotify, and a later mount would retry.
  it('falls through to spotify when the bandcamp resolve returns a 5xx', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 503,
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      expect(screen.getByTitle('Test Artist on Spotify')).toBeInTheDocument()
    })
  })

  it('renders fallback link for bandcamp profile URL when no album URL', async () => {
    render(
      <MusicEmbed
        bandcampProfileUrl="https://band.bandcamp.com"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      const link = screen.getByText('Listen to Test Artist on Bandcamp')
      expect(link).toHaveAttribute('href', 'https://band.bandcamp.com')
    })
  })

  it('returns null when no URLs are provided', async () => {
    const { container } = render(
      <MusicEmbed artistName="Test Artist" />
    )

    await waitFor(() => {
      // After resolving, the section should not be present
      expect(container.querySelector('section')).not.toBeInTheDocument()
    })
  })

  it('prioritizes bandcamp over spotify', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      json: async () => ({ kind: 'album', id: '99999' }),
    } as Response)

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      expect(screen.getByTitle('Test Artist on Bandcamp')).toBeInTheDocument()
      expect(screen.queryByTitle('Test Artist on Spotify')).not.toBeInTheDocument()
    })
  })

  it('falls back to spotify when bandcamp fetch throws an error', async () => {
    vi.spyOn(global, 'fetch').mockRejectedValueOnce(new Error('Network error'))

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    // When bandcamp fetch throws, the catch block fires, then priority 2 (spotify) is checked
    // Since spotify URL is valid, it wins over the bandcamp fallback link
    await waitFor(() => {
      expect(screen.getByTitle('Test Artist on Spotify')).toBeInTheDocument()
    })
  })

  it('falls back to bandcamp link when fetch throws and no spotify URL', async () => {
    vi.spyOn(global, 'fetch').mockRejectedValueOnce(new Error('Network error'))

    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
      />
    )

    await waitFor(() => {
      expect(screen.getByText('Listen to Test Artist on Bandcamp')).toBeInTheDocument()
    })
  })

  it('uses compact height for spotify iframe', async () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
        compact={true}
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toHaveStyle({ height: '152px' })
    })
  })

  it('uses full height for spotify iframe when not compact', async () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
        compact={false}
      />
    )

    await waitFor(() => {
      const iframe = screen.getByTitle('Test Artist on Spotify')
      expect(iframe).toHaveStyle({ height: '352px' })
    })
  })

  // PSY-1966. The fallback link is the sink: `artists.bandcamp_embed_url` and
  // `social.bandcamp` are contributor-writable, and nine surfaces hand this
  // component the raw column, so the gate lives here rather than at the callers.
  // A value that is not provably a Bandcamp page must render NO link: an
  // outbound href labelled "Listen to <artist> on Bandcamp" is a trusted label
  // on an attacker-chosen destination.
  describe('outbound-link gate', () => {
    // A resolve that finds no embed is the ordinary way to reach the fallback
    // (deleted or renamed release), not only an outage.
    const noEmbed = () =>
      vi.spyOn(global, 'fetch').mockResolvedValue({ ok: false, status: 404 } as Response)

    const hostileAlbumUrls = [
      'https://evil.test/album/checkout',
      'https://bandcamp.com.attacker.test/album/x',
      'https://evil.test/?next=https://band.bandcamp.com/album/y',
      // On a Bandcamp host but not a release page.
      'https://band.bandcamp.com/merch/shirt?ref=/album/x',
      // http renders nothing: the resolver refuses to fetch it, so this only
      // ever reaches the fallback, and the fallback refuses it too.
      'http://band.bandcamp.com/album/test',
    ]

    it.each(hostileAlbumUrls)('renders no link for album URL %s', async (url) => {
      noEmbed()
      const { container } = render(
        <MusicEmbed bandcampAlbumUrl={url} artistName="Test Artist" />
      )

      await waitFor(() => {
        expect(screen.queryByText('Listen to Test Artist on Bandcamp')).not.toBeInTheDocument()
      })
      expect(container.querySelector('a')).toBeNull()
    })

    it('renders no link for a hostile profile URL', async () => {
      const { container } = render(
        <MusicEmbed bandcampProfileUrl="https://evil.test/band" artistName="Test Artist" />
      )

      await waitFor(() => {
        expect(container.querySelector('a')).toBeNull()
      })
    })

    // A bad album URL must not take a good profile link down with it: the
    // reader still gets somewhere real.
    it('falls through to a valid profile link when the album URL is rejected', async () => {
      noEmbed()
      render(
        <MusicEmbed
          bandcampAlbumUrl="https://evil.test/album/checkout"
          bandcampProfileUrl="https://band.bandcamp.com"
          artistName="Test Artist"
        />
      )

      await waitFor(() => {
        expect(screen.getByText('Listen to Test Artist on Bandcamp')).toHaveAttribute(
          'href',
          'https://band.bandcamp.com'
        )
      })
    })

    // The gate must not close the ordinary path.
    it('still links a real release page when the embed cannot be resolved', async () => {
      noEmbed()
      render(
        <MusicEmbed
          bandcampAlbumUrl="https://band.bandcamp.com/track/leyenda"
          artistName="Test Artist"
        />
      )

      await waitFor(() => {
        expect(screen.getByText('Listen to Test Artist on Bandcamp')).toHaveAttribute(
          'href',
          'https://band.bandcamp.com/track/leyenda'
        )
      })
    })

    // The tripwire for the claim that hasRenderableMusic is a NECESSARY
    // condition for this component rendering anything. It is a restatement of
    // MusicEmbed's entry conditions, not a call into it, so without this the
    // "cannot drift" claim is carried by prose alone: every caller test mocks
    // the component away, and musicAvailability.test.ts exercises the predicate
    // in isolation.
    //
    // ALL THREE arms, not just the album URL: the predicate answers a question
    // about the whole input, so a source added to deriveEmbedState without
    // adding it there would silently hide sections that do render.
    const unrenderable: [string, Record<string, string>][] = [
      ['album: foreign host', { bandcampAlbumUrl: 'https://evil.test/album/checkout' }],
      ['album: lookalike host', { bandcampAlbumUrl: 'https://bandcamp.com.attacker.test/album/x' }],
      ['album: http', { bandcampAlbumUrl: 'http://band.bandcamp.com/album/test' }],
      [
        'album: open redirect',
        { bandcampAlbumUrl: 'https://evil.test/?next=https://band.bandcamp.com/album/y' },
      ],
      ['album: blank', { bandcampAlbumUrl: '   ' }],
      ['profile: foreign host', { bandcampProfileUrl: 'https://evil.test/band' }],
      ['profile: http', { bandcampProfileUrl: 'http://band.bandcamp.com' }],
      ['spotify: unparseable id', { spotifyUrl: 'https://open.spotify.com/playlist/abc' }],
      [
        'spotify: foreign host',
        { spotifyUrl: 'https://evil.test/artist/4Z8W4fKeB5YxbusRsdQVPb' },
      ],
    ]

    it.each(unrenderable)(
      'renders nothing whenever hasRenderableMusic is false: %s',
      async (_name, props) => {
        expect(hasRenderableMusic(props)).toBe(false)

        const fetchSpy = vi.spyOn(global, 'fetch')
        const { container } = render(<MusicEmbed {...props} artistName="Test Artist" />)

        await waitFor(() => {
          expect(container.querySelector('section')).not.toBeInTheDocument()
        })
        // And it never asked the resolver: a URL the route would 400 must not
        // cost a round trip or hold the loading placeholder open on the way to
        // nothing.
        expect(fetchSpy).not.toHaveBeenCalled()
      }
    )

    // The iframe branch is unaffected: its src is built from a resolved numeric
    // id, never from the stored string, so a rejected URL that DOES resolve
    // still plays. Only the href is gated.
    it('leaves the resolved iframe alone', async () => {
      vi.spyOn(global, 'fetch').mockResolvedValue({
        ok: true,
        json: async () => ({ kind: 'album', id: '123456' }),
      } as Response)

      render(
        <MusicEmbed
          bandcampAlbumUrl="https://band.bandcamp.com/album/test"
          artistName="Test Artist"
        />
      )

      await waitFor(() => {
        expect(screen.getByTitle('Test Artist on Bandcamp')).toHaveAttribute(
          'src',
          expect.stringContaining('https://bandcamp.com/EmbeddedPlayer/album=123456')
        )
      })
    })
  })
})

describe('MusicEmbed slim size', () => {
  const SPOTIFY_URL = 'https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb'
  const ALBUM_URL = 'https://band.bandcamp.com/album/test'

  beforeEach(() => {
    vi.restoreAllMocks()
    theme.resolved = undefined
  })

  function resolvesTo(id: string) {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ kind: 'album', id }),
    } as Response)
  }

  it('renders Bandcamp s small player at its own height', async () => {
    resolvesTo('12345')
    render(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )

    const iframe = await screen.findByTitle('Test Artist on Bandcamp')
    expect(BANDCAMP_SLIM_EMBED_HEIGHT_PX).toBe(42)
    expect(iframe).toHaveStyle({ height: '42px' })
    const src = iframe.getAttribute('src') ?? ''
    expect(src).toContain('album=12345')
    expect(src).toContain('size=small')
    expect(src).toContain('transparent=true')
    expect(src).not.toContain('artwork=')
  })

  it('colours the Bandcamp player for the dark page', async () => {
    theme.resolved = 'dark'
    resolvesTo('12345')
    render(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )

    const src =
      (await screen.findByTitle('Test Artist on Bandcamp')).getAttribute('src') ?? ''
    expect(src).toContain('bgcol=0d0805')
    expect(src).toContain('linkcol=e89960')
  })

  it('colours the Bandcamp player for the light page', async () => {
    theme.resolved = 'light'
    resolvesTo('12345')
    render(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )

    const src =
      (await screen.findByTitle('Test Artist on Bandcamp')).getAttribute('src') ?? ''
    expect(src).toContain('bgcol=f4f1ea')
    expect(src).toContain('linkcol=d2541b')
  })

  // The colours are baked into the src, so a player that kept its element
  // across a theme change would keep painting the old ones.
  it('remounts the Bandcamp player with the other colours on a theme change', async () => {
    theme.resolved = 'dark'
    resolvesTo('12345')
    const { rerender } = render(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )
    const dark = await screen.findByTitle('Test Artist on Bandcamp')

    theme.resolved = 'light'
    rerender(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )

    const light = screen.getByTitle('Test Artist on Bandcamp')
    expect(light).not.toBe(dark)
    expect(dark).not.toBeInTheDocument()
    expect(light.getAttribute('src')).toContain('bgcol=f4f1ea')
  })

  it('remounts the Spotify player on a theme change', () => {
    theme.resolved = 'dark'
    const { rerender } = render(
      <MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />
    )
    const dark = screen.getByTitle('Test Artist on Spotify')

    theme.resolved = 'light'
    rerender(<MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />)

    expect(screen.getByTitle('Test Artist on Spotify')).not.toBe(dark)
  })

  it('holds the loading placeholder at the small player s height', () => {
    vi.spyOn(global, 'fetch').mockImplementation(() => new Promise(() => {}))
    const { container } = render(
      <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Test Artist" size="slim" />
    )

    const placeholder = container.querySelector('.animate-spin')?.parentElement
    expect(placeholder).toHaveStyle({ minHeight: '42px' })
    expect(placeholder).not.toHaveClass('rounded-md')
  })

  it('renders Spotify s card at 80px with no host-side rounding', () => {
    render(<MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />)

    const iframe = screen.getByTitle('Test Artist on Spotify')
    expect(iframe).toHaveStyle({ height: '80px' })
    expect(iframe.style.borderRadius).toBe('')
    expect(iframe.parentElement).not.toHaveClass('music-embed-container')
  })

  // A color-scheme mismatch between the iframe and its document paints an
  // opaque canvas behind the vendor's rounded card on the dark page.
  it('declares the vendors color-scheme on both slim iframes', async () => {
    resolvesTo('12345')
    render(
      <>
        <MusicEmbed bandcampAlbumUrl={ALBUM_URL} artistName="Bandcamp Act" size="slim" />
        <MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Spotify Act" size="slim" />
      </>
    )

    const bandcamp = await screen.findByTitle('Bandcamp Act on Bandcamp')
    expect(bandcamp.getAttribute('style')).toContain('color-scheme: normal')
    expect(
      screen.getByTitle('Spotify Act on Spotify').getAttribute('style')
    ).toContain('color-scheme: normal')
  })

  it('gives Spotify s dark card on the dark page and its own card on the light page', () => {
    theme.resolved = 'dark'
    const { unmount } = render(
      <MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />
    )
    expect(screen.getByTitle('Test Artist on Spotify').getAttribute('src')).toContain(
      'theme=0'
    )
    unmount()

    theme.resolved = 'light'
    render(<MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />)
    expect(
      screen.getByTitle('Test Artist on Spotify').getAttribute('src')
    ).not.toContain('theme=')
  })

  it('renders no heading and no outer margin', () => {
    const { container } = render(
      <MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />
    )

    expect(screen.queryByText('Music')).not.toBeInTheDocument()
    expect(container.querySelector('section')?.className ?? '').toBe('')
  })

  // The server cannot know the reader's theme, and next-themes reads it in the
  // browser's FIRST render: a src derived from it before hydration would make
  // the hydration render disagree with the server HTML.
  it('renders the dark src in server HTML whatever the stored theme', () => {
    theme.resolved = 'light'
    const html = renderToString(
      <QueryClientProvider client={createTestQueryClient()}>
        <MusicEmbed spotifyUrl={SPOTIFY_URL} artistName="Test Artist" size="slim" />
      </QueryClientProvider>
    )

    expect(html).toContain('theme=0')
  })
})

// The show page's listen cards and every other caller use the default size,
// whose players are unchanged in both themes.
describe('MusicEmbed default size', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    theme.resolved = 'light'
  })

  it('keeps the 120px Bandcamp player with its fixed dark colours', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ kind: 'album', id: '12345' }),
    } as Response)
    render(
      <MusicEmbed
        bandcampAlbumUrl="https://band.bandcamp.com/album/test"
        artistName="Test Artist"
        compact
      />
    )

    const iframe = await screen.findByTitle('Test Artist on Bandcamp')
    expect(iframe).toHaveStyle({ height: '120px' })
    const src = iframe.getAttribute('src') ?? ''
    expect(src).toContain('size=large')
    expect(src).toContain('bgcol=1a1a1a')
    expect(src).not.toContain('transparent')
    expect(iframe.parentElement).toHaveClass('music-embed-container')
  })

  it('keeps the headed 352px rounded dark Spotify card when not compact', () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
      />
    )

    expect(screen.getByText('Music')).toBeInTheDocument()
    const iframe = screen.getByTitle('Test Artist on Spotify')
    expect(iframe).toHaveStyle({ height: '352px', borderRadius: '12px' })
    expect(iframe.getAttribute('src')).toContain('theme=0')
    expect(iframe.parentElement).toHaveClass('music-embed-container')
  })

  it('keeps the 152px rounded dark Spotify card when compact', () => {
    render(
      <MusicEmbed
        spotifyUrl="https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
        artistName="Test Artist"
        compact
      />
    )

    const iframe = screen.getByTitle('Test Artist on Spotify')
    expect(iframe).toHaveStyle({ height: '152px', borderRadius: '12px' })
    expect(iframe.getAttribute('src')).toContain('theme=0')
  })
})
