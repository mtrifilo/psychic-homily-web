import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ReportEntityDialog } from './ReportEntityDialog'

// useReportEntity is a useMutation hook; mock it so we can drive the dialog
// into each render state without a live QueryClient or network.
let mockMutation: {
  mutate: ReturnType<typeof vi.fn>
  reset: ReturnType<typeof vi.fn>
  isError: boolean
  isSuccess: boolean
  isPending: boolean
  error: Error | null
}

vi.mock('../hooks/useReportEntity', () => ({
  useReportEntity: () => mockMutation,
}))

const baseProps = {
  open: true,
  onOpenChange: vi.fn(),
  entityType: 'artist' as const,
  entityId: 1,
  entityName: 'Test Artist',
}

beforeEach(() => {
  mockMutation = {
    mutate: vi.fn(),
    reset: vi.fn(),
    isError: false,
    isSuccess: false,
    isPending: false,
    error: null,
  }
})

describe('ReportEntityDialog', () => {
  it('renders the form (report-type options) in the idle state', () => {
    render(<ReportEntityDialog {...baseProps} />)

    expect(screen.getByText("What's the issue?")).toBeInTheDocument()
    // Artist taxonomy option present.
    expect(screen.getByText('Inaccurate Information')).toBeInTheDocument()
    // No duplicate banner in the idle state.
    expect(
      screen.queryByTestId('report-duplicate-banner')
    ).not.toBeInTheDocument()
  })

  it('shows the duplicate state as a theme-aware pending StatusBanner (PSY-965)', () => {
    mockMutation.isError = true
    // The exact production message from
    // backend/internal/errors/entity_report.go (ErrEntityReportDuplicatePending),
    // so this test guards the `isDuplicateError` regex against the real contract.
    mockMutation.error = new Error(
      'you already have a pending report for this entity'
    )

    render(<ReportEntityDialog {...baseProps} />)

    // PSY-965: the old hardcoded-orange div is now a StatusBanner (role=status).
    const banner = screen.getByTestId('report-duplicate-banner')
    expect(banner).toHaveAttribute('role', 'status')
    expect(banner).toHaveClass('bg-pending', 'border-pending-foreground')
    expect(banner).not.toHaveClass('border-orange-800', 'bg-orange-950/50')
    expect(screen.getByText('Already reported')).toBeInTheDocument()

    // The form is suppressed while the duplicate notice is shown.
    expect(screen.queryByText("What's the issue?")).not.toBeInTheDocument()
  })

  it('routes a non-duplicate error to the generic error block, not the pending banner', () => {
    mockMutation.isError = true
    mockMutation.error = new Error('Server unavailable')

    render(<ReportEntityDialog {...baseProps} />)

    // A generic error must NOT trip the duplicate (pending) banner...
    expect(
      screen.queryByTestId('report-duplicate-banner')
    ).not.toBeInTheDocument()
    // ...it surfaces the error message, and the form stays available to retry.
    expect(screen.getByText('Server unavailable')).toBeInTheDocument()
    expect(screen.getByText("What's the issue?")).toBeInTheDocument()
  })
})
