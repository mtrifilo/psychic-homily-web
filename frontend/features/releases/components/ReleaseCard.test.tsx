import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ReleaseCard } from './ReleaseCard'
import type { ReleaseListItem } from '../types'

// next/link → plain anchor so href assertions work in jsdom.
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

// EntityCardTitle renders the title as a link in comfortable/expanded modes.
vi.mock('@/components/shared', () => ({
  EntityCardTitle: ({ name, href }: { name: string; href: string }) => (
    <a href={href} data-testid="entity-card-title">
      {name}
    </a>
  ),
}))

function makeRelease(overrides: Partial<ReleaseListItem> = {}): ReleaseListItem {
  return {
    id: 1,
    title: 'In Rainbows',
    slug: 'in-rainbows',
    release_type: 'lp',
    release_year: 2007,
    cover_art_url: 'https://example.com/in-rainbows.jpg',
    artist_count: 1,
    artists: [{ id: 1, name: 'Radiohead', slug: 'radiohead' }],
    label_name: 'XL Recordings',
    label_slug: 'xl-recordings',
    ...overrides,
  }
}

describe('ReleaseCard', () => {
  it('renders as an article element', () => {
    render(<ReleaseCard release={makeRelease()} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('renders the cover art with descriptive alt text', () => {
    render(<ReleaseCard release={makeRelease()} />)
    const img = screen.getByAltText('In Rainbows cover art')
    expect(img).toHaveAttribute('src', 'https://example.com/in-rainbows.jpg')
  })

  it('renders the release type label, not the raw type', () => {
    render(<ReleaseCard release={makeRelease({ release_type: 'ep' })} />)
    expect(screen.getByText('EP')).toBeInTheDocument()
  })

  it('renders the release year', () => {
    render(<ReleaseCard release={makeRelease()} />)
    expect(screen.getByText('2007')).toBeInTheDocument()
  })

  it('links the title to the release detail page', () => {
    render(<ReleaseCard release={makeRelease()} />)
    const title = screen.getByTestId('entity-card-title')
    expect(title).toHaveAttribute('href', '/releases/in-rainbows')
  })

  it('links a single artist to its artist page', () => {
    render(<ReleaseCard release={makeRelease()} />)
    const artist = screen.getByText('Radiohead')
    expect(artist.closest('a')).toHaveAttribute('href', '/artists/radiohead')
  })

  it('links the label to its label page when a slug is present', () => {
    render(<ReleaseCard release={makeRelease()} />)
    const label = screen.getByText('XL Recordings')
    expect(label.closest('a')).toHaveAttribute('href', '/labels/xl-recordings')
  })

  it('renders the label as plain text when there is no slug', () => {
    render(
      <ReleaseCard
        release={makeRelease({ label_name: 'Self-Released', label_slug: null })}
      />
    )
    const label = screen.getByText('Self-Released')
    expect(label.closest('a')).toBeNull()
  })

  it('collapses 4+ artists to "Various Artists"', () => {
    const release = makeRelease({
      artists: [
        { id: 1, name: 'A', slug: 'a' },
        { id: 2, name: 'B', slug: 'b' },
        { id: 3, name: 'C', slug: 'c' },
        { id: 4, name: 'D', slug: 'd' },
      ],
      artist_count: 4,
    })
    render(<ReleaseCard release={release} />)
    expect(screen.getByText('Various Artists')).toBeInTheDocument()
    expect(screen.queryByText('A')).not.toBeInTheDocument()
  })

  it('joins 2-3 artist names with commas', () => {
    const release = makeRelease({
      artists: [
        { id: 1, name: 'First', slug: 'first' },
        { id: 2, name: 'Second', slug: 'second' },
      ],
      artist_count: 2,
    })
    render(<ReleaseCard release={release} />)
    expect(screen.getByText('First')).toBeInTheDocument()
    expect(screen.getByText('Second')).toBeInTheDocument()
  })

  describe('placeholder when no cover art', () => {
    it('renders no img element', () => {
      const { container } = render(
        <ReleaseCard release={makeRelease({ cover_art_url: null })} />
      )
      expect(container.querySelector('img')).toBeNull()
    })
  })

  describe('missing year', () => {
    it('omits the year when null', () => {
      render(<ReleaseCard release={makeRelease({ release_year: null })} />)
      expect(screen.queryByText('2007')).not.toBeInTheDocument()
    })
  })

  describe('compact density', () => {
    it('renders a flat row without a card border', () => {
      render(<ReleaseCard release={makeRelease()} density="compact" />)
      const article = screen.getByRole('article')
      expect(article.className).toContain('hover:bg-muted/50')
      expect(article.className).not.toContain('border')
    })

    it('renders the title inline with the artist name', () => {
      render(<ReleaseCard release={makeRelease()} density="compact" />)
      // Compact mode renders "Artist — Title" in a single link.
      expect(
        screen.getByText('Radiohead — In Rainbows')
      ).toBeInTheDocument()
    })

    it('renders just the title when there are no artists', () => {
      render(
        <ReleaseCard
          release={makeRelease({ artists: [], artist_count: 0 })}
          density="compact"
        />
      )
      expect(screen.getByText('In Rainbows')).toBeInTheDocument()
    })
  })

  describe('expanded density', () => {
    it('renders a bordered card', () => {
      render(<ReleaseCard release={makeRelease()} density="expanded" />)
      const article = screen.getByRole('article')
      expect(article.className).toContain('border')
      expect(article.className).toContain('p-6')
    })

    it('still renders cover art and title link', () => {
      render(<ReleaseCard release={makeRelease()} density="expanded" />)
      expect(screen.getByAltText('In Rainbows cover art')).toBeInTheDocument()
      expect(screen.getByTestId('entity-card-title')).toHaveAttribute(
        'href',
        '/releases/in-rainbows'
      )
    })
  })

  describe('comfortable density (default)', () => {
    it('renders a bordered card with p-4 padding', () => {
      render(<ReleaseCard release={makeRelease()} />)
      const article = screen.getByRole('article')
      expect(article.className).toContain('border')
      expect(article.className).toContain('p-4')
    })
  })
})
