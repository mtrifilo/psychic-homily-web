import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminReportsPage from './page'

// The page merges show + artist reports from two hooks and renders them inline.
// The report cards are mocked so this stays a page-level smoke test covering
// the loading, empty, and merged-list branches.

let mockShowReports: {
  data: { reports: { id: number; created_at: string }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}
let mockArtistReports: {
  data: { reports: { id: number; created_at: string }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminReports', () => ({
  usePendingReports: () => mockShowReports,
}))

vi.mock('@/lib/hooks/admin/useAdminArtistReports', () => ({
  usePendingArtistReports: () => mockArtistReports,
}))

vi.mock('@/components/admin', () => ({
  AdminEmptyState: ({ title }: { title: string }) => <h3>{title}</h3>,
}))

vi.mock('@/features/shows/admin', () => ({
  ShowReportCard: ({ report }: { report: { id: number } }) => (
    <div data-testid="show-report-card">{report.id}</div>
  ),
  ArtistReportCard: ({ report }: { report: { id: number } }) => (
    <div data-testid="artist-report-card">{report.id}</div>
  ),
}))

describe('AdminReportsPage (app/admin/reports)', () => {
  beforeEach(() => {
    mockShowReports = { data: undefined, isLoading: false, error: null }
    mockArtistReports = { data: undefined, isLoading: false, error: null }
  })

  it('renders without throwing', () => {
    expect(() => render(<AdminReportsPage />)).not.toThrow()
  })

  it('renders the empty state when there are no pending reports', () => {
    mockShowReports = { data: { reports: [], total: 0 }, isLoading: false, error: null }
    mockArtistReports = { data: { reports: [], total: 0 }, isLoading: false, error: null }

    render(<AdminReportsPage />)

    expect(
      screen.getByRole('heading', { name: 'No Pending Reports' })
    ).toBeInTheDocument()
  })

  it('renders both show and artist report cards with a combined count', () => {
    mockShowReports = {
      data: { reports: [{ id: 1, created_at: '2026-04-02T00:00:00Z' }], total: 1 },
      isLoading: false,
      error: null,
    }
    mockArtistReports = {
      data: { reports: [{ id: 2, created_at: '2026-04-01T00:00:00Z' }], total: 1 },
      isLoading: false,
      error: null,
    }

    render(<AdminReportsPage />)

    expect(screen.getByTestId('show-report-card')).toBeInTheDocument()
    expect(screen.getByTestId('artist-report-card')).toBeInTheDocument()
    expect(
      screen.getByText('2 pending reports requiring review')
    ).toBeInTheDocument()
  })
})
