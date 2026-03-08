import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock the report artist hook
const mockMutate = vi.fn()
const mockReportMutation = vi.fn()
vi.mock('@/lib/hooks/useArtistReports', () => ({
  useReportArtist: () => mockReportMutation(),
}))

import { ReportArtistDialog } from './ReportArtistDialog'

describe('ReportArtistDialog', () => {
  const defaultProps = {
    artistId: 1,
    artistName: 'Test Artist',
    open: true,
    onOpenChange: vi.fn(),
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockMutate.mockReset()
    mockReportMutation.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders dialog title', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText('Report Issue')).toBeInTheDocument()
  })

  it('shows artist name in description', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(
      screen.getByText(/Report an issue with "Test Artist"/)
    ).toBeInTheDocument()
  })

  it('renders report type options', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText('Inaccurate Info')).toBeInTheDocument()
    expect(screen.getByText('Removal Request')).toBeInTheDocument()
  })

  it('renders option descriptions', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(
      screen.getByText('Some information on this page is incorrect')
    ).toBeInTheDocument()
    expect(
      screen.getByText("I'm the artist and want this page removed")
    ).toBeInTheDocument()
  })

  it('shows submit button disabled when no type selected', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText('Submit Report').closest('button')).toBeDisabled()
  })

  it('shows details textarea after selecting a type', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    // Details should not be visible before selection
    expect(screen.queryByLabelText(/Additional details/)).not.toBeInTheDocument()

    await user.click(screen.getByText('Inaccurate Info'))

    expect(screen.getByLabelText(/Additional details/)).toBeInTheDocument()
  })

  it('shows "(recommended)" hint for inaccurate type', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    await user.click(screen.getByText('Inaccurate Info'))

    expect(screen.getByText(/recommended/)).toBeInTheDocument()
  })

  it('shows "(optional)" hint for removal request type', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    await user.click(screen.getByText('Removal Request'))

    expect(screen.getByText(/optional/)).toBeInTheDocument()
  })

  it('enables submit button when type is selected', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    await user.click(screen.getByText('Inaccurate Info'))

    expect(
      screen.getByText('Submit Report').closest('button')
    ).not.toBeDisabled()
  })

  it('calls mutate with correct params on submit', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    await user.click(screen.getByText('Inaccurate Info'))
    const textarea = screen.getByPlaceholderText(
      'Please describe what information is incorrect...'
    )
    await user.type(textarea, 'Wrong hometown listed')
    await user.click(screen.getByText('Submit Report'))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        artistId: 1,
        reportType: 'inaccurate',
        details: 'Wrong hometown listed',
      },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('submits with undefined details when field is empty', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    await user.click(screen.getByText('Removal Request'))
    await user.click(screen.getByText('Submit Report'))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        artistId: 1,
        reportType: 'removal_request',
        details: undefined,
      },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('renders cancel button', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText('Cancel')).toBeInTheDocument()
  })

  it('calls onOpenChange(false) on cancel click', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    renderWithProviders(
      <ReportArtistDialog {...defaultProps} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByText('Cancel'))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows error message when mutation fails', () => {
    mockReportMutation.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: new Error('Server error'),
    })

    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows generic error when no error message', () => {
    mockReportMutation.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: null,
    })

    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(
      screen.getByText('Failed to submit report. Please try again.')
    ).toBeInTheDocument()
  })

  it('shows loading state when submitting', async () => {
    const user = userEvent.setup()
    mockReportMutation.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })

    renderWithProviders(<ReportArtistDialog {...defaultProps} />)

    expect(screen.getByText('Submitting...')).toBeInTheDocument()
    // Cancel should be disabled during submission
    expect(screen.getByText('Cancel').closest('button')).toBeDisabled()
  })

  it('does not render dialog when closed', () => {
    renderWithProviders(
      <ReportArtistDialog {...defaultProps} open={false} />
    )
    expect(screen.queryByText('Report Issue')).not.toBeInTheDocument()
  })

  it('has prompt label "What\'s the issue?"', () => {
    renderWithProviders(<ReportArtistDialog {...defaultProps} />)
    expect(screen.getByText("What's the issue?")).toBeInTheDocument()
  })
})
