import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RejectedShowCard } from './RejectedShowCard'
import type { ShowResponse } from '@/features/shows'

// Mock the ApproveShowDialog to avoid deep dependency trees
vi.mock('./ApproveShowDialog', () => ({
  ApproveShowDialog: ({
    open,
    show,
  }: {
    open: boolean
    show: ShowResponse
    onOpenChange: (v: boolean) => void
  }) => {
    return open ? (
      <div data-testid="approve-dialog">Approve Dialog for {show.title}</div>
    ) : null
  },
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'rejected',
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
      {
        id: 2,
        slug: 'opener',
        name: 'Opening Act',
        is_headliner: false,
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

describe('RejectedShowCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders show title', () => {
    render(<RejectedShowCard show={makeShow()} />)
    expect(screen.getByText('Test Show')).toBeInTheDocument()
  })

  it('shows artist names as fallback when title is empty', () => {
    render(<RejectedShowCard show={makeShow({ title: '' })} />)
    expect(
      screen.getByText('Headliner Band, Opening Act')
    ).toBeInTheDocument()
  })

  it('shows "Untitled Show" when title and artists are empty', () => {
    render(
      <RejectedShowCard show={makeShow({ title: '', artists: [] })} />
    )
    expect(screen.getByText('Untitled Show')).toBeInTheDocument()
  })

  it('renders Rejected badge', () => {
    render(<RejectedShowCard show={makeShow()} />)
    expect(screen.getByText('Rejected')).toBeInTheDocument()
  })

  it('renders venue name and city', () => {
    render(<RejectedShowCard show={makeShow()} />)
    expect(screen.getByText(/Test Venue, Phoenix/)).toBeInTheDocument()
  })

  it('does not render venue info when venues array is empty', () => {
    render(<RejectedShowCard show={makeShow({ venues: [] })} />)
    expect(screen.queryByText(/Test Venue/)).not.toBeInTheDocument()
  })

  it('renders artist names in content area', () => {
    render(<RejectedShowCard show={makeShow()} />)
    expect(
      screen.getByText(/Headliner Band, Opening Act/)
    ).toBeInTheDocument()
  })

  it('renders rejection reason when provided', () => {
    render(
      <RejectedShowCard
        show={makeShow({ rejection_reason: 'Duplicate submission' })}
      />
    )
    expect(screen.getByText(/Reason:/)).toBeInTheDocument()
    expect(screen.getByText(/Duplicate submission/)).toBeInTheDocument()
  })

  it('does not render rejection reason when not provided', () => {
    render(<RejectedShowCard show={makeShow({ rejection_reason: null })} />)
    expect(screen.queryByText(/Reason:/)).not.toBeInTheDocument()
  })

  it('renders Approve button', () => {
    render(<RejectedShowCard show={makeShow()} />)
    expect(
      screen.getByRole('button', { name: /Approve/i })
    ).toBeInTheDocument()
  })

  it('opens approve dialog when Approve button is clicked', async () => {
    const user = userEvent.setup()
    render(<RejectedShowCard show={makeShow({ title: 'My Show' })} />)

    await user.click(screen.getByRole('button', { name: /Approve/i }))

    expect(screen.getByTestId('approve-dialog')).toBeInTheDocument()
    expect(
      screen.getByText('Approve Dialog for My Show')
    ).toBeInTheDocument()
  })

  it('renders event date information', () => {
    render(<RejectedShowCard show={makeShow()} />)
    // The component renders the day number from the event date
    expect(screen.getByText('15')).toBeInTheDocument()
  })
})
