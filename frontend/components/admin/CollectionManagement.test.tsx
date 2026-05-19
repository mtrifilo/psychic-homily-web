import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CollectionManagement } from './CollectionManagement'
import type { Collection } from '@/features/collections'

// Mock the collections feature module. `useSetFeatured` is the focus of this
// test — the inner mutate fn is a spy whose behaviour each test case
// controls (success vs error) by overriding `nextSetFeaturedOutcome`.
type MutateOptions = {
  onSuccess?: () => void
  onError?: (err: unknown) => void
}

let nextSetFeaturedOutcome:
  | { kind: 'success' }
  | { kind: 'error'; error: Error } = { kind: 'success' }

const mockSetFeaturedMutate = vi.fn(
  (_vars: { slug: string; featured: boolean }, options: MutateOptions = {}) => {
    if (nextSetFeaturedOutcome.kind === 'success') {
      options.onSuccess?.()
    } else {
      options.onError?.(nextSetFeaturedOutcome.error)
    }
  }
)

const mockDeleteMutate = vi.fn()

vi.mock('@/features/collections', () => ({
  useCollections: () => mockUseCollections(),
  useCollection: () => ({ data: undefined, isLoading: false }),
  useCollectionStats: () => ({ data: undefined, isLoading: false }),
  useSetFeatured: () => ({
    mutate: mockSetFeaturedMutate,
    isPending: false,
  }),
  useDeleteCollection: () => ({
    mutate: mockDeleteMutate,
    isPending: false,
    error: null,
  }),
}))

const mockUseCollections = vi.fn()

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

describe('CollectionManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    nextSetFeaturedOutcome = { kind: 'success' }
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
      nextSetFeaturedOutcome = {
        kind: 'error',
        error: new Error('Network timeout'),
      }

      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))

      const banner = await screen.findByTestId('featured-toggle-error')
      expect(banner).toBeInTheDocument()
      expect(banner.textContent).toContain('Network timeout')
    })

    // Regression for PSY-729: a failed toggle sets the inline error banner,
    // and the next successful toggle must clear it. Previously, only the
    // onError callback was wired to setFeaturedError, so the banner
    // persisted across subsequent successful toggles until the user
    // reloaded or manually dismissed.
    //
    // This test specifically validates that an onSuccess callback is
    // wired into the mutate options and that invoking it clears the
    // banner. We intentionally bypass the click handler for the success
    // step — the click handler already clears the banner pre-mutate
    // (line 292), which would mask a missing onSuccess.
    it('clears error banner on the next successful featured toggle', async () => {
      const user = userEvent.setup()
      // First click fails → banner surfaces via onError.
      nextSetFeaturedOutcome = {
        kind: 'error',
        error: new Error('Network timeout'),
      }
      render(<CollectionManagement />)
      await user.click(screen.getByRole('switch'))
      expect(
        await screen.findByTestId('featured-toggle-error')
      ).toBeInTheDocument()

      // Grab the most recent mutate call's options and invoke its
      // onSuccess directly. This proves the convention is wired in
      // the mutation contract itself, not just in the click handler.
      const lastCall =
        mockSetFeaturedMutate.mock.calls[
          mockSetFeaturedMutate.mock.calls.length - 1
        ]
      const options = lastCall[1] as MutateOptions
      expect(options.onSuccess).toBeDefined()

      // Invoke onSuccess (this is what TanStack Query does on a
      // successful mutation). Wrap in act so the state update flushes.
      const { act } = await import('@testing-library/react')
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
      nextSetFeaturedOutcome = {
        kind: 'error',
        error: 'not-an-error-instance' as unknown as Error,
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
})
