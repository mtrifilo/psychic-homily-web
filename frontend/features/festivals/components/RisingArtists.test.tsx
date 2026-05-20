import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { FestivalBreakouts } from '../types'

const mockUseBreakouts = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useFestivalBreakouts: (opts: unknown) => mockUseBreakouts(opts),
}))

import { RisingArtists } from './RisingArtists'

function makeBreakouts(overrides: Partial<FestivalBreakouts> = {}): FestivalBreakouts {
  return {
    breakouts: [
      {
        artist: { id: 1, name: 'Rising Star', slug: 'rising-star' },
        current_tier: 'sub_headliner',
        trajectory: [
          { festival_name: 'SXSW', festival_slug: 'sxsw-2023', year: 2023, tier: 'opener' },
          { festival_name: 'Pitchfork', festival_slug: 'pitchfork-2024', year: 2024, tier: 'mid_card' },
        ],
        tier_improvement: 2,
        breakout_score: 0.8,
      },
    ],
    milestones: [
      {
        artist: { id: 2, name: 'Debutant', slug: 'debutant' },
        milestone: 'first_festival_appearance',
        tier: 'local',
        festival: 'FORM',
      },
    ],
    ...overrides,
  }
}

describe('RisingArtists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a loading spinner while fetching', () => {
    mockUseBreakouts.mockReturnValue({ data: undefined, isLoading: true })
    const { container } = render(<RisingArtists festivalIdOrSlug={1} />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders nothing when there is no data', () => {
    mockUseBreakouts.mockReturnValue({ data: null, isLoading: false })
    const { container } = render(<RisingArtists festivalIdOrSlug={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when both breakouts and milestones are empty', () => {
    mockUseBreakouts.mockReturnValue({
      data: { breakouts: [], milestones: [] },
      isLoading: false,
    })
    const { container } = render(<RisingArtists festivalIdOrSlug={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders breakout artists with their trajectory and tier', () => {
    mockUseBreakouts.mockReturnValue({ data: makeBreakouts(), isLoading: false })
    render(<RisingArtists festivalIdOrSlug={1} />)

    expect(screen.getByText('Rising Artists')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'Rising Star' })
    expect(link).toHaveAttribute('href', '/artists/rising-star')
    // current tier badge
    expect(screen.getByText('Sub-Headliner')).toBeInTheDocument()
    // trajectory festival names appear
    expect(screen.getByText(/SXSW/)).toBeInTheDocument()
    expect(screen.getByText(/Pitchfork/)).toBeInTheDocument()
  })

  it('renders the milestones section with the milestone label', () => {
    mockUseBreakouts.mockReturnValue({ data: makeBreakouts(), isLoading: false })
    render(<RisingArtists festivalIdOrSlug={1} />)

    expect(screen.getByText('Milestones')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Debutant' })).toBeInTheDocument()
    expect(screen.getByText('Festival Debut')).toBeInTheDocument()
  })
})
