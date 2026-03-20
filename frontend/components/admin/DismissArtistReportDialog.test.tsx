import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DismissArtistReportDialog } from './DismissArtistReportDialog'
import type { ArtistReportResponse } from '@/features/artists'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminArtistReports', () => ({
  useDismissArtistReport: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
    isError: mockIsError,
    error: mockError,
  }),
}))

function makeReport(
  overrides: Partial<ArtistReportResponse> = {}
): ArtistReportResponse {
  return {
    id: 1,
    artist_id: 10,
    report_type: 'inaccurate',
    status: 'pending',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    artist: {
      id: 10,
      name: 'Test Artist',
      slug: 'test-artist',
    },
    ...overrides,
  }
}

describe('DismissArtistReportDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
    mockIsError = false
    mockError = null
  })

  it('renders nothing when closed', () => {
    render(
      <DismissArtistReportDialog
        report={makeReport()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Dismiss Report')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <DismissArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByRole('heading', { name: /Dismiss Report/i })).toBeInTheDocument()
    expect(
      screen.getByText(/Dismiss this report for "Test Artist"/)
    ).toBeInTheDocument()
  })

  it('shows "Unknown Artist" when artist info is missing', () => {
    render(
      <DismissArtistReportDialog
        report={makeReport({ artist: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Dismiss this report for "Unknown Artist"/)
    ).toBeInTheDocument()
  })

  it('renders optional notes textarea', () => {
    render(
      <DismissArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByLabelText('Notes (optional)')).toBeInTheDocument()
  })

  it('allows submitting without notes', async () => {
    const user = userEvent.setup()
    render(
      <DismissArtistReportDialog
        report={makeReport({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss')
    )
    const submitButton = dismissButtons[dismissButtons.length - 1]
    expect(submitButton).not.toBeDisabled()

    await user.click(submitButton)

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 42,
        notes: undefined,
      },
      expect.any(Object)
    )
  })

  it('calls mutate with notes when provided', async () => {
    const user = userEvent.setup()
    render(
      <DismissArtistReportDialog
        report={makeReport({ id: 5 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), 'Spam report')

    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss')
    )
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 5,
        notes: 'Spam report',
      },
      expect.any(Object)
    )
  })

  it('sends undefined for whitespace-only notes', async () => {
    const user = userEvent.setup()
    render(
      <DismissArtistReportDialog
        report={makeReport({ id: 1 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), '   ')

    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss')
    )
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 1,
        notes: undefined,
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <DismissArtistReportDialog
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
      <DismissArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Dismissing...')).toBeInTheDocument()
  })

  it('shows error message when mutation fails', () => {
    mockIsError = true
    mockError = new Error('Network error')
    render(
      <DismissArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByText('Network error')).toBeInTheDocument()
  })

  it('shows fallback error message when error has no message', () => {
    mockIsError = true
    mockError = null
    render(
      <DismissArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(
      screen.getByText('Failed to dismiss report. Please try again.')
    ).toBeInTheDocument()
  })
})
