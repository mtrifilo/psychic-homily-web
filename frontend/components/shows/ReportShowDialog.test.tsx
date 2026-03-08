import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReportShowDialog } from './ReportShowDialog'

const mockMutate = vi.fn()
const mockReportHook = vi.fn(() => ({
  mutate: mockMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/lib/hooks/useShowReports', () => ({
  useReportShow: () => mockReportHook(),
}))

describe('ReportShowDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockReportHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders nothing when closed', () => {
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Report Issue')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Report Issue')).toBeInTheDocument()
    expect(screen.getByText(/Test Show/)).toBeInTheDocument()
  })

  it('renders all three report type options', () => {
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Cancelled')).toBeInTheDocument()
    expect(screen.getByText('Sold Out')).toBeInTheDocument()
    expect(screen.getByText('Inaccurate Info')).toBeInTheDocument()
  })

  it('renders descriptions for each report type', () => {
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('This show has been cancelled')).toBeInTheDocument()
    expect(screen.getByText('This show is sold out')).toBeInTheDocument()
    expect(screen.getByText('Some information is incorrect')).toBeInTheDocument()
  })

  it('disables submit button when no report type selected', () => {
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByRole('button', { name: /Submit Report/ })).toBeDisabled()
  })

  it('enables submit button after selecting a report type', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByText('Cancelled'))
    expect(screen.getByRole('button', { name: /Submit Report/ })).not.toBeDisabled()
  })

  it('shows details textarea after selecting a report type', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // No textarea initially
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

    await user.click(screen.getByText('Cancelled'))
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })

  it('shows "(recommended)" label for inaccurate type details', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByText('Inaccurate Info'))
    expect(screen.getByText(/recommended/)).toBeInTheDocument()
  })

  it('shows "(optional)" label for non-inaccurate type details', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByText('Cancelled'))
    expect(screen.getByText(/optional/)).toBeInTheDocument()
  })

  it('calls mutate with report data on submit', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={5}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByText('Sold Out'))
    await user.click(screen.getByRole('button', { name: /Submit Report/ }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showId: 5,
        reportType: 'sold_out',
        details: undefined,
      },
      expect.any(Object)
    )
  })

  it('includes details text in mutation when provided', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={5}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByText('Inaccurate Info'))
    await user.type(screen.getByRole('textbox'), 'Wrong date listed')
    await user.click(screen.getByRole('button', { name: /Submit Report/ }))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showId: 5,
        reportType: 'inaccurate',
        details: 'Wrong date listed',
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) on cancel', async () => {
    const user = userEvent.setup()
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows pending state during mutation', () => {
    mockReportHook.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Submitting...')).toBeInTheDocument()
  })

  it('shows error message on failure', () => {
    mockReportHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: { message: 'Already reported' },
    })
    render(
      <ReportShowDialog
        showId={1}
        showTitle="Test Show"
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Already reported')).toBeInTheDocument()
  })
})
