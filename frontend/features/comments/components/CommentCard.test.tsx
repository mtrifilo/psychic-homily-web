/**
 * CommentCard tests — focused on PSY-297 admin edit-history trigger gating.
 * (Full CommentCard interaction coverage lives in CommentThread.test.tsx and
 * in the E2E suite.)
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CommentCard } from './CommentCard'
import type { Comment } from '../types'

const mockAuthContext = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }
const mockUseReplyToComment = vi.fn()

vi.mock('../hooks', async () => {
  // Bring through formatCommentSubmissionError (PSY-589) so the form
  // renders the same banner copy in tests as it does in the real card.
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useReplyToComment: () => mockUseReplyToComment(),
    useUpdateComment: () => defaultMutationReturn,
    useUpdateReplyPermission: () => defaultMutationReturn,
    useDeleteComment: () => defaultMutationReturn,
    useVoteComment: () => defaultMutationReturn,
    useUnvoteComment: () => defaultMutationReturn,
    useCommentThread: () => ({ data: undefined }),
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: () => null,
}))

// Stub the edit history dialog — we only care about its render condition.
vi.mock('./CommentEditHistory', () => ({
  CommentEditHistory: () => <div data-testid="stub-edit-history-dialog" />,
}))

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 1,
    entity_type: 'artist',
    entity_id: 10,
    user_id: 99,
    author_name: 'Author',
    body: 'Body',
    body_html: '<p>Body</p>',
    parent_id: null,
    root_id: null,
    depth: 0,
    ups: 0,
    downs: 0,
    score: 0,
    visibility: 'visible',
    reply_permission: 'anyone',
    edit_count: 2,
    is_edited: true,
    created_at: '2026-04-01T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
    ...overrides,
  }
}

describe('CommentCard — admin edit history trigger (PSY-297)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseReplyToComment.mockReturnValue(defaultMutationReturn)
  })

  const defaultProps = {
    entityType: 'artist',
    entityId: 10,
  }

  it('does NOT render the edit history button for anonymous viewers', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })

    render(<CommentCard {...defaultProps} comment={makeComment()} />)

    expect(
      screen.queryByTestId('admin-edit-history-button')
    ).not.toBeInTheDocument()
  })

  it('does NOT render the edit history button for non-admin users', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '5', email: 'a@a.com', is_admin: false },
    })

    render(<CommentCard {...defaultProps} comment={makeComment()} />)

    expect(
      screen.queryByTestId('admin-edit-history-button')
    ).not.toBeInTheDocument()
  })

  it('renders the edit history button for admins when the comment has edits', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '5', email: 'a@a.com', is_admin: true },
    })

    render(<CommentCard {...defaultProps} comment={makeComment()} />)

    const btn = screen.getByTestId('admin-edit-history-button')
    expect(btn).toBeInTheDocument()
    // Count is surfaced on the button label.
    expect(btn).toHaveTextContent('Edit history (2)')
  })

  it('hides the edit history button for admins when the comment has never been edited', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '5', email: 'a@a.com', is_admin: true },
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ edit_count: 0, is_edited: false })}
      />
    )

    expect(
      screen.queryByTestId('admin-edit-history-button')
    ).not.toBeInTheDocument()
  })
})

// PSY-513: pending-review badge — author-only visibility.
describe('CommentCard — pending review badge (PSY-513)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseReplyToComment.mockReturnValue(defaultMutationReturn)
  })

  const defaultProps = {
    entityType: 'artist',
    entityId: 10,
  }

  it('renders the pending-review badge for the comment author', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '99', email: 'me@me.com' },
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ visibility: 'pending_review', user_id: 99 })}
      />
    )

    expect(screen.getByTestId('pending-review-badge')).toBeInTheDocument()
    expect(screen.getByText('Pending review')).toBeInTheDocument()
  })

  it('does NOT render the pending-review badge for non-authors', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '7', email: 'other@user.com' },
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ visibility: 'pending_review', user_id: 99 })}
      />
    )

    expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
  })

  it('does NOT render the pending-review badge for anonymous viewers', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ visibility: 'pending_review', user_id: 99 })}
      />
    )

    expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
  })

  it('does NOT render the pending-review badge on a normal visible comment', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '99', email: 'me@me.com' },
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ visibility: 'visible', user_id: 99 })}
      />
    )

    expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
  })
})

// PSY-514: top-level comments with zero replies must NOT render a "Show
// replies" affordance. Previously the button rendered unconditionally on
// every top-level comment; clicking it removed the button without showing
// anything else (no replies to load) — read as a no-op, and was actively
// misleading on `author_only` comments where replies are impossible.
describe('CommentCard — Show replies button gating (PSY-514)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
  })

  const defaultProps = {
    entityType: 'artist',
    entityId: 10,
  }

  it('does NOT render "Show replies" when reply_count is 0', () => {
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ reply_count: 0 })}
      />
    )

    expect(
      screen.queryByTestId('show-replies-button')
    ).not.toBeInTheDocument()
  })

  it('does NOT render "Show replies" when reply_count is missing (undefined)', () => {
    // Older comment payloads (or paths that don't populate reply_count) are
    // treated as zero-reply for rendering purposes.
    render(<CommentCard {...defaultProps} comment={makeComment()} />)

    expect(
      screen.queryByTestId('show-replies-button')
    ).not.toBeInTheDocument()
  })

  it('does NOT render "Show replies" on author_only comments with zero replies', () => {
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({
          reply_permission: 'author_only',
          reply_count: 0,
        })}
      />
    )

    expect(
      screen.queryByTestId('show-replies-button')
    ).not.toBeInTheDocument()
  })

  it('renders "Show replies" when reply_count > 0', () => {
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ reply_count: 3 })}
      />
    )

    expect(screen.getByTestId('show-replies-button')).toBeInTheDocument()
  })

  it('does NOT render "Show replies" on a reply (depth > 0) even with reply_count > 0', () => {
    // Defense in depth: the button is only the expand-replies affordance
    // on top-level comments. Nested replies use the inline rendering path.
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ depth: 1, reply_count: 5 })}
      />
    )

    expect(
      screen.queryByTestId('show-replies-button')
    ).not.toBeInTheDocument()
  })
})

// PSY-552: linkable author byline. When author_username is set the byline
// renders as a Link to /users/:username; otherwise it falls back to plain
// text (matches the PSY-353 collection contributor pattern).
describe('CommentCard — author byline linkability (PSY-552)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
  })

  const defaultProps = {
    entityType: 'artist',
    entityId: 10,
  }

  it('links the byline to /users/:username when author_username is set', () => {
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({
          author_name: 'Jane Doe',
          author_username: 'janedoe',
        })}
      />
    )

    const link = screen.getByTestId('comment-author-link')
    expect(link).toHaveAttribute('href', '/users/janedoe')
    expect(link).toHaveTextContent('Jane Doe')
    expect(screen.queryByTestId('comment-author-name')).not.toBeInTheDocument()
  })

  it('renders the byline as plain text when author_username is null', () => {
    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({
          author_name: 'jane',
          author_username: null,
        })}
      />
    )

    expect(screen.getByTestId('comment-author-name')).toHaveTextContent('jane')
    expect(screen.queryByTestId('comment-author-link')).not.toBeInTheDocument()
  })

  it('renders the byline as plain text when author_username is missing', () => {
    // Older payloads or paths that haven't propagated the field yet.
    render(<CommentCard {...defaultProps} comment={makeComment()} />)

    expect(screen.getByTestId('comment-author-name')).toBeInTheDocument()
    expect(screen.queryByTestId('comment-author-link')).not.toBeInTheDocument()
  })
})

// PSY-589: reply form must surface 429 inline, not silently clear.
describe('CommentCard — reply rate-limit banner (PSY-589)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseReplyToComment.mockReturnValue(defaultMutationReturn)
  })

  const defaultProps = {
    entityType: 'artist',
    entityId: 10,
  }

  it('renders inline 429 banner with countdown copy when reply mutation rate-limits', () => {
    const err = Object.assign(
      new Error('please wait 60 seconds between comments on the same entity'),
      { status: 429, retryAfter: 60 }
    )
    mockUseReplyToComment.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      error: err,
    })
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '7', email: 'rate@example.com' },
    })

    render(
      <CommentCard
        {...defaultProps}
        comment={makeComment({ user_id: 99, edit_count: 0, is_edited: false })}
      />
    )

    // Open the reply form.
    fireEvent.click(screen.getByText('Reply'))

    const banner = screen.getByTestId('comment-form-error')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveTextContent('Please wait 60s before commenting again.')
  })
})
