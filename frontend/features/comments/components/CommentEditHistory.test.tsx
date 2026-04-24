import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CommentEditHistory, EditHistoryBody } from './CommentEditHistory'
import type { CommentEditHistoryResponse } from '@/lib/hooks/admin/useAdminComments'

// ─── Mocks ───────────────────────────────────────────────────────────────────

const mockUseHistory = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminComments', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/lib/hooks/admin/useAdminComments')>()
  return {
    ...actual,
    useAdminCommentEditHistory: (...args: unknown[]) => mockUseHistory(...args),
  }
})

// The Radix Dialog needs a portal root in jsdom and a stable context; we
// short-circuit it to a plain div so we can render the body content directly.
vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="mock-dialog-root">{children}</div> : null,
  DialogContent: ({
    children,
    ...rest
  }: {
    children: React.ReactNode
    'data-testid'?: string
  }) => <div {...rest}>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => (
    <p>{children}</p>
  ),
}))

function makeHistory(
  overrides: Partial<CommentEditHistoryResponse> = {}
): CommentEditHistoryResponse {
  return {
    comment_id: 42,
    current_body: 'Version 3',
    edits: [
      {
        id: 1,
        comment_id: 42,
        old_body: 'Version 1',
        edited_at: '2026-04-20T10:00:00Z',
        editor_user_id: 7,
        editor_username: 'alice',
        editor_name: 'Alice',
      },
      {
        id: 2,
        comment_id: 42,
        old_body: 'Version 2',
        edited_at: '2026-04-21T11:00:00Z',
        editor_user_id: 7,
        editor_username: 'alice',
        editor_name: 'Alice',
      },
    ],
    ...overrides,
  }
}

describe('CommentEditHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fires the history query only when open', () => {
    mockUseHistory.mockReturnValue({ data: undefined, isLoading: false, error: null })

    render(
      <CommentEditHistory open={false} onOpenChange={() => {}} commentId={42} />
    )
    expect(mockUseHistory).toHaveBeenCalledWith(42, false)
  })

  it('passes enabled=true to the query when open', () => {
    mockUseHistory.mockReturnValue({ data: undefined, isLoading: false, error: null })

    render(
      <CommentEditHistory open={true} onOpenChange={() => {}} commentId={42} />
    )
    expect(mockUseHistory).toHaveBeenCalledWith(42, true)
  })

  it('shows a loading indicator while fetching', () => {
    mockUseHistory.mockReturnValue({ data: undefined, isLoading: true, error: null })

    render(
      <CommentEditHistory open={true} onOpenChange={() => {}} commentId={42} />
    )

    expect(screen.getByTestId('edit-history-loading')).toBeInTheDocument()
  })

  it('shows an error message when the query fails', () => {
    mockUseHistory.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Backend down'),
    })

    render(
      <CommentEditHistory open={true} onOpenChange={() => {}} commentId={42} />
    )

    expect(screen.getByTestId('edit-history-error')).toHaveTextContent(
      'Backend down'
    )
  })
})

describe('EditHistoryBody', () => {
  it('renders every edit row with its previous/next bodies', () => {
    render(<EditHistoryBody data={makeHistory()} />)

    // Current body is shown at the top.
    expect(screen.getByTestId('edit-history-current')).toHaveTextContent(
      'Version 3'
    )

    // Two transition rows, newest-first.
    const entries = screen.getAllByTestId('edit-history-entry')
    expect(entries).toHaveLength(2)

    // Newest transition (row 0): old=Version 2, new=Version 3 (= current body)
    const beforeBlocks = screen.getAllByTestId('edit-history-before')
    const afterBlocks = screen.getAllByTestId('edit-history-after')
    expect(beforeBlocks[0]).toHaveTextContent('Version 2')
    expect(afterBlocks[0]).toHaveTextContent('Version 3')

    // Older transition (row 1): old=Version 1, new=Version 2
    expect(beforeBlocks[1]).toHaveTextContent('Version 1')
    expect(afterBlocks[1]).toHaveTextContent('Version 2')
  })

  it('labels the editor by username when available', () => {
    render(<EditHistoryBody data={makeHistory()} />)

    const editors = screen.getAllByTestId('edit-history-editor')
    expect(editors[0]).toHaveTextContent('@alice')
    expect(editors[1]).toHaveTextContent('@alice')
  })

  it('falls back to name, then user id, when username is missing', () => {
    const data = makeHistory({
      edits: [
        {
          id: 1,
          comment_id: 42,
          old_body: 'earlier',
          edited_at: '2026-04-20T10:00:00Z',
          editor_user_id: 9,
          editor_name: 'Bob',
        },
        {
          id: 2,
          comment_id: 42,
          old_body: 'middle',
          edited_at: '2026-04-21T11:00:00Z',
          editor_user_id: 9,
        },
      ],
    })
    render(<EditHistoryBody data={data} />)

    const editors = screen.getAllByTestId('edit-history-editor')
    // The newest row (index 0) corresponds to edits[1] (no username or name)
    expect(editors[0]).toHaveTextContent('user #9')
    // The older row (index 1) corresponds to edits[0] (has a name)
    expect(editors[1]).toHaveTextContent('Bob')
  })

  it('shows an empty-state message when no edits exist', () => {
    render(
      <EditHistoryBody
        data={{ comment_id: 42, current_body: 'pristine', edits: [] }}
      />
    )

    expect(screen.getByTestId('edit-history-current')).toHaveTextContent(
      'pristine'
    )
    expect(screen.getByTestId('edit-history-empty')).toBeInTheDocument()
    expect(screen.queryByTestId('edit-history-entry')).not.toBeInTheDocument()
  })
})
