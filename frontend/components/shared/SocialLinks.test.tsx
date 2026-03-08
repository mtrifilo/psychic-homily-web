import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SocialLinks } from './SocialLinks'

describe('SocialLinks', () => {
  it('returns null when social is null', () => {
    const { container } = render(<SocialLinks social={null} />)
    expect(container.firstChild).toBeNull()
  })

  it('returns null when social is undefined', () => {
    const { container } = render(<SocialLinks />)
    expect(container.firstChild).toBeNull()
  })

  it('returns null when all social fields are null', () => {
    const { container } = render(
      <SocialLinks
        social={{
          website: null,
          instagram: null,
          facebook: null,
          twitter: null,
          youtube: null,
          spotify: null,
          bandcamp: null,
          soundcloud: null,
        }}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders a link for a full website URL', () => {
    render(<SocialLinks social={{ website: 'https://example.com' }} />)
    const link = screen.getByTitle('Website')
    expect(link).toHaveAttribute('href', 'https://example.com')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('renders Instagram link from handle', () => {
    render(<SocialLinks social={{ instagram: 'bandname' }} />)
    const link = screen.getByTitle('Instagram')
    expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
  })

  it('renders Instagram link from handle with @ prefix', () => {
    render(<SocialLinks social={{ instagram: '@bandname' }} />)
    const link = screen.getByTitle('Instagram')
    expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
  })

  it('renders Instagram link from full URL', () => {
    render(<SocialLinks social={{ instagram: 'https://instagram.com/bandname' }} />)
    const link = screen.getByTitle('Instagram')
    expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
  })

  it('renders Facebook link from handle', () => {
    render(<SocialLinks social={{ facebook: 'bandpage' }} />)
    const link = screen.getByTitle('Facebook')
    expect(link).toHaveAttribute('href', 'https://facebook.com/bandpage')
  })

  it('renders Twitter link from handle', () => {
    render(<SocialLinks social={{ twitter: 'bandname' }} />)
    const link = screen.getByTitle('Twitter/X')
    expect(link).toHaveAttribute('href', 'https://twitter.com/bandname')
  })

  it('renders YouTube link from handle', () => {
    render(<SocialLinks social={{ youtube: 'channel123' }} />)
    const link = screen.getByTitle('YouTube')
    expect(link).toHaveAttribute('href', 'https://youtube.com/channel123')
  })

  it('renders Spotify link from handle', () => {
    render(<SocialLinks social={{ spotify: 'artist/123' }} />)
    const link = screen.getByTitle('Spotify')
    expect(link).toHaveAttribute('href', 'https://open.spotify.com/artist/123')
  })

  it('renders Bandcamp link from full URL', () => {
    render(<SocialLinks social={{ bandcamp: 'https://band.bandcamp.com' }} />)
    const link = screen.getByTitle('Bandcamp')
    expect(link).toHaveAttribute('href', 'https://band.bandcamp.com')
  })

  it('renders SoundCloud link from handle', () => {
    render(<SocialLinks social={{ soundcloud: 'bandname' }} />)
    const link = screen.getByTitle('SoundCloud')
    expect(link).toHaveAttribute('href', 'https://soundcloud.com/bandname')
  })

  it('renders multiple social links when multiple are present', () => {
    render(
      <SocialLinks
        social={{
          instagram: 'band',
          spotify: 'https://open.spotify.com/artist/abc',
          website: 'https://band.com',
        }}
      />
    )
    expect(screen.getByTitle('Instagram')).toBeInTheDocument()
    expect(screen.getByTitle('Spotify')).toBeInTheDocument()
    expect(screen.getByTitle('Website')).toBeInTheDocument()
  })

  it('only renders links for non-null social fields', () => {
    render(
      <SocialLinks
        social={{
          instagram: 'band',
          facebook: null,
          twitter: null,
        }}
      />
    )
    expect(screen.getByTitle('Instagram')).toBeInTheDocument()
    expect(screen.queryByTitle('Facebook')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Twitter/X')).not.toBeInTheDocument()
  })

  it('renders screen reader labels for each link', () => {
    render(
      <SocialLinks
        social={{
          instagram: 'band',
          youtube: 'channel',
        }}
      />
    )
    expect(screen.getByText('Instagram')).toBeInTheDocument()
    expect(screen.getByText('YouTube')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(
      <SocialLinks social={{ website: 'https://example.com' }} className="mt-4" />
    )
    expect(container.firstChild).toHaveClass('mt-4')
  })

  it('normalizes partial URL with domain but no protocol', () => {
    render(<SocialLinks social={{ website: 'example.com/page' }} />)
    const link = screen.getByTitle('Website')
    expect(link).toHaveAttribute('href', 'https://example.com/page')
  })
})
