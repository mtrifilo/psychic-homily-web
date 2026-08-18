import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CollectionCoverImage } from './CollectionCoverImage'
import { stubImageLoadState } from '@/test/imageLoadState'

function Fallback() {
  return <span data-testid="fallback">fallback content</span>
}

describe('CollectionCoverImage', () => {
  it('renders the image when a URL is provided', () => {
    render(
      <CollectionCoverImage
        url="https://example.com/cover.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )

    const img = screen.getByAltText('cover') as HTMLImageElement
    expect(img).toBeInTheDocument()
    expect(img.src).toBe('https://example.com/cover.jpg')
    expect(screen.queryByTestId('fallback')).not.toBeInTheDocument()
  })

  it('renders the fallback when the URL is null', () => {
    render(
      <CollectionCoverImage url={null} alt="cover" fallback={<Fallback />} />
    )

    expect(screen.getByTestId('fallback')).toBeInTheDocument()
    expect(screen.queryByAltText('cover')).not.toBeInTheDocument()
  })

  it('renders the fallback when the URL is an empty string', () => {
    render(
      <CollectionCoverImage url="" alt="cover" fallback={<Fallback />} />
    )

    expect(screen.getByTestId('fallback')).toBeInTheDocument()
  })

  it('renders the fallback when the URL is whitespace-only', () => {
    render(
      <CollectionCoverImage url="   " alt="cover" fallback={<Fallback />} />
    )

    expect(screen.getByTestId('fallback')).toBeInTheDocument()
  })

  it('falls back to the fallback when the image errors (404)', () => {
    render(
      <CollectionCoverImage
        url="https://example.com/missing.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )

    const img = screen.getByAltText('cover')
    expect(screen.queryByTestId('fallback')).not.toBeInTheDocument()
    fireEvent.error(img)
    expect(screen.getByTestId('fallback')).toBeInTheDocument()
    expect(screen.queryByAltText('cover')).not.toBeInTheDocument()
  })

  it('forwards className to the outer container in both image and fallback states', () => {
    const { rerender } = render(
      <CollectionCoverImage
        url="https://example.com/cover.jpg"
        alt="cover"
        className="h-24 w-24 rounded-lg"
        fallback={<Fallback />}
      />
    )
    const imageContainer = screen.getByAltText('cover').parentElement
    expect(imageContainer).toHaveClass('h-24', 'w-24', 'rounded-lg')

    rerender(
      <CollectionCoverImage
        url={null}
        alt="cover"
        className="h-24 w-24 rounded-lg"
        fallback={<Fallback />}
      />
    )
    // Same outer container, fallback now inside.
    const fallbackContainer = screen.getByTestId('fallback').parentElement?.parentElement
    expect(fallbackContainer).toHaveClass('h-24', 'w-24', 'rounded-lg')
  })

  it('clears the errored state when the URL changes to a new image', () => {
    const { rerender } = render(
      <CollectionCoverImage
        url="https://example.com/missing.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )
    fireEvent.error(screen.getByAltText('cover'))
    expect(screen.getByTestId('fallback')).toBeInTheDocument()

    rerender(
      <CollectionCoverImage
        url="https://example.com/working.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )
    // New URL, errored flag reset, image rendered again.
    const img = screen.getByAltText('cover') as HTMLImageElement
    expect(img.src).toBe('https://example.com/working.jpg')
    expect(screen.queryByTestId('fallback')).not.toBeInTheDocument()
  })

  // `/collections/[slug]` prefetches on the server and hydrates, so this
  // `<img>` is in the initial HTML and the browser fetches it while parsing. A
  // dead cover therefore 404s before React attaches `onError`, and that event
  // is lost. Without the mount-time read the cover slot stays blank instead of
  // showing the fallback the caller supplied.
  it('falls back for a cover that already failed before the handler attached', () => {
    const img = stubImageLoadState({ complete: true, naturalWidth: 0 })

    try {
      render(
        <CollectionCoverImage
          url="https://example.com/gone.jpg"
          alt="cover"
          fallback={<Fallback />}
        />
      )

      expect(screen.getByTestId('fallback')).toBeInTheDocument()
      expect(screen.queryByAltText('cover')).not.toBeInTheDocument()
    } finally {
      img.restore()
    }
  })

  // The other half of the predicate, and the only test that pins it. Loosening
  // the check to `complete` alone would blank every cover the browser HAS
  // decoded; nothing else here catches that, because jsdom reports
  // `complete: false` for an http src, so the tests above never reach that
  // branch. (A loosening to a bare `naturalWidth === 0` is already caught —
  // jsdom reports 0 for every image, so those same tests would fail.)
  it('keeps a cover that already finished loading before mount', () => {
    const img = stubImageLoadState({ complete: true, naturalWidth: 600 })

    try {
      render(
        <CollectionCoverImage
          url="https://example.com/cached.jpg"
          alt="cover"
          fallback={<Fallback />}
        />
      )

      expect(screen.getByAltText('cover')).toBeInTheDocument()
      expect(screen.queryByTestId('fallback')).not.toBeInTheDocument()
    } finally {
      img.restore()
    }
  })
})
