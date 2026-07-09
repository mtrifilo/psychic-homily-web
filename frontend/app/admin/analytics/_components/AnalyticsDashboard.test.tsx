import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { AnalyticsDashboard, COLORS, ENTITY_DASH } from './AnalyticsDashboard'

// Mock recharts to avoid SVG rendering issues in JSDOM
vi.mock('recharts', () => {
  const MockResponsiveContainer = ({ children }: { children: React.ReactNode }) => (
    <div data-testid="responsive-container">{children}</div>
  )
  const MockLineChart = ({ children }: { children: React.ReactNode }) => (
    <div data-testid="line-chart">{children}</div>
  )
  const MockAreaChart = ({ children }: { children: React.ReactNode }) => (
    <div data-testid="area-chart">{children}</div>
  )
  const MockBarChart = ({ children }: { children: React.ReactNode }) => (
    <div data-testid="bar-chart">{children}</div>
  )
  const MockLine = () => <div data-testid="chart-line" />
  const MockArea = () => <div data-testid="chart-area" />
  const MockBar = () => <div data-testid="chart-bar" />
  const MockXAxis = () => <div />
  const MockYAxis = () => <div />
  const MockCartesianGrid = () => <div />
  const MockTooltip = () => <div />
  const MockLegend = () => <div />

  return {
    ResponsiveContainer: MockResponsiveContainer,
    LineChart: MockLineChart,
    Line: MockLine,
    AreaChart: MockAreaChart,
    Area: MockArea,
    BarChart: MockBarChart,
    Bar: MockBar,
    XAxis: MockXAxis,
    YAxis: MockYAxis,
    CartesianGrid: MockCartesianGrid,
    Tooltip: MockTooltip,
    Legend: MockLegend,
  }
})

// Mock the hooks
const mockGrowthData = {
  shows: [
    { month: '2025-10', count: 50 },
    { month: '2025-11', count: 65 },
  ],
  artists: [
    { month: '2025-10', count: 20 },
    { month: '2025-11', count: 25 },
  ],
  venues: [
    { month: '2025-10', count: 5 },
    { month: '2025-11', count: 7 },
  ],
  releases: [
    { month: '2025-10', count: 10 },
    { month: '2025-11', count: 12 },
  ],
  labels: [
    { month: '2025-10', count: 2 },
    { month: '2025-11', count: 3 },
  ],
  users: [
    { month: '2025-10', count: 15 },
    { month: '2025-11', count: 18 },
  ],
}

const mockEngagementData = {
  bookmarks: [{ month: '2025-10', count: 30 }],
  tags_added: [{ month: '2025-10', count: 20 }],
  tag_votes: [{ month: '2025-10', count: 50 }],
  collection_items: [{ month: '2025-10', count: 10 }], // API field name; displayed as "Collection Items"
  requests: [{ month: '2025-10', count: 5 }],
  request_votes: [{ month: '2025-10', count: 15 }],
  revisions: [{ month: '2025-10', count: 8 }],
  follows: [{ month: '2025-10', count: 25 }],
}

const mockCommunityData = {
  active_contributors_30d: 42,
  contributions_per_week: [
    { week: '2026-W10', count: 15 },
    { week: '2026-W11', count: 20 },
  ],
  request_fulfillment_rate: 0.72,
  new_collections_30d: 8, // API field name; displayed as "New Collections (30d)"
  top_contributors: [
    { user_id: 1, username: 'alice', display_name: 'Alice M.', count: 50 },
    { user_id: 2, username: 'bob', count: 35 },
  ],
}

const mockDataQualityData = {
  shows_approved: [{ month: '2025-10', count: 100 }],
  shows_rejected: [{ month: '2025-10', count: 15 }],
  pending_review_count: 23,
  artists_without_releases: 45,
  inactive_venues_90d: 12,
}

const mockUseGrowthMetrics = vi.fn()
const mockUseEngagementMetrics = vi.fn()
const mockUseCommunityHealth = vi.fn()
const mockUseDataQualityTrends = vi.fn()

vi.mock('@/lib/hooks/admin/useAnalytics', () => ({
  useGrowthMetrics: (...args: unknown[]) => mockUseGrowthMetrics(...args),
  useEngagementMetrics: (...args: unknown[]) => mockUseEngagementMetrics(...args),
  useCommunityHealth: (...args: unknown[]) => mockUseCommunityHealth(...args),
  useDataQualityTrends: (...args: unknown[]) => mockUseDataQualityTrends(...args),
}))

describe('AnalyticsDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()

    // Default: growth data loaded
    mockUseGrowthMetrics.mockReturnValue({
      data: mockGrowthData,
      isLoading: false,
      error: null,
    })
    mockUseEngagementMetrics.mockReturnValue({
      data: mockEngagementData,
      isLoading: false,
      error: null,
    })
    mockUseCommunityHealth.mockReturnValue({
      data: mockCommunityData,
      isLoading: false,
      error: null,
    })
    mockUseDataQualityTrends.mockReturnValue({
      data: mockDataQualityData,
      isLoading: false,
      error: null,
    })
  })

  it('renders the header', () => {
    renderWithProviders(<AnalyticsDashboard />)

    expect(screen.getByText('Analytics')).toBeInTheDocument()
    expect(
      screen.getByText('Platform growth, engagement, and data health metrics.')
    ).toBeInTheDocument()
  })

  it('renders all view selector buttons', () => {
    renderWithProviders(<AnalyticsDashboard />)

    expect(screen.getByRole('button', { name: /Growth/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Engagement/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Community Health/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Data Quality/i })).toBeInTheDocument()
  })

  it('renders month range selector on growth view', () => {
    renderWithProviders(<AnalyticsDashboard />)

    expect(screen.getByText('Range:')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '3mo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '6mo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '12mo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '24mo' })).toBeInTheDocument()
  })

  it('shows growth chart on default view', () => {
    renderWithProviders(<AnalyticsDashboard />)

    expect(screen.getByText('Entity Creation Trends')).toBeInTheDocument()
  })

  it('shows loading state when growth data is loading', () => {
    mockUseGrowthMetrics.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<AnalyticsDashboard />)

    // Loading spinner should be present (Loader2 icon)
    expect(screen.queryByText('Entity Creation Trends')).not.toBeInTheDocument()
  })

  it('shows error state when growth data fails', () => {
    mockUseGrowthMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed'),
    })

    renderWithProviders(<AnalyticsDashboard />)

    expect(screen.getByText('Failed to load growth metrics.')).toBeInTheDocument()
  })

  it('switches to engagement view on click', () => {
    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Engagement/i }))

    expect(screen.getByText('Content Curation')).toBeInTheDocument()
    expect(screen.getByText('Requests & Voting')).toBeInTheDocument()
    expect(screen.getByText('Social Engagement')).toBeInTheDocument()
  })

  it('switches to community health view on click', () => {
    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Community Health/i }))

    expect(screen.getByText('Active Contributors (30d)')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Request Fulfillment Rate')).toBeInTheDocument()
    expect(screen.getByText('72%')).toBeInTheDocument()
    expect(screen.getByText('New Collections (30d)')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
  })

  it('hides month range selector on community health view', () => {
    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Community Health/i }))

    expect(screen.queryByText('Range:')).not.toBeInTheDocument()
  })

  it('renders top contributors table in community view', () => {
    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Community Health/i }))

    expect(screen.getByText('Top Contributors (30d)')).toBeInTheDocument()
    expect(screen.getByText('Alice M.')).toBeInTheDocument()
    expect(screen.getByText('@alice')).toBeInTheDocument()
    expect(screen.getByText('50 contributions')).toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
    expect(screen.getByText('35 contributions')).toBeInTheDocument()
  })

  it('switches to data quality view on click', () => {
    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Data Quality/i }))

    expect(screen.getByText('Pending Review')).toBeInTheDocument()
    expect(screen.getByText('23')).toBeInTheDocument()
    expect(screen.getByText('Artists Without Releases')).toBeInTheDocument()
    expect(screen.getByText('45')).toBeInTheDocument()
    expect(screen.getByText('Inactive Venues (90d)')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('Show Approval Trends')).toBeInTheDocument()
  })

  it('changes month range when selector is clicked', () => {
    renderWithProviders(<AnalyticsDashboard />)

    // Click 12mo
    fireEvent.click(screen.getByRole('button', { name: '12mo' }))

    // The hook should be called with 12
    expect(mockUseGrowthMetrics).toHaveBeenCalledWith(12)
  })

  it('shows loading state for community health', () => {
    mockUseCommunityHealth.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Community Health/i }))

    expect(screen.queryByText('Active Contributors (30d)')).not.toBeInTheDocument()
  })

  it('shows error state for data quality trends', () => {
    mockUseDataQualityTrends.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed'),
    })

    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Data Quality/i }))

    expect(
      screen.getByText('Failed to load data quality trends.')
    ).toBeInTheDocument()
  })

  it('shows empty top contributors message', () => {
    mockUseCommunityHealth.mockReturnValue({
      data: {
        ...mockCommunityData,
        top_contributors: [],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<AnalyticsDashboard />)

    fireEvent.click(screen.getByRole('button', { name: /Community Health/i }))

    expect(
      screen.getByText('No contributions in the last 30 days.')
    ).toBeInTheDocument()
  })
})

describe('COLORS palette invariants (PSY-908)', () => {
  // Series rendered in the SAME chart must use distinct tokens, or two lines
  // become visually indistinguishable. Tokens are intentionally reused ACROSS
  // charts (see the COLORS doc comment), so distinctness is asserted per chart.
  const chartGroups: Record<string, (keyof typeof COLORS)[]> = {
    'Entity Creation': [
      'shows',
      'artists',
      'venues',
      'releases',
      'labels',
      'users',
    ],
    'Content Curation': ['tags_added', 'tag_votes', 'collection_items'],
    'Requests & Voting': ['requests', 'request_votes'],
    'Social Engagement': ['bookmarks', 'follows', 'revisions'],
    'Show Approval': ['approved', 'rejected'],
  }

  it.each(Object.entries(chartGroups))(
    'assigns a distinct token to every series in the %s chart',
    (_chart, keys) => {
      const tokens = keys.map((k) => COLORS[k])
      expect(new Set(tokens).size).toBe(keys.length)
    }
  )

  it('binds every series to a design-system var() token (no raw hex / recharts defaults)', () => {
    for (const value of Object.values(COLORS)) {
      expect(value).toMatch(/^var\(--[a-z0-9-]+\)$/)
    }
  })

  it('keeps approval trends semantic: approved = chart-2 (green), rejected = destructive (red)', () => {
    expect(COLORS.approved).toBe('var(--chart-2)')
    expect(COLORS.rejected).toBe('var(--destructive)')
    expect(COLORS.approved).not.toBe(COLORS.rejected)
  })

  it('interleaves cool accents into the dense Entity Creation chart (PSY-947, no all-warm regression)', () => {
    const entity = chartGroups['Entity Creation'].map((k) => COLORS[k])
    // each cool accent must appear so the 6 lines don't cluster warm
    for (const cool of ['var(--chart-6)', 'var(--chart-7)', 'var(--chart-8)']) {
      expect(entity).toContain(cool)
    }
    // every series must be a categorical chart token — not a non-categorical
    // fallback like --foreground / --muted-foreground (catches any regression,
    // not just the one retired --foreground string)
    for (const token of entity) {
      expect(token).toMatch(/^var\(--chart-[1-8]\)$/)
    }
  })

  it('gives each of the 6 Entity Creation lines a DISTINCT dash pattern (PSY-947 disambiguator)', () => {
    // The muted palette has near-hues (e.g. green vs teal), so the dash pattern
    // — not color — carries distinctness. Every entity series needs an entry and
    // all 6 patterns (incl. solid = undefined) must be unique.
    const entityKeys = chartGroups['Entity Creation']
    for (const k of entityKeys) {
      expect(Object.prototype.hasOwnProperty.call(ENTITY_DASH, k)).toBe(true)
    }
    const patterns = entityKeys.map((k) => ENTITY_DASH[k])
    expect(new Set(patterns).size).toBe(entityKeys.length)
  })
})
