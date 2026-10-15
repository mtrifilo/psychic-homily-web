import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { CommentThread } from './CommentThread'
import {
  makeComment,
  productionLikeClient,
  routeGets,
} from '../hooks/inflightOrdering.fixtures'
import type { CommentListResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: true,
    user: { id: '7', email: 'author@test.local' },
  }),
}))

vi.mock('next/navigation', () => ({
  usePathname: () => '/venues/test-venue',
}))

const BODY = 'Posted while the first list fetch was in flight'
const LIST = '/entities/venue/42/comments?sort='

describe('CommentThread: a comment posted while the first list fetch is in flight', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('refetches after the create and renders the comment, never showing the empty state', async () => {
    const created = makeComment({ id: 101, body: BODY, body_html: `<p>${BODY}</p>` })
    const staleList = Promise.withResolvers<CommentListResponse>()
    const gets = routeGets({
      [LIST]: {
        stale: staleList.promise,
        fresh: { comments: [created], total: 1, has_more: false },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/entities/venue/42/comments') && init?.method === 'POST') {
        return Promise.resolve(created)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const { container } = renderWithProviders(
      <CommentThread entityType="venue" entityId={42} />,
      { queryClient: productionLikeClient() }
    )

    // Records every insertion of the empty state, including one that a later
    // render removes before an assertion could look.
    let emptyStateRendered = false
    const isOrHoldsEmptyState = (node: Node) =>
      node instanceof Element &&
      (node.matches('[data-testid="empty-state"]') ||
        node.querySelector('[data-testid="empty-state"]') !== null)
    const observer = new MutationObserver((records) => {
      for (const record of records) {
        if (Array.from(record.addedNodes).some(isOrHoldsEmptyState)) {
          emptyStateRendered = true
        }
      }
    })
    observer.observe(container, { childList: true, subtree: true })

    await waitFor(() => expect(gets.counts[LIST]).toBe(1))

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: BODY },
    })
    fireEvent.click(screen.getByTestId('comment-submit'))

    // The create has resolved and its success handlers ran: the form cleared.
    await waitFor(() =>
      expect(screen.getByTestId('comment-textarea')).toHaveValue('')
    )

    // The stale first response arrives only after the create was handled.
    await act(async () => {
      staleList.resolve({ comments: [], total: 0, has_more: false })
      await staleList.promise
    })

    expect(await screen.findByText(BODY)).toBeInTheDocument()
    expect(gets.counts[LIST]).toBe(2)
    observer.disconnect()
    expect(emptyStateRendered).toBe(false)
  })
})
