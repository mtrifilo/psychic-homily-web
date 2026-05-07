import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, waitFor, screen } from '@/test/utils'

const mockPush = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseMyPendingEdits = vi.fn()
const mockUseCancelPendingEdit = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// Mock the contributions feature module — the page consumes
// `<MyPendingEditsList />`, which in turn consumes the hooks below. We mock
// the hooks (not the component) so we exercise the real list render path.
vi.mock('@/features/contributions/hooks/useMyPendingEdits', () => ({
  useMyPendingEdits: (...args: unknown[]) => mockUseMyPendingEdits(...args),
}))

vi.mock('@/features/contributions/hooks/useCancelPendingEdit', () => ({
  useCancelPendingEdit: () => mockUseCancelPendingEdit(),
}))

import SubmissionsPage from './page'

describe('SubmissionsPage (PSY-600)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCancelPendingEdit.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('redirects unauthenticated users to /auth with the correct returnTo', async () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
    })
    mockUseMyPendingEdits.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
    })

    renderWithProviders(<SubmissionsPage />)

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fsubmissions')
    })
  })

  it('renders the empty state when the user has no pending edits', async () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: 1, email: 'alice@example.com' },
    })
    mockUseMyPendingEdits.mockReturnValue({
      data: { edits: [], total: 0 },
      isLoading: false,
      isError: false,
    })

    renderWithProviders(<SubmissionsPage />)

    expect(await screen.findByText(/no pending edits yet/i)).toBeTruthy()
    // The submit-a-show shortcut still appears so contributors can flow to
    // the show form from this surface.
    expect(screen.getByRole('link', { name: /submit a show/i })).toBeTruthy()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('renders rows for each pending edit with status, entity link, and rejection reason', async () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: 1, email: 'alice@example.com' },
    })
    mockUseMyPendingEdits.mockReturnValue({
      data: {
        edits: [
          {
            id: 10,
            entity_type: 'artist',
            entity_id: 42,
            entity_name: 'Phantogram',
            entity_slug: 'phantogram',
            submitted_by: 1,
            submitter_name: 'Alice',
            submitter_username: 'alice',
            field_changes: [
              { field: 'description', old_value: 'old', new_value: 'new' },
            ],
            summary: 'fix bio typo',
            status: 'pending',
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
          {
            id: 11,
            entity_type: 'venue',
            entity_id: 7,
            entity_name: 'Valley Bar',
            entity_slug: 'valley-bar',
            submitted_by: 1,
            submitter_name: 'Alice',
            submitter_username: 'alice',
            field_changes: [
              { field: 'address', old_value: 'old', new_value: 'new' },
            ],
            summary: 'fix address',
            status: 'rejected',
            rejection_reason: 'Address looks correct on the venue website.',
            reviewed_by: 9,
            reviewer_name: 'Mod',
            reviewer_username: 'mod',
            reviewed_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        ],
        total: 2,
      },
      isLoading: false,
      isError: false,
    })

    renderWithProviders(<SubmissionsPage />)

    // Two rows
    const rows = await screen.findAllByTestId('pending-edit-row')
    expect(rows).toHaveLength(2)

    // Pending row links to /artists/phantogram (slug-based — broken with id alone)
    const artistLink = screen.getByRole('link', { name: /phantogram/i })
    expect(artistLink.getAttribute('href')).toBe('/artists/phantogram')

    // Rejected row surfaces the moderator response
    expect(
      screen.getByText(/address looks correct on the venue website/i)
    ).toBeTruthy()
    expect(screen.getByTestId('rejection-reason')).toBeTruthy()

    // Status badges visible (use getAllByText since the second row also
    // contains "rejected" elsewhere — we only care that BOTH statuses
    // surface somewhere in the rendered tree).
    expect(screen.getAllByText(/^pending$/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/^rejected$/i).length).toBeGreaterThan(0)
  })

  it('renders an error state when the pending-edits query fails', async () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: 1, email: 'alice@example.com' },
    })
    mockUseMyPendingEdits.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('boom'),
    })

    renderWithProviders(<SubmissionsPage />)

    expect(await screen.findByText(/failed to load your pending edits/i)).toBeTruthy()
  })
})
