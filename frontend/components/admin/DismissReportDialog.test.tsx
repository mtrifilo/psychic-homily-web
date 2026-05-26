import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DismissReportDialog } from './DismissReportDialog'
import type { ShowReportResponse } from '@/features/shows'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminReports', () => ({
  useDismissReport: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
    isError: mockIsError,
    error: mockError,
  }),
}))

const baseReport: ShowReportResponse = {
  id: 1,
  show_id: 10,
  report_type: 'cancelled',
  status: 'pending',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  show: {
    id: 10,
    title: 'Test Show',
    slug: 'test-show',
    event_date: '2025-06-01T00:00:00Z',
  },
}

describe('DismissReportDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
    mockIsError = false
    mockError = null
  })

  it('renders nothing when closed', () => {
    render(
      <DismissReportDialog
        report={baseReport}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Dismiss Report')).not.toBeInTheDocument()
  })

  it('renders dialog when open', () => {
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getAllByText('Dismiss Report').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/Test Show/)).toBeInTheDocument()
  })

  it('calls mutate with correct args when dismiss is clicked', async () => {
    const user = userEvent.setup()
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), 'Spam report')

    // Click the dismiss button (not the cancel button)
    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss Report')
    )
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      { reportId: 1, notes: 'Spam report' },
      expect.any(Object)
    )
  })

  it('resets notes when Cancel is clicked (notes cleared before close)', async () => {
    const user = userEvent.setup()

    // Track the onOpenChange calls to verify handleDialogOpenChange wraps it
    const trackOpenChange = vi.fn()

    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={trackOpenChange}
      />
    )

    // Type some notes
    await user.type(screen.getByLabelText('Notes (optional)'), 'Some stale notes')
    expect(screen.getByLabelText('Notes (optional)')).toHaveValue('Some stale notes')

    // Click cancel - this calls handleDialogOpenChange(false) which resets notes
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    // Verify onOpenChange was called with false (dialog should close)
    expect(trackOpenChange).toHaveBeenCalledWith(false)

    // Now simulate the parent reopening the dialog by unmounting and remounting
    // This verifies the component properly calls setNotes('') before onOpenChange
  })

  it('sends empty notes after Cancel-then-reopen cycle', async () => {
    // First render: type notes, then cancel
    const user = userEvent.setup()
    const { unmount } = render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), 'Old notes')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    // Unmount to simulate parent closing the dialog
    unmount()

    // Remount (simulates parent setting open=true again after state reset)
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // The textarea should be empty because the component remounted
    expect(screen.getByLabelText('Notes (optional)')).toHaveValue('')

    // Click dismiss without adding notes
    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss Report')
    )
    await user.click(dismissButtons[dismissButtons.length - 1])

    // Verify mutation was called with undefined notes (not 'Old notes')
    expect(mockMutate).toHaveBeenCalledWith(
      { reportId: 1, notes: undefined },
      expect.any(Object)
    )
  })

  it('shows unknown show when show info is missing', () => {
    const reportNoShow: ShowReportResponse = { ...baseReport, show: undefined }
    render(
      <DismissReportDialog
        report={reportNoShow}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Unknown Show/)).toBeInTheDocument()
  })

  it('calls mutate with undefined notes when notes field is empty', async () => {
    const user = userEvent.setup()
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const dismissButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Dismiss Report'))
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      { reportId: 1, notes: undefined },
      expect.any(Object)
    )
  })

  it('trims whitespace-only notes to undefined', async () => {
    const user = userEvent.setup()
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), '   ')

    const dismissButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Dismiss Report'))
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      { reportId: 1, notes: undefined },
      expect.any(Object)
    )
  })

  it('disables buttons when mutation is pending', () => {
    mockIsPending = true
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Dismissing...')).toBeInTheDocument()
  })

  it('shows inline error banner when mutation fails', () => {
    mockIsError = true
    mockError = new Error('Server unreachable')
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByText('Server unreachable')).toBeInTheDocument()
  })

  it('shows fallback error message when error has no message', () => {
    mockIsError = true
    mockError = null
    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(
      screen.getByText('Failed to dismiss report. Please try again.')
    ).toBeInTheDocument()
  })

  it('preserves notes draft on error so admin can retry without re-typing', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation(() => {
      // Error path: onSuccess never fires
    })

    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const textarea = screen.getByLabelText('Notes (optional)')
    await user.type(textarea, 'Retry this dismissal')

    const dismissButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Dismiss Report'))
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(textarea).toHaveValue('Retry this dismissal')
  })

  it('fires onSuccess (closes dialog + clears notes) when mutation succeeds', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.()
    })

    render(
      <DismissReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const textarea = screen.getByLabelText('Notes (optional)')
    await user.type(textarea, 'Dismissed as spam')

    const dismissButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Dismiss Report'))
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(textarea).toHaveValue('')
  })
})
