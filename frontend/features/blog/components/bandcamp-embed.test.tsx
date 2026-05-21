import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { Bandcamp } from './bandcamp-embed'

/**
 * Bandcamp builds its embed/link URLs from props (it has no single `url`
 * prop). The iframe's child <a> is the fallback shown when the embed can't
 * load, so the tests assert both the iframe src and that fallback anchor.
 */
describe('Bandcamp', () => {
  it('renders an iframe titled "<title> by <artist>"', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="my-album" />
    )
    const iframe = container.querySelector('iframe')
    expect(iframe).toBeInTheDocument()
    expect(iframe).toHaveAttribute('title', 'my-album by myband')
  })

  it('embeds the album id in the src when album is provided', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="my-album" />
    )
    const src = container.querySelector('iframe')?.getAttribute('src') ?? ''
    expect(src).toContain('https://bandcamp.com/EmbeddedPlayer/')
    expect(src).toContain('album=123')
    expect(src).not.toContain('track=')
  })

  it('embeds the track id in the src when track is provided', () => {
    const { container } = render(
      <Bandcamp track="456" artist="myband" title="my-track" />
    )
    const src = container.querySelector('iframe')?.getAttribute('src') ?? ''
    expect(src).toContain('track=456')
    expect(src).not.toContain('album=')
  })

  it('applies default embed params to the src', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="my-album" />
    )
    const src = container.querySelector('iframe')?.getAttribute('src') ?? ''
    expect(src).toContain('size=large')
    expect(src).toContain('bgcol=ffffff')
    expect(src).toContain('linkcol=0687f5')
    expect(src).toContain('tracklist=false')
    expect(src).toContain('artwork=small')
    expect(src).toContain('transparent=true')
  })

  it('overrides defaults with custom embed params', () => {
    const { container } = render(
      <Bandcamp
        album="123"
        artist="myband"
        title="my-album"
        size="small"
        artwork="big"
        bgcol="000000"
        linkcol="ffcc00"
        tracklist="true"
      />
    )
    const src = container.querySelector('iframe')?.getAttribute('src') ?? ''
    expect(src).toContain('size=small')
    expect(src).toContain('artwork=big')
    expect(src).toContain('bgcol=000000')
    expect(src).toContain('linkcol=ffcc00')
    expect(src).toContain('tracklist=true')
  })

  it('renders an album fallback link when album is provided', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="my-album" />
    )
    const link = container.querySelector('iframe a')
    expect(link).toHaveAttribute(
      'href',
      'https://myband.bandcamp.com/album/my-album'
    )
    expect(link).toHaveTextContent('my-album by myband')
  })

  it('renders a track fallback link when only track is provided', () => {
    const { container } = render(
      <Bandcamp track="456" artist="myband" title="my-track" />
    )
    const link = container.querySelector('iframe a')
    expect(link).toHaveAttribute(
      'href',
      'https://myband.bandcamp.com/track/my-track'
    )
  })

  it('URL-encodes the title path segment so a non-slug title stays well-formed', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="live at the / venue & more" />
    )
    const link = container.querySelector('iframe a')
    expect(link).toHaveAttribute(
      'href',
      'https://myband.bandcamp.com/album/live%20at%20the%20%2F%20venue%20%26%20more'
    )
  })

  it('applies the height prop to the iframe style', () => {
    const { container } = render(
      <Bandcamp album="123" artist="myband" title="my-album" height="350" />
    )
    const iframe = container.querySelector('iframe')
    expect(iframe).toHaveStyle({ height: '350px' })
  })
})
