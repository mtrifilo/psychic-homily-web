/**
 * CommentVoteControls tests — focused on the PSY-593 owner-hide branch and
 * the PSY-608 auto-dismiss banner wiring that the primitive owns (PSY-632).
 * Consumer-level coverage (CommentCard.test.tsx, FieldNoteCard.test.tsx)
 * remains the integration check; this file is the unit-level guard.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CommentVoteControls } from './CommentVoteControls'
import type { Comment } from '../types'

const mockAuthContext = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }
const mockUseVoteComment = vi.fn()
const mockUseUnvoteComment = vi.fn()

vi.mock('../hooks', async () => {
  // Real useAutoDismissError + formatCommentSubmissionError so the banner
  // exercises canonical state + copy paths.
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useVoteComment: () => mockUseVoteComment(),
    useUnvoteComment: () => mockUseUnvoteComment(),
    useAutoDismissError: actual.useAutoDismissError,
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

function resetMutationMocks() {
  mockUseVoteComment.mockReturnValue(defaultMutationReturn)
  mockUseUnvoteComment.mockReturnValue(defaultMutationReturn)
}

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 1,
    entity_type: 'show',
    entity_id: 10,
    user_id: 99,
    author_name: 'Author',
    body: 'Body',
    body_html: '<p>Body</p>',
    parent_id: null,
    root_id: null,
    depth: 0,
    ups: 3,
    downs: 1,
    score: 2,
    visibility: 'visible',
    reply_permission: 'anyone',
    edit_count: 0,
    is_edited: false,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides,
  }
}

const defaultProps = {
  entityType: 'show',
  entityId: 10,
}

describe('CommentVoteControls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMutationMocks()
  })

  // PSY-593: authors cannot vote on their own rows. The frontend hides the
  // up/down buttons; the score remains visible as a plain span. This is the
  // load-bearing invariant the primitive was extracted to enforce uniformly
  // across CommentCard + FieldNoteCard.
  describe('self-vote button hiding (PSY-593)', () => {
    it('hides Upvote and Downvote buttons when the viewer is the author', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '99', email: 'me@me.com' },
      })

      render(
        <CommentVoteControls
          {...defaultProps}
          comment={makeComment({ user_id: 99, ups: 5, downs: 1 })}
        />
      )

      expect(screen.queryByTestId('upvote-button')).not.toBeInTheDocument()
      expect(screen.queryByTestId('downvote-button')).not.toBeInTheDocument()
      // Score still visible — authors can see their own score, muted.
      expect(screen.getByTestId('vote-score')).toHaveTextContent('4')
    })

    it('renders Upvote and Downvote buttons when the viewer is not the author', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'other@user.com' },
      })

      render(
        <CommentVoteControls
          {...defaultProps}
          comment={makeComment({ user_id: 99, ups: 5, downs: 1 })}
        />
      )

      expect(screen.getByTestId('upvote-button')).toBeInTheDocument()
      expect(screen.getByTestId('downvote-button')).toBeInTheDocument()
      expect(screen.getByTestId('vote-score')).toHaveTextContent('4')
    })

    it('renders vote buttons for anonymous viewers (no author match possible)', () => {
      // Guards against accidentally hiding the affordance when user.id is
      // absent — anonymous viewers can't be the author by definition, and
      // the buttons remain disabled via isAuthenticated.
      mockAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })

      render(
        <CommentVoteControls
          {...defaultProps}
          comment={makeComment({ user_id: 99 })}
        />
      )

      expect(screen.getByTestId('upvote-button')).toBeInTheDocument()
      expect(screen.getByTestId('downvote-button')).toBeInTheDocument()
      expect(screen.getByTestId('upvote-button')).toBeDisabled()
      expect(screen.getByTestId('downvote-button')).toBeDisabled()
    })
  })

  // PSY-608: optimistic vote/unvote rollback hides the failure visually.
  // The primitive surfaces a brief auto-dismiss banner via useAutoDismissError
  // so the user knows the action was reverted.
  describe('auto-dismiss vote-error banner (PSY-608)', () => {
    it('renders the banner when useVoteComment rejects via onError', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'other@user.com' },
      })
      const voteError = Object.assign(new Error('vote failed'), { status: 500 })
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onError?: (err: unknown) => void }) => {
          opts?.onError?.(voteError)
        }
      )
      mockUseVoteComment.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })

      render(
        <CommentVoteControls
          {...defaultProps}
          comment={makeComment({ user_id: 99 })}
        />
      )

      expect(screen.queryByTestId('vote-error-banner')).not.toBeInTheDocument()

      fireEvent.click(screen.getByLabelText('Upvote'))

      const banner = screen.getByTestId('vote-error-banner')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('role', 'alert')
      expect(banner).toHaveTextContent('Vote failed')
    })

    it('renders the banner when useUnvoteComment rejects (toggle-off path)', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'other@user.com' },
      })
      const voteError = Object.assign(new Error('rate limited'), {
        status: 429,
        retryAfter: 60,
      })
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onError?: (err: unknown) => void }) => {
          opts?.onError?.(voteError)
        }
      )
      mockUseUnvoteComment.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })

      // Already upvoted — clicking upvote toggles off (unvote path).
      render(
        <CommentVoteControls
          {...defaultProps}
          comment={makeComment({ user_id: 99, user_vote: 1 })}
        />
      )

      fireEvent.click(screen.getByLabelText('Upvote'))

      const banner = screen.getByTestId('vote-error-banner')
      expect(banner).toBeInTheDocument()
      // Reuses formatCommentSubmissionError → 429 countdown copy.
      expect(banner).toHaveTextContent('Please wait 60s before commenting again.')
    })
  })

  // The primitive owns the action-row container so consumers can pass their
  // remaining action buttons as children — confirms the slot keeps siblings
  // on the same flex line as the chevrons + score.
  it('renders children passed in as action-row siblings of the chevrons', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '7', email: 'other@user.com' },
    })

    render(
      <CommentVoteControls
        {...defaultProps}
        comment={makeComment({ user_id: 99 })}
      >
        <button data-testid="child-reply">Reply</button>
      </CommentVoteControls>
    )

    expect(screen.getByTestId('child-reply')).toBeInTheDocument()
    expect(screen.getByTestId('upvote-button')).toBeInTheDocument()
  })
})
