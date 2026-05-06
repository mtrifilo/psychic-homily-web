import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FieldNoteCard } from './FieldNoteCard'
import type { Comment } from '../types'

// --- Mocks ---

const mockAuthContext = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }
// PSY-608: per-mutation overrides so we can assert reply + vote error UI.
const mockUseReplyToComment = vi.fn()
const mockUseVoteComment = vi.fn()
const mockUseUnvoteComment = vi.fn()

vi.mock('../hooks', async () => {
  // PSY-608: bring through the real formatCommentSubmissionError +
  // useAutoDismissError so the FieldNoteCard renders the canonical inline
  // error banner copy in tests.
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useReplyToComment: () => mockUseReplyToComment(),
    useVoteComment: () => mockUseVoteComment(),
    useUnvoteComment: () => mockUseUnvoteComment(),
    useCommentThread: () => ({ data: undefined }),
    useAutoDismissError: actual.useAutoDismissError,
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

function resetFieldNoteCardMocks() {
  mockUseReplyToComment.mockReturnValue(defaultMutationReturn)
  mockUseVoteComment.mockReturnValue(defaultMutationReturn)
  mockUseUnvoteComment.mockReturnValue(defaultMutationReturn)
}

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: () => null,
}))

function makeFieldNote(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 1,
    entity_type: 'show',
    entity_id: 10,
    user_id: 2,
    author_name: 'TestUser',
    body: 'Amazing show!',
    body_html: '<p>Amazing show!</p>',
    parent_id: null,
    root_id: null,
    depth: 0,
    ups: 5,
    downs: 1,
    score: 4,
    visibility: 'visible',
    reply_permission: 'anyone',
    edit_count: 0,
    is_edited: false,
    created_at: '2026-04-01T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
    structured_data: {
      sound_quality: 4,
      crowd_energy: 5,
      notable_moments: 'Played 3 new songs',
      setlist_spoiler: false,
      is_verified_attendee: true,
    },
    ...overrides,
  }
}

describe('FieldNoteCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetFieldNoteCardMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
    })
  })

  it('renders field note with body', () => {
    render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

    expect(screen.getByTestId('field-note-card')).toBeInTheDocument()
    expect(screen.getByTestId('field-note-body')).toBeInTheDocument()
    expect(screen.getByText('TestUser')).toBeInTheDocument()
  })

  it('shows verified attendee badge', () => {
    render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

    expect(screen.getByTestId('verified-badge')).toBeInTheDocument()
    expect(screen.getByText('Verified Attendee')).toBeInTheDocument()
  })

  it('does not show verified badge when not verified', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            setlist_spoiler: false,
            is_verified_attendee: false,
          },
        })}
        showId={10}
      />
    )

    expect(screen.queryByTestId('verified-badge')).not.toBeInTheDocument()
  })

  it('displays sound quality and crowd energy ratings', () => {
    render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

    expect(screen.getByTestId('ratings-display')).toBeInTheDocument()
    expect(screen.getByTestId('sound-quality-display')).toBeInTheDocument()
    expect(screen.getByTestId('crowd-energy-display')).toBeInTheDocument()
  })

  it('does not display ratings when not provided', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            setlist_spoiler: false,
            is_verified_attendee: false,
          },
        })}
        showId={10}
      />
    )

    expect(screen.queryByTestId('ratings-display')).not.toBeInTheDocument()
  })

  it('displays notable moments in highlighted box', () => {
    render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

    expect(screen.getByTestId('notable-moments')).toBeInTheDocument()
    expect(screen.getByText('Played 3 new songs')).toBeInTheDocument()
  })

  it('hides body behind spoiler gate when setlist_spoiler is true', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            setlist_spoiler: true,
            is_verified_attendee: false,
          },
        })}
        showId={10}
      />
    )

    expect(screen.getByTestId('spoiler-gate')).toBeInTheDocument()
    expect(screen.queryByTestId('field-note-body')).not.toBeInTheDocument()
    expect(
      screen.getByText('Contains setlist spoilers — click to reveal')
    ).toBeInTheDocument()
  })

  it('reveals body when spoiler gate is clicked', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            setlist_spoiler: true,
            is_verified_attendee: false,
          },
        })}
        showId={10}
      />
    )

    fireEvent.click(screen.getByText('Contains setlist spoilers — click to reveal'))

    expect(screen.queryByTestId('spoiler-gate')).not.toBeInTheDocument()
    expect(screen.getByTestId('field-note-body')).toBeInTheDocument()
  })

  it('displays artist attribution when show_artist_id matches', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            show_artist_id: 42,
            setlist_spoiler: false,
            is_verified_attendee: true,
          },
        })}
        showId={10}
        artists={[
          { id: 42, name: 'The Band' },
          { id: 43, name: 'Other Band' },
        ]}
      />
    )

    expect(screen.getByTestId('artist-attribution')).toBeInTheDocument()
    expect(screen.getByText(/During The Band/)).toBeInTheDocument()
  })

  it('displays song position when provided', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            song_position: 7,
            setlist_spoiler: false,
            is_verified_attendee: false,
          },
        })}
        showId={10}
      />
    )

    expect(screen.getByTestId('song-position')).toBeInTheDocument()
    expect(screen.getByText('Song #7')).toBeInTheDocument()
  })

  it('displays vote score', () => {
    render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

    expect(screen.getByTestId('vote-score')).toHaveTextContent('4')
  })

  it('shows deleted state', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({ visibility: 'hidden_by_user' })}
        showId={10}
      />
    )

    expect(screen.getByTestId('field-note-deleted')).toBeInTheDocument()
    expect(screen.getByText('[deleted]')).toBeInTheDocument()
  })

  it('shows removed state', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({ visibility: 'hidden_by_mod' })}
        showId={10}
      />
    )

    expect(screen.getByTestId('field-note-deleted')).toBeInTheDocument()
    expect(screen.getByText('[removed]')).toBeInTheDocument()
  })

  it('hides notable moments behind spoiler gate', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({
          structured_data: {
            setlist_spoiler: true,
            is_verified_attendee: false,
            notable_moments: 'Secret setlist info',
          },
        })}
        showId={10}
      />
    )

    expect(screen.queryByTestId('notable-moments')).not.toBeInTheDocument()
  })

  it('shows edited badge', () => {
    render(
      <FieldNoteCard
        comment={makeFieldNote({ is_edited: true })}
        showId={10}
      />
    )

    expect(screen.getByText('Edited')).toBeInTheDocument()
  })

  // PSY-552: linkable author byline — same shape as CommentCard.
  describe('author byline linkability (PSY-552)', () => {
    it('links the byline to /users/:username when author_username is set', () => {
      render(
        <FieldNoteCard
          comment={makeFieldNote({
            author_name: 'Jane Doe',
            author_username: 'janedoe',
          })}
          showId={10}
        />
      )

      const link = screen.getByTestId('field-note-author-link')
      expect(link).toHaveAttribute('href', '/users/janedoe')
      expect(link).toHaveTextContent('Jane Doe')
      expect(
        screen.queryByTestId('field-note-author-name')
      ).not.toBeInTheDocument()
    })

    it('renders plain text byline when author_username is null', () => {
      render(
        <FieldNoteCard
          comment={makeFieldNote({
            author_name: 'jane',
            author_username: null,
          })}
          showId={10}
        />
      )

      expect(screen.getByTestId('field-note-author-name')).toHaveTextContent(
        'jane'
      )
      expect(
        screen.queryByTestId('field-note-author-link')
      ).not.toBeInTheDocument()
    })
  })

  // PSY-514: same zero-reply gating that applies to CommentCard.
  describe('Show replies button gating (PSY-514)', () => {
    it('does NOT render "Show replies" when reply_count is 0', () => {
      render(
        <FieldNoteCard
          comment={makeFieldNote({ reply_count: 0 })}
          showId={10}
        />
      )

      expect(
        screen.queryByTestId('show-replies-button')
      ).not.toBeInTheDocument()
    })

    it('does NOT render "Show replies" when reply_count is missing', () => {
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      expect(
        screen.queryByTestId('show-replies-button')
      ).not.toBeInTheDocument()
    })

    it('renders "Show replies" when reply_count > 0', () => {
      render(
        <FieldNoteCard
          comment={makeFieldNote({ reply_count: 2 })}
          showId={10}
        />
      )

      expect(screen.getByTestId('show-replies-button')).toBeInTheDocument()
    })
  })

  // PSY-608: vote/unvote optimistic-rollback shows an auto-dismiss banner;
  // reply form shows a sticky banner via the shared CommentForm slot.
  describe('mutation error surfacing (PSY-608)', () => {
    function authedUser() {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '7', email: 'rate@example.com' },
      })
    }

    it('renders inline 429 banner with countdown copy when reply mutation rate-limits', () => {
      authedUser()
      const err = Object.assign(
        new Error('please wait 60 seconds between comments on the same entity'),
        { status: 429, retryAfter: 60 }
      )
      mockUseReplyToComment.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        error: err,
      })

      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      // Open the reply form.
      fireEvent.click(screen.getByText('Reply'))

      const banner = screen.getByTestId('comment-form-error')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveTextContent('Please wait 60s before commenting again.')
    })

    it('renders the auto-dismiss vote-error banner when useVoteComment rejects', () => {
      authedUser()
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

      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      expect(screen.queryByTestId('vote-error-banner')).not.toBeInTheDocument()

      fireEvent.click(screen.getByLabelText('Upvote'))

      const banner = screen.getByTestId('vote-error-banner')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('role', 'alert')
      expect(banner).toHaveTextContent('Vote failed')
    })

    it('renders the auto-dismiss vote-error banner when useUnvoteComment rejects', () => {
      authedUser()
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
        <FieldNoteCard
          comment={makeFieldNote({ user_vote: 1 })}
          showId={10}
        />
      )

      fireEvent.click(screen.getByLabelText('Upvote'))

      const banner = screen.getByTestId('vote-error-banner')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveTextContent(
        'Please wait 60s before commenting again.'
      )
    })
  })
})
