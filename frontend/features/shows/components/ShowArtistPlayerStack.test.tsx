import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '../../../test/utils'
import { ShowArtistPlayerStack } from './ShowArtistMusic'
import type { ArtistResponse } from '../types'

// The REAL `MusicEmbed` renders here, so what is asserted is the players the
// stack actually ships: their heights, their order, and that nothing else
// surrounds them.

function makeArtist(overrides: Partial<ArtistResponse>): ArtistResponse {
  return { id: 1, name: 'Act', slug: 'act', ...overrides } as ArtistResponse
}

const BANDCAMP_ACT = makeArtist({
  id: 1,
  name: 'Sammy Rae',
  slug: 'sammy-rae',
  city: 'Brooklyn',
  state: 'NY',
  bandcamp_embed_url: 'https://sammyrae.bandcamp.com/album/sun',
  socials: { bandcamp: 'https://sammyrae.bandcamp.com', instagram: 'sammyrae' },
} as never)

const SILENT_ACT = makeArtist({ id: 2, name: 'Anamanaguchi', slug: 'anamanaguchi' })

const SPOTIFY_ACT = makeArtist({
  id: 3,
  name: 'Melt',
  slug: 'melt',
  city: 'New York',
  state: 'NY',
  socials: { spotify: 'https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb' },
} as never)

afterEach(() => {
  vi.restoreAllMocks()
})

function renderStack(artists: ArtistResponse[]) {
  vi.spyOn(global, 'fetch').mockResolvedValue({
    ok: true,
    json: async () => ({ kind: 'album', id: '12345' }),
  } as Response)
  return render(<ShowArtistPlayerStack artists={artists} />)
}

describe('ShowArtistPlayerStack', () => {
  it('renders one slim player per act with music, in bill order', async () => {
    const { container } = renderStack([BANDCAMP_ACT, SILENT_ACT, SPOTIFY_ACT])

    await screen.findByTitle('Sammy Rae on Bandcamp')
    const iframes = Array.from(container.querySelectorAll('iframe'))
    expect(iframes.map(frame => frame.title)).toEqual([
      'Sammy Rae on Bandcamp',
      'Melt on Spotify',
    ])
    expect(iframes[0]).toHaveStyle({ height: '42px' })
    expect(iframes[0].getAttribute('src')).toContain('size=small')
    expect(iframes[1]).toHaveStyle({ height: '80px' })
  })

  // The row above names every act, and each player names its own act.
  it('prints no act name, hometown or social link around the players', async () => {
    const { container } = renderStack([BANDCAMP_ACT, SPOTIFY_ACT])

    await screen.findByTitle('Sammy Rae on Bandcamp')
    expect(screen.queryByText('Sammy Rae')).toBeNull()
    expect(screen.queryByText('Melt')).toBeNull()
    expect(screen.queryByText(/based in/)).toBeNull()
    expect(screen.queryByText(/Brooklyn/)).toBeNull()
    expect(container.querySelector('a')).toBeNull()
  })

  it('stacks the players 6px apart, no wider than a Bandcamp player', async () => {
    renderStack([BANDCAMP_ACT, SPOTIFY_ACT])

    const stack = (await screen.findByTitle('Sammy Rae on Bandcamp')).closest(
      '.space-y-1\\.5'
    )
    expect(stack).not.toBeNull()
    expect(stack).toHaveStyle({ maxWidth: '700px' })
  })

  it('takes its outer spacing from the caller', () => {
    render(<ShowArtistPlayerStack artists={[SPOTIFY_ACT]} className="pt-3" />)

    expect(screen.getByTestId('artist-player-stack')).toHaveClass('pt-3')
  })

  // MusicEmbed's outbound link stands in for a player it cannot render, and
  // is the only text the stack prints.
  it('links a profile-only act to Bandcamp in its slot, in bill order', async () => {
    const profileOnly = makeArtist({
      id: 4,
      name: 'Profile Only',
      slug: 'profile-only',
      socials: { bandcamp: 'https://profileonly.bandcamp.com' },
    } as never)
    renderStack([profileOnly, SPOTIFY_ACT])

    const stack = await screen.findByTestId('artist-player-stack')
    const link = screen.getByRole('link', { name: /Listen to Profile Only on Bandcamp/ })
    expect(link).toHaveAttribute('href', 'https://profileonly.bandcamp.com')
    expect(stack.querySelectorAll('a')).toHaveLength(1)
    expect(stack.querySelector('iframe')?.title).toBe('Melt on Spotify')
    expect(
      link.compareDocumentPosition(stack.querySelector('iframe') as Node) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('renders nothing when no act has music', () => {
    const { container } = renderStack([SILENT_ACT])

    expect(container.firstChild).toBeNull()
  })
})
