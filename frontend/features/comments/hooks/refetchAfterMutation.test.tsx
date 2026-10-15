import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, useQuery } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  useComments,
  useCommentThread,
  useCreateFieldNote,
  useFieldNotes,
  useReplyToComment,
} from './index'
import { refetchAfterMutation } from './refetchAfterMutation'
import type { Comment, CommentListResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 1,
    entity_type: 'venue',
    entity_id: 42,
    user_id: 7,
    author_name: 'Author',
    body: 'body',
    body_html: '<p>body</p>',
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

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}

// A 15 minute staleTime, as in production, so nothing refetches on its own.
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

/**
 * Routes GETs by URL fragment. The first GET for each fragment returns the
 * held `stale` response; later GETs return `fresh`.
 */
function routeGets(
  routes: Record<string, { stale: Promise<unknown>; fresh: unknown }>
) {
  const counts: Record<string, number> = {}
  const handle = (url: string, init?: RequestInit) => {
    if ((init?.method ?? 'GET') !== 'GET') return undefined
    const fragment = Object.keys(routes).find((f) => url.includes(f))
    if (!fragment) return undefined
    counts[fragment] = (counts[fragment] ?? 0) + 1
    return counts[fragment] === 1
      ? routes[fragment].stale
      : Promise.resolve(routes[fragment].fresh)
  }
  return { counts, handle }
}

describe('refetchAfterMutation', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('starts a new request when the first fetch is still in flight, and discards the late response', async () => {
    const client = productionLikeClient()
    const first = deferred<string>()
    const queryFn = vi
      .fn<() => Promise<string>>()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce('after write')

    const { result } = renderHook(
      () => useQuery({ queryKey: ['comments', 'venue', 42, 'best'], queryFn }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1))

    await act(() => refetchAfterMutation(client, ['comments', 'venue', 42]))
    await act(async () => {
      first.resolve('before write')
      await first.promise
    })

    await waitFor(() => expect(result.current.data).toBe('after write'))
    expect(queryFn).toHaveBeenCalledTimes(2)
  })

  it('keeps showing existing data while it refetches', async () => {
    const client = productionLikeClient()
    const second = deferred<string>()
    const queryFn = vi
      .fn<() => Promise<string>>()
      .mockResolvedValueOnce('before write')
      .mockReturnValueOnce(second.promise)

    const { result } = renderHook(
      () => useQuery({ queryKey: ['comments', 'venue', 42, 'best'], queryFn }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(result.current.data).toBe('before write'))

    act(() => {
      void refetchAfterMutation(client, ['comments', 'venue', 42])
    })
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(2))
    expect(result.current.data).toBe('before write')

    await act(async () => {
      second.resolve('after write')
      await second.promise
    })
    await waitFor(() => expect(result.current.data).toBe('after write'))
  })
})

describe('mutations that refetch after a write while the first fetch is in flight', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useReplyToComment refetches the entity list and the thread', async () => {
    const parent = makeComment({ id: 10 })
    const reply = makeComment({ id: 11, parent_id: 10, root_id: 10, depth: 1 })
    const staleList = deferred<CommentListResponse>()
    const staleThread = deferred<{ comments: Comment[] }>()
    const gets = routeGets({
      '/entities/venue/42/comments?sort=': {
        stale: staleList.promise,
        fresh: { comments: [parent], total: 1, has_more: false },
      },
      '/comments/10/thread': {
        stale: staleThread.promise,
        fresh: { comments: [parent, reply] },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/comments/10/replies') && init?.method === 'POST') {
        return Promise.resolve(reply)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = productionLikeClient()
    const { result } = renderHook(
      () => ({
        list: useComments('venue', 42),
        thread: useCommentThread(10, true),
        reply: useReplyToComment(),
      }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => {
      expect(gets.counts['/entities/venue/42/comments?sort=']).toBe(1)
      expect(gets.counts['/comments/10/thread']).toBe(1)
    })

    await act(() =>
      result.current.reply.mutateAsync({
        commentId: 10,
        body: 'a reply',
        entityType: 'venue',
        entityId: 42,
      })
    )
    await act(async () => {
      staleList.resolve({ comments: [], total: 0, has_more: false })
      staleThread.resolve({ comments: [parent] })
      await Promise.all([staleList.promise, staleThread.promise])
    })

    await waitFor(() => {
      expect(result.current.thread.data?.replies.map((c) => c.id)).toEqual([11])
      expect(result.current.list.data?.comments.map((c) => c.id)).toEqual([10])
    })
    expect(gets.counts['/entities/venue/42/comments?sort=']).toBe(2)
    expect(gets.counts['/comments/10/thread']).toBe(2)
  })

  it('useCreateFieldNote refetches the show field notes', async () => {
    const note = makeComment({ id: 20, entity_type: 'show', entity_id: 5 })
    const staleNotes = deferred<CommentListResponse>()
    const gets = routeGets({
      '/shows/5/field-notes': {
        stale: staleNotes.promise,
        fresh: { comments: [note], total: 1, has_more: false },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/shows/5/field-notes') && init?.method === 'POST') {
        return Promise.resolve(note)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = productionLikeClient()
    const { result } = renderHook(
      () => ({
        notes: useFieldNotes(5),
        create: useCreateFieldNote(),
      }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts['/shows/5/field-notes']).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({
        showId: 5,
        input: { body: 'a note' },
      })
    )
    await act(async () => {
      staleNotes.resolve({ comments: [], total: 0, has_more: false })
      await staleNotes.promise
    })

    await waitFor(() =>
      expect(result.current.notes.data?.comments.map((c) => c.id)).toEqual([20])
    )
    expect(gets.counts['/shows/5/field-notes']).toBe(2)
  })
})
