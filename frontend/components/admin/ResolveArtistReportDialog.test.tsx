import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ResolveArtistReportDialog } from './ResolveArtistReportDialog'
import type { ArtistReportResponse } from '@/features/artists'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminArtistReports', () => ({
  useResolveArtistReport: () => ({
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

describe('ResolveArtistReportDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
    mockIsError = false
    mockError = null
  })

  it('renders nothing when closed', () => {
    render(
      <ResolveArtistReportDialog
        report={makeReport()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Resolve Report')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <ResolveArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Resolve Report')).toBeInTheDocument()
    expect(
      screen.getByText(/Mark this report for "Test Artist" as resolved/)
    ).toBeInTheDocument()
  })

  it('shows "Unknown Artist" when artist info is missing', () => {
    render(
      <ResolveArtistReportDialog
        report={makeReport({ artist: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Mark this report for "Unknown Artist" as resolved/)
    ).toBeInTheDocument()
  })

  it('renders optional notes textarea', () => {
    render(
      <ResolveArtistReportDialog
        report={makeReport()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByLabelText('Action taken (optional)')).toBeInTheDocument()
  })

  it('allows submitting without notes', async () => {
    const user = userEvent.setup()
    render(
      <ResolveArtistReportDialog
        report={makeReport({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

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
      <ResolveArtistReportDialog
        report={makeReport({ id: 5 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(
      screen.getByLabelText('Action taken (optional)'),
      'Updated artist info'
    )
    await user.click(screen.getByRole('button', { name: /Mark as Resolved/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        reportId: 5,
        notes: 'Updated artist info',
      },
      expect.any(Object)
    )
  })

  it('sends undefined for whitespace-only notes', async () => {
    const user = userEvent.setup()
    render(
      <ResolveArtistReportDialog
        report={makeReport({ id: 1 })}
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
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <ResolveArtistReportDialog
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
      <ResolveArtistReportDialog
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
      <ResolveArtistReportDialog
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
      <ResolveArtistReportDialog
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
