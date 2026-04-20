import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RejectVenueEditDialog } from './RejectVenueEditDialog'
import type { PendingVenueEdit } from '@/features/venues'

const mockMutate = vi.fn()
let mockIsPending = false

vi.mock('@/lib/hooks/admin/useAdminVenueEdits', () => ({
  useRejectVenueEdit: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
  }),
}))

function makeEdit(overrides: Partial<PendingVenueEdit> = {}): PendingVenueEdit {
  return {
    id: 1,
    venue_id: 10,
    submitted_by: 5,
    status: 'pending' as const,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    venue: {
      id: 10,
      slug: 'test-venue',
      name: 'Test Venue',
      address: '123 Main St',
      city: 'Phoenix',
      state: 'AZ',
      verified: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    },
    submitter_name: 'Jane Doe',
    ...overrides,
  }
}

describe('RejectVenueEditDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
  })

  it('renders nothing when closed', () => {
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Reject Venue Edit')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Reject Venue Edit')).toBeInTheDocument()
    expect(
      screen.getByText(/Reject the proposed changes to "Test Venue"/)
    ).toBeInTheDocument()
  })

  it('falls back to venue ID when venue object is missing', () => {
    render(
      <RejectVenueEditDialog
        edit={makeEdit({ venue: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Reject the proposed changes to "Venue #10"/)
    ).toBeInTheDocument()
  })

  it('renders textarea for rejection reason', () => {
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByLabelText('Reason for rejection')).toBeInTheDocument()
  })

  it('disables reject button when reason is empty', () => {
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
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
      <RejectVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), 'Inaccurate info')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    const submitButton = rejectButtons[rejectButtons.length - 1]
    expect(submitButton).not.toBeDisabled()
  })

  it('does not call mutate when reason is only whitespace', async () => {
    const user = userEvent.setup()
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
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
      <RejectVenueEditDialog
        edit={makeEdit({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.type(screen.getByLabelText('Reason for rejection'), 'Spam submission')

    const rejectButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Reject')
    )
    await user.click(rejectButtons[rejectButtons.length - 1])

    expect(mockMutate).toHaveBeenCalledWith(
      {
        editId: 42,
        reason: 'Spam submission',
      },
      expect.any(Object)
    )
  })

  it('trims whitespace from reason before submitting', async () => {
    const user = userEvent.setup()
    render(
      <RejectVenueEditDialog
        edit={makeEdit({ id: 1 })}
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
        editId: 1,
        reason: 'Bad data',
      },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <RejectVenueEditDialog
        edit={makeEdit()}
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
      <RejectVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Rejecting...')).toBeInTheDocument()
  })
})
