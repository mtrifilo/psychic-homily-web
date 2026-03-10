import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BatchRejectDialog } from './BatchRejectDialog'

const mockMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useBatchRejectShows: () => ({
    mutate: mockMutate,
    isPending: false,
  }),
}))

describe('BatchRejectDialog', () => {
  const onOpenChange = vi.fn()
  const onSuccess = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when closed', () => {
    render(
      <BatchRejectDialog
        showIds={[1, 2]}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Reject 2 Shows')).not.toBeInTheDocument()
  })

  it('renders dialog with correct count for multiple shows', () => {
    render(
      <BatchRejectDialog
        showIds={[1, 2, 3]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    // Title and button both contain the text
    expect(screen.getAllByText(/Reject 3 Shows/).length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/3 selected shows/)).toBeInTheDocument()
  })

  it('renders singular text for single show', () => {
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getAllByText(/Reject 1 Show/).length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/1 selected show\b/)).toBeInTheDocument()
  })

  it('disables reject button when reason is empty', () => {
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).toBeDisabled()
  })

  it('enables reject button when reason is provided', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason'), 'Not a real event')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).not.toBeDisabled()
  })

  it('calls mutate with correct args when submitted', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[1, 2]}
        open={true}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    )

    await user.type(screen.getByLabelText('Reason'), 'Duplicate listing')
    await user.selectOptions(screen.getByLabelText('Category'), 'duplicate')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showIds: [1, 2],
        reason: 'Duplicate listing',
        category: 'duplicate',
      },
      expect.any(Object)
    )
  })

  it('calls mutate without category when none selected', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[5]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason'), 'Bad data')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showIds: [5],
        reason: 'Bad data',
        category: undefined,
      },
      expect.any(Object)
    )
  })

  it('pre-fills category and reason from defaults', () => {
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
        defaultCategory="non_music"
        defaultReason="Not a music event"
      />
    )

    expect(screen.getByLabelText('Reason')).toHaveValue('Not a music event')
    expect(screen.getByLabelText('Category')).toHaveValue('non_music')
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
