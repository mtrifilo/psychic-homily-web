import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BatchRejectDialog } from './BatchRejectDialog'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useBatchRejectShows: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
    isError: mockIsError,
    error: mockError,
  }),
}))

describe('BatchRejectDialog', () => {
  const onOpenChange = vi.fn()
  const onSuccess = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    // mockReset (not just clearAllMocks) is required because tests below
    // use mockImplementation to simulate success/error paths. clearAllMocks
    // only resets call history — implementations persist across tests.
    mockMutate.mockReset()
    mockIsPending = false
    mockIsError = false
    mockError = null
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

  it('does not call mutate when reason is only whitespace', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason'), '   ')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).toBeDisabled()
  })

  it('trims whitespace from reason before submitting', async () => {
    const user = userEvent.setup()
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason'), '  Padded reason  ')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showIds: [1],
        reason: 'Padded reason',
        category: undefined,
      },
      expect.any(Object)
    )
  })

  it('disables buttons when mutation is pending', () => {
    mockIsPending = true
    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Rejecting...')).toBeInTheDocument()
  })

  it('shows batch reject error banner when useBatchRejectShows is in error state', () => {
    mockIsError = true
    mockError = new Error('Server unreachable')
    render(
      <BatchRejectDialog
        showIds={[1, 2]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // Dialog stays open AND banner renders with the error message.
    expect(screen.getAllByText(/Reject 2 Shows/).length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('Server unreachable')).toBeInTheDocument()
  })

  it('falls back to canned copy when error has no message', () => {
    mockIsError = true
    mockError = null
    render(
      <BatchRejectDialog
        showIds={[1, 2]}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getAllByText(/Reject 2 Shows/).length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText('Failed to batch reject shows. Please try again.')
    ).toBeInTheDocument()
  })

  it('fires onSuccess callback, clears form, and closes dialog when mutation succeeds', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.()
    })

    render(
      <BatchRejectDialog
        showIds={[1, 2]}
        open={true}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    )

    const reasonTextarea = screen.getByLabelText('Reason')
    await user.type(reasonTextarea, 'Duplicate listing')
    await user.selectOptions(screen.getByLabelText('Category'), 'duplicate')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(onSuccess).toHaveBeenCalledTimes(1)
    expect(onOpenChange).toHaveBeenCalledWith(false)
    // Form state cleared via setReason('') and setCategory('').
    expect(reasonTextarea).toHaveValue('')
    expect(screen.getByLabelText('Category')).toHaveValue('')
  })

  it('does not fire onSuccess or clear form when mutation does not invoke onSuccess (error path)', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation(() => {
      // Error path: onSuccess never fires
    })

    render(
      <BatchRejectDialog
        showIds={[1]}
        open={true}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    )

    const reasonTextarea = screen.getByLabelText('Reason')
    await user.type(reasonTextarea, 'Will not clear')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    // Draft preserved so the admin can retry.
    expect(reasonTextarea).toHaveValue('Will not clear')
  })
})
