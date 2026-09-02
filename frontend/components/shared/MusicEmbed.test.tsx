import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
// MusicEmbed resolves its Bandcamp embed via TanStack Query (PSY-1102), so it
// must render inside a QueryClientProvider. `renderWithProviders` (re-exported
// as `render`) wraps each render in a fresh client with retries disabled, which
// keeps the `mockRejectedValueOnce` error-path tests deterministic.
import { render } from '../../test/utils'
import { MusicEmbed } from './MusicEmbed'
import { hasRenderableMusic } from '@/lib/musicAvailability'


describe('MusicEmbed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
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
  // A value that is not provably a Bandcamp page must render NO link — an
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
    it.each([
      'https://evil.test/album/checkout',
      'https://bandcamp.com.attacker.test/album/x',
      'http://band.bandcamp.com/album/test',
      'https://evil.test/?next=https://band.bandcamp.com/album/y',
      '   ',
    ])('renders nothing whenever hasRenderableMusic is false: %s', async (url) => {
      expect(hasRenderableMusic({ bandcampAlbumUrl: url })).toBe(false)

      const fetchSpy = vi.spyOn(global, 'fetch')
      const { container } = render(
        <MusicEmbed bandcampAlbumUrl={url} artistName="Test Artist" />
      )

      await waitFor(() => {
        expect(container.querySelector('section')).not.toBeInTheDocument()
      })
      // And it never asked the resolver: a URL the route would 400 must not cost
      // a round trip or hold the loading placeholder open on the way to nothing.
      expect(fetchSpy).not.toHaveBeenCalled()
    })

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
