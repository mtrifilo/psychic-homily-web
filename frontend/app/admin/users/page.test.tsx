import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminUsersPage from './page'

// The page renders a debounced search input + a list from useAdminUsers(). The
// AdminUserCard child is mocked so this stays a page-level smoke test covering
// the loading, empty, and populated branches.

let mockUsers: {
  data: { users: { id: number }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminUsers', () => ({
  useAdminUsers: () => mockUsers,
}))

vi.mock('@/app/admin/users/_components/AdminUserCard', () => ({
  AdminUserCard: ({ user }: { user: { id: number } }) => (
    <div data-testid="admin-user-card">{user.id}</div>
  ),
}))

describe('AdminUsersPage (app/admin/users)', () => {
  beforeEach(() => {
    mockUsers = { data: undefined, isLoading: false, error: null }
  })

  it('renders the search input while loading without throwing', () => {
    mockUsers = { data: undefined, isLoading: true, error: null }

    render(<AdminUsersPage />)

    expect(
      screen.getByPlaceholderText('Search by email or username...')
    ).toBeInTheDocument()
  })

  it('renders the empty state when no users match', () => {
    mockUsers = { data: { users: [], total: 0 }, isLoading: false, error: null }

    render(<AdminUsersPage />)

    expect(
      screen.getByRole('heading', { name: 'No Users Found' })
    ).toBeInTheDocument()
  })

  it('renders user cards and the count summary', () => {
    mockUsers = {
      data: { users: [{ id: 1 }, { id: 2 }], total: 2 },
      isLoading: false,
      error: null,
    }

    render(<AdminUsersPage />)

    expect(screen.getAllByTestId('admin-user-card')).toHaveLength(2)
    expect(screen.getByText('2 users')).toBeInTheDocument()
  })
})
