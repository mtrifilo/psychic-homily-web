import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CommentThread } from './CommentThread'
import type { Comment } from '../types'

// --- Mocks ---

const mockUseComments = vi.fn()
const mockUseCreateComment = vi.fn()
const mockUseAuthContext = vi.fn()

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }

vi.mock('../hooks', async () => {
  // PSY-589: bring through the real formatCommentSubmissionError so the
  // CommentThread test can assert on the exact banner copy under 429.
  // PSY-958: the vote auto-dismiss banner now uses the shared
  // useAutoDismissBanner primitive directly (unmocked), so the mock no longer
  // needs to forward a comments-local hook for it.
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useComments: (...args: unknown[]) => mockUseComments(...args),
    useCreateComment: () => mockUseCreateComment(),
    useReplyToComment: () => defaultMutationReturn,
    useUpdateComment: () => defaultMutationReturn,
    useUpdateReplyPermission: () => defaultMutationReturn,
    useDeleteComment: () => defaultMutationReturn,
    useVoteComment: () => defaultMutationReturn,
    useUnvoteComment: () => defaultMutationReturn,
    useCommentThread: () => ({ data: undefined as unknown }),
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// PSY-1512: the deep-link hook owns hash parsing + resolution queries; mock
// it so CommentThread tests don't need a QueryClientProvider. Its own logic
// is covered in hooks/useCommentDeepLink.test.tsx.
const mockUseCommentDeepLink = vi.fn()

vi.mock('../hooks/useCommentDeepLink', () => ({
  useCommentDeepLink: (...args: unknown[]) => mockUseCommentDeepLink(...args),
}))

const inertDeepLink = {
  highlightId: null,
  expandRootId: null,
  linkedThread: null,
}

describe('CommentThread', () => {
  const defaultProps = {
    entityType: 'artist',
    entityId: 42,
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateComment.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })
    mockUseCommentDeepLink.mockReturnValue(inertDeepLink)
  })

  it('renders empty state when no comments', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    mockUseComments.mockReturnValue({
      data: { comments: [], total: 0, has_more: false },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('comment-thread')).toBeInTheDocument()
    expect(screen.getByTestId('empty-state')).toBeInTheDocument()
    expect(
      screen.getByText('No comments yet. Be the first to share your thoughts.')
    ).toBeInTheDocument()
  })

  it('renders auth gate for unauthenticated users', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    mockUseComments.mockReturnValue({
      data: { comments: [], total: 0, has_more: false },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('auth-gate')).toBeInTheDocument()
    expect(screen.getByText('Sign in')).toBeInTheDocument()
  })

  it('renders comment form for authenticated users', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1', email: 'test@test.com' },
    })
    mockUseComments.mockReturnValue({
      data: { comments: [], total: 0, has_more: false },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('comment-textarea')).toBeInTheDocument()
    expect(screen.getByTestId('comment-submit')).toBeInTheDocument()
  })

  it('renders loading skeleton', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    mockUseComments.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('comment-thread')).toBeInTheDocument()
    // Should not show empty state while loading
    expect(screen.queryByTestId('empty-state')).not.toBeInTheDocument()
  })

  it('renders comment count in heading', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    mockUseComments.mockReturnValue({
      data: {
        comments: [
          {
            id: 1,
            entity_type: 'artist',
            entity_id: 42,
            user_id: 2,
            author_name: 'TestUser',
            body: 'Great artist!',
            body_html: '<p>Great artist!</p>',
            parent_id: null,
            root_id: null,
            depth: 0,
            ups: 3,
            downs: 0,
            score: 3,
            visibility: 'visible',
            reply_permission: 'anyone',
            edit_count: 0,
            is_edited: false,
            created_at: '2026-04-01T00:00:00Z',
            updated_at: '2026-04-01T00:00:00Z',
          },
        ],
        total: 1,
        has_more: false,
      },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByText('(1)')).toBeInTheDocument()
    expect(screen.getByText('Discussion')).toBeInTheDocument()
  })

  it('renders sort buttons when comments exist', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    mockUseComments.mockReturnValue({
      data: {
        comments: [
          {
            id: 1,
            entity_type: 'artist',
            entity_id: 42,
            user_id: 2,
            author_name: 'TestUser',
            body: 'Test',
            body_html: '<p>Test</p>',
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
            created_at: '2026-04-01T00:00:00Z',
            updated_at: '2026-04-01T00:00:00Z',
          },
        ],
        total: 1,
        has_more: false,
      },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByText('Best')).toBeInTheDocument()
    expect(screen.getByText('New')).toBeInTheDocument()
    expect(screen.getByText('Top')).toBeInTheDocument()
  })

  // PSY-513: pending-review feedback.
  describe('pending-review feedback (PSY-513)', () => {
    function makePending(overrides: Partial<Comment> = {}): Comment {
      return {
        id: 9001,
        entity_type: 'artist',
        entity_id: 42,
        user_id: 7,
        author_name: 'NewUser',
        body: 'Will it appear?',
        body_html: '<p>Will it appear?</p>',
        parent_id: null,
        root_id: null,
        depth: 0,
        ups: 0,
        downs: 0,
        score: 0,
        visibility: 'pending_review',
        reply_permission: 'anyone',
        edit_count: 0,
        is_edited: false,
        created_at: '2026-04-29T18:00:00Z',
        updated_at: '2026-04-29T18:00:00Z',
        ...overrides,
      }
    }

    it('renders banner + optimistic comment with badge when POST returns pending_review', () => {
      const pending = makePending()
      // Make the mocked mutate invoke onSuccess synchronously with a
      // pending_review response — emulates a new_user-tier submit.
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onSuccess?: (data: Comment) => void }) => {
          opts?.onSuccess?.(pending)
        }
      )
      mockUseCreateComment.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'newuser@example.com' },
      })
      mockUseComments.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(<CommentThread {...defaultProps} />)

      // Empty state visible before submit.
      expect(screen.getByTestId('empty-state')).toBeInTheDocument()
      expect(screen.queryByTestId('pending-review-banner')).not.toBeInTheDocument()

      // Submit the form.
      fireEvent.change(screen.getByTestId('comment-textarea'), {
        target: { value: 'Will it appear?' },
      })
      fireEvent.click(screen.getByTestId('comment-submit'))

      // Banner appears.
      expect(screen.getByTestId('pending-review-banner')).toBeInTheDocument()
      expect(
        screen.getByText(/awaiting moderation/i)
      ).toBeInTheDocument()

      // Empty-state is suppressed once a pending comment exists.
      expect(screen.queryByTestId('empty-state')).not.toBeInTheDocument()

      // Optimistic comment with the Pending review badge is rendered.
      expect(screen.getByTestId('pending-review-badge')).toBeInTheDocument()
      expect(screen.getByText('Will it appear?')).toBeInTheDocument()
    })

    it('does NOT render banner when POST returns visible (trusted-tier auto-publish)', () => {
      const visible: Comment = {
        ...makePending({ visibility: 'visible' }),
      }
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onSuccess?: (data: Comment) => void }) => {
          opts?.onSuccess?.(visible)
        }
      )
      mockUseCreateComment.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'trusted@example.com' },
      })
      mockUseComments.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(<CommentThread {...defaultProps} />)

      fireEvent.change(screen.getByTestId('comment-textarea'), {
        target: { value: 'Auto-published' },
      })
      fireEvent.click(screen.getByTestId('comment-submit'))

      expect(screen.queryByTestId('pending-review-banner')).not.toBeInTheDocument()
      expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
    })

    // PSY-589: when the create mutation 429s, the form must surface an
    // inline banner instead of silently clearing.
    it('renders inline 429 banner with countdown copy when create mutation rate-limits', () => {
      const err = Object.assign(
        new Error('please wait 60 seconds between comments on the same entity'),
        { status: 429, retryAfter: 60 }
      )
      mockUseCreateComment.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        error: err,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'rate@example.com' },
      })
      mockUseComments.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(<CommentThread {...defaultProps} />)

      const banner = screen.getByTestId('comment-form-error')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveTextContent('Please wait 60s before commenting again.')
    })

    it('drops the optimistic entry once the canonical row appears in the list (post-approval refetch)', () => {
      const pending = makePending()
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onSuccess?: (data: Comment) => void }) => {
          opts?.onSuccess?.(pending)
        }
      )
      mockUseCreateComment.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'newuser@example.com' },
      })

      // Initially the canonical list is empty (server still has it pending).
      mockUseComments.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      const { rerender } = render(<CommentThread {...defaultProps} />)

      fireEvent.change(screen.getByTestId('comment-textarea'), {
        target: { value: 'Will it appear?' },
      })
      fireEvent.click(screen.getByTestId('comment-submit'))

      expect(screen.getByTestId('pending-review-banner')).toBeInTheDocument()

      // Simulate a refetch after admin approval — same id, now visible.
      mockUseComments.mockReturnValue({
        data: {
          comments: [{ ...pending, visibility: 'visible' }],
          total: 1,
          has_more: false,
        },
        isLoading: false,
      })

      rerender(<CommentThread {...defaultProps} />)

      // Optimistic entry de-duped; banner gone.
      expect(screen.queryByTestId('pending-review-banner')).not.toBeInTheDocument()
      expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
    })
  })
})

// PSY-1512: comment deep links.
describe('CommentThread — deep links (PSY-1512)', () => {
  const defaultProps = { entityType: 'artist', entityId: 42 }

  function makeComment(overrides: Partial<Comment> = {}): Comment {
    return {
      id: 1,
      entity_type: 'artist',
      entity_id: 42,
      user_id: 2,
      author_name: 'TestUser',
      body: 'Test',
      body_html: '<p>Test</p>',
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
      created_at: '2026-04-01T00:00:00Z',
      updated_at: '2026-04-01T00:00:00Z',
      ...overrides,
    }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateComment.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseAuthContext.mockReturnValue({ isAuthenticated: false, user: null })
    mockUseCommentDeepLink.mockReturnValue(inertDeepLink)
  })

  it('gives the section the "comments" anchor id (deep-link fallback target)', () => {
    mockUseComments.mockReturnValue({
      data: { comments: [], total: 0, has_more: false },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('comment-thread')).toHaveAttribute(
      'id',
      'comments'
    )
  })

  it('renders the linked thread block when the target root is beyond the fetched page', () => {
    mockUseComments.mockReturnValue({
      data: {
        comments: [makeComment({ id: 1 })],
        total: 25,
        has_more: true,
      },
      isLoading: false,
    })
    mockUseCommentDeepLink.mockReturnValue({
      highlightId: 30,
      expandRootId: null,
      linkedThread: {
        comment: makeComment({ id: 21, body_html: '<p>Page-two root</p>' }),
        replies: [
          makeComment({
            id: 30,
            parent_id: 21,
            root_id: 21,
            depth: 1,
            body_html: '<p>Deep-linked reply</p>',
          }),
        ],
      },
    })

    render(<CommentThread {...defaultProps} />)

    const block = screen.getByTestId('deep-link-thread')
    expect(block).toBeInTheDocument()
    expect(screen.getByText('Page-two root')).toBeInTheDocument()
    expect(screen.getByText('Deep-linked reply')).toBeInTheDocument()
    // The deep-linked reply carries the anchor + highlight.
    const cards = screen.getAllByTestId('comment-card')
    const target = cards.find((c) => c.id === 'comment-30')
    expect(target).toBeDefined()
    expect(target?.className).toContain('outline')
  })

  it('auto-expands the in-page root when the deep link targets one of its replies', () => {
    mockUseComments.mockReturnValue({
      data: {
        comments: [makeComment({ id: 5, reply_count: 2 })],
        total: 1,
        has_more: false,
      },
      isLoading: false,
    })
    mockUseCommentDeepLink.mockReturnValue({
      highlightId: null,
      expandRootId: 5,
      linkedThread: null,
    })

    render(<CommentThread {...defaultProps} />)

    // autoExpandThread suppresses the manual "Show replies" affordance —
    // the thread is being loaded without a click.
    expect(screen.queryByTestId('show-replies-button')).not.toBeInTheDocument()
  })

  it('renders the linked thread even when the first page has no visible top-level comments', () => {
    mockUseComments.mockReturnValue({
      data: { comments: [], total: 0, has_more: false },
      isLoading: false,
    })
    mockUseCommentDeepLink.mockReturnValue({
      highlightId: 30,
      expandRootId: null,
      linkedThread: {
        comment: makeComment({
          id: 21,
          visibility: 'hidden_by_user',
          body_html: '<p>hidden root</p>',
        }),
        replies: [
          makeComment({
            id: 30,
            parent_id: 21,
            root_id: 21,
            depth: 1,
            body_html: '<p>Reply under hidden root</p>',
          }),
        ],
      },
    })

    render(<CommentThread {...defaultProps} />)

    // The empty state must NOT swallow the resolved thread.
    expect(screen.queryByTestId('empty-state')).not.toBeInTheDocument()
    expect(screen.getByTestId('deep-link-thread')).toBeInTheDocument()
    expect(screen.getByText('Reply under hidden root')).toBeInTheDocument()
  })

  it('still renders the manual "Show replies" affordance when no deep link targets the thread', () => {
    mockUseComments.mockReturnValue({
      data: {
        comments: [makeComment({ id: 5, reply_count: 2 })],
        total: 1,
        has_more: false,
      },
      isLoading: false,
    })

    render(<CommentThread {...defaultProps} />)

    expect(screen.getByTestId('show-replies-button')).toBeInTheDocument()
  })
})
