import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RejectShowDialog } from './RejectShowDialog'
import type { ShowResponse } from '@/features/shows'

const mockMutate = vi.fn()
let mockIsPending = false
let mockIsError = false
let mockError: Error | null = null

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useRejectShow: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
    isError: mockIsError,
    error: mockError,
  }),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'pending',
    venues: [
      {
        id: 1,
        slug: 'test-venue',
        name: 'Test Venue',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      {
        id: 1,
        slug: 'headliner',
        name: 'Headliner Band',
        is_headliner: true,
        set_type: 'headliner',
        position: 1,
        socials: {},
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('RejectShowDialog', () => {
  const onOpenChange = vi.fn()

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
      <RejectShowDialog
        show={makeShow()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Reject Show')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByRole('heading', { name: /Reject Show/i })).toBeInTheDocument()
    expect(
      screen.getByText(/Reject "Test Show". Please provide a reason/)
    ).toBeInTheDocument()
  })

  it('shows artist names as fallback title when show title is empty', () => {
    render(
      <RejectShowDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Reject "Headliner Band"/)
    ).toBeInTheDocument()
  })

  it('shows "Untitled Show" when title and artists are empty', () => {
    render(
      <RejectShowDialog
        show={makeShow({ title: '', artists: [] })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Reject "Untitled Show"/)
    ).toBeInTheDocument()
  })

  it('renders textarea for rejection reason', () => {
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByLabelText('Reason for rejection')).toBeInTheDocument()
  })

  it('disables reject button when reason is empty', () => {
    render(
      <RejectShowDialog
        show={makeShow()}
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
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), 'Duplicate listing')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).not.toBeDisabled()
  })

  it('does not call mutate when reason is only whitespace', async () => {
    const user = userEvent.setup()
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), '   ')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).toBeDisabled()
  })

  it('calls mutate with correct args when submitted', async () => {
    const user = userEvent.setup()
    render(
      <RejectShowDialog
        show={makeShow({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), 'Not a real event')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showId: 42,
        reason: 'Not a real event',
      },
      expect.any(Object)
    )
  })

  it('trims whitespace from reason before submitting', async () => {
    const user = userEvent.setup()
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), '  Bad data  ')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        showId: 1,
        reason: 'Bad data',
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <RejectShowDialog
        show={makeShow()}
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
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Rejecting...')).toBeInTheDocument()
  })

  it('shows reject error banner when useRejectShow is in error state', () => {
    mockIsError = true
    mockError = new Error('Server unreachable')
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // Dialog stays open AND banner renders with the error message.
    expect(screen.getByRole('heading', { name: /Reject Show/i })).toBeInTheDocument()
    expect(screen.getByText('Server unreachable')).toBeInTheDocument()
  })

  it('falls back to canned copy when error has no message', () => {
    mockIsError = true
    mockError = null
    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('heading', { name: /Reject Show/i })).toBeInTheDocument()
    expect(
      screen.getByText('Failed to reject show. Please try again.')
    ).toBeInTheDocument()
  })

  it('clears reason and closes dialog when mutation succeeds (onSuccess fires)', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.()
    })

    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const textarea = screen.getByLabelText('Reason for rejection')
    await user.type(textarea, 'Duplicate listing')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(onOpenChange).toHaveBeenCalledWith(false)
    // After onSuccess: reason was cleared via setReason('') — the textarea
    // bound to the state should now be empty.
    expect(textarea).toHaveValue('')
  })

  it('preserves reason draft when mutation does not invoke onSuccess (error path)', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation(() => {
      // Simulate error: onSuccess never fires
    })

    render(
      <RejectShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    const textarea = screen.getByLabelText('Reason for rejection')
    await user.type(textarea, 'Bad data')

    const rejectButtons = screen
      .getAllByRole('button')
      .filter(b => b.textContent?.includes('Reject'))
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    // Draft survives so admin can retry without re-typing.
    expect(textarea).toHaveValue('Bad data')
  })
})
