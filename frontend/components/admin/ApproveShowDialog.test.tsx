import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ApproveShowDialog } from './ApproveShowDialog'
import type { ShowResponse } from '@/features/shows'

const mockMutate = vi.fn()
let mockIsPending = false

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useApproveShow: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
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

describe('ApproveShowDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
  })

  it('renders nothing when closed', () => {
    render(
      <ApproveShowDialog
        show={makeShow()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Approve Show')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <ApproveShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Approve Show')).toBeInTheDocument()
    expect(
      screen.getByText(/Approve "Test Show" to make it visible to the public/)
    ).toBeInTheDocument()
  })

  it('shows artist names as fallback title when show title is empty', () => {
    render(
      <ApproveShowDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Approve "Headliner Band" to make it visible/)
    ).toBeInTheDocument()
  })

  it('shows "Untitled Show" when title and artists are empty', () => {
    render(
      <ApproveShowDialog
        show={makeShow({ title: '', artists: [] })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/Approve "Untitled Show" to make it visible/)
    ).toBeInTheDocument()
  })

  it('shows verified message when all venues are verified', () => {
    render(
      <ApproveShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(
      screen.getByText(/All venues for this show are already verified/)
    ).toBeInTheDocument()
    expect(screen.queryByText('Unverified Venues')).not.toBeInTheDocument()
  })

  it('shows unverified venues section when venues are unverified', () => {
    render(
      <ApproveShowDialog
        show={makeShow({
          venues: [
            {
              id: 2,
              slug: 'new-venue',
              name: 'New Venue',
              city: 'Tempe',
              state: 'AZ',
              verified: false,
            },
          ],
        })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText('Unverified Venues')).toBeInTheDocument()
    expect(screen.getByText('New Venue - Tempe, AZ')).toBeInTheDocument()
    expect(screen.getByText(/Also verify this venue/)).toBeInTheDocument()
  })

  it('uses plural text for multiple unverified venues', () => {
    render(
      <ApproveShowDialog
        show={makeShow({
          venues: [
            {
              id: 2,
              slug: 'venue-a',
              name: 'Venue A',
              city: 'Tempe',
              state: 'AZ',
              verified: false,
            },
            {
              id: 3,
              slug: 'venue-b',
              name: 'Venue B',
              city: 'Mesa',
              state: 'AZ',
              verified: false,
            },
          ],
        })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Also verify these venues/)).toBeInTheDocument()
  })

  it('calls mutate with verifyVenues true when approving with unverified venues and checkbox checked', async () => {
    const user = userEvent.setup()
    render(
      <ApproveShowDialog
        show={makeShow({
          venues: [
            {
              id: 2,
              slug: 'new-venue',
              name: 'New Venue',
              city: 'Tempe',
              state: 'AZ',
              verified: false,
            },
          ],
        })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Approve/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { showId: 1, verifyVenues: true },
      expect.any(Object)
    )
  })

  it('calls mutate with verifyVenues false when checkbox is unchecked', async () => {
    const user = userEvent.setup()
    render(
      <ApproveShowDialog
        show={makeShow({
          venues: [
            {
              id: 2,
              slug: 'new-venue',
              name: 'New Venue',
              city: 'Tempe',
              state: 'AZ',
              verified: false,
            },
          ],
        })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // Uncheck the checkbox (it's checked by default)
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /Approve/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { showId: 1, verifyVenues: false },
      expect.any(Object)
    )
  })

  it('calls mutate with verifyVenues false when all venues are already verified', async () => {
    const user = userEvent.setup()
    render(
      <ApproveShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Approve/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { showId: 1, verifyVenues: false },
      expect.any(Object)
    )
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <ApproveShowDialog
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
      <ApproveShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Approving...')).toBeInTheDocument()
  })
})
