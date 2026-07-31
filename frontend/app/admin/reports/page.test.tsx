import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminReportsPage from './page'

// The page renders pending SHOW reports inline. The report card is mocked so
// this stays a page-level smoke test covering the loading, empty, and list
// branches. PSY-1633 removed the artist half: artist reports are entity
// reports now and are reviewed in /admin/moderation.

let mockShowReports: {
  data: { reports: { id: number; created_at: string }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminReports', () => ({
  usePendingReports: () => mockShowReports,
}))

vi.mock('@/components/admin', () => ({
  AdminEmptyState: ({ title }: { title: string }) => <h3>{title}</h3>,
}))

vi.mock('@/features/shows/admin', () => ({
  ShowReportCard: ({ report }: { report: { id: number } }) => (
    <div data-testid="show-report-card">{report.id}</div>
  ),
}))

describe('AdminReportsPage (app/admin/reports)', () => {
  beforeEach(() => {
    mockShowReports = { data: undefined, isLoading: false, error: null }
  })

  it('renders without throwing', () => {
    expect(() => render(<AdminReportsPage />)).not.toThrow()
  })

  it('renders the empty state when there are no pending reports', () => {
    mockShowReports = { data: { reports: [], total: 0 }, isLoading: false, error: null }

    render(<AdminReportsPage />)

    expect(
      screen.getByRole('heading', { name: 'No Pending Reports' })
    ).toBeInTheDocument()
  })

  it('renders show report cards with a count', () => {
    mockShowReports = {
      data: {
        reports: [
          { id: 1, created_at: '2026-04-02T00:00:00Z' },
          { id: 2, created_at: '2026-04-01T00:00:00Z' },
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    }

    render(<AdminReportsPage />)

    expect(screen.getAllByTestId('show-report-card')).toHaveLength(2)
    expect(
      screen.getByText('2 pending reports requiring review')
    ).toBeInTheDocument()
  })
})
