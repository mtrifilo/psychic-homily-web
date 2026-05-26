import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FeaturedCollectionCard } from './FeaturedCollectionCard'
import type { ExploreFeaturedCollection } from '../types'

const baseCollection: ExploreFeaturedCollection = {
  id: 7,
  slug: 'arizona-noise',
  title: 'Arizona Noise Roundup',
  curator_note_html: '<p>Sound from the desert.</p>',
}

describe('FeaturedCollectionCard', () => {
  it('renders the title and a view-collection link', () => {
    render(<FeaturedCollectionCard collection={baseCollection} />)
    expect(screen.getByText('Arizona Noise Roundup')).toBeInTheDocument()
    const cta = screen.getByRole('link', { name: /view collection/i })
    expect(cta).toHaveAttribute('href', '/collections/arizona-noise')
  })

  it('renders the curator note as sanitized HTML', () => {
    const { container } = render(
      <FeaturedCollectionCard collection={baseCollection} />,
    )
    expect(container.querySelector('p')?.textContent).toBe(
      'Sound from the desert.',
    )
  })

  it('renders the cover image when cover_image_url is set', () => {
    render(
      <FeaturedCollectionCard
        collection={{ ...baseCollection, cover_image_url: '/cover.jpg' }}
      />,
    )
    expect(
      screen.getByRole('img', { name: 'Arizona Noise Roundup' }),
    ).toBeInTheDocument()
  })

  it('omits the cover image when cover_image_url is absent', () => {
    render(<FeaturedCollectionCard collection={baseCollection} />)
    expect(screen.queryByRole('img')).toBeNull()
  })
})
