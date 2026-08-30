import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ImageAttribution } from './ImageAttribution'

describe('ImageAttribution', () => {
  it('renders nothing for an unknown or null source', () => {
    const { container: a } = render(<ImageAttribution source={null} />)
    expect(a).toBeEmptyDOMElement()
    const { container: b } = render(<ImageAttribution source="mystery-db" sourceUrl="https://x.test" />)
    expect(b).toBeEmptyDOMElement()
  })

  it('renders "Cover via Spotify" with a linkback when source=spotify', () => {
    render(
      <ImageAttribution source="spotify" sourceUrl="https://open.spotify.com/album/abc" kind="cover" />
    )
    expect(screen.getByText(/cover via/i)).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /spotify/i })
    expect(link).toHaveAttribute('href', 'https://open.spotify.com/album/abc')
  })

  // A provider linkback always leaves the app, so it must go through the
  // `external` branch that owns BOTH the target/rel hygiene and the
  // announcement. Announcing a new tab over a same-tab link is the drift this
  // pins shut. The ↗ stays visual-only, hence the ariaLabel at the call site.
  it('opens the provider linkback in a new tab, announced once, with rel hygiene', () => {
    render(
      <ImageAttribution source="spotify" sourceUrl="https://open.spotify.com/album/abc" kind="cover" />
    )
    // Anchored on this call site's own half of the name. The suffix wording
    // belongs to BracketLink and is pinned in its suite, not re-owned here.
    const link = screen.getByRole('link', { name: /^Spotify\b/ })
    expect(link.getAttribute('aria-label')).not.toMatch(/↗/)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('uses the required "Data provided by Discogs" phrasing', () => {
    render(<ImageAttribution source="discogs" sourceUrl="https://discogs.com/release/1" />)
    expect(screen.getByText(/data provided by/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /discogs/i })).toBeInTheDocument()
  })

  it('varies the noun by kind (photo)', () => {
    render(<ImageAttribution source="commons" sourceUrl="https://commons.wikimedia.org/x" kind="photo" />)
    expect(screen.getByText(/photo via/i)).toBeInTheDocument()
  })

  it('renders contributor + public-domain credits without an external link', () => {
    const { rerender } = render(<ImageAttribution source="user" />)
    expect(screen.getByText(/added by a contributor/i)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
    rerender(<ImageAttribution source="public_domain" />)
    expect(screen.getByText(/public domain/i)).toBeInTheDocument()
  })

  it('shows the provider name as plain text when no linkback URL is given', () => {
    render(<ImageAttribution source="spotify" kind="cover" />)
    expect(screen.getByText(/cover via/i)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('renders the CC author + license credit for a Commons photo (PSY-1232)', () => {
    const { container } = render(
      <ImageAttribution
        source="commons"
        sourceUrl="https://commons.wikimedia.org/wiki/File:X.jpg"
        author="Jane Doe"
        license="CC BY-SA 4.0"
        kind="photo"
      />
    )
    expect(container.textContent).toMatch(/Photo: Jane Doe · CC BY-SA 4\.0 · via/)
    expect(screen.getByRole('link', { name: /wikimedia commons/i })).toHaveAttribute(
      'href',
      'https://commons.wikimedia.org/wiki/File:X.jpg'
    )
  })

  it('omits the author prefix for a Commons photo with no author (PSY-1232)', () => {
    const { container } = render(
      <ImageAttribution
        source="commons"
        sourceUrl="https://commons.wikimedia.org/wiki/File:Y.jpg"
        license="Public domain"
        kind="photo"
      />
    )
    expect(container.textContent).toMatch(/^Public domain · via/)
    expect(container.textContent).not.toMatch(/Photo:/)
  })
})
