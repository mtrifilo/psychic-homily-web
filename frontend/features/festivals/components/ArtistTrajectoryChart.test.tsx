import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistTrajectory } from '../types'

const mockUseTrajectory = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useArtistFestivalTrajectory: (opts: unknown) => mockUseTrajectory(opts),
}))

import { ArtistTrajectoryChart } from './ArtistTrajectoryChart'

function makeTrajectory(overrides: Partial<ArtistTrajectory> = {}): ArtistTrajectory {
  return {
    artist: { id: 1, name: 'Test Artist', slug: 'test-artist' },
    appearances: [
      { festival_slug: 'sxsw-2024', festival_name: 'SXSW 2024', year: 2024, tier: 'mid_card' },
      { festival_slug: 'pitchfork-2023', festival_name: 'Pitchfork 2023', year: 2023, tier: 'opener' },
    ],
    best_tier: 'mid_card',
    total_appearances: 2,
    ...overrides,
  }
}

describe('ArtistTrajectoryChart', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when the artist has no festival appearances', () => {
    mockUseTrajectory.mockReturnValue({ data: null, isLoading: false })
    const { container } = renderWithProviders(
      <ArtistTrajectoryChart artistIdOrSlug={1} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the chart with header + appearances when data is present', () => {
    mockUseTrajectory.mockReturnValue({
      data: makeTrajectory(),
      isLoading: false,
    })
    renderWithProviders(<ArtistTrajectoryChart artistIdOrSlug={1} />)
    expect(screen.getByText('Festival History')).toBeInTheDocument()
    expect(screen.getByText('SXSW 2024')).toBeInTheDocument()
    expect(screen.getByText('Pitchfork 2023')).toBeInTheDocument()
  })

  it('renders the header with a [Show] toggle when defaultCollapsed and body is hidden', () => {
    mockUseTrajectory.mockReturnValue({
      data: makeTrajectory(),
      isLoading: false,
    })
    renderWithProviders(
      <ArtistTrajectoryChart artistIdOrSlug={1} defaultCollapsed />
    )
    expect(screen.getByText('Festival History')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show' })).toBeInTheDocument()
    expect(screen.queryByText('SXSW 2024')).not.toBeInTheDocument()
  })

  it('reveals the body when [Show] is clicked in collapsed mode', async () => {
    const user = userEvent.setup()
    mockUseTrajectory.mockReturnValue({
      data: makeTrajectory(),
      isLoading: false,
    })
    renderWithProviders(
      <ArtistTrajectoryChart artistIdOrSlug={1} defaultCollapsed />
    )
    await user.click(screen.getByRole('button', { name: 'Show' }))
    expect(screen.getByText('SXSW 2024')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Hide' })).toBeInTheDocument()
  })

  it('still hides the entire component when collapsed and there are no appearances', () => {
    mockUseTrajectory.mockReturnValue({ data: null, isLoading: false })
    const { container } = renderWithProviders(
      <ArtistTrajectoryChart artistIdOrSlug={1} defaultCollapsed />
    )
    expect(container.firstChild).toBeNull()
  })
})
