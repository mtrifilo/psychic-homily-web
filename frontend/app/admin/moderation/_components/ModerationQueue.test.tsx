import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { ModerationQueue } from './ModerationQueue'

// --- Mock data ---

const mockPendingEdit = {
  id: 1,
  entity_type: 'artist',
  entity_id: 10,
  entity_name: 'Test Artist',
  submitted_by: 2,
  submitter_name: 'editor1',
  field_changes: [{ field: 'name', old_value: 'Old', new_value: 'New' }],
  summary: 'Updated name',
  status: 'pending' as const,
  created_at: '2026-04-01T00:00:00Z',
  updated_at: '2026-04-01T00:00:00Z',
}

const mockEntityReport = {
  id: 2,
  entity_type: 'venue',
  entity_id: 20,
  entity_name: 'Test Venue',
  reported_by: 3,
  reporter_name: 'reporter1',
  report_type: 'wrong_address',
  details: 'Address is outdated',
  status: 'pending',
  created_at: '2026-04-02T00:00:00Z',
}

const mockPendingComment = {
  id: 3,
  entity_type: 'artist',
  entity_id: 10,
  entity_name: 'Test Artist',
  user_id: 4,
  author_name: 'commenter1',
  body: 'This is a pending comment body',
  body_html: '<p>This is a pending comment body</p>',
  parent_id: null,
  depth: 0,
  visibility: 'pending',
  trust_tier: 'new',
  created_at: '2026-04-03T00:00:00Z',
  updated_at: '2026-04-03T00:00:00Z',
}

const mockCommentReport = {
  id: 4,
  entity_type: 'comment',
  entity_id: 50,
  entity_name: 'Comment #50',
  reported_by: 5,
  reporter_name: 'reporter2',
  report_type: 'spam',
  details: 'This is spam content',
  status: 'pending',
  created_at: '2026-04-04T00:00:00Z',
}

// PSY-357: collection-typed report payload. Includes entity_slug because
// the moderation card uses it to deep-link to the public collection page
// and to call the admin hide endpoint.
const mockCollectionReport = {
  id: 5,
  entity_type: 'collection',
  entity_id: 60,
  entity_name: 'Test Collection',
  entity_slug: 'test-collection',
  reported_by: 6,
  reporter_name: 'reporter3',
  report_type: 'spam',
  details: 'This collection is spam',
  status: 'pending',
  created_at: '2026-04-05T00:00:00Z',
}

// --- Mocks ---

const mockUseAdminPendingEdits = vi.fn()
const mockUseApprovePendingEdit = vi.fn()
const mockUseRejectPendingEdit = vi.fn()
const mockUseAdminEntityReports = vi.fn()
const mockUseResolveEntityReport = vi.fn()
const mockUseDismissEntityReport = vi.fn()
const mockUseAdminHideCollection = vi.fn()
const mockUseAdminPendingComments = vi.fn()
const mockUseAdminApproveComment = vi.fn()
const mockUseAdminRejectComment = vi.fn()
const mockUseAdminHideComment = vi.fn()

const defaultMutationReturn = { mutate: vi.fn(), isPending: false, isError: false, error: null }

vi.mock('@/lib/hooks/admin/useAdminPendingEdits', () => ({
  useAdminPendingEdits: (...args: unknown[]) => mockUseAdminPendingEdits(...args),
  useApprovePendingEdit: () => mockUseApprovePendingEdit(),
  useRejectPendingEdit: () => mockUseRejectPendingEdit(),
}))

vi.mock('@/lib/hooks/admin/useAdminEntityReports', () => ({
  useAdminEntityReports: (...args: unknown[]) => mockUseAdminEntityReports(...args),
  useResolveEntityReport: () => mockUseResolveEntityReport(),
  useDismissEntityReport: () => mockUseDismissEntityReport(),
  // PSY-357: hide-collection mutation, only invoked from CollectionReportCard.
  useAdminHideCollection: () => mockUseAdminHideCollection(),
}))

vi.mock('@/lib/hooks/admin/useAdminComments', () => ({
  useAdminPendingComments: (...args: unknown[]) => mockUseAdminPendingComments(...args),
  useAdminApproveComment: () => mockUseAdminApproveComment(),
  useAdminRejectComment: () => mockUseAdminRejectComment(),
  useAdminHideComment: () => mockUseAdminHideComment(),
  useAdminCommentEditHistory: () => ({ data: undefined, isLoading: false, error: null }),
}))

// PSY-297: stub the edit-history dialog so the badge interaction test doesn't
// depend on Radix Dialog or the query client.
vi.mock('@/features/comments', () => ({
  CommentEditHistory: ({ open }: { open: boolean }) =>
    open ? <div data-testid="stub-edit-history-dialog" /> : null,
}))

describe('ModerationQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseApprovePendingEdit.mockReturnValue(defaultMutationReturn)
    mockUseRejectPendingEdit.mockReturnValue(defaultMutationReturn)
    mockUseResolveEntityReport.mockReturnValue(defaultMutationReturn)
    mockUseDismissEntityReport.mockReturnValue(defaultMutationReturn)
    mockUseAdminApproveComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminRejectComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminHideComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminHideCollection.mockReturnValue(defaultMutationReturn)
  })

  function setDefaultMocks(overrides?: {
    edits?: unknown[]
    reports?: unknown[]
    comments?: unknown[]
  }) {
    mockUseAdminPendingEdits.mockReturnValue({
      data: { edits: overrides?.edits ?? [], total: overrides?.edits?.length ?? 0 },
      isLoading: false,
      error: null,
    })
    mockUseAdminEntityReports.mockReturnValue({
      data: { reports: overrides?.reports ?? [], total: overrides?.reports?.length ?? 0 },
      isLoading: false,
      error: null,
    })
    mockUseAdminPendingComments.mockReturnValue({
      data: { comments: overrides?.comments ?? [], total: overrides?.comments?.length ?? 0 },
      isLoading: false,
      error: null,
    })
  }

  it('renders empty state when no items', () => {
    setDefaultMocks()

    render(<ModerationQueue />)

    expect(screen.getByText('Queue Clear')).toBeInTheDocument()
  })

  it('renders pending edit card', () => {
    setDefaultMocks({ edits: [mockPendingEdit] })

    render(<ModerationQueue />)

    expect(screen.getByText('Edit')).toBeInTheDocument()
    expect(screen.getByText('Artist')).toBeInTheDocument()
    expect(screen.getByText('Test Artist')).toBeInTheDocument()
  })

  it('renders entity report card', () => {
    setDefaultMocks({ reports: [mockEntityReport] })

    render(<ModerationQueue />)

    expect(screen.getByText('Report')).toBeInTheDocument()
    expect(screen.getByText('Venue')).toBeInTheDocument()
    expect(screen.getByText('Test Venue')).toBeInTheDocument()
  })

  it('renders pending comment card', () => {
    setDefaultMocks({ comments: [mockPendingComment] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('pending-comment-card')).toBeInTheDocument()
    expect(screen.getByText('Comment')).toBeInTheDocument()
    expect(screen.getByText('by commenter1')).toBeInTheDocument()
    expect(screen.getByTestId('comment-body')).toBeInTheDocument()
  })

  it('renders comment report card for comment-type reports', () => {
    setDefaultMocks({ reports: [mockCommentReport] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('comment-report-card')).toBeInTheDocument()
    expect(screen.getByText('Spam')).toBeInTheDocument()
    expect(screen.getByText('Hide Comment')).toBeInTheDocument()
    expect(screen.getByText('Dismiss Report')).toBeInTheDocument()
  })

  // PSY-357: collection reports get a dedicated card with a "Hide from
  // Public Browse" action that flips is_public=false. The slug is required
  // to render the link and to enable the Hide button.
  it('renders collection report card for collection-type reports', () => {
    setDefaultMocks({ reports: [mockCollectionReport] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('collection-report-card')).toBeInTheDocument()
    expect(screen.getByText('Test Collection')).toBeInTheDocument()
    expect(screen.getByText('Hide from Public Browse')).toBeInTheDocument()
    expect(screen.getByText('Dismiss Report')).toBeInTheDocument()
  })

  it('disables Hide on collection report when slug is missing (deleted)', () => {
    setDefaultMocks({
      reports: [{ ...mockCollectionReport, entity_slug: undefined }],
    })

    render(<ModerationQueue />)

    const hideButton = screen.getByText('Hide from Public Browse').closest('button')
    expect(hideButton).toBeDisabled()
    // Dismiss is still available so admins can clear stale reports.
    const dismissButton = screen.getByText('Dismiss Report').closest('button')
    expect(dismissButton).not.toBeDisabled()
  })

  it('shows correct counts in filter buttons', () => {
    setDefaultMocks({
      edits: [mockPendingEdit],
      reports: [mockEntityReport, mockCommentReport],
      comments: [mockPendingComment],
    })

    render(<ModerationQueue />)

    // Total: 1 edit + 2 reports + 1 comment = 4
    expect(screen.getByText('4')).toBeInTheDocument() // All count
    expect(screen.getByText('2')).toBeInTheDocument() // Reports count
    // Edits (1) and Comments (1) have the same count, so use getAllByText
    const onesElems = screen.getAllByText('1')
    expect(onesElems.length).toBeGreaterThanOrEqual(2) // Edits + Comments
  })

  it('shows pending comment trust tier badge', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, trust_tier: 'trusted' }],
    })

    render(<ModerationQueue />)

    expect(screen.getByText('trusted')).toBeInTheDocument()
  })

  it('renders approve and reject buttons on pending comment card', () => {
    setDefaultMocks({ comments: [mockPendingComment] })

    render(<ModerationQueue />)

    const card = screen.getByTestId('pending-comment-card')
    expect(within(card).getByText('Approve')).toBeInTheDocument()
    expect(within(card).getByText('Reject')).toBeInTheDocument()
  })

  it('displays all item types in unified view', () => {
    setDefaultMocks({
      edits: [mockPendingEdit],
      reports: [mockEntityReport],
      comments: [mockPendingComment],
    })

    render(<ModerationQueue />)

    // Should show items from all three types
    expect(screen.getByText('Edit')).toBeInTheDocument()
    expect(screen.getByText('Report')).toBeInTheDocument()
    expect(screen.getByTestId('pending-comment-card')).toBeInTheDocument()
    expect(screen.getByText('3 items pending review')).toBeInTheDocument()
  })

  // ─── PSY-297: edit history badge on pending comment cards ─────────────────

  it('does NOT render the edit-count badge when the pending comment has no edits', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 0 }],
    })

    render(<ModerationQueue />)

    expect(
      screen.queryByTestId('pending-comment-edit-badge')
    ).not.toBeInTheDocument()
  })

  it('renders the edit-count badge with a pluralized label when the pending comment was edited', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 3 }],
    })

    render(<ModerationQueue />)

    const badge = screen.getByTestId('pending-comment-edit-badge')
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent('3 edits')
  })

  it('uses the singular form when the pending comment was edited exactly once', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 1 }],
    })

    render(<ModerationQueue />)

    expect(screen.getByTestId('pending-comment-edit-badge')).toHaveTextContent(
      '1 edit'
    )
  })

  // ─── PSY-603: page-level success banner on Approve / Reject ───────────────
  //
  // The banner state lives on ModerationQueue (not the card) because the card
  // unmounts on success when the pending-edits query invalidates. These tests
  // drive the success path by overriding the approve/reject mutation mocks to
  // immediately invoke the per-call onSuccess option.

  describe('Approve / Reject success banner (PSY-603)', () => {
    function captureMutationOnSuccess() {
      // Approve takes (editId, options); reject takes (variables, options).
      // Both pass options as the SECOND argument, so the same shape works.
      const approveMutate = vi.fn(
        (_args: unknown, opts?: { onSuccess?: () => void }) => {
          opts?.onSuccess?.()
        }
      )
      const rejectMutate = vi.fn(
        (_args: unknown, opts?: { onSuccess?: () => void }) => {
          opts?.onSuccess?.()
        }
      )
      mockUseApprovePendingEdit.mockReturnValue({
        ...defaultMutationReturn,
        mutate: approveMutate,
      })
      mockUseRejectPendingEdit.mockReturnValue({
        ...defaultMutationReturn,
        mutate: rejectMutate,
      })
      return { approveMutate, rejectMutate }
    }

    it('does NOT render the banner before any action is taken', () => {
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)

      expect(
        screen.queryByTestId('moderation-success-banner')
      ).not.toBeInTheDocument()
    })

    it('renders the success banner with entity name after Approve succeeds', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Approve'))

      const banner = screen.getByTestId('moderation-success-banner')
      expect(banner).toHaveTextContent('Approved')
      expect(banner).toHaveTextContent('Test Artist')
    })

    it('renders the success banner with submitter-notified copy after Reject succeeds', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      // Open the rejection-reason input, fill it, confirm.
      fireEvent.click(screen.getByText('Reject'))
      const textarea = screen.getByPlaceholderText(/Rejection reason/i)
      fireEvent.change(textarea, { target: { value: 'Inaccurate change' } })
      fireEvent.click(screen.getByText('Confirm Reject'))

      const banner = screen.getByTestId('moderation-success-banner')
      expect(banner).toHaveTextContent('Rejected')
      expect(banner).toHaveTextContent(/submitter notified/i)
    })

    it('auto-dismisses the banner after the timeout elapses', () => {
      vi.useFakeTimers()
      try {
        captureMutationOnSuccess()
        setDefaultMocks({ edits: [mockPendingEdit] })

        render(<ModerationQueue />)
        fireEvent.click(screen.getByText('Approve'))
        expect(
          screen.getByTestId('moderation-success-banner')
        ).toBeInTheDocument()

        // Advance just past the 5s timeout.
        act(() => {
          vi.advanceTimersByTime(5001)
        })

        expect(
          screen.queryByTestId('moderation-success-banner')
        ).not.toBeInTheDocument()
      } finally {
        vi.useRealTimers()
      }
    })

    it('clears the banner immediately when the admin switches filter tabs', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Approve'))
      expect(
        screen.getByTestId('moderation-success-banner')
      ).toBeInTheDocument()

      // Click the "Reports" filter tab.
      fireEvent.click(screen.getByText('Reports'))

      expect(
        screen.queryByTestId('moderation-success-banner')
      ).not.toBeInTheDocument()
    })
  })
})
