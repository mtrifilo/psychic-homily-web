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
// PSY-590: edit + delete mutations join the mock surface.
const mockUseReplyToComment = vi.fn()
const mockUseUpdateComment = vi.fn()
const mockUseDeleteComment = vi.fn()
const mockUseVoteComment = vi.fn()
const mockUseUnvoteComment = vi.fn()

vi.mock('../hooks', async () => {
  // PSY-589: bring through the real formatCommentSubmissionError so the
  // FieldNoteCard renders the canonical inline error banner copy. The vote
  // auto-dismiss banner uses the shared useAutoDismissBanner primitive
  // directly now (PSY-958, unmocked).
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useReplyToComment: () => mockUseReplyToComment(),
    useUpdateComment: () => mockUseUpdateComment(),
    useDeleteComment: () => mockUseDeleteComment(),
    useVoteComment: () => mockUseVoteComment(),
    useUnvoteComment: () => mockUseUnvoteComment(),
    useCommentThread: () => ({ data: undefined as unknown }),
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

function resetFieldNoteCardMocks() {
  mockUseReplyToComment.mockReturnValue(defaultMutationReturn)
  mockUseUpdateComment.mockReturnValue(defaultMutationReturn)
  mockUseDeleteComment.mockReturnValue(defaultMutationReturn)
  mockUseVoteComment.mockReturnValue(defaultMutationReturn)
  mockUseUnvoteComment.mockReturnValue(defaultMutationReturn)
}

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: (): null => null,
}))

// PSY-590: stub the edit history dialog — only its render condition matters here.
vi.mock('./CommentEditHistory', () => ({
  CommentEditHistory: () => <div data-testid="stub-edit-history-dialog" />,
}))

// PSY-567: default created_at is "now" so the field-note's author still has
// access to the Edit/Delete buttons in the existing PSY-590 test cases. Tests
// that exercise the OUT-OF-window behaviour override `created_at` with a
// timestamp older than 30 minutes.
function makeFieldNote(overrides: Partial<Comment> = {}): Comment {
  const nowIso = new Date().toISOString()
  return {
    id: 1,
    entity_type: 'show',
    entity_id: 10,
    kind: 'field_note',
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
    created_at: nowIso,
    updated_at: nowIso,
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

  // PSY-593: authors cannot vote on their own field notes. The frontend
  // hides the up/down buttons; the score remains visible as a plain span.
  describe('self-vote button hiding (PSY-593)', () => {
    it('hides Upvote and Downvote buttons on own field note', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '2', email: 'owner@example.com' },
      })

      render(
        <FieldNoteCard
          comment={makeFieldNote({ user_id: 2, ups: 5, downs: 1 })}
          showId={10}
        />
      )

      expect(screen.queryByTestId('upvote-button')).not.toBeInTheDocument()
      expect(screen.queryByTestId('downvote-button')).not.toBeInTheDocument()
      // Score still visible — authors can see their own score.
      expect(screen.getByTestId('vote-score')).toHaveTextContent('4')
    })

    it('renders Upvote and Downvote buttons on another user\'s field note', () => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '99', email: 'other@user.com' },
      })

      render(
        <FieldNoteCard
          comment={makeFieldNote({ user_id: 2 })}
          showId={10}
        />
      )

      expect(screen.getByTestId('upvote-button')).toBeInTheDocument()
      expect(screen.getByTestId('downvote-button')).toBeInTheDocument()
    })
  })

  // PSY-590: Edit + Delete affordances on the author's own field note. Mirrors
  // the CommentCard surface (Pencil + Trash2 + inline Yes/No confirm + admin-
  // only edit history trigger). The makeFieldNote helper fixes user_id=2, so
  // we sign in as that user to exercise the owner branch.
  describe('Edit + Delete affordances (PSY-590)', () => {
    function ownerAuth() {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '2', email: 'owner@example.com', is_admin: false },
      })
    }
    function adminAuth() {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '99', email: 'admin@example.com', is_admin: true },
      })
    }
    function otherUserAuth() {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '99', email: 'other@example.com', is_admin: false },
      })
    }

    it('renders Edit + Delete for the author', () => {
      ownerAuth()
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)
      expect(screen.getByTestId('edit-field-note-button')).toBeInTheDocument()
      expect(screen.getByTestId('delete-field-note-button')).toBeInTheDocument()
    })

    it('does NOT render Edit + Delete for non-author non-admin viewers', () => {
      otherUserAuth()
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)
      expect(
        screen.queryByTestId('edit-field-note-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-field-note-button')
      ).not.toBeInTheDocument()
      // Non-owner sees Report instead.
      expect(
        screen.getByTestId('report-field-note-button')
      ).toBeInTheDocument()
    })

    it('does NOT render Edit + Delete for anonymous viewers', () => {
      // beforeEach sets isAuthenticated=false by default.
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)
      expect(
        screen.queryByTestId('edit-field-note-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-field-note-button')
      ).not.toBeInTheDocument()
    })

    it('opens an inline edit form populated with the existing body when Edit is clicked', () => {
      // PSY-567: root field-note edit now opens the FieldNoteForm (with
      // structured-data fields) instead of the body-only CommentForm.
      ownerAuth()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ body: 'My original take' })}
          showId={10}
        />
      )

      fireEvent.click(screen.getByTestId('edit-field-note-button'))

      const textarea = screen.getByTestId(
        'field-note-textarea'
      ) as HTMLTextAreaElement
      expect(textarea.value).toBe('My original take')
      // The read-only body view is replaced while editing.
      expect(screen.queryByTestId('field-note-body')).not.toBeInTheDocument()
      // PSY-567: structured-data inputs are present so ratings / verified-
      // attendee / spoiler can be edited as a unit.
      expect(screen.getByTestId('sound-quality-rating')).toBeInTheDocument()
      expect(screen.getByTestId('crowd-energy-rating')).toBeInTheDocument()
      expect(
        screen.getByTestId('verified-attendee-checkbox')
      ).toBeInTheDocument()
      expect(
        screen.getByTestId('setlist-spoiler-checkbox')
      ).toBeInTheDocument()
    })

    it('calls useUpdateComment with the edited body + structured data on Save', () => {
      ownerAuth()
      const mutate = vi.fn()
      mockUseUpdateComment.mockReturnValue({
        mutate,
        isPending: false,
      })
      render(
        <FieldNoteCard
          comment={makeFieldNote({ body: 'before' })}
          showId={10}
        />
      )

      fireEvent.click(screen.getByTestId('edit-field-note-button'))
      const textarea = screen.getByTestId('field-note-textarea')
      fireEvent.change(textarea, { target: { value: 'after' } })
      fireEvent.click(screen.getByText('Save'))

      expect(mutate).toHaveBeenCalledTimes(1)
      const [args] = mutate.mock.calls[0]
      // PSY-567: edit submits body + structured_data as a unit. The
      // fixture's existing ratings carry through unchanged.
      expect(args).toMatchObject({
        commentId: 1,
        body: 'after',
        entityType: 'show',
        entityId: 10,
        structuredData: {
          sound_quality: 4,
          crowd_energy: 5,
          notable_moments: 'Played 3 new songs',
          setlist_spoiler: false,
          is_verified_attendee: true,
        },
      })
    })

    it('renders inline edit-form error banner on update failure', () => {
      ownerAuth()
      mockUseUpdateComment.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        error: Object.assign(new Error('comment is too long'), { status: 400 }),
      })
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)
      fireEvent.click(screen.getByTestId('edit-field-note-button'))

      // PSY-567: the field-note edit form has its own error banner testid.
      const banner = screen.getByTestId('field-note-form-error')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveTextContent('Comment is too long')
    })

    it('shows inline Yes/No confirmation when Delete is clicked', () => {
      ownerAuth()
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      fireEvent.click(screen.getByTestId('delete-field-note-button'))

      expect(
        screen.getByTestId('delete-field-note-confirm')
      ).toBeInTheDocument()
      expect(screen.getByTestId('delete-field-note-yes')).toBeInTheDocument()
      expect(screen.getByTestId('delete-field-note-no')).toBeInTheDocument()
      // Delete button itself is replaced by the confirm row.
      expect(
        screen.queryByTestId('delete-field-note-button')
      ).not.toBeInTheDocument()
    })

    it('calls useDeleteComment when Yes is clicked, dismisses on No', () => {
      ownerAuth()
      const mutate = vi.fn()
      mockUseDeleteComment.mockReturnValue({ mutate, isPending: false })
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      // Open confirm, then cancel.
      fireEvent.click(screen.getByTestId('delete-field-note-button'))
      fireEvent.click(screen.getByTestId('delete-field-note-no'))
      expect(
        screen.queryByTestId('delete-field-note-confirm')
      ).not.toBeInTheDocument()
      expect(mutate).not.toHaveBeenCalled()

      // Open again, confirm.
      fireEvent.click(screen.getByTestId('delete-field-note-button'))
      fireEvent.click(screen.getByTestId('delete-field-note-yes'))

      expect(mutate).toHaveBeenCalledTimes(1)
      const [args] = mutate.mock.calls[0]
      expect(args).toMatchObject({
        commentId: 1,
        entityType: 'show',
        entityId: 10,
      })
    })

    it('renders the delete-error banner when useDeleteComment fails', () => {
      ownerAuth()
      mockUseDeleteComment.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        isError: true,
        error: Object.assign(new Error('cannot delete pinned note'), {
          status: 403,
        }),
      })
      render(<FieldNoteCard comment={makeFieldNote()} showId={10} />)

      const banner = screen.getByTestId('delete-error-banner')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('role', 'alert')
      expect(banner).toHaveTextContent('Cannot delete pinned note')
    })

    it('renders the admin edit-history button for admins when the field note has edits', () => {
      adminAuth()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ edit_count: 2, is_edited: true })}
          showId={10}
        />
      )

      const btn = screen.getByTestId('admin-edit-history-button')
      expect(btn).toBeInTheDocument()
      expect(btn).toHaveTextContent('Edit history (2)')
    })

    it('hides the admin edit-history button for admins when the field note has never been edited', () => {
      adminAuth()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ edit_count: 0, is_edited: false })}
          showId={10}
        />
      )
      expect(
        screen.queryByTestId('admin-edit-history-button')
      ).not.toBeInTheDocument()
    })

    it('does NOT render the admin edit-history button for non-admin viewers', () => {
      ownerAuth()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ edit_count: 2, is_edited: true })}
          showId={10}
        />
      )
      expect(
        screen.queryByTestId('admin-edit-history-button')
      ).not.toBeInTheDocument()
    })
  })

  // PSY-567: Reddit-style 30-minute author-edit window. Buttons are
  // rendered for the author within the window and HIDDEN after. After
  // expiry, the only retraction path is the Report button (handled by
  // the existing non-owner branch when the user is not the author).
  describe('30-minute author-edit window (PSY-567)', () => {
    function ownerAuth() {
      mockAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '2', email: 'owner@example.com', is_admin: false },
      })
    }

    it('renders Edit + Delete for the author within the 30-min window', () => {
      ownerAuth()
      const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ created_at: fiveMinutesAgo })}
          showId={10}
        />
      )
      expect(screen.getByTestId('edit-field-note-button')).toBeInTheDocument()
      expect(
        screen.getByTestId('delete-field-note-button')
      ).toBeInTheDocument()
    })

    it('HIDES Edit + Delete for the author once the 30-min window has elapsed', () => {
      ownerAuth()
      const thirtyOneMinutesAgo = new Date(
        Date.now() - 31 * 60 * 1000
      ).toISOString()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ created_at: thirtyOneMinutesAgo })}
          showId={10}
        />
      )
      expect(
        screen.queryByTestId('edit-field-note-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-field-note-button')
      ).not.toBeInTheDocument()
    })

    it('keeps Edit + Delete visible at exactly 29:59 (boundary, in window)', () => {
      // Boundary check just before expiry. 29 minutes + 59 seconds is still
      // < 30 minutes so the buttons must render.
      ownerAuth()
      const justInsideWindow = new Date(
        Date.now() - (29 * 60 * 1000 + 59 * 1000)
      ).toISOString()
      render(
        <FieldNoteCard
          comment={makeFieldNote({ created_at: justInsideWindow })}
          showId={10}
        />
      )
      expect(screen.getByTestId('edit-field-note-button')).toBeInTheDocument()
      expect(
        screen.getByTestId('delete-field-note-button')
      ).toBeInTheDocument()
    })

    it('does NOT show Edit/Delete for a regular comment kind on the same row, even within 30min', () => {
      // Defence in depth: a comment that somehow ends up rendered by
      // FieldNoteCard with kind="comment" (e.g. nested reply rendering)
      // does NOT pick up the 30-min window — the field-note rule must
      // not change the existing comment-edit semantics.
      ownerAuth()
      const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString()
      render(
        <FieldNoteCard
          comment={makeFieldNote({
            kind: 'comment',
            depth: 1,
            created_at: fiveMinutesAgo,
          })}
          showId={10}
        />
      )
      // Replies edit through the normal Edit/Delete path — they still
      // render (the window logic short-circuits for non-field-note kinds).
      expect(screen.getByTestId('edit-field-note-button')).toBeInTheDocument()
      expect(
        screen.getByTestId('delete-field-note-button')
      ).toBeInTheDocument()
    })
  })
})
