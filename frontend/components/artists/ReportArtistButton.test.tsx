import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: () => '/artists/test-artist',
}))

// Mock auth context
const mockAuthContext = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock artist reports hook
const mockMyReport = vi.fn()
vi.mock('@/lib/hooks/artists/useArtistReports', () => ({
  useMyArtistReport: (artistId: number | string | null) => mockMyReport(artistId),
  useReportArtist: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

// Mock the dialog components
vi.mock('./ReportArtistDialog', () => ({
  ReportArtistDialog: ({
    open,
    artistName,
  }: {
    open: boolean
    artistName: string
    artistId: number
    onOpenChange: (open: boolean) => void
  }) =>
    open ? <div data-testid="report-dialog">Report {artistName}</div> : null,
}))

vi.mock('@/components/auth/LoginPromptDialog', () => ({
  LoginPromptDialog: ({
    open,
    title,
  }: {
    open: boolean
    title: string
    description: string
    returnTo: string
    onOpenChange: (open: boolean) => void
  }) =>
    open ? <div data-testid="login-prompt">{title}</div> : null,
}))

import { ReportArtistButton } from './ReportArtistButton'

describe('ReportArtistButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
    })
    mockMyReport.mockReturnValue({
      data: { report: null },
      isLoading: false,
    })
  })

  it('renders "Report Issue" button for unauthenticated user', () => {
    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    expect(screen.getByText('Report Issue')).toBeInTheDocument()
  })

  it('renders "Report Issue" button for authenticated user without report', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockMyReport.mockReturnValue({
      data: { report: null },
      isLoading: false,
    })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    expect(screen.getByText('Report Issue')).toBeInTheDocument()
  })

  it('shows "Reported" disabled button when user has already reported', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockMyReport.mockReturnValue({
      data: {
        report: {
          id: 1,
          artist_id: 1,
          report_type: 'inaccurate',
          status: 'pending',
        },
      },
      isLoading: false,
    })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    const button = screen.getByText('Reported')
    expect(button).toBeInTheDocument()
    expect(button.closest('button')).toBeDisabled()
  })

  it('opens login prompt for unauthenticated user on click', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: false })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )

    await user.click(screen.getByText('Report Issue'))
    expect(screen.getByTestId('login-prompt')).toBeInTheDocument()
    expect(screen.getByText('Sign in to report')).toBeInTheDocument()
  })

  it('opens report dialog for authenticated user on click', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockMyReport.mockReturnValue({
      data: { report: null },
      isLoading: false,
    })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )

    await user.click(screen.getByText('Report Issue'))
    expect(screen.getByTestId('report-dialog')).toBeInTheDocument()
    expect(screen.getByText('Report Test Artist')).toBeInTheDocument()
  })

  it('disables button while loading report status', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    // When data is undefined (loading), myReport?.report is undefined,
    // and undefined !== null evaluates to true, so hasReported is true
    // and the "Reported" disabled button is shown instead
    mockMyReport.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    // During loading, the component shows "Reported" (disabled) because
    // hasReported defaults to true when data hasn't loaded yet
    const button = screen.getByText('Reported').closest('button')
    expect(button).toBeDisabled()
  })

  it('does not query report status when unauthenticated', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: false })

    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    // Should be called with null when unauthenticated
    expect(mockMyReport).toHaveBeenCalledWith(null)
  })

  it('queries report status when authenticated', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockMyReport.mockReturnValue({
      data: { report: null },
      isLoading: false,
    })

    renderWithProviders(
      <ReportArtistButton artistId={42} artistName="Test Artist" />
    )
    expect(mockMyReport).toHaveBeenCalledWith(42)
  })

  it('has the correct title attribute', () => {
    renderWithProviders(
      <ReportArtistButton artistId={1} artistName="Test Artist" />
    )
    expect(
      screen.getByTitle('Report an issue with this artist')
    ).toBeInTheDocument()
  })
})
