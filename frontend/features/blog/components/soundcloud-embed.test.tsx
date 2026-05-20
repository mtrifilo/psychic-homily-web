import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SoundCloud } from './soundcloud-embed'

const PLAYER_URL = 'https://w.soundcloud.com/player/?url=track'

describe('SoundCloud', () => {
  it('renders an iframe with the provided url as src', () => {
    const { container } = render(<SoundCloud url={PLAYER_URL} />)
    const iframe = container.querySelector('iframe')
    expect(iframe).toBeInTheDocument()
    expect(iframe).toHaveAttribute('src', PLAYER_URL)
  })

  it('falls back to a default iframe title when none is given', () => {
    render(<SoundCloud url={PLAYER_URL} />)
    expect(screen.getByTitle('SoundCloud Player')).toBeInTheDocument()
  })

  it('uses the provided title as the iframe title', () => {
    render(<SoundCloud url={PLAYER_URL} title="My Track" />)
    expect(screen.getByTitle('My Track')).toBeInTheDocument()
  })

  it('omits the attribution block when neither artist nor title is set', () => {
    const { container } = render(<SoundCloud url={PLAYER_URL} />)
    // Only the iframe child should exist inside the wrapper div.
    const wrapper = container.querySelector('div.mb-6')
    expect(wrapper?.childElementCount).toBe(1)
    expect(wrapper?.firstElementChild?.tagName).toBe('IFRAME')
  })

  it('renders a plain-text artist when artist is set without a url', () => {
    render(<SoundCloud url={PLAYER_URL} artist="DJ Test" />)
    expect(screen.getByText('DJ Test')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'DJ Test' })).toBeNull()
  })

  it('renders an artist link when artist_url is provided', () => {
    render(
      <SoundCloud
        url={PLAYER_URL}
        artist="DJ Test"
        artist_url="https://soundcloud.com/djtest"
      />
    )
    const link = screen.getByRole('link', { name: 'DJ Test' })
    expect(link).toHaveAttribute('href', 'https://soundcloud.com/djtest')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('renders a plain-text title when title is set without a track url', () => {
    render(<SoundCloud url={PLAYER_URL} title="My Track" />)
    expect(screen.getByText('My Track')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'My Track' })).toBeNull()
  })

  it('renders a track link when track_url is provided', () => {
    render(
      <SoundCloud
        url={PLAYER_URL}
        title="My Track"
        track_url="https://soundcloud.com/djtest/my-track"
      />
    )
    const link = screen.getByRole('link', { name: 'My Track' })
    expect(link).toHaveAttribute(
      'href',
      'https://soundcloud.com/djtest/my-track'
    )
  })

  it('separates artist and title with a middot when both are present', () => {
    const { container } = render(
      <SoundCloud url={PLAYER_URL} artist="DJ Test" title="My Track" />
    )
    expect(container.textContent).toContain('·')
  })
})
