import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
// MusicEmbed resolves its Bandcamp embed via TanStack Query (PSY-1102), so it
// must render inside a QueryClientProvider. `renderWithProviders` (re-exported
// as `render`) wraps each render in a fresh client with retries disabled, which
// keeps the `mockRejectedValueOnce` error-path tests deterministic.
import { render } from '../../test/utils'
import { MusicEmbed } from './MusicEmbed'


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
        spotifyUrl="https://open.spotify.com/artist/abc123"
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
})
