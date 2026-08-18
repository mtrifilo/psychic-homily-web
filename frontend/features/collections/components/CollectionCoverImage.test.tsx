import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CollectionCoverImage } from './CollectionCoverImage'
import { stubAllImagesLoadState } from '@/test/stubAllImagesLoadState'

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
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })

    render(
      <CollectionCoverImage
        url="https://example.com/gone.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )

    expect(screen.getByTestId('fallback')).toBeInTheDocument()
    expect(screen.queryByAltText('cover')).not.toBeInTheDocument()
  })

  // The `<img>` is deliberately not keyed on the URL, so swapping a WORKING
  // cover for another one reuses the element — that is the seamless swap the
  // missing key buys. This pins both halves of that decision: the element is
  // genuinely reused, and a failure on the replacement still reaches the
  // fallback, which on this path is `onError`'s job alone.
  it('reuses the element across a URL change and still catches a later failure', () => {
    const { rerender } = render(
      <CollectionCoverImage
        url="https://example.com/first.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )
    const first = screen.getByAltText('cover')

    rerender(
      <CollectionCoverImage
        url="https://example.com/second.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )
    const second = screen.getByAltText('cover') as HTMLImageElement

    expect(second).toBe(first) // same DOM node: no remount, no re-fetch flash
    expect(second.src).toBe('https://example.com/second.jpg')

    fireEvent.error(second)
    expect(screen.getByTestId('fallback')).toBeInTheDocument()
  })

  // The composition side of the other half of the predicate (the predicate
  // itself is pinned in the hook's own test): a cover the browser HAS decoded
  // must keep its image. Nothing else in this file reaches that branch,
  // because jsdom reports `complete: false` for an http src.
  it('keeps a cover that already finished loading before mount', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 600 })

    render(
      <CollectionCoverImage
        url="https://example.com/cached.jpg"
        alt="cover"
        fallback={<Fallback />}
      />
    )

    expect(screen.getByAltText('cover')).toBeInTheDocument()
    expect(screen.queryByTestId('fallback')).not.toBeInTheDocument()
  })
})
