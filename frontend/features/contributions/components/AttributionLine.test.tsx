import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AttributionLine } from './AttributionLine'

// --- Mocks ---

const mockUseEntityAttribution = vi.fn()

vi.mock('../hooks/useEntityAttribution', () => ({
  useEntityAttribution: (...args: unknown[]) => mockUseEntityAttribution(...args),
}))

vi.mock('next/link', () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('AttributionLine', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when no attribution data', () => {
    mockUseEntityAttribution.mockReturnValue({ data: null })
    const { container } = render(
      <AttributionLine entityType="artist" entityId={42} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders "Last edited by" with username', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'alice',
        createdAt: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText(/Last edited by/)).toBeInTheDocument()
  })

  it('links username to profile page', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'alice',
        createdAt: new Date().toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    const link = screen.getByText('alice').closest('a')
    expect(link).toHaveAttribute('href', '/users/alice')
  })

  it('shows relative time for recent edits', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'bob',
        createdAt: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="venue" entityId={10} />)
    expect(screen.getByText(/3 days ago/)).toBeInTheDocument()
  })

  it('shows "just now" for very recent edits', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'carol',
        createdAt: new Date(Date.now() - 10 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="festival" entityId={5} />)
    expect(screen.getByText(/just now/)).toBeInTheDocument()
  })

  it('shows hours for edits a few hours ago', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'dave',
        createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={1} />)
    expect(screen.getByText(/2 hours ago/)).toBeInTheDocument()
  })

  it('shows date for edits older than 30 days', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'eve',
        createdAt: '2025-01-15T12:00:00Z',
      },
    })
    render(<AttributionLine entityType="artist" entityId={1} />)
    // Should show a formatted date like "Jan 15, 2025"
    expect(screen.getByText(/Jan 15, 2025/)).toBeInTheDocument()
  })

  it('passes entity type and id to hook', () => {
    mockUseEntityAttribution.mockReturnValue({ data: null })
    render(<AttributionLine entityType="venue" entityId={99} />)
    expect(mockUseEntityAttribution).toHaveBeenCalledWith('venue', 99)
  })

  it('has muted styling', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        userName: 'testuser',
        createdAt: new Date().toISOString(),
      },
    })
    const { container } = render(
      <AttributionLine entityType="artist" entityId={1} />
    )
    const p = container.querySelector('p')
    expect(p).toHaveClass('text-xs', 'text-muted-foreground')
  })
})
