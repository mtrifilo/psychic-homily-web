import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CollectionCoverImage } from './CollectionCoverImage'

const FALLBACK_TEXT = 'fallback content'

function Fallback() {
  return <span data-testid="fallback">{FALLBACK_TEXT}</span>
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
})
