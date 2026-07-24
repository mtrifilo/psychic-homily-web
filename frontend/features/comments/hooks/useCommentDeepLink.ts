'use client'

import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { commentEndpoints, commentQueryKeys } from '../api'
import { useCommentThread } from './index'
import type { Comment } from '../types'

/**
 * DOM id for a comment card. Backend notification/email URLs append
 * `#comment-{id}` (see filter_service.go CommentURL and
 * comment_notification.go buildCommentURL) — this is the frontend half of
 * that contract.
 */
export function commentAnchorId(commentId: number): string {
  return `comment-${commentId}`
}

/** DOM id of the whole comments section — the deep-link fallback target. */
export const COMMENTS_SECTION_ANCHOR = 'comments'

const HASH_PATTERN = /^#comment-(\d+)$/

/** How long the target comment keeps its highlight after scrolling. */
const HIGHLIGHT_DURATION_MS = 2500

/**
 * Bounded wait for the target element to appear once resolution has
 * settled (the reply-thread fetch still renders asynchronously). The retry
 * loop only starts after we know WHERE the target lives, so this budget
 * covers render latency, not network latency. 100 x 100ms ≈ 10s, after
 * which we fall back to the comments section instead of silently doing
 * nothing.
 */
const SCROLL_MAX_ATTEMPTS = 100
const SCROLL_RETRY_MS = 100

export interface CommentDeepLinkState {
  /** Parsed `#comment-{id}` target, or null when the hash isn't a comment link. */
  targetId: number | null
  /** Comment id to highlight right now (cleared after HIGHLIGHT_DURATION_MS). */
  highlightId: number | null
  /**
   * Top-level comment (already in the first page) whose thread should be
   * auto-expanded because the target is one of its replies.
   */
  expandRootId: number | null
  /**
   * Thread to render in addition to the first page, because the target's
   * root comment is beyond the fetched page. Null when not needed.
   */
  linkedThread: { comment: Comment; replies: Comment[] } | null
}

function scrollToElement(el: HTMLElement) {
  const prefersReducedMotion =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  // Optional-call guard: jsdom doesn't implement scrollIntoView.
  el.scrollIntoView?.({
    behavior: prefersReducedMotion ? 'auto' : 'smooth',
    block: 'center',
  })
}

/**
 * Resolve a `#comment-{id}` URL fragment to a rendered, scrolled-to,
 * briefly-highlighted comment card (PSY-1512).
 *
 * Resolution is bounded to at most two extra requests:
 *   1. Target is a top-level comment in the fetched page → scroll directly.
 *   2. Otherwise GET /comments/{id} to learn its root. Root in the fetched
 *      page → auto-expand that card's replies (`expandRootId`).
 *   3. Root beyond the fetched page → GET /comments/{root}/thread and render
 *      it as an extra block (`linkedThread`).
 * If the target can't be resolved (deleted, wrong entity, bad id), we scroll
 * to the comments section instead of leaving the user at the page top.
 */
export function useCommentDeepLink(
  entityType: string,
  entityId: number,
  listComments: Comment[] | undefined,
  isListLoading: boolean
): CommentDeepLinkState {
  const hash = useUrlHash()
  const match = HASH_PATTERN.exec(hash)
  const targetId = match ? Number(match[1]) : null

  const listReady = !isListLoading && listComments !== undefined
  const comments = listComments ?? []
  const targetInList = targetId !== null && comments.some((c) => c.id === targetId)

  // Step 2: resolve the target's root when it isn't in the fetched page.
  const singleQuery = useQuery<Comment>({
    queryKey: [...commentQueryKeys.all, 'single', targetId ?? 0],
    queryFn: () =>
      apiRequest<Comment>(commentEndpoints.SINGLE(targetId ?? 0)),
    enabled: targetId !== null && listReady && !targetInList,
    retry: false,
    staleTime: Infinity,
  })

  // Guard against a hash carrying a comment id from a different entity —
  // without this we'd render a foreign thread into this page.
  const single =
    singleQuery.data &&
    singleQuery.data.entity_type === entityType &&
    singleQuery.data.entity_id === entityId
      ? singleQuery.data
      : undefined
  const wrongEntity = Boolean(singleQuery.data) && !single

  const rootId = targetInList
    ? targetId
    : single
      ? (single.root_id ?? single.id)
      : null
  const rootInList = rootId !== null && comments.some((c) => c.id === rootId)

  // Target is a reply of an already-rendered top-level comment: expand it.
  const expandRootId =
    rootId !== null && rootInList && rootId !== targetId ? rootId : null

  // Step 3: root is beyond the fetched page — fetch its whole thread.
  const needsLinkedThread = rootId !== null && !rootInList
  const threadQuery = useCommentThread(rootId ?? 0, needsLinkedThread)
  const linkedThread =
    needsLinkedThread && threadQuery.data?.comment
      ? { comment: threadQuery.data.comment, replies: threadQuery.data.replies }
      : null

  const unreachable =
    targetId !== null &&
    listReady &&
    (singleQuery.isError || threadQuery.isError || wrongEntity)

  // Scroll + highlight once the target element exists in the DOM.
  const [highlightId, setHighlightId] = useState<number | null>(null)
  const scrolledRef = useRef(false)
  const linkedThreadReady = linkedThread !== null

  // Don't start the scroll-retry loop until resolution has settled —
  // otherwise a slow single-comment/thread fetch burns the whole retry
  // budget and we fall back to the section even though the target is
  // seconds away from rendering.
  const readyToScroll =
    targetInList || expandRootId !== null || linkedThreadReady

  useEffect(() => {
    if (targetId === null || scrolledRef.current) return

    if (unreachable) {
      scrolledRef.current = true
      const section = document.getElementById(COMMENTS_SECTION_ANCHOR)
      if (section) scrollToElement(section)
      return
    }

    if (!listReady || !readyToScroll) return

    let cancelled = false
    let attempts = 0
    let retryTimer: ReturnType<typeof setTimeout> | null = null

    const tryScroll = () => {
      if (cancelled || scrolledRef.current) return
      const el = document.getElementById(commentAnchorId(targetId))
      if (el) {
        scrolledRef.current = true
        scrollToElement(el)
        setHighlightId(targetId)
        return
      }
      attempts += 1
      if (attempts < SCROLL_MAX_ATTEMPTS) {
        retryTimer = setTimeout(tryScroll, SCROLL_RETRY_MS)
      } else {
        // Bounded give-up: land at the comments section, not the page top.
        scrolledRef.current = true
        const section = document.getElementById(COMMENTS_SECTION_ANCHOR)
        if (section) scrollToElement(section)
      }
    }
    tryScroll()

    return () => {
      cancelled = true
      if (retryTimer) clearTimeout(retryTimer)
    }
  }, [targetId, listReady, unreachable, readyToScroll, expandRootId, linkedThreadReady])

  // Clear the highlight after its dwell time. Kept separate from the scroll
  // effect so a dep change there can't cancel the pending clear and leave
  // the tint applied forever.
  useEffect(() => {
    if (highlightId === null) return
    const timer = setTimeout(() => setHighlightId(null), HIGHLIGHT_DURATION_MS)
    return () => clearTimeout(timer)
  }, [highlightId])

  return { targetId, highlightId, expandRootId, linkedThread }
}
