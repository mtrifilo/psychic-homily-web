import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Collection, CollectionDetail, CollectionStats } from '@/features/collections'

type MutateOptions = {
  onSuccess?: () => void
  onError?: (err: unknown) => void
}

// `vi.hoisted` lets us share mock handles between the hoisted vi.mock factory
// and the test bodies, since `vi.mock` is hoisted above module-level
// const declarations. Matches the pattern PipelineVenues.test.tsx established.
const {
  mockSetFeaturedMutate,
  mockDeleteMutate,
  mocks,
  mockUseCollections,
  mockUseCollection,
  mockUseCollectionStats,
  mockUseDeleteCollection,
} = vi.hoisted(() => {
  const mocks = {
    nextSetFeaturedOutcome: { kind: 'success' } as
      | { kind: 'success' }
      | { kind: 'error'; error: unknown },
    lastDeleteMutateOpts: null as MutateOptions | null,
    deleteError: null as Error | null,
  }
  const mockSetFeaturedMutate = vi.fn(
    (_vars: { slug: string; featured: boolean }, options: MutateOptions = {}) => {
      if (mocks.nextSetFeaturedOutcome.kind === 'success') {
        options.onSuccess?.()
      } else {
        options.onError?.(mocks.nextSetFeaturedOutcome.error)
      }
    }
  )
  const mockDeleteMutate = vi.fn(
    (_vars: { slug: string }, options?: MutateOptions) => {
      mocks.lastDeleteMutateOpts = options ?? null
    }
  )
  // Per-test overridable hook returns. Default values match the original
  // tests (empty detail/stats so the detail panel renders the empty branches).
  const mockUseCollections = vi.fn()
  const mockUseCollection = vi.fn()
  const mockUseCollectionStats = vi.fn()
  const mockUseDeleteCollection = vi.fn()
  return {
    mockSetFeaturedMutate,
    mockDeleteMutate,
    mocks,
    mockUseCollections,
    mockUseCollection,
    mockUseCollectionStats,
    mockUseDeleteCollection,
  }
})

vi.mock('../hooks', () => ({
  useCollections: () => mockUseCollections(),
  useCollection: () => mockUseCollection(),
  useCollectionStats: () => mockUseCollectionStats(),
  useSetFeatured: () => ({
    mutate: mockSetFeaturedMutate,
    isPending: false,
  }),
  useDeleteCollection: () => mockUseDeleteCollection(),
}))

import { CollectionManagement } from './CollectionManagement'

function makeCollection(overrides: Partial<Collection> = {}): Collection {
  return {
    id: 1,
    title: 'Test Collection',
    slug: 'test-collection',
    description: 'A test collection',
    creator_id: 1,
    creator_name: 'testuser',
    collaborative: false,
    is_public: true,
    is_featured: false,
    display_mode: 'unranked',
    item_count: 5,
    subscriber_count: 3,
    contributor_count: 1,
    forks_count: 0,
    forked_from_collection_id: null,
    like_count: 0,
    user_likes_this: false,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeStats(overrides: Partial<CollectionStats> = {}): CollectionStats {
  return {
    item_count: 5,
    subscriber_count: 3,
    contributor_count: 1,
    entity_type_counts: {},
    ...overrides,
  }
}

function makeDetail(
  collection: Collection,
  items: CollectionDetail['items'] = []
): CollectionDetail {
  // `Collection` carries `tags?: TagSummary[]` while `CollectionDetail` carries
  // `tags?: EntityTag[]` via Omit<...>. Strip `tags` from the spread so the
  // CollectionDetail typing isn't broken by the wider Collection variant.
  const { tags: _tags, ...rest } = collection
  return {
    ...rest,
    items,
    is_subscribed: false,
  }
}

describe('CollectionManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.nextSetFeaturedOutcome = { kind: 'success' }
    mocks.lastDeleteMutateOpts = null
    mocks.deleteError = null
    mockUseCollections.mockReturnValue({
      data: {
        collections: [
          makeCollection({ id: 1, title: 'Coll A', slug: 'coll-a' }),
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    mockUseCollection.mockReturnValue({ data: undefined, isLoading: false })
    mockUseCollectionStats.mockReturnValue({ data: undefined, isLoading: false })
    mockUseDeleteCollection.mockReturnValue({
      mutate: mockDeleteMutate,
      isPending: false,
      get error() {
        return mocks.deleteError
      },
    })
  })

  describe('featured toggle error banner (PSY-729)', () => {
    it('does not show error banner initially', () => {
      render(<CollectionManagement />)
      expect(
        screen.queryByTestId('featured-toggle-error')
      ).not.toBeInTheDocument()
    })

    it('shows error banner when featured toggle fails', async () => {
      const user = userEvent.setup()
      mocks.nextSetFeaturedOutcome = {
        kind: 'error',
        error: new Error('Network timeout'),
      }

      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))

      const banner = await screen.findByTestId('featured-toggle-error')
      expect(banner).toBeInTheDocument()
      expect(banner.textContent).toContain('Network timeout')
    })

    // The click handler also clears the banner pre-mutate, which would
    // mask a missing onSuccess. Bypass it: invoke the captured onSuccess
    // directly to prove the clear is wired into the mutation contract.
    it('clears error banner on the next successful featured toggle', async () => {
      const user = userEvent.setup()
      mocks.nextSetFeaturedOutcome = {
        kind: 'error',
        error: new Error('Network timeout'),
      }
      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))
      expect(
        await screen.findByTestId('featured-toggle-error')
      ).toBeInTheDocument()

      const lastCall =
        mockSetFeaturedMutate.mock.calls[
          mockSetFeaturedMutate.mock.calls.length - 1
        ]
      const options = lastCall[1] as MutateOptions
      expect(options.onSuccess).toBeDefined()

      act(() => {
        options.onSuccess?.()
      })

      expect(
        screen.queryByTestId('featured-toggle-error')
      ).not.toBeInTheDocument()
    })

    it('uses fallback copy when the rejection is not an Error instance', async () => {
      const user = userEvent.setup()
      // Non-Error rejection (e.g. plain string thrown by the fetch helper).
      // The onError fallback must still surface a human-readable message.
      mocks.nextSetFeaturedOutcome = {
        kind: 'error',
        error: 'not-an-error-instance',
      }

      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))

      const banner = await screen.findByTestId('featured-toggle-error')
      expect(banner.textContent).toContain(
        'Failed to update featured status'
      )
    })

    it('passes through slug and featured flag to mutate', async () => {
      const user = userEvent.setup()
      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))

      expect(mockSetFeaturedMutate).toHaveBeenCalledWith(
        { slug: 'coll-a', featured: true },
        expect.any(Object)
      )
    })
  })

  describe('list header + empty state', () => {
    it('renders the total collection count from the list response', () => {
      mockUseCollections.mockReturnValueOnce({
        data: {
          collections: [
            makeCollection({ id: 1, slug: 'coll-a', title: 'Coll A' }),
          ],
          total: 47,
        },
        isLoading: false,
        error: null,
      })
      render(<CollectionManagement />)
      expect(screen.getByText('47 total')).toBeInTheDocument()
    })

    it('renders the empty state when there are no collections', () => {
      mockUseCollections.mockReturnValueOnce({
        data: { collections: [], total: 0 },
        isLoading: false,
        error: null,
      })
      render(<CollectionManagement />)
      expect(screen.getByText('No collections yet')).toBeInTheDocument()
      // No table should render either.
      expect(screen.queryByRole('table')).not.toBeInTheDocument()
    })

    it('renders a loading state while the list is fetching', () => {
      mockUseCollections.mockReturnValueOnce({
        data: undefined,
        isLoading: true,
        error: null,
      })
      render(<CollectionManagement />)
      expect(screen.getByText('Loading collections...')).toBeInTheDocument()
    })

    it('renders an error state when the list fetch fails', () => {
      mockUseCollections.mockReturnValueOnce({
        data: undefined,
        isLoading: false,
        error: new Error('boom'),
      })
      render(<CollectionManagement />)
      expect(screen.getByText('Failed to load collections')).toBeInTheDocument()
    })
  })

  describe('detail panel — stats display', () => {
    it('renders the stats grid (items / subscribers / contributors) when selected', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({
        id: 1,
        slug: 'coll-a',
        title: 'Coll A',
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollectionStats.mockReturnValue({
        data: makeStats({
          item_count: 12,
          subscriber_count: 8,
          contributor_count: 3,
        }),
        isLoading: false,
      })

      render(<CollectionManagement />)
      // Click the title cell to open the detail panel.
      await user.click(screen.getByText('Coll A'))

      // Anchor on the detail panel's "Stats" heading so we scope into that
      // section. The list table also has Items / Subscribers column headers,
      // so a bare getByText would match the wrong element.
      const statsHeading = screen.getByRole('heading', { name: 'Stats' })
      const statsSection = statsHeading.parentElement!
      expect(statsSection.textContent).toContain('Items')
      expect(statsSection.textContent).toContain('Subscribers')
      expect(statsSection.textContent).toContain('Contributors')
      expect(statsSection.textContent).toContain('12')
      expect(statsSection.textContent).toContain('8')
      expect(statsSection.textContent).toContain('3')
    })

    it('renders the entity-type breakdown when stats include counts', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({
        id: 1,
        slug: 'coll-a',
        title: 'Coll A',
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollectionStats.mockReturnValue({
        data: makeStats({
          entity_type_counts: { artist: 4, release: 2 },
        }),
        isLoading: false,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(screen.getByText('Entity type breakdown:')).toBeInTheDocument()
      // EntityTypeBadge text + count both visible in the breakdown rows.
      expect(screen.getByText('artist')).toBeInTheDocument()
      expect(screen.getByText('release')).toBeInTheDocument()
      expect(screen.getByText('4')).toBeInTheDocument()
      expect(screen.getByText('2')).toBeInTheDocument()
    })

    it('shows a loading message while stats are fetching', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollectionStats.mockReturnValue({
        data: undefined,
        isLoading: true,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(screen.getByText('Loading stats...')).toBeInTheDocument()
    })
  })

  describe('detail panel — items list', () => {
    it('renders item rows with name, type badge, added-by, and 1-indexed position', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollection.mockReturnValue({
        data: makeDetail(collection, [
          {
            id: 10,
            entity_type: 'artist',
            entity_id: 100,
            entity_name: 'Pavement',
            entity_slug: 'pavement',
            position: 0,
            added_by_user_id: 7,
            added_by_name: 'curator',
            created_at: '2026-04-01T12:00:00Z',
          },
        ]),
        isLoading: false,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(screen.getByText('Pavement')).toBeInTheDocument()
      // EntityTypeBadge renders the raw type string in the item row.
      // Use getAllByText since 'artist' may appear in both detail panel
      // sections (items list and entity-type breakdown when present).
      expect(screen.getAllByText('artist').length).toBeGreaterThan(0)
      expect(screen.getByText('curator')).toBeInTheDocument()
      // position+1 — first item shows "1".
      const itemsHeader = screen.getByRole('columnheader', { name: '#' })
      const itemsTable = itemsHeader.closest('table')!
      expect(itemsTable.textContent).toContain('1')
    })

    it('renders a "(note)" affordance only when the item has notes', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollection.mockReturnValue({
        data: makeDetail(collection, [
          {
            id: 10,
            entity_type: 'artist',
            entity_id: 100,
            entity_name: 'Pavement',
            entity_slug: 'pavement',
            position: 0,
            added_by_user_id: 7,
            added_by_name: 'curator',
            notes: 'Best slowcore record of 1994',
            created_at: '2026-04-01T12:00:00Z',
          },
          {
            id: 11,
            entity_type: 'release',
            entity_id: 200,
            entity_name: 'Slanted and Enchanted',
            entity_slug: 'slanted-and-enchanted',
            position: 1,
            added_by_user_id: 7,
            added_by_name: 'curator',
            created_at: '2026-04-01T12:00:00Z',
          },
        ]),
        isLoading: false,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      // Exactly one (note) marker for the first item; the second item has no notes.
      expect(screen.getAllByText('(note)')).toHaveLength(1)
    })

    it('shows the empty-items message when detail.items is empty', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      mockUseCollection.mockReturnValue({
        data: makeDetail(collection, []),
        isLoading: false,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(
        screen.getByText('No items in this collection')
      ).toBeInTheDocument()
    })

    it('toggles the detail panel closed when the same row is clicked again', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({
        slug: 'coll-a',
        title: 'Coll A',
        description: 'A unique detail description marker',
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })

      render(<CollectionManagement />)
      // After the panel opens, "Coll A" appears in BOTH the row cell and the
      // detail panel header. Pin the click target to the row by querying for
      // the title cell's slug sibling and walking up to the <tr>.
      const titleInRow = screen.getByText('/coll-a').closest('tr')!
      await user.click(titleInRow)
      expect(
        screen.getByText('A unique detail description marker')
      ).toBeInTheDocument()
      // Second click on the same row — closes.
      await user.click(titleInRow)
      expect(
        screen.queryByText('A unique detail description marker')
      ).not.toBeInTheDocument()
    })
  })

  describe('deletion confirmation', () => {
    it('does NOT fire the delete mutation when the user cancels the confirm dialog', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))
      await user.click(screen.getByRole('button', { name: 'Delete Collection' }))

      expect(confirmSpy).toHaveBeenCalledTimes(1)
      expect(confirmSpy.mock.calls[0][0]).toContain('Coll A')
      expect(mockDeleteMutate).not.toHaveBeenCalled()

      confirmSpy.mockRestore()
    })

    it('fires the delete mutation with the collection slug when the user confirms', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))
      await user.click(screen.getByRole('button', { name: 'Delete Collection' }))

      expect(mockDeleteMutate).toHaveBeenCalledWith(
        { slug: 'coll-a' },
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )

      confirmSpy.mockRestore()
    })

    it('closes the detail panel when the delete mutation succeeds', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const collection = makeCollection({
        slug: 'coll-a',
        title: 'Coll A',
        description: 'Detail panel marker',
      })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))
      // Sanity check: the detail panel is open.
      expect(screen.getByText('Detail panel marker')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Delete Collection' }))
      // Mutation was captured; fire its onSuccess to simulate completion.
      act(() => {
        mocks.lastDeleteMutateOpts?.onSuccess?.()
      })

      expect(screen.queryByText('Detail panel marker')).not.toBeInTheDocument()

      confirmSpy.mockRestore()
    })

    it('surfaces the delete-mutation error message when delete fails', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      // Stage an error on the mutation hook. The component reads
      // `deleteCollection.error` directly, so the getter wired up in
      // beforeEach surfaces this value on the next render.
      mocks.deleteError = new Error('Forbidden: not owner')

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(screen.getByText('Forbidden: not owner')).toBeInTheDocument()

      confirmSpy.mockRestore()
    })

    it('falls back to "Delete failed" when the error is not an Error instance', async () => {
      const user = userEvent.setup()
      const collection = makeCollection({ slug: 'coll-a', title: 'Coll A' })
      mockUseCollections.mockReturnValue({
        data: { collections: [collection], total: 1 },
        isLoading: false,
        error: null,
      })
      // Non-Error rejection (e.g. plain string from a misbehaving fetch helper).
      mocks.deleteError = 'string-mode failure' as unknown as Error

      render(<CollectionManagement />)
      await user.click(screen.getByText('Coll A'))

      expect(screen.getByText('Delete failed')).toBeInTheDocument()
    })
  })
})
