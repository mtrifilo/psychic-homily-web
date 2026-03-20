import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ArtistReportCard } from './ArtistReportCard'
import type { ArtistReportResponse } from '@/features/artists'

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
vi.mock('./DismissArtistReportDialog', () => ({
  DismissArtistReportDialog: ({
    open,
  }: {
    open: boolean
    onOpenChange: (v: boolean) => void
  }) => {
    return open ? <div data-testid="dismiss-dialog">Dismiss Dialog</div> : null
  },
}))

vi.mock('./ResolveArtistReportDialog', () => ({
  ResolveArtistReportDialog: ({
    open,
  }: {
    open: boolean
    onOpenChange: (v: boolean) => void
  }) => {
    return open ? (
      <div data-testid="resolve-dialog">Resolve Dialog</div>
    ) : null
  },
}))

function makeReport(
  overrides: Partial<ArtistReportResponse> = {}
): ArtistReportResponse {
  return {
    id: 1,
    artist_id: 10,
    report_type: 'inaccurate',
    status: 'pending',
    created_at: '2024-06-15T12:00:00Z',
    updated_at: '2024-06-15T12:00:00Z',
    artist: {
      id: 10,
      name: 'Test Artist',
      slug: 'test-artist',
    },
    ...overrides,
  }
}

describe('ArtistReportCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders artist name', () => {
    render(<ArtistReportCard report={makeReport()} />)
    expect(screen.getByText('Test Artist')).toBeInTheDocument()
  })

  it('shows "Unknown Artist" when artist info is missing', () => {
    render(<ArtistReportCard report={makeReport({ artist: undefined })} />)
    expect(screen.getByText('Unknown Artist')).toBeInTheDocument()
  })

  it('renders link to artist page when slug is available', () => {
    render(<ArtistReportCard report={makeReport()} />)
    const link = screen.getByTitle('View artist')
    expect(link).toHaveAttribute('href', '/artists/test-artist')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('does not render artist link when slug is missing', () => {
    render(
      <ArtistReportCard
        report={makeReport({
          artist: { id: 10, name: 'No Slug Artist', slug: '' },
        })}
      />
    )
    expect(screen.queryByTitle('View artist')).not.toBeInTheDocument()
  })

  it('shows "Artist" entity badge', () => {
    render(<ArtistReportCard report={makeReport()} />)
    expect(screen.getByText('Artist')).toBeInTheDocument()
  })

  it('shows "Inaccurate" badge for inaccurate report type', () => {
    render(
      <ArtistReportCard report={makeReport({ report_type: 'inaccurate' })} />
    )
    expect(screen.getByText('Inaccurate')).toBeInTheDocument()
  })

  it('shows "Removal Request" badge for removal_request report type', () => {
    render(
      <ArtistReportCard
        report={makeReport({ report_type: 'removal_request' })}
      />
    )
    expect(screen.getByText('Removal Request')).toBeInTheDocument()
  })

  it('uses raw type string for unknown report types', () => {
    render(
      <ArtistReportCard
        report={makeReport({ report_type: 'custom_type' as any })}
      />
    )
    expect(screen.getByText('custom_type')).toBeInTheDocument()
  })

  it('renders report details when provided', () => {
    render(
      <ArtistReportCard
        report={makeReport({ details: 'Wrong genre listed' })}
      />
    )
    expect(screen.getByText('Wrong genre listed')).toBeInTheDocument()
    expect(screen.getByText("Reporter's Details:")).toBeInTheDocument()
  })

  it('does not render details section when details are null', () => {
    render(<ArtistReportCard report={makeReport({ details: null })} />)
    expect(screen.queryByText("Reporter's Details:")).not.toBeInTheDocument()
  })

  it('renders Resolve and Dismiss buttons', () => {
    render(<ArtistReportCard report={makeReport()} />)
    expect(
      screen.getByRole('button', { name: /Resolve/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Dismiss/i })
    ).toBeInTheDocument()
  })

  it('opens resolve dialog when Resolve is clicked', async () => {
    const user = userEvent.setup()
    render(<ArtistReportCard report={makeReport()} />)

    await user.click(screen.getByRole('button', { name: /Resolve/i }))

    expect(screen.getByTestId('resolve-dialog')).toBeInTheDocument()
  })

  it('opens dismiss dialog when Dismiss is clicked', async () => {
    const user = userEvent.setup()
    render(<ArtistReportCard report={makeReport()} />)

    await user.click(screen.getByRole('button', { name: /Dismiss/i }))

    expect(screen.getByTestId('dismiss-dialog')).toBeInTheDocument()
  })
})
