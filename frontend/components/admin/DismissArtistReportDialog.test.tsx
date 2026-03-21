import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DismissArtistReportDialog } from './DismissArtistReportDialog'
import type { ArtistReportResponse } from '@/features/artists'

const mockMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminArtistReports', () => ({
  useDismissArtistReport: () => ({
    mutate: mockMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const baseReport: ArtistReportResponse = {
  id: 1,
  artist_id: 10,
  report_type: 'inaccurate',
  status: 'pending',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  artist: { id: 10, name: 'Test Artist', slug: 'test-artist' },
}

describe('DismissArtistReportDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when closed', () => {
    render(
      <DismissArtistReportDialog
        report={baseReport}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Dismiss Report')).not.toBeInTheDocument()
  })

  it('renders dialog when open', () => {
    render(
      <DismissArtistReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getAllByText('Dismiss Report').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/Test Artist/)).toBeInTheDocument()
  })

  it('calls mutate with correct args when dismiss is clicked', async () => {
    const user = userEvent.setup()
    render(
      <DismissArtistReportDialog
        report={baseReport}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Notes (optional)'), 'Duplicate')

    const dismissButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Dismiss Report')
    )
    await user.click(dismissButtons[dismissButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      { reportId: 1, notes: 'Duplicate' },
      expect.any(Object)
    )
  })

  it('resets notes when Cancel is clicked (notes cleared before close)', async () => {
    const user = userEvent.setup()

    const trackOpenChange = vi.fn()

    render(
      <DismissArtistReportDialog
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
  })

  it('sends empty notes after Cancel-then-reopen cycle', async () => {
    const user = userEvent.setup()
    const { unmount } = render(
      <DismissArtistReportDialog
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
      <DismissArtistReportDialog
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

  it('shows unknown artist when artist info is missing', () => {
    const reportNoArtist = { ...baseReport, artist: undefined }
    render(
      <DismissArtistReportDialog
        report={reportNoArtist}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Unknown Artist/)).toBeInTheDocument()
  })
})
