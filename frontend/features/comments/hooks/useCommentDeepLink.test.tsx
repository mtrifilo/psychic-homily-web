import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import {
  useCommentDeepLink,
  commentAnchorId,
  COMMENTS_SECTION_ANCHOR,
} from './useCommentDeepLink'
import type { Comment } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// jsdom doesn't implement scrollIntoView — install a spy so we can assert
// the deep link actually scrolled somewhere.
const scrollIntoViewMock = vi.fn()

function makeComment(overrides: Partial<Comment>): Comment {
  return {
    id: 1,
    entity_type: 'artist',
    entity_id: 42,
    user_id: 7,
    author_name: 'Test User',
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

/** Insert a DOM node carrying a comment anchor so tryScroll can find it. */
function mountAnchor(commentId: number): HTMLElement {
  const el = document.createElement('div')
  el.id = commentAnchorId(commentId)
  document.body.appendChild(el)
  return el
}

function mountSection(): HTMLElement {
  const el = document.createElement('section')
  el.id = COMMENTS_SECTION_ANCHOR
  document.body.appendChild(el)
  return el
}

describe('useCommentDeepLink (PSY-1512)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.HTMLElement.prototype.scrollIntoView = scrollIntoViewMock
  })

  afterEach(() => {
    window.location.hash = ''
    document.body.innerHTML = ''
  })

  it('is inert when the URL has no comment hash', () => {
    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 1 })], false),
      { wrapper: createWrapper() }
    )

    expect(result.current.targetId).toBeNull()
    expect(result.current.highlightId).toBeNull()
    expect(result.current.linkedThread).toBeNull()
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(scrollIntoViewMock).not.toHaveBeenCalled()
  })

  it('ignores non-comment hashes like #graph', () => {
    window.location.hash = '#graph'
    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 1 })], false),
      { wrapper: createWrapper() }
    )

    expect(result.current.targetId).toBeNull()
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('scrolls to and highlights an in-page top-level comment without extra fetches', async () => {
    window.location.hash = '#comment-5'
    mountAnchor(5)

    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 5 })], false),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(scrollIntoViewMock).toHaveBeenCalled())
    expect(result.current.highlightId).toBe(5)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('clears the highlight after the dwell time', () => {
    // The in-page path is fully synchronous (no queries fire), so fake
    // timers can drive both the scroll retry loop and the highlight-clear
    // timer without waitFor.
    vi.useFakeTimers()
    try {
      window.location.hash = '#comment-5'
      mountAnchor(5)

      const { result } = renderHook(
        () => useCommentDeepLink('artist', 42, [makeComment({ id: 5 })], false),
        { wrapper: createWrapper() }
      )

      expect(result.current.highlightId).toBe(5)
      act(() => {
        vi.advanceTimersByTime(3000)
      })
      expect(result.current.highlightId).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })

  it('resolves a reply whose root is in the list to expandRootId', async () => {
    window.location.hash = '#comment-9'
    mountAnchor(9)
    mockApiRequest.mockResolvedValueOnce(
      makeComment({ id: 9, parent_id: 5, root_id: 5, depth: 1 })
    )

    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 5 })], false),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.expandRootId).toBe(5))
    expect(result.current.linkedThread).toBeNull()
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
    expect(String(mockApiRequest.mock.calls[0][0])).toContain('/comments/9')
  })

  it('fetches the whole thread when the root is beyond the fetched page', async () => {
    window.location.hash = '#comment-30'
    mountAnchor(30)
    const root = makeComment({ id: 21 })
    const reply = makeComment({ id: 30, parent_id: 21, root_id: 21, depth: 1 })
    mockApiRequest.mockImplementation((url: string) => {
      if (String(url).endsWith('/comments/30')) return Promise.resolve(reply)
      if (String(url).endsWith('/comments/21/thread'))
        return Promise.resolve({ comments: [root, reply] })
      return Promise.reject(new Error(`unexpected url ${url}`))
    })

    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 1 })], false),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.linkedThread).not.toBeNull())
    expect(result.current.linkedThread?.comment.id).toBe(21)
    expect(result.current.linkedThread?.replies.map((r) => r.id)).toEqual([30])
    expect(result.current.expandRootId).toBeNull()
  })

  it('falls back to the comments section when the target cannot be resolved', async () => {
    window.location.hash = '#comment-404'
    const section = mountSection()
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('not found'), { status: 404 })
    )

    renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 1 })], false),
      { wrapper: createWrapper() }
    )

    await waitFor(() =>
      expect(scrollIntoViewMock.mock.contexts).toContain(section)
    )
  })

  it('treats a comment from a different entity as unreachable', async () => {
    window.location.hash = '#comment-9'
    const section = mountSection()
    mockApiRequest.mockResolvedValueOnce(
      makeComment({ id: 9, entity_type: 'venue', entity_id: 99 })
    )

    const { result } = renderHook(
      () => useCommentDeepLink('artist', 42, [makeComment({ id: 1 })], false),
      { wrapper: createWrapper() }
    )

    await waitFor(() =>
      expect(scrollIntoViewMock.mock.contexts).toContain(section)
    )
    expect(result.current.linkedThread).toBeNull()
    expect(result.current.expandRootId).toBeNull()
  })

  it('does nothing while the comment list is still loading', () => {
    window.location.hash = '#comment-5'
    renderHook(() => useCommentDeepLink('artist', 42, undefined, true), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(scrollIntoViewMock).not.toHaveBeenCalled()
  })
})
