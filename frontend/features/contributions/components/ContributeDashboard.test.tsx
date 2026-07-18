import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { DataQualityCategory, DataQualitySummary } from '../types'

const mockUseContributeOpportunities = vi.fn()
const mockUseContributeCategory = vi.fn()

vi.mock('../hooks', () => ({
  useContributeOpportunities: () => mockUseContributeOpportunities(),
  useContributeCategory: (category: string) =>
    mockUseContributeCategory(category),
}))

// Import after mocks are wired.
import { ContributeDashboard } from './ContributeDashboard'

const GLOBAL_CATEGORY: DataQualityCategory = {
  key: 'artists_missing_links',
  label: 'Artists missing links',
  entity_type: 'artist',
  count: 434,
  description: 'Artists with no Bandcamp or Spotify',
}

const FOLLOWED_CATEGORY: DataQualityCategory = {
  key: 'followed_artists_missing_links',
  label: 'Artists you follow missing links',
  entity_type: 'artist',
  count: 3,
  description: 'Followed artists missing streaming links',
}

const CHARTING_CATEGORY: DataQualityCategory = {
  key: 'charting_artists_missing_links',
  label: 'Charting artists missing links',
  entity_type: 'artist',
  count: 6,
  description: 'Artists moving on the charts missing streaming links',
}

function summary(categories: DataQualityCategory[]): DataQualitySummary {
  return {
    categories,
    total_items: categories.reduce((sum, c) => sum + c.count, 0),
  }
}

function mockSummary(data: DataQualitySummary) {
  mockUseContributeOpportunities.mockReturnValue({
    data,
    isLoading: false,
    error: null,
  })
}

describe('ContributeDashboard — Loose Ends band', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Drill-in items query stays idle in these render-only assertions.
    mockUseContributeCategory.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    })
  })

  it('does not render the Loose Ends band when no loose-ends categories are present', () => {
    mockSummary(summary([GLOBAL_CATEGORY]))

    render(<ContributeDashboard />)

    expect(
      screen.queryByRole('region', { name: 'Loose Ends' })
    ).not.toBeInTheDocument()
    // The global category still renders in the grid.
    expect(screen.getByText('Artists missing links')).toBeInTheDocument()
  })

  it('does not render the band when the only loose-ends categories are empty', () => {
    mockSummary(
      summary([
        GLOBAL_CATEGORY,
        { ...FOLLOWED_CATEGORY, count: 0 },
        { ...CHARTING_CATEGORY, count: 0 },
      ])
    )

    render(<ContributeDashboard />)

    expect(
      screen.queryByRole('region', { name: 'Loose Ends' })
    ).not.toBeInTheDocument()
  })

  it('renders the band with the followed + charting lists when non-empty', () => {
    mockSummary(summary([GLOBAL_CATEGORY, FOLLOWED_CATEGORY, CHARTING_CATEGORY]))

    render(<ContributeDashboard />)

    const band = screen.getByRole('region', { name: 'Loose Ends' })
    expect(band).toBeInTheDocument()
    expect(
      within(band).getByRole('heading', { name: 'Loose Ends' })
    ).toBeInTheDocument()
    expect(
      within(band).getByText('Artists you follow missing links')
    ).toBeInTheDocument()
    expect(
      within(band).getByText('Charting artists missing links')
    ).toBeInTheDocument()
  })

  it('keeps loose-ends categories out of the global grid', () => {
    mockSummary(summary([GLOBAL_CATEGORY, FOLLOWED_CATEGORY, CHARTING_CATEGORY]))

    render(<ContributeDashboard />)

    const band = screen.getByRole('region', { name: 'Loose Ends' })

    // The followed list appears exactly once — inside the band, not duplicated
    // as a peer card in the global grid below.
    const followedMatches = screen.getAllByText(
      'Artists you follow missing links'
    )
    expect(followedMatches).toHaveLength(1)
    expect(band).toContainElement(followedMatches[0])

    // A genuinely global category is NOT inside the band.
    expect(
      within(band).queryByText('Artists missing links')
    ).not.toBeInTheDocument()
    expect(screen.getByText('Artists missing links')).toBeInTheDocument()
  })

  it('omits an empty loose-ends category while surfacing a non-empty peer', () => {
    mockSummary(
      summary([
        GLOBAL_CATEGORY,
        { ...FOLLOWED_CATEGORY, count: 0 },
        CHARTING_CATEGORY,
      ])
    )

    render(<ContributeDashboard />)

    const band = screen.getByRole('region', { name: 'Loose Ends' })
    expect(
      within(band).getByText('Charting artists missing links')
    ).toBeInTheDocument()
    expect(
      within(band).queryByText('Artists you follow missing links')
    ).not.toBeInTheDocument()
  })
})
