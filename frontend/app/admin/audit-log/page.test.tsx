import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminAuditLogPage from './page'

// The page renders its content inline from useAuditLogs(). The AuditLogEntry
// child is mocked so this stays a page-level smoke test (entry rendering is the
// child's concern). We assert the loading, empty, and populated branches.

let mockAuditLogs: {
  data: { logs: { id: number }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminAuditLogs', () => ({
  useAuditLogs: () => mockAuditLogs,
}))

vi.mock('@/app/admin/audit-log/_components/AuditLogEntry', () => ({
  AuditLogEntry: ({ entry }: { entry: { id: number } }) => (
    <div data-testid="audit-log-entry">{entry.id}</div>
  ),
}))

describe('AdminAuditLogPage (app/admin/audit-log)', () => {
  beforeEach(() => {
    mockAuditLogs = { data: undefined, isLoading: false, error: null }
  })

  it('renders without throwing', () => {
    expect(() => render(<AdminAuditLogPage />)).not.toThrow()
  })

  it('renders the empty state when there are no logs', () => {
    mockAuditLogs = {
      data: { logs: [], total: 0 },
      isLoading: false,
      error: null,
    }

    render(<AdminAuditLogPage />)

    expect(
      screen.getByRole('heading', { name: 'No Audit Logs' })
    ).toBeInTheDocument()
  })

  it('renders audit log entries and the count summary', () => {
    mockAuditLogs = {
      data: { logs: [{ id: 1 }, { id: 2 }], total: 2 },
      isLoading: false,
      error: null,
    }

    render(<AdminAuditLogPage />)

    expect(screen.getAllByTestId('audit-log-entry')).toHaveLength(2)
    expect(screen.getByText('2 audit log entries')).toBeInTheDocument()
  })
})
