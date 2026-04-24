/**
 * CommentCard tests — focused on PSY-297 admin edit-history trigger gating.
 * (Full CommentCard interaction coverage lives in CommentThread.test.tsx and
 * in the E2E suite.)
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CommentCard } from './CommentCard'
import type { Comment } from '../types'

const mockAuthContext = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }

vi.mock('../hooks', () => ({
  useReplyToComment: () => defaultMutationReturn,
  useUpdateComment: () => defaultMutationReturn,
  useUpdateReplyPermission: () => defaultMutationReturn,
  useDeleteComment: () => defaultMutationReturn,
  useVoteComment: () => defaultMutationReturn,
  useUnvoteComment: () => defaultMutationReturn,
  useCommentThread: () => ({ data: undefined }),
}))

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
