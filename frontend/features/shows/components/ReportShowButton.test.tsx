import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReportShowButton } from './ReportShowButton'

vi.mock('next/navigation', () => ({
  usePathname: () => '/shows/test-show',
}))

const mockAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const mockMyShowReport = vi.fn((..._args: unknown[]) => ({
  data: { report: null },
  isLoading: false,
}))
vi.mock('../hooks/useShowReports', () => ({
  useMyShowReport: (...args: unknown[]) => mockMyShowReport(...args),
}))

vi.mock('./ReportShowDialog', () => ({
  ReportShowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="report-dialog">Report Dialog</div> : null,
}))

vi.mock('@/features/auth', () => ({
  LoginPromptDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="login-prompt">Login Prompt</div> : null,
}))

describe('ReportShowButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockMyShowReport.mockReturnValue({
      data: { report: null },
      isLoading: false,
    })
  })

  it('renders "Report Issue" button for unauthenticated user', () => {
    render(<ReportShowButton showId={1} showTitle="Test Show" />)
    expect(screen.getByRole('button', { name: /Report Issue/ })).toBeInTheDocument()
  })

  it('opens login prompt when unauthenticated user clicks report', async () => {
    const user = userEvent.setup()
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    await user.click(screen.getByRole('button', { name: /Report Issue/ }))
    expect(screen.getByTestId('login-prompt')).toBeInTheDocument()
  })

  it('does not open report dialog when unauthenticated', async () => {
    const user = userEvent.setup()
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    await user.click(screen.getByRole('button', { name: /Report Issue/ }))
    expect(screen.queryByTestId('report-dialog')).not.toBeInTheDocument()
  })

  it('renders "Report Issue" button for authenticated user who has not reported', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<ReportShowButton showId={1} showTitle="Test Show" />)
    expect(screen.getByRole('button', { name: /Report Issue/ })).toBeInTheDocument()
  })

  it('opens report dialog when authenticated user clicks report', async () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    const user = userEvent.setup()
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    await user.click(screen.getByRole('button', { name: /Report Issue/ }))
    expect(screen.getByTestId('report-dialog')).toBeInTheDocument()
  })

  it('shows "Reported" button when user has already reported', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockMyShowReport.mockReturnValue({
      data: { report: { id: 1, report_type: 'cancelled' } },
      isLoading: false,
    })
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    const reportedButton = screen.getByRole('button', { name: /Reported/ })
    expect(reportedButton).toBeInTheDocument()
    expect(reportedButton).toBeDisabled()
  })

  it('disables button while loading report status', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockMyShowReport.mockReturnValue({
      data: { report: null },
      isLoading: true,
    })
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    expect(screen.getByRole('button', { name: /Report Issue/ })).toBeDisabled()
  })

  // PSY-476: during the first render of a React Query hook, `data` is
  // actually `undefined` (not `{report: null}` as the earlier test
  // implied). The previous `hasReported = myReport?.report !== null`
  // guard evaluated `undefined !== null` → true and flashed the
  // "Reported" disabled button before real data arrived.
  it('does not flash "Reported" when query is loading with undefined data', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockMyShowReport.mockReturnValue({
      data: undefined,
      isLoading: true,
    })
    render(<ReportShowButton showId={1} showTitle="Test Show" />)

    expect(screen.queryByRole('button', { name: /^Reported$/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Report Issue/ })).toBeDisabled()
  })
})
