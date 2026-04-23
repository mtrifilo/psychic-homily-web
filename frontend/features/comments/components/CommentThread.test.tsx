import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CommentThread } from './CommentThread'

// --- Mocks ---

const mockUseComments = vi.fn()
const mockUseCreateComment = vi.fn()
const mockUseAuthContext = vi.fn()

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }

vi.mock('../hooks', () => ({
  useComments: (...args: unknown[]) => mockUseComments(...args),
  useCreateComment: () => mockUseCreateComment(),
  useReplyToComment: () => defaultMutationReturn,
  useUpdateComment: () => defaultMutationReturn,
  useUpdateReplyPermission: () => defaultMutationReturn,
  useDeleteComment: () => defaultMutationReturn,
  useVoteComment: () => defaultMutationReturn,
  useUnvoteComment: () => defaultMutationReturn,
  useCommentThread: () => ({ data: undefined }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

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
})
