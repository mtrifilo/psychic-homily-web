import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { RevisionItem, RollbackResponse } from '@/lib/hooks/common/useRevisions'
import { RevisionHistory } from './RevisionHistory'

// --- Mocks ---

const mockRevisions: RevisionItem[] = [
  {
    id: 1,
    entity_type: 'artist',
    entity_id: 42,
    user_id: 10,
    user_name: 'alice',
    user_username: 'alice',
    changes: [
      { field: 'name', old_value: 'Old Name', new_value: 'New Name' },
      { field: 'city', old_value: null, new_value: 'Phoenix' },
    ],
    summary: 'Updated artist info',
    created_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(), // 5 mins ago
  },
  // Editor with no username slug — backend resolved their display name
  // through the email-prefix branch, but the row is not linkable. PSY-560.
  {
    id: 2,
    entity_type: 'artist',
    entity_id: 42,
    user_id: 20,
    user_name: 'asdf',
    user_username: null,
    changes: [
      { field: 'state', old_value: 'CA', new_value: 'AZ' },
    ],
    created_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(), // 3 days ago
  },
]

const mockUseEntityRevisions = vi.fn((..._args: unknown[]) => ({
  data: null as { revisions: RevisionItem[]; total: number } | null,
  isLoading: false,
  error: null as Error | null,
}))

const mockRollbackMutate = vi.fn()

// The mutation state the panel reads: `variables` is the revision id the last
// rollback was called with, which is how a shared mutation's result is matched
// to the one row it belongs to.
type MockRollback = {
  mutate: typeof mockRollbackMutate
  isPending: boolean
  variables?: number
  data?: RollbackResponse
  isError?: boolean
  error?: Error | null
}

const mockUseRollbackRevision = vi.fn(
  (): MockRollback => ({
    mutate: mockRollbackMutate,
    isPending: false,
  })
)

vi.mock('@/lib/hooks/common/useRevisions', async () => {
  const actual = await vi.importActual<typeof import('@/lib/hooks/common/useRevisions')>('@/lib/hooks/common/useRevisions')
  return {
    ...actual,
    useEntityRevisions: (...args: unknown[]) => mockUseEntityRevisions(...args),
    useRollbackRevision: () => mockUseRollbackRevision(),
  }
})

vi.mock('next/link', () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('RevisionHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseEntityRevisions.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    })
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
    })
  })

  it('renders collapsed by default with "History" label', () => {
    render(<RevisionHistory entityType="artist" entityId={42} />)
    expect(screen.getByText('History')).toBeInTheDocument()
  })

  it('does not show revision content when collapsed', () => {
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)
    expect(screen.queryByText('alice')).not.toBeInTheDocument()
  })

  it('shows loading spinner when expanded and loading', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: null,
      isLoading: true,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    // The Loader2 icon should be present (it has animate-spin class)
    const loaders = document.querySelectorAll('.animate-spin')
    expect(loaders.length).toBeGreaterThan(0)
  })

  it('shows error message when fetch fails', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: null,
      isLoading: false,
      error: new Error('Network error'),
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('Failed to load revision history')).toBeInTheDocument()
  })

  it('shows empty state when no revisions', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: [], total: 0 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('No edit history')).toBeInTheDocument()
  })

  it('shows total count badge when expanded with revisions', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('displays revision entries with user names', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('alice')).toBeInTheDocument()
  })

  it('links user name to profile page', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    const link = screen.getByText('alice').closest('a')
    expect(link).toHaveAttribute('href', '/users/alice')
  })

  // PSY-560: when user_username is null the byline must be plain text (no
  // /users/:username link, since it would 404). The display name itself is
  // resolved server-side through the resolveUserName chain — first/last,
  // email-prefix, "Anonymous" — so we no longer fall back to "User #N".
  it('renders display name as plain text when user_username is null', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('asdf')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'asdf' })).not.toBeInTheDocument()
    expect(screen.queryByText(/User #/)).not.toBeInTheDocument()
  })

  // PSY-1940 REPLACES PSY-560's "Anonymous" fallback here. An absent user_name
  // is now a deliberate backend decision — the author hid their contributions,
  // or their only resolvable name would be an email fragment — so the row drops
  // the byline entirely. A placeholder would assert a person the backend
  // declined to name.
  //
  // The rest of the row survives: history stays auditable, only the person goes
  // unnamed.
  it('renders no byline at all when user_name is absent', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: {
        revisions: [
          {
            id: 99,
            entity_type: 'artist',
            entity_id: 42,
            // user_id, user_name and user_username are withheld together:
            // the backend suppresses the whole credit, id included, because
            // the id alone would recover the name off another public payload.
            changes: [{ field: 'x', old_value: 'a', new_value: 'b' }],
            summary: 'Fixed the city',
            created_at: new Date().toISOString(),
          },
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.queryByText('Anonymous')).not.toBeInTheDocument()
    expect(screen.queryByText(/User #/)).not.toBeInTheDocument()
    expect(screen.getByText('just now')).toBeInTheDocument()
    expect(screen.getByText('Fixed the city')).toBeInTheDocument()
    expect(screen.getByText('1 field changed')).toBeInTheDocument()
  })

  it('shows relative time for recent revisions', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('5 minutes ago')).toBeInTheDocument()
    expect(screen.getByText('3 days ago')).toBeInTheDocument()
  })

  it('shows change count for each revision', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('2 fields changed')).toBeInTheDocument()
    expect(screen.getByText('1 field changed')).toBeInTheDocument()
  })

  it('shows summary text when available', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('Updated artist info')).toBeInTheDocument()
  })

  it('expands revision entry to show field diffs', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    // Open the section
    await user.click(screen.getByText('History'))
    // Click on alice's revision to expand
    await user.click(screen.getByText('2 fields changed'))

    // Should show field names and diffs
    expect(screen.getByText('name:')).toBeInTheDocument()
    expect(screen.getByText('Old Name')).toBeInTheDocument()
    expect(screen.getByText('New Name')).toBeInTheDocument()
    expect(screen.getByText('city:')).toBeInTheDocument()
    expect(screen.getByText('(empty)')).toBeInTheDocument()
    expect(screen.getByText('Phoenix')).toBeInTheDocument()
  })

  it('does not show rollback button for non-admin users', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} isAdmin={false} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('2 fields changed'))
    expect(screen.queryByText('Rollback')).not.toBeInTheDocument()
  })

  it('shows rollback button for admin users when expanded', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} isAdmin={true} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('2 fields changed'))
    expect(screen.getByText('Rollback')).toBeInTheDocument()
  })

  it('calls rollback.mutate with confirm when admin clicks Rollback', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} isAdmin={true} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('2 fields changed'))
    await user.click(screen.getByText('Rollback'))

    expect(confirmSpy).toHaveBeenCalledOnce()
    expect(mockRollbackMutate).toHaveBeenCalledWith(1)
    confirmSpy.mockRestore()
  })

  it('does not call rollback.mutate when admin cancels confirm', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} isAdmin={true} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('2 fields changed'))
    await user.click(screen.getByText('Rollback'))

    expect(confirmSpy).toHaveBeenCalledOnce()
    expect(mockRollbackMutate).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  it('shows "Load more" button when there are more revisions', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 30 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.getByText('Load more')).toBeInTheDocument()
  })

  it('does not show "Load more" when all revisions are loaded', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    await user.click(screen.getByText('History'))
    expect(screen.queryByText('Load more')).not.toBeInTheDocument()
  })

  it('toggles section open and closed', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={42} />)

    // Open
    await user.click(screen.getByText('History'))
    expect(screen.getByText('alice')).toBeInTheDocument()

    // Close
    await user.click(screen.getByText('History'))
    expect(screen.queryByText('alice')).not.toBeInTheDocument()
  })
})

// Test the formatValue helper function indirectly through rendered diffs
describe('RevisionHistory - formatValue edge cases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
    })
  })

  it('renders boolean values as "true" or "false"', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: {
        revisions: [{
          id: 1,
          entity_type: 'venue',
          entity_id: 1,
          user_id: 1,
          user_name: 'admin',
          changes: [{ field: 'verified', old_value: false, new_value: true }],
          created_at: new Date().toISOString(),
        }],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="venue" entityId={1} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('1 field changed'))

    expect(screen.getByText('false')).toBeInTheDocument()
    expect(screen.getByText('true')).toBeInTheDocument()
  })

  it('renders null/undefined as "(empty)"', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: {
        revisions: [{
          id: 1,
          entity_type: 'artist',
          entity_id: 1,
          user_id: 1,
          user_name: 'admin',
          changes: [{ field: 'city', old_value: null, new_value: 'Phoenix' }],
          created_at: new Date().toISOString(),
        }],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={1} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('1 field changed'))

    const empties = screen.getAllByText('(empty)')
    expect(empties.length).toBeGreaterThanOrEqual(1)
  })

  it('renders empty string as "(empty)"', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: {
        revisions: [{
          id: 1,
          entity_type: 'artist',
          entity_id: 1,
          user_id: 1,
          user_name: 'admin',
          changes: [{ field: 'city', old_value: '', new_value: 'Phoenix' }],
          created_at: new Date().toISOString(),
        }],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={1} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('1 field changed'))

    const empties = screen.getAllByText('(empty)')
    expect(empties.length).toBeGreaterThanOrEqual(1)
  })

  it('renders numeric values as strings', async () => {
    const user = userEvent.setup()
    mockUseEntityRevisions.mockReturnValue({
      data: {
        revisions: [{
          id: 1,
          entity_type: 'artist',
          entity_id: 1,
          user_id: 1,
          user_name: 'admin',
          changes: [{ field: 'position', old_value: 0, new_value: 1 }],
          created_at: new Date().toISOString(),
        }],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    render(<RevisionHistory entityType="artist" entityId={1} />)

    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('1 field changed'))

    // Use more specific selectors since '0' and '1' can match multiple elements
    const diffLines = document.querySelectorAll('.text-red-400, .text-green-400')
    const values = Array.from(diffLines).map(el => el.textContent)
    expect(values).toContain('0')
    expect(values).toContain('1')
  })
})

// A rollback restores the fields that pass the server's write gates and refuses
// the rest, so the panel has to say which ones it left alone. Without that the
// button reads as a completed undo while part of the edit is still live.
describe('RevisionHistory - partial rollback report', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseEntityRevisions.mockReturnValue({
      data: { revisions: mockRevisions, total: 2 },
      isLoading: false,
      error: null,
    })
  })

  async function expandFirstRevision() {
    const user = userEvent.setup()
    render(<RevisionHistory entityType="artist" entityId={42} isAdmin={true} />)
    await user.click(screen.getByText('History'))
    await user.click(screen.getByText('2 fields changed'))
    return user
  }

  it('names each skipped field and the reason it was refused', async () => {
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
      variables: 1,
      data: {
        success: true,
        applied_fields: ['name'],
        skipped_fields: [
          { field: 'spotify', reason: 'Spotify URL must be on spotify.com' },
        ],
      },
      isError: false,
      error: null,
    })

    await expandFirstRevision()

    expect(screen.getByText(/1 field was left unchanged/)).toBeInTheDocument()
    expect(screen.getByText('spotify')).toBeInTheDocument()
    expect(screen.getByText(/Spotify URL must be on spotify\.com/)).toBeInTheDocument()
  })

  it('reports nothing extra when every field was restored', async () => {
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
      variables: 1,
      data: { success: true, applied_fields: ['name', 'city'], skipped_fields: [] },
      isError: false,
      error: null,
    })

    await expandFirstRevision()

    expect(screen.queryByText(/left unchanged/)).not.toBeInTheDocument()
  })

  // One mutation is shared by every row, so an unmatched result is another
  // row's and must not be attributed to this one.
  it('does not report another revision\'s outcome on this row', async () => {
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
      variables: 2,
      data: {
        success: true,
        applied_fields: [],
        skipped_fields: [{ field: 'spotify', reason: 'refused' }],
      },
      isError: false,
      error: null,
    })

    await expandFirstRevision()

    expect(screen.queryByText(/left unchanged/)).not.toBeInTheDocument()
  })

  it('surfaces a rollback the server refused outright', async () => {
    mockUseRollbackRevision.mockReturnValue({
      mutate: mockRollbackMutate,
      isPending: false,
      variables: 1,
      data: undefined,
      isError: true,
      error: new Error('no field of this revision can be restored: website (bad scheme)'),
    })

    await expandFirstRevision()

    expect(screen.getByText(/Rollback failed/)).toBeInTheDocument()
    expect(screen.getByText(/no field of this revision can be restored/)).toBeInTheDocument()
  })
})
