import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowReportCard } from './ShowReportCard'
import type { ShowReportResponse } from '@/features/shows'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

// Mock the dialog sub-components to avoid deep dependency trees
vi.mock('./DismissReportDialog', () => ({
  DismissReportDialog: ({
    open,
  }: {
    open: boolean
    onOpenChange: (v: boolean) => void
  }) => {
    return open ? <div data-testid="dismiss-dialog">Dismiss Dialog</div> : null
  },
}))

vi.mock('./ResolveReportDialog', () => ({
  ResolveReportDialog: ({
    open,
  }: {
    open: boolean
    onOpenChange: (v: boolean) => void
  }) => {
    return open ? <div data-testid="resolve-dialog">Resolve Dialog</div> : null
  },
}))

function makeReport(
  overrides: Partial<ShowReportResponse> = {}
): ShowReportResponse {
  return {
    id: 1,
    show_id: 10,
    report_type: 'cancelled',
    status: 'pending',
    created_at: '2024-06-15T12:00:00Z',
    updated_at: '2024-06-15T12:00:00Z',
    show: {
      id: 10,
      title: 'Summer Festival',
      slug: 'summer-festival',
      event_date: '2026-07-04T20:00:00Z',
      city: 'Phoenix',
      state: 'AZ',
    },
    ...overrides,
  }
}

describe('ShowReportCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders show title', () => {
    render(<ShowReportCard report={makeReport()} />)
    expect(screen.getByText('Summer Festival')).toBeInTheDocument()
  })

  it('shows "Unknown Show" when show info is missing', () => {
    render(<ShowReportCard report={makeReport({ show: undefined })} />)
    expect(screen.getByText('Unknown Show')).toBeInTheDocument()
  })

  it('renders link to show page when slug is available', () => {
    render(<ShowReportCard report={makeReport()} />)
    const link = screen.getByTitle('View show')
    expect(link).toHaveAttribute('href', '/shows/summer-festival')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('does not render show link when slug is missing', () => {
    render(
      <ShowReportCard
        report={makeReport({
          show: {
            id: 10,
            title: 'Test',
            slug: '',
            event_date: '2026-04-15T20:00:00Z',
          },
        })}
      />
    )
    expect(screen.queryByTitle('View show')).not.toBeInTheDocument()
  })

  it('shows "Cancelled" badge for cancelled report type', () => {
    render(<ShowReportCard report={makeReport({ report_type: 'cancelled' })} />)
    expect(screen.getByText('Cancelled')).toBeInTheDocument()
  })

  it('shows "Sold Out" badge for sold_out report type', () => {
    render(<ShowReportCard report={makeReport({ report_type: 'sold_out' })} />)
    expect(screen.getByText('Sold Out')).toBeInTheDocument()
  })

  it('shows "Inaccurate" badge for inaccurate report type', () => {
    render(
      <ShowReportCard report={makeReport({ report_type: 'inaccurate' })} />
    )
    expect(screen.getByText('Inaccurate')).toBeInTheDocument()
  })

  it('uses raw type string for unknown report types', () => {
    render(
      <ShowReportCard
        report={makeReport({ report_type: 'custom_type' as any })}
      />
    )
    expect(screen.getByText('custom_type')).toBeInTheDocument()
  })

  it('renders city and state when available', () => {
    render(<ShowReportCard report={makeReport()} />)
    expect(screen.getByText('(Phoenix, AZ)')).toBeInTheDocument()
  })

  it('renders report details when provided', () => {
    render(
      <ShowReportCard
        report={makeReport({ details: 'The show was cancelled by the venue' })}
      />
    )
    expect(
      screen.getByText('The show was cancelled by the venue')
    ).toBeInTheDocument()
    expect(screen.getByText("Reporter's Details:")).toBeInTheDocument()
  })

  it('does not render details section when details are null', () => {
    render(<ShowReportCard report={makeReport({ details: null })} />)
    expect(screen.queryByText("Reporter's Details:")).not.toBeInTheDocument()
  })

  it('renders Resolve and Dismiss buttons', () => {
    render(<ShowReportCard report={makeReport()} />)
    expect(
      screen.getByRole('button', { name: /Resolve/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Dismiss/i })
    ).toBeInTheDocument()
  })

  it('opens resolve dialog when Resolve is clicked', async () => {
    const user = userEvent.setup()
    render(<ShowReportCard report={makeReport()} />)

    await user.click(screen.getByRole('button', { name: /Resolve/i }))

    expect(screen.getByTestId('resolve-dialog')).toBeInTheDocument()
  })

  it('opens dismiss dialog when Dismiss is clicked', async () => {
    const user = userEvent.setup()
    render(<ShowReportCard report={makeReport()} />)

    await user.click(screen.getByRole('button', { name: /Dismiss/i }))

    expect(screen.getByTestId('dismiss-dialog')).toBeInTheDocument()
  })
})
