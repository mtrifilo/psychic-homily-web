import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import {
  useComments,
  useCommentThread,
  useCreateFieldNote,
  useFieldNotes,
  useReplyToComment,
} from './index'
import { makeComment } from './inflightOrdering.fixtures'
import type { Comment, CommentListResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

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

    const client = clientWithAppDefaults()
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

    const client = clientWithAppDefaults()
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
