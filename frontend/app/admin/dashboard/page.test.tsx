import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import DashboardPage from './page'

// DashboardPage is a thin wrapper that forwards onNavigate to AdminDashboard.
// The dashboard internals are covered by app/admin/page.test.tsx (which mounts
// the real AdminDashboard with stats hooks mocked); here we only assert the
// wrapper renders its child and threads the prop through.

const mockAdminDashboard = vi.fn()

vi.mock('@/app/admin/dashboard/_components/AdminDashboard', () => ({
  AdminDashboard: (props: { onNavigate?: (tab: string) => void }) => {
    mockAdminDashboard(props)
    return <div data-testid="admin-dashboard" />
  },
}))

describe('DashboardPage (app/admin/dashboard)', () => {
  it('renders AdminDashboard', () => {
    render(<DashboardPage />)

    expect(screen.getByTestId('admin-dashboard')).toBeInTheDocument()
  })

  it('forwards the onNavigate handler to AdminDashboard', () => {
    const onNavigate = vi.fn()
    render(<DashboardPage onNavigate={onNavigate} />)

    expect(mockAdminDashboard).toHaveBeenCalledWith(
      expect.objectContaining({ onNavigate })
    )
  })
})
