import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import PendingShowsPage from './page'

// The page renders its header + content inline and toggles between a pending
// and rejected view. Cards and the reject dialog are mocked so this stays a
// page-level smoke test; we assert the header and the empty-state branch.

let mockPending: {
  data: { shows: { id: number; venues: { name: string }[]; source?: string }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}
let mockRejected: {
  data: { shows: { id: number }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  usePendingShows: () => mockPending,
  useRejectedShows: () => mockRejected,
  useBatchApproveShows: () => ({ mutate: vi.fn(), isPending: false }),
  useBatchRejectShows: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: { is_admin: true } }),
}))

vi.mock('@/components/admin', () => ({
  PendingShowCard: ({ show }: { show: { id: number } }) => (
    <div data-testid="pending-show-card">{show.id}</div>
  ),
  RejectedShowCard: ({ show }: { show: { id: number } }) => (
    <div data-testid="rejected-show-card">{show.id}</div>
  ),
}))

vi.mock('@/components/admin/BatchRejectDialog', () => ({
  BatchRejectDialog: (): null => null,
}))

describe('PendingShowsPage (app/admin/pending-shows)', () => {
  beforeEach(() => {
    mockPending = { data: undefined, isLoading: false, error: null }
    mockRejected = { data: undefined, isLoading: false, error: null }
  })

  it('renders the Show Review heading without throwing', () => {
    render(<PendingShowsPage />)

    expect(
      screen.getByRole('heading', { name: 'Show Review' })
    ).toBeInTheDocument()
  })

  it('renders the no-pending-shows empty state', () => {
    mockPending = { data: { shows: [], total: 0 }, isLoading: false, error: null }
    mockRejected = { data: { shows: [], total: 0 }, isLoading: false, error: null }

    render(<PendingShowsPage />)

    expect(
      screen.getByRole('heading', { name: 'No Pending Shows' })
    ).toBeInTheDocument()
  })

  it('renders a pending show card when shows are present', () => {
    mockPending = {
      data: { shows: [{ id: 1, venues: [{ name: 'The Venue' }], source: 'user' }], total: 1 },
      isLoading: false,
      error: null,
    }
    mockRejected = { data: { shows: [], total: 0 }, isLoading: false, error: null }

    render(<PendingShowsPage />)

    expect(screen.getByTestId('pending-show-card')).toBeInTheDocument()
  })
})
