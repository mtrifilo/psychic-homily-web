import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Breadcrumb } from './Breadcrumb'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('Breadcrumb', () => {
  it('renders category link and entity name', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Macie Stewart"
      />
    )

    const nav = screen.getByRole('navigation', { name: /Breadcrumb/ })
    expect(nav).toBeInTheDocument()

    const link = screen.getByRole('link', { name: 'Artists' })
    expect(link).toHaveAttribute('href', '/artists')

    expect(screen.getByText('Macie Stewart')).toBeInTheDocument()
  })

  it('current page is not a link', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Macie Stewart"
      />
    )

    const currentPageElement = screen.getByText('Macie Stewart')
    expect(currentPageElement.tagName).toBe('SPAN')
    expect(currentPageElement.closest('a')).toBeNull()
  })

  it('renders separator between category and entity', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/shows', label: 'Shows' }}
        currentPage="Jeff Tweedy at Van Buren"
      />
    )

    // Check for separator character (rsaquo)
    const separators = screen.getAllByText((_, element) =>
      element?.getAttribute('aria-hidden') === 'true' && element?.textContent === '\u203A'
    )
    expect(separators).toHaveLength(1)
  })

  it('renders as an ordered list for accessibility', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Test"
      />
    )

    const list = screen.getByRole('list')
    expect(list).toBeInTheDocument()
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2) // category + current page
  })

  it('renders with different entity types', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/releases', label: 'Releases' }}
        currentPage="Satori"
      />
    )

    expect(screen.getByRole('link', { name: 'Releases' })).toHaveAttribute('href', '/releases')
    expect(screen.getByText('Satori')).toBeInTheDocument()
  })

  it('only has one link (the category)', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/venues', label: 'Venues' }}
        currentPage="The Van Buren"
      />
    )

    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(1)
    expect(links[0]).toHaveTextContent('Venues')
  })

  // PSY-486: deep breadcrumbs for nested entity hierarchies (genre tags).
  it('renders intermediate crumbs as links between fallback and current page', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        intermediate={[{ href: '/tags/post-punk', label: 'post-punk' }]}
        currentPage="shoegaze"
      />
    )

    // Three list items: Tags (link) > post-punk (link) > shoegaze (text)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)

    const tagsLink = screen.getByRole('link', { name: 'Tags' })
    expect(tagsLink).toHaveAttribute('href', '/tags')
    const parentLink = screen.getByRole('link', { name: 'post-punk' })
    expect(parentLink).toHaveAttribute('href', '/tags/post-punk')

    // Current page is still plain text.
    const current = screen.getByText('shoegaze')
    expect(current.tagName).toBe('SPAN')
    expect(current.closest('a')).toBeNull()

    // One separator per gap between crumbs.
    const separators = screen.getAllByText((_, element) =>
      element?.getAttribute('aria-hidden') === 'true' && element?.textContent === '\u203A'
    )
    expect(separators).toHaveLength(2)
  })

  it('supports multiple intermediate crumbs in order', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        intermediate={[
          { href: '/tags/music', label: 'music' },
          { href: '/tags/post-punk', label: 'post-punk' },
        ]}
        currentPage="shoegaze"
      />
    )

    const links = screen.getAllByRole('link')
    expect(links.map(l => l.textContent)).toEqual([
      'Tags',
      'music',
      'post-punk',
    ])
  })

  it('falls back to two-level rendering when intermediate is empty', () => {
    render(
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        intermediate={[]}
        currentPage="rock"
      />
    )

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2)
  })
})
