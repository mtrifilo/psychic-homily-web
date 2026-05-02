import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FieldNotesSection } from './FieldNotesSection'
import type { Comment } from '../types'

// --- Mocks ---

const mockUseFieldNotes = vi.fn()
const mockUseCreateFieldNote = vi.fn()
const mockUseAuthContext = vi.fn()

const defaultMutationReturn = { mutate: vi.fn(), isPending: false }

vi.mock('../hooks', () => ({
  useFieldNotes: (...args: unknown[]) => mockUseFieldNotes(...args),
  useCreateFieldNote: () => mockUseCreateFieldNote(),
  useReplyToComment: () => defaultMutationReturn,
  useVoteComment: () => defaultMutationReturn,
  useUnvoteComment: () => defaultMutationReturn,
  useCommentThread: () => ({ data: undefined }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

vi.mock('@/features/contributions', () => ({
  ReportEntityDialog: () => null,
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
})
