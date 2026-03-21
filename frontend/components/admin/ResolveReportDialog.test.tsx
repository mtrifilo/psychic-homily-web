import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ResolveReportDialog } from './ResolveReportDialog'
import type { ShowReportResponse } from '@/features/shows'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminReports', () => ({
  useResolveReport: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
    isError: mockIsError,
    error: mockError,
  }),
}))

function makeReport(overrides: Partial<ShowReportResponse> = {}): ShowReportResponse {
  return {
    id: 1,
    show_id: 10,
    report_type: 'cancelled',
    status: 'pending',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    show: {
      id: 10,
      title: 'Test Show',
      slug: 'test-show',
      event_date: '2026-04-15T20:00:00Z',
    },
    ...overrides,
  }
}

describe('ResolveReportDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
    mockIsError = false
    mockError = null
  })

  it('renders nothing when closed', () => {
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Resolve Report')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Resolve Report')).toBeInTheDocument()
    expect(
      screen.getByText(/Mark this report for "Test Show" as resolved/)
    ).toBeInTheDocument()
  })

  it('shows "Unknown Show" when show info is missing', () => {
    render(
      <ResolveReportDialog
        report={makeReport({ show: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Mark this report for "Unknown Show" as resolved/)
    ).toBeInTheDocument()
  })

  it('shows "Mark show as Cancelled" checkbox for cancelled reports', () => {
    render(
      <ResolveReportDialog
        report={makeReport({ report_type: 'cancelled' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Mark show as Cancelled')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
  })

  it('shows "Mark show as Sold Out" checkbox for sold_out reports', () => {
    render(
      <ResolveReportDialog
        report={makeReport({ report_type: 'sold_out' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Mark show as Sold Out')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
  })

  it('does not show flag checkbox for inaccurate reports', () => {
    render(
      <ResolveReportDialog
        report={makeReport({ report_type: 'inaccurate' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.queryByText('Mark show as Cancelled')).not.toBeInTheDocument()
    expect(screen.queryByText('Mark show as Sold Out')).not.toBeInTheDocument()
  })

  it('renders optional notes textarea', () => {
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByLabelText('Action taken (optional)')).toBeInTheDocument()
  })

  it('calls mutate with setShowFlag true for cancelled report with checkbox checked', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport({ id: 42, report_type: 'cancelled' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 42,
        notes: undefined,
        setShowFlag: true,
      },
      expect.any(Object)
    )
  })

  it('calls mutate with setShowFlag false when checkbox is unchecked', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport({ id: 42, report_type: 'cancelled' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 42,
        notes: undefined,
        setShowFlag: false,
      },
      expect.any(Object)
    )
  })

  it('calls mutate with setShowFlag undefined for inaccurate reports', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport({ id: 42, report_type: 'inaccurate' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 42,
        notes: undefined,
        setShowFlag: undefined,
      },
      expect.any(Object)
    )
  })

  it('calls mutate with notes when provided', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport({ id: 5, report_type: 'inaccurate' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(
      screen.getByLabelText('Action taken (optional)'),
      'Updated show date'
    )
    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 5,
        notes: 'Updated show date',
        setShowFlag: undefined,
      },
      expect.any(Object)
    )
  })

  it('sends undefined for whitespace-only notes', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport({ id: 1, report_type: 'inaccurate' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Action taken (optional)'), '   ')
    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 1,
        notes: undefined,
        setShowFlag: undefined,
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('disables buttons when mutation is pending', () => {
    mockIsPending = true
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Resolving...')).toBeInTheDocument()
  })

  it('shows error message when mutation fails', () => {
    mockIsError = true
    mockError = new Error('Server error')
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows fallback error message when error has no message', () => {
    mockIsError = true
    mockError = null
    render(
      <ResolveReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(
      screen.getByText('Failed to resolve report. Please try again.')
    ).toBeInTheDocument()
  })
})
