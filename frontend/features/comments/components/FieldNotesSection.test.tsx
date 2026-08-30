import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FieldNotesSection } from './FieldNotesSection'
import type { Comment } from '../types'

// --- Mocks ---

const mockUseFieldNotes = vi.fn()
const mockUseCreateFieldNote = vi.fn()
const mockUseAuthContext = vi.fn()

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
    useCommentThread: () => ({ data: undefined as unknown }),
    formatCommentSubmissionError: actual.formatCommentSubmissionError,
  }
})

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// PSY-1870: the logged-out prompt builds its returnTo from the current
// pathname, so the navigation hook needs a value outside a router context.
vi.mock('next/navigation', () => ({
  usePathname: () => '/shows/test-show',
}))

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: (): null => null,
}))

// PSY-590: stub the admin edit-history dialog so the section renders even when
// authenticated as admin (cards may try to lazy-mount the dialog).
vi.mock('./CommentEditHistory', () => ({
  CommentEditHistory: (): null => null,
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
        <FieldNotesSection
          showId={1}
          showDate={pastDate}
          lifecycle="past"
          artists={mockArtists}
        />
      )

      expect(screen.getByTestId('field-notes-section')).toBeInTheDocument()
      expect(screen.getByTestId('field-notes-empty')).toBeInTheDocument()
      expect(
        screen.getByText('No field notes yet. Were you there? Share what you saw.')
      ).toBeInTheDocument()
    })

    // The form's gate opens at the START INSTANT; the lifecycle turns past at
    // venue-local MIDNIGHT. For the whole evening in between, the page's
    // stripe says TONIGHT — so the empty state must not ask "were you there?"
    // about a band currently on stage.
    it('keeps the present tense while the show is in progress', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection
          showId={1}
          showDate={pastDate}
          lifecycle="today"
          artists={mockArtists}
        />
      )

      expect(screen.getByTestId('field-notes-empty')).toHaveTextContent(
        'Attend this show and share your experience!'
      )
    })

    // A cancelled show did not happen, so it can be neither attended nor
    // remembered. Both prompts are wrong; the bare sentence asks nothing.
    it('asks nothing about a cancelled show that has passed', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection
          showId={1}
          showDate={pastDate}
          lifecycle="past"
          isCancelled
          artists={mockArtists}
        />
      )

      const empty = screen.getByTestId('field-notes-empty')
      expect(empty).toHaveTextContent('No field notes yet.')
      expect(empty).not.toHaveTextContent('Were you there?')
      expect(empty).not.toHaveTextContent('Attend this show')
    })

    // `hasShowStarted` counts an undateable show as started and the lifecycle
    // counts it as past; neither is evidence the show actually happened, so
    // the archive prompt stays off.
    it('makes no archive claim for a show whose date cannot be read', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })

      render(
        <FieldNotesSection
          showId={1}
          showDate="not-a-date"
          lifecycle="past"
          artists={mockArtists}
        />
      )

      expect(screen.getByTestId('field-notes-empty')).not.toHaveTextContent(
        'Were you there?'
      )
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
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
      )

      expect(screen.getByTestId('field-note-auth-gate')).toBeInTheDocument()
      // PSY-1870: must point at the real auth route, not the dead `/login`,
      // and must come back to the notes rather than the top of the page.
      expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
        'href',
        '/auth?returnTo=%2Fshows%2Ftest-show%23field-notes'
      )
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
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
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
              },
            },
          ],
          total: 1,
          has_more: false,
        },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
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
              },
            },
          ],
          total: 1,
          has_more: false,
        },
        isLoading: false,
      })

      render(
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
      )

      expect(screen.getByTestId('field-note-card')).toBeInTheDocument()
      expect(screen.getByText('TestUser')).toBeInTheDocument()
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
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
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
        <FieldNotesSection showId={1} showDate={futureDate} lifecycle="upcoming" artists={mockArtists} />
      )

      expect(screen.getByTestId('future-show-message')).toBeInTheDocument()
      // The EXACT string. A looser matcher passed against the old copy too,
      // which named a calendar date in front of a gate that opens at the start
      // instant, so nothing stopped that regression from coming back.
      expect(
        screen.getByText('Field notes will be available after the show starts.')
      ).toBeInTheDocument()
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
        <FieldNotesSection showId={1} showDate={futureDate} lifecycle="upcoming" artists={mockArtists} />
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
        <FieldNotesSection showId={1} showDate={futureDate} lifecycle="upcoming" artists={mockArtists} />
      )

      expect(screen.queryByTestId('field-note-auth-gate')).not.toBeInTheDocument()
    })
  })

  // The gate mirrors the API's own `ErrFieldNoteShowFuture` rejection, which
  // fires on the START INSTANT. It deliberately does NOT use the venue-local
  // day boundary `isShowPast` draws: holding the form shut until local midnight
  // would hide a form the API would have accepted.
  describe('boundary (start instant, matching the API gate)', () => {
    const showDate = '2026-03-15T03:00:00Z' // 20:00 Mar 14 Phoenix

    beforeEach(() => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        user: { id: '1', email: 'test@test.com' },
      })
      mockUseFieldNotes.mockReturnValue({
        data: { comments: [], total: 0, has_more: false },
        isLoading: false,
      })
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('gates the form one minute before the show starts', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-15T02:59:00Z'))

      render(
        <FieldNotesSection showId={1} showDate={showDate} lifecycle="today" />
      )

      expect(screen.getByTestId('future-show-message')).toBeInTheDocument()
    })

    it('opens the form mid-show, hours before venue-local midnight', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-03-15T03:01:00Z')) // 20:01 Mar 14 Phoenix

      render(
        <FieldNotesSection showId={1} showDate={showDate} lifecycle="today" />
      )

      expect(screen.queryByTestId('future-show-message')).not.toBeInTheDocument()
      expect(screen.getByTestId('field-note-form')).toBeInTheDocument()
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
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
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
        <FieldNotesSection showId={1} showDate={pastDate} lifecycle="past" artists={mockArtists} />
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
          lifecycle="past"
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

})
