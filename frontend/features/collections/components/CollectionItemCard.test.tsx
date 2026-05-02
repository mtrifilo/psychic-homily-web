import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

// Mock the remove mutation hook so we can assert it was invoked with the
// right slug + itemId without standing up a QueryClientProvider. The
// `mutate` impl pulls `onSuccess` from its options arg so we can assert
// post-success state-resets too.
const mockRemoveMutate = vi.fn()
const mockRemoveIsPending = vi.fn(() => false)

vi.mock('../hooks', () => ({
  useRemoveCollectionItem: () => ({
    mutate: mockRemoveMutate,
    isPending: mockRemoveIsPending(),
  }),
}))

// Import after mocks register so the component picks up the stubbed hook.
import { CollectionItemCard } from './CollectionItemCard'

beforeEach(() => {
  mockRemoveMutate.mockReset()
  mockRemoveIsPending.mockReset()
  mockRemoveIsPending.mockReturnValue(false)
})

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

  describe('PSY-526: Remove control (creator-only)', () => {
    it('does not render the Remove control when isCreator is false', () => {
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={false}
          slug="my-coll"
        />
      )
      expect(
        screen.queryByTestId('collection-item-card-remove')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('collection-item-card-actions')
      ).not.toBeInTheDocument()
    })

    it('does not render the Remove control when slug is missing', () => {
      // Defensive: slug is structurally optional but functionally
      // required for the mutation. The card opts out rather than
      // rendering a broken control.
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
        />
      )
      expect(
        screen.queryByTestId('collection-item-card-remove')
      ).not.toBeInTheDocument()
    })

    it('renders both desktop X and touch kebab when isCreator is true', () => {
      // CSS-driven visibility is split media-query side; both controls
      // are unconditionally in the DOM so the right one is available
      // when the user's pointer environment matches.
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )
      expect(
        screen.getByTestId('collection-item-card-remove')
      ).toBeInTheDocument()
      expect(
        screen.getByTestId('collection-item-card-actions')
      ).toBeInTheDocument()
    })

    it('clicking the X reveals the destructive confirm button', async () => {
      const user = userEvent.setup()
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      // Idle state — confirm button absent.
      expect(
        screen.queryByTestId('collection-item-card-remove-confirm')
      ).not.toBeInTheDocument()

      await user.click(screen.getByTestId('collection-item-card-remove'))

      // Confirm step — destructive button + Cancel both present.
      expect(
        screen.getByTestId('collection-item-card-remove-confirm')
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Cancel' })
      ).toBeInTheDocument()
    })

    it('confirm button calls useRemoveCollectionItem with the right args', async () => {
      const user = userEvent.setup()
      render(
        <CollectionItemCard
          item={makeItem({ id: 42 })}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      await user.click(screen.getByTestId('collection-item-card-remove'))
      await user.click(
        screen.getByTestId('collection-item-card-remove-confirm')
      )

      expect(mockRemoveMutate).toHaveBeenCalledTimes(1)
      const [variables, options] = mockRemoveMutate.mock.calls[0]
      expect(variables).toEqual({ slug: 'my-coll', itemId: 42 })
      // The component supplies an onSuccess that resets local state;
      // assert it exists so a future refactor that drops it fails here.
      expect(options).toMatchObject({ onSuccess: expect.any(Function) })
    })

    it('cancel returns to idle without calling the mutation', async () => {
      const user = userEvent.setup()
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      await user.click(screen.getByTestId('collection-item-card-remove'))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))

      expect(mockRemoveMutate).not.toHaveBeenCalled()
      expect(
        screen.queryByTestId('collection-item-card-remove-confirm')
      ).not.toBeInTheDocument()
      // X is back in DOM (idle state).
      expect(
        screen.getByTestId('collection-item-card-remove')
      ).toBeInTheDocument()
    })

    it('clicking the kebab opens the touch popover with a Remove menu item', async () => {
      const user = userEvent.setup()
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      // Menu item not in DOM until kebab is clicked.
      expect(
        screen.queryByTestId('collection-item-card-remove-menu-item')
      ).not.toBeInTheDocument()

      await user.click(screen.getByTestId('collection-item-card-actions'))

      const menuItem = screen.getByTestId(
        'collection-item-card-remove-menu-item'
      )
      expect(menuItem).toBeInTheDocument()
      expect(menuItem).toHaveTextContent(/remove from collection/i)
    })

    it('selecting the menu item transitions to the confirm step', async () => {
      const user = userEvent.setup()
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      await user.click(screen.getByTestId('collection-item-card-actions'))
      await user.click(
        screen.getByTestId('collection-item-card-remove-menu-item')
      )

      expect(
        screen.getByTestId('collection-item-card-remove-confirm')
      ).toBeInTheDocument()
      // Menu closes when transitioning to confirm so the user only sees
      // one decision at a time.
      expect(
        screen.queryByTestId('collection-item-card-remove-menu-item')
      ).not.toBeInTheDocument()
    })

    it('renders the Remove control as a sibling of (not inside) the wrapping <Link>', () => {
      // Project memory note: image + title sit inside a single <Link>
      // so Playwright `getByRole('link', { name })` strict-mode resolves
      // cleanly. The Remove control must be a sibling of that <Link>,
      // not nested inside it (which would also be invalid HTML —
      // <button> inside <a>). Assert the DOM relationship directly.
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )

      const link = screen.getByRole('link')
      const removeBtn = screen.getByTestId('collection-item-card-remove')

      // Remove button must NOT be a descendant of the link.
      expect(link.contains(removeBtn)).toBe(false)
      // …but should still live inside the same article wrapper.
      expect(
        screen.getByTestId('collection-item-card').contains(removeBtn)
      ).toBe(true)
    })

    it('keeps the title="Remove from collection" smoke-test selector intact', () => {
      // PSY-526: the existing E2E smoke test in add-to-collection.spec.ts
      // queries by title to find the Remove trigger. Preserve that
      // contract on the desktop path so the workaround commit
      // (78df8f7c, "switch add-to-collection cleanup to list view") can
      // be reverted in a follow-up.
      render(
        <CollectionItemCard
          item={makeItem({ entity_name: 'Some Show' })}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )
      const trigger = screen.getByTestId('collection-item-card-remove')
      expect(trigger).toHaveAttribute('title', 'Remove from collection')
    })

    it('disables the X trigger while the mutation is pending', () => {
      // When the mutation is mid-flight (e.g. user double-clicked, or
      // an adjacent card already triggered a remove), the idle X is
      // disabled to prevent stacking concurrent deletes.
      mockRemoveIsPending.mockReturnValue(true)
      render(
        <CollectionItemCard
          item={makeItem()}
          density="comfortable"
          isCreator={true}
          slug="my-coll"
        />
      )
      expect(
        screen.getByTestId('collection-item-card-remove')
      ).toBeDisabled()
      expect(
        screen.getByTestId('collection-item-card-actions')
      ).toBeDisabled()
    })
  })
})
