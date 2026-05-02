import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CollectionItemCard } from './CollectionItemCard'
import type { CollectionItem } from '../types'

// Mock next/link so href assertions work without the App Router runtime.
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

function makeItem(overrides: Partial<CollectionItem> = {}): CollectionItem {
  return {
    id: 1,
    entity_type: 'release',
    entity_id: 100,
    entity_name: 'Test Item',
    entity_slug: 'test-item',
    image_url: null,
    position: 0,
    added_by_user_id: 7,
    added_by_name: 'curator',
    notes: null,
    notes_html: undefined,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('CollectionItemCard', () => {
  describe('image vs typed-icon fallback', () => {
    it('renders the entity image when image_url is present', () => {
      render(
        <CollectionItemCard
          item={makeItem({
            entity_type: 'release',
            entity_name: 'Hard Drugs',
            entity_slug: 'hard-drugs',
            image_url: 'https://example.com/cover.jpg',
          })}
          density="comfortable"
        />
      )

      const img = screen.getByTestId('collection-item-card-image')
      expect(img).toBeInTheDocument()
      expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg')
      // Typed-icon fallback should not render when image is present.
      expect(
        screen.queryByTestId('collection-item-card-fallback')
      ).not.toBeInTheDocument()
    })

    it('renders typed icon fallback when image_url is null', () => {
      render(
        <CollectionItemCard
          item={makeItem({ image_url: null })}
          density="comfortable"
        />
      )
      expect(
        screen.getByTestId('collection-item-card-fallback')
      ).toBeInTheDocument()
      expect(
        screen.queryByTestId('collection-item-card-image')
      ).not.toBeInTheDocument()
    })

    it('renders typed icon fallback when image_url is undefined', () => {
      render(
        <CollectionItemCard
          item={makeItem({ image_url: undefined })}
          density="comfortable"
        />
      )
      expect(
        screen.getByTestId('collection-item-card-fallback')
      ).toBeInTheDocument()
    })

    // One assertion per entity_type so the icon mapping stays honest.
    // The test is intentionally weak (renders without throwing, fallback
    // surface present) because the actual icon SVGs are interchangeable
    // visual choices the design team owns.
    it.each([
      ['artist'],
      ['venue'],
      ['show'],
      ['release'],
      ['label'],
      ['festival'],
    ])(
      'renders typed icon fallback for entity_type=%s',
      (entityType) => {
        render(
          <CollectionItemCard
            item={makeItem({
              entity_type: entityType,
              image_url: null,
            })}
            density="comfortable"
          />
        )

        const card = screen.getByTestId('collection-item-card')
        expect(card).toHaveAttribute('data-entity-type', entityType)
        expect(
          screen.getByTestId('collection-item-card-fallback')
        ).toBeInTheDocument()
      }
    )
  })

  describe('caption / notes rendering', () => {
    it('renders notes_html via MarkdownContent when present', () => {
      render(
        <CollectionItemCard
          item={makeItem({
            id: 42,
            notes_html: '<p>Curator <strong>note</strong></p>',
          })}
          density="comfortable"
        />
      )

      const caption = screen.getByTestId('collection-item-card-notes-42')
      expect(caption).toBeInTheDocument()
      // Sanitized HTML round-trips through innerHTML.
      expect(caption.innerHTML).toContain('<strong>note</strong>')
    })

    it('does not render the caption when notes_html is empty', () => {
      render(
        <CollectionItemCard
          item={makeItem({ id: 99, notes_html: undefined })}
          density="comfortable"
        />
      )

      expect(
        screen.queryByTestId('collection-item-card-notes-99')
      ).not.toBeInTheDocument()
    })
  })

  describe('position badge', () => {
    it('renders the position badge when position is provided', () => {
      render(
        <CollectionItemCard
          item={makeItem()}
          position={3}
          density="comfortable"
        />
      )

      const badge = screen.getByTestId('collection-item-card-position')
      expect(badge).toHaveTextContent('3')
      expect(badge).toHaveAttribute('aria-label', 'Position 3')
    })

    it('does not render the position badge when position is omitted', () => {
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
        />
      )
      expect(
        screen.queryByTestId('collection-item-card-position')
      ).not.toBeInTheDocument()
    })
  })

  describe('navigation', () => {
    // One test per entity type confirms `getEntityUrl` is wired
    // correctly. If pluralization rules change in the helper, these
    // tests drive the failure here, not deep in a click handler.
    it.each([
      ['artist', '/artists/some-slug'],
      ['venue', '/venues/some-slug'],
      ['show', '/shows/some-slug'],
      ['release', '/releases/some-slug'],
      ['label', '/labels/some-slug'],
      ['festival', '/festivals/some-slug'],
    ])(
      'links to the correct entity URL for entity_type=%s',
      (entityType, expectedHref) => {
        render(
          <CollectionItemCard
            item={makeItem({
              entity_type: entityType,
              entity_slug: 'some-slug',
              entity_name: 'Some Entity',
            })}
            density="comfortable"
          />
        )

        // The card is one wrapping <a> covering image + title; assert the
        // single link points at the entity URL.
        const cardLink = screen.getByRole('link')
        expect(cardLink).toHaveAttribute('href', expectedHref)
      }
    )
  })

  describe('density', () => {
    // The density prop is used to pick icon size + title font size.
    // Snapshot the actual class string applied to the title link so a
    // regression in the density mapping fails loudly.
    it.each([
      ['compact', 'text-xs'],
      ['comfortable', 'text-sm'],
      ['expanded', 'text-base'],
    ] as const)(
      'applies density=%s title size class %s',
      (density, expectedClass) => {
        render(
          <CollectionItemCard
            item={makeItem({ entity_name: 'Density Probe' })}
            density={density}
          />
        )

        const titleEl = screen.getByTestId('collection-item-card-title')
        expect(titleEl.className).toContain(expectedClass)
      }
    )
  })
})
