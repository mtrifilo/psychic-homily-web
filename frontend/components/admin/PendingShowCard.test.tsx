import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PendingShowCard } from './PendingShowCard'
import type { ShowResponse } from '@/lib/types/show'

const mockApproveMutate = vi.fn()
const mockRejectMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useApproveShow: () => ({
    mutate: mockApproveMutate,
    isPending: false,
  }),
  useRejectShow: () => ({
    mutate: mockRejectMutate,
    isPending: false,
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

describe('PendingShowCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders show title, venue, and artists', () => {
    render(<PendingShowCard show={makeShow()} />)

    expect(screen.getByText('Test Show')).toBeInTheDocument()
    expect(screen.getByText('Headliner Band')).toBeInTheDocument()
    expect(screen.getByText('With: Opening Act')).toBeInTheDocument()
    expect(screen.getByText('Test Venue')).toBeInTheDocument()
  })

  it('shows Pending Review badge', () => {
    render(<PendingShowCard show={makeShow()} />)
    expect(screen.getByText('Pending Review')).toBeInTheDocument()
  })

  it('shows Discovery Import badge for discovery source', () => {
    render(<PendingShowCard show={makeShow({ source: 'discovery' })} />)
    expect(screen.getByText('Discovery Import')).toBeInTheDocument()
  })

  it('shows source_venue badge when present', () => {
    render(
      <PendingShowCard
        show={makeShow({ source: 'discovery', source_venue: 'rebel-lounge' })}
      />
    )
    expect(screen.getByText('rebel-lounge')).toBeInTheDocument()
  })

  it('shows Unverified Venue badge for unverified venues', () => {
    render(
      <PendingShowCard
        show={makeShow({
          venues: [
            { id: 1, slug: 'v', name: 'New Venue', city: 'Phoenix', state: 'AZ', verified: false },
          ],
        })}
      />
    )
    expect(screen.getByText('Unverified Venue')).toBeInTheDocument()
    expect(screen.getByText('(New venue)')).toBeInTheDocument()
  })

  it('shows duplicate link when duplicate_of_show_id is set', () => {
    render(
      <PendingShowCard show={makeShow({ duplicate_of_show_id: 42 })} />
    )
    expect(screen.getByText(/Potential duplicate of/)).toBeInTheDocument()
    expect(screen.getByText('show #42')).toBeInTheDocument()
  })

  it('renders Approve and Reject buttons', () => {
    render(<PendingShowCard show={makeShow()} />)
    expect(screen.getByRole('button', { name: /Approve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Reject/i })).toBeInTheDocument()
  })

  it('does not render checkbox when onSelectChange is not provided', () => {
    render(<PendingShowCard show={makeShow()} />)
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('renders checkbox when onSelectChange is provided', () => {
    const onSelectChange = vi.fn()
    render(
      <PendingShowCard
        show={makeShow()}
        selected={false}
        onSelectChange={onSelectChange}
      />
    )
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
  })

  it('calls onSelectChange when checkbox is toggled', async () => {
    const user = userEvent.setup()
    const onSelectChange = vi.fn()
    render(
      <PendingShowCard
        show={makeShow()}
        selected={false}
        onSelectChange={onSelectChange}
      />
    )

    await user.click(screen.getByRole('checkbox'))
    expect(onSelectChange).toHaveBeenCalledWith(true)
  })

  it('applies selected ring styling when selected', () => {
    const onSelectChange = vi.fn()
    const { container } = render(
      <PendingShowCard
        show={makeShow()}
        selected={true}
        onSelectChange={onSelectChange}
      />
    )
    const card = container.querySelector('[class*="ring-2"]')
    expect(card).toBeInTheDocument()
  })

  it('renders Not Music quick-reject button when callback provided', () => {
    const onQuickReject = vi.fn()
    render(
      <PendingShowCard
        show={makeShow()}
        onQuickRejectNotMusic={onQuickReject}
      />
    )
    expect(screen.getByRole('button', { name: /Not Music/i })).toBeInTheDocument()
  })

  it('calls onQuickRejectNotMusic with show ID when clicked', async () => {
    const user = userEvent.setup()
    const onQuickReject = vi.fn()
    render(
      <PendingShowCard
        show={makeShow({ id: 99 })}
        onQuickRejectNotMusic={onQuickReject}
      />
    )

    await user.click(screen.getByRole('button', { name: /Not Music/i }))
    expect(onQuickReject).toHaveBeenCalledWith(99)
  })

  it('does not render Not Music button when callback not provided', () => {
    render(<PendingShowCard show={makeShow()} />)
    expect(
      screen.queryByRole('button', { name: /Not Music/i })
    ).not.toBeInTheDocument()
  })

  it('uses artist names as title fallback when title is empty', () => {
    render(
      <PendingShowCard
        show={makeShow({ title: '' })}
      />
    )
    expect(screen.getByText('Headliner Band, Opening Act')).toBeInTheDocument()
  })
})
