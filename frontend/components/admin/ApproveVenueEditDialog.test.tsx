import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ApproveVenueEditDialog } from './ApproveVenueEditDialog'
import type { PendingVenueEdit } from '@/features/venues'

const mockMutate = vi.fn()
let mockIsPending = false

vi.mock('@/lib/hooks/admin/useAdminVenueEdits', () => ({
  useApproveVenueEdit: () => ({
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

describe('ApproveVenueEditDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
  })

  it('renders nothing when closed', () => {
    render(
      <ApproveVenueEditDialog
        edit={makeEdit()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Approve Venue Edit')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <ApproveVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Approve Venue Edit')).toBeInTheDocument()
    expect(
      screen.getByText(/Approve the proposed changes to "Test Venue"/)
    ).toBeInTheDocument()
  })

  it('shows submitter name in description', () => {
    render(
      <ApproveVenueEditDialog
        edit={makeEdit({ submitter_name: 'John Smith' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('John Smith')).toBeInTheDocument()
  })

  it('falls back to user ID when submitter_name is not provided', () => {
    render(
      <ApproveVenueEditDialog
        edit={makeEdit({ submitter_name: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('User #5')).toBeInTheDocument()
  })

  it('falls back to venue ID when venue object is missing', () => {
    render(
      <ApproveVenueEditDialog
        edit={makeEdit({ venue: undefined })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Approve the proposed changes to "Venue #10"/)
    ).toBeInTheDocument()
  })

  it('calls mutate with edit ID when Approve button is clicked', async () => {
    const user = userEvent.setup()
    render(
      <ApproveVenueEditDialog
        edit={makeEdit({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Approve Changes/i }))

    expect(mockMutate).toHaveBeenCalledWith(42, expect.any(Object))
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <ApproveVenueEditDialog
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
      <ApproveVenueEditDialog
        edit={makeEdit()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Approving...')).toBeInTheDocument()
  })
})
