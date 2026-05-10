import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FieldNotesSection } from './FieldNotesSection'
import type { Comment } from '../types'

// --- Mocks ---

const mockUseFieldNotes = vi.fn()
const mockUseCreateFieldNote = vi.fn()
const mockUseAuthContext = vi.fn()
// PSY-568: FieldNotesSection now reads attendance to pre-check the
// "I attended this show" checkbox. Default to "no Going RSVP" for tests
// that don't care; PSY-568-specific tests override per case.
type MockAttendance = {
  show_id?: number
  going_count?: number
  interested_count?: number
  user_status?: string
}
const mockUseShowAttendance = vi.fn(
  (_showId: number) => ({ data: undefined as MockAttendance | undefined })
)

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }

vi.mock('../hooks', async () => {
  // PSY-608: bring through the real formatCommentSubmissionError so the
  // FieldNotesSection test can assert on the exact 4xx banner copy.
  // PSY-590: FieldNoteCard now consumes useUpdateComment + useDeleteComment;
  // stub them with the same neutral mutation return so the cards render.
  const actual = await vi.importActual<typeof import('../hooks')>('../hooks')
  return {
    useFieldNotes: (...args: unknown[]) => mockUseFieldNotes(...args),
    useCreateFieldNote: () => mockUseCreateFieldNote(),
    useReplyToComment: () => defaultMutationReturn,
    useUpdateComment: () => defaultMutationReturn,
    useDeleteComment: () => defaultMutationReturn,
    useVoteComment: () => defaultMutationReturn,
    useUnvoteComment: () => defaultMutationReturn,
    useCommentThread: () => ({ data: undefined }),
    useAutoDismissError: actual.useAutoDismissError,
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// PSY-568: stub the shows hooks barrel — only useShowAttendance is consumed.
vi.mock('@/features/shows/hooks', () => ({
  useShowAttendance: (showId: number) => mockUseShowAttendance(showId),
}))

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: () => null,
}))

// PSY-590: stub the admin edit-history dialog so the section renders even when
// authenticated as admin (cards may try to lazy-mount the dialog).
vi.mock('./CommentEditHistory', () => ({
  CommentEditHistory: () => null,
}))

const pastDate = '2025-01-15T20:00:00Z'
const futureDate = '2099-12-31T20:00:00Z'

const mockArtists = [
  { id: 1, name: 'Band A' },
  { id: 2, name: 'Band B' },
]

describe('FieldNotesSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateFieldNote.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })
    // PSY-568: default to no attendance data (user not Going). Tests that
    // exercise the pre-checked checkbox override this per case.
    mockUseShowAttendance.mockReturnValue({ data: undefined })
  })

  describe('past show (field notes available)', () => {
    it('renders empty state when no field notes', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('field-notes-section')).toBeInTheDocument()
      expect(screen.getByTestId('field-notes-empty')).toBeInTheDocument()
      expect(
        screen.getByText('No field notes yet. Attend this show and share your experience!')
      ).toBeInTheDocument()
    })

    it('renders auth gate for unauthenticated users', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('field-note-auth-gate')).toBeInTheDocument()
      expect(screen.getByText('Sign in')).toBeInTheDocument()
    })

    it('renders form for authenticated users', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'test@test.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('field-note-form')).toBeInTheDocument()
      expect(screen.getByTestId('field-note-textarea')).toBeInTheDocument()
    })

    it('renders field note count in heading', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: {
          comments: [
            {
              id: 1,
              entity_type: 'show',
              entity_id: 1,
              user_id: 2,
              author_name: 'TestUser',
              body: 'Great show!',
              body_html: '<p>Great show!</p>',
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
              structured_data: {
                setlist_spoiler: false,
                is_verified_attendee: false,
              },
            },
          ],
          total: 1,
          has_more: false,
        },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByText('Field Notes')).toBeInTheDocument()
      expect(screen.getByText('(1)')).toBeInTheDocument()
    })

    it('renders field note cards', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: {
          comments: [
            {
              id: 1,
              entity_type: 'show',
              entity_id: 1,
              user_id: 2,
              author_name: 'TestUser',
              body: 'Great show!',
              body_html: '<p>Great show!</p>',
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
              structured_data: {
                sound_quality: 4,
                crowd_energy: 5,
                setlist_spoiler: false,
                is_verified_attendee: true,
              },
            },
          ],
          total: 1,
          has_more: false,
        },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('field-note-card')).toBeInTheDocument()
      expect(screen.getByText('TestUser')).toBeInTheDocument()
      expect(screen.getByTestId('verified-badge')).toBeInTheDocument()
    })

    it('renders loading skeleton', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: undefined,
        isLoading: true,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('field-notes-section')).toBeInTheDocument()
      expect(screen.queryByTestId('field-notes-empty')).not.toBeInTheDocument()
    })
  })

  describe('future show (field notes not available)', () => {
    it('shows future date message', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'test@test.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={futureDate} artists={mockArtists} />
      )

      expect(screen.getByTestId('future-show-message')).toBeInTheDocument()
      expect(screen.getByText(/Field notes will be available after/)).toBeInTheDocument()
    })

    it('does not show form for future show', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'test@test.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={futureDate} artists={mockArtists} />
      )

      expect(screen.queryByTestId('field-note-form')).not.toBeInTheDocument()
      expect(screen.queryByTestId('field-notes-empty')).not.toBeInTheDocument()
    })

    it('does not show auth gate for future show', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={futureDate} artists={mockArtists} />
      )

      expect(screen.queryByTestId('field-note-auth-gate')).not.toBeInTheDocument()
    })
  })

  // PSY-513: pending-review feedback for field notes (mirrors CommentThread).
  describe('pending-review feedback (PSY-513)', () => {
    function makePendingNote(overrides: Partial<Comment> = {}): Comment {
      return {
        id: 7777,
        entity_type: 'show',
        entity_id: 1,
        user_id: 8,
        author_name: 'Newcomer',
        body: 'My take on the show',
        body_html: '<p>My take on the show</p>',
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
        structured_data: {
          setlist_spoiler: false,
          is_verified_attendee: false,
        },
        ...overrides,
      }
    }

    it('renders banner + optimistic note when POST returns pending_review', () => {
      const pending = makePendingNote()
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onSuccess?: (data: Comment) => void }) => {
          opts?.onSuccess?.(pending)
        }
      )
      mockUseCreateFieldNote.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '8', email: 'newcomer@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      // Empty state visible before submit.
      expect(screen.getByTestId('field-notes-empty')).toBeInTheDocument()

      // Submit the form.
      fireEvent.change(screen.getByTestId('field-note-textarea'), {
        target: { value: 'My take on the show' },
      })
      fireEvent.click(screen.getByTestId('field-note-submit'))

      // Banner appears, empty-state suppressed, badge rendered.
      expect(screen.getByTestId('pending-review-banner')).toBeInTheDocument()
      expect(screen.queryByTestId('field-notes-empty')).not.toBeInTheDocument()
      expect(screen.getByTestId('pending-review-badge')).toBeInTheDocument()
    })

    it('does NOT render banner when POST returns visible (trusted-tier auto-publish)', () => {
      const visible = makePendingNote({ visibility: 'visible' })
      const mutateImpl = vi.fn(
        (_args: unknown, opts?: { onSuccess?: (data: Comment) => void }) => {
          opts?.onSuccess?.(visible)
        }
      )
      mockUseCreateFieldNote.mockReturnValue({
        mutate: mutateImpl,
        isPending: false,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '8', email: 'trusted@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      fireEvent.change(screen.getByTestId('field-note-textarea'), {
        target: { value: 'Auto-published note' },
      })
      fireEvent.click(screen.getByTestId('field-note-submit'))

      expect(screen.queryByTestId('pending-review-banner')).not.toBeInTheDocument()
      expect(screen.queryByTestId('pending-review-badge')).not.toBeInTheDocument()
    })
  })

  // PSY-608: createFieldNote 4xx must surface inline (was silent — same
  // failure mode as PSY-589 on createComment).
  describe('mutation error surfacing (PSY-608)', () => {
    it('renders inline 429 banner with countdown copy when create mutation rate-limits', () => {
      const err = Object.assign(
        new Error('please wait 60 seconds between comments on the same entity'),
        { status: 429, retryAfter: 60 }
      )
      mockUseCreateFieldNote.mockReturnValue({
        mutate: vi.fn(),
        isPending: false,
        error: err,
      })
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '8', email: 'rate@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection
          showId={1}
          showDate={pastDate}
          artists={mockArtists}
        />
      )

      const banner = screen.getByTestId('field-note-form-error')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('role', 'alert')
      expect(banner).toHaveTextContent(
        'Please wait 60s before commenting again.'
      )
    })
  })

  // PSY-568: self-claim verified-attendee. Default state of the checkbox
  // mirrors the user's current Going RSVP, but the checkbox value is
  // authoritative (snapshot at post time).
  describe('verified-attendee default state (PSY-568)', () => {
    it('checkbox is unchecked by default when user has no Going RSVP', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'u@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })
      // No attendance data — user is not Going.
      mockUseShowAttendance.mockReturnValue({ data: undefined })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toHaveAttribute('aria-checked', 'false')
    })

    it('checkbox is unchecked when user is only Interested (not Going)', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'u@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })
      mockUseShowAttendance.mockReturnValue({
        data: {
          show_id: 1,
          going_count: 0,
          interested_count: 1,
          user_status: 'interested',
        },
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toHaveAttribute('aria-checked', 'false')
    })

    it('checkbox is pre-checked when user has Going set on this show', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'u@example.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })
      mockUseShowAttendance.mockReturnValue({
        data: {
          show_id: 1,
          going_count: 1,
          interested_count: 0,
          user_status: 'going',
        },
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} artists={mockArtists} />
      )

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toHaveAttribute('aria-checked', 'true')
    })
  })
})
