import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { CommentThread } from './CommentThread'
import type { Comment, CommentListResponse } from '../types'

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

const ENTITY_TYPE = 'venue'
const ENTITY_ID = 42
const BODY = 'Posted while the first list fetch was in flight'

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 101,
    entity_type: ENTITY_TYPE,
    entity_id: ENTITY_ID,
    user_id: 7,
    author_name: 'Author',
    body: BODY,
    body_html: `<p>${BODY}</p>`,
    parent_id: null,
    root_id: null,
    depth: 0,
    ups: 0,
    downs: 0,
    score: 0,
    visibility: 'visible',
    reply_permission: 'anyone',
    edit_count: 0,
    is_edited: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function listResponse(comments: Comment[]): CommentListResponse {
  return { comments, total: comments.length, has_more: false }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}

// The production client's knobs that decide whether anything refetches on its
// own: a 15 minute staleTime and no refetch on focus.
function productionLikeClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 15 * 60 * 1000,
        refetchOnWindowFocus: false,
      },
      mutations: { retry: false },
    },
  })
}

const isListRequest = (url: string, init?: RequestInit) =>
  url.includes(`/entities/${ENTITY_TYPE}/${ENTITY_ID}/comments?sort=`) &&
  (init?.method ?? 'GET') === 'GET'

const isCreateRequest = (url: string, init?: RequestInit) =>
  url.endsWith(`/entities/${ENTITY_TYPE}/${ENTITY_ID}/comments`) &&
  init?.method === 'POST'

describe('CommentThread: a comment posted while the first list fetch is in flight', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('refetches after the create and renders the comment, never showing the empty state', async () => {
    const created = makeComment()
    const firstList = deferred<CommentListResponse>()
    let listRequests = 0

    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      if (isListRequest(url, init)) {
        listRequests += 1
        // The first list response was produced before the create committed.
        if (listRequests === 1) return firstList.promise
        return Promise.resolve(listResponse([created]))
      }
      if (isCreateRequest(url, init)) return Promise.resolve(created)
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const { container } = renderWithProviders(
      <CommentThread entityType={ENTITY_TYPE} entityId={ENTITY_ID} />,
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

    await waitFor(() => expect(listRequests).toBe(1))

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
      firstList.resolve(listResponse([]))
      await firstList.promise
    })

    expect(await screen.findByText(BODY)).toBeInTheDocument()
    expect(listRequests).toBe(2)
    observer.disconnect()
    expect(emptyStateRendered).toBe(false)
  })
})
