import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CommunityPulseBand } from './CommunityPulseBand'

const useCommunityPulse = vi.fn()

vi.mock('../hooks/useCommunityPulse', () => ({
  useCommunityPulse: () => useCommunityPulse(),
}))

beforeEach(() => {
  useCommunityPulse.mockReset()
})

describe('CommunityPulseBand', () => {
  it('renders the two locked stats with thousands separators', () => {
    useCommunityPulse.mockReturnValue({
      data: { shows_this_week: 84, entities_in_graph: 18432 },
      isLoading: false,
      isError: false,
    })

    render(<CommunityPulseBand />)

    expect(
      screen.getByRole('region', { name: /community pulse/i })
    ).toBeInTheDocument()
    expect(screen.getByText('Community pulse')).toBeInTheDocument()
    expect(screen.getByText('84')).toBeInTheDocument()
    expect(screen.getByText('shows this week')).toBeInTheDocument()
    expect(screen.getByText('18,432')).toBeInTheDocument()
    expect(screen.getByText('entities in the graph')).toBeInTheDocument()
  })

  it('shows a loading skeleton while the pulse is fetching', () => {
    useCommunityPulse.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })

    const { container } = render(<CommunityPulseBand />)
    expect(
      screen.getByRole('region', { name: /community pulse/i })
    ).toBeInTheDocument()
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(
      0
    )
    expect(screen.queryByText('shows this week')).not.toBeInTheDocument()
  })

  it('self-hides on error so the homepage never breaks on a pulse outage', () => {
    useCommunityPulse.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })

    const { container } = render(<CommunityPulseBand />)
    expect(container).toBeEmptyDOMElement()
  })
})
