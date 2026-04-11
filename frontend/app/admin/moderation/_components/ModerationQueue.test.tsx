import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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

// --- Mocks ---

const mockUseAdminPendingEdits = vi.fn()
const mockUseApprovePendingEdit = vi.fn()
const mockUseRejectPendingEdit = vi.fn()
const mockUseAdminEntityReports = vi.fn()
const mockUseResolveEntityReport = vi.fn()
const mockUseDismissEntityReport = vi.fn()
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
}))

vi.mock('@/lib/hooks/admin/useAdminComments', () => ({
  useAdminPendingComments: (...args: unknown[]) => mockUseAdminPendingComments(...args),
  useAdminApproveComment: () => mockUseAdminApproveComment(),
  useAdminRejectComment: () => mockUseAdminRejectComment(),
  useAdminHideComment: () => mockUseAdminHideComment(),
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
})
