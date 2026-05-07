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
        user_name: 'alice',
        user_username: 'alice',
        created_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText(/Last edited by/)).toBeInTheDocument()
  })

  it('links username to profile page', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'alice',
        user_username: 'alice',
        created_at: new Date().toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    const link = screen.getByText('alice').closest('a')
    expect(link).toHaveAttribute('href', '/users/alice')
  })

  // PSY-560: when the editor has no username slug, the byline must render
  // the resolved display name (first/last, email-prefix, "Anonymous") as
  // plain text — never as a link, since /users/:username would 404. Mirrors
  // CommentCard byline behavior (PSY-552).
  it('renders display name as plain text when user_username is null', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'asdf',
        user_username: null,
        created_at: new Date().toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    expect(screen.getByText('asdf')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'asdf' })).not.toBeInTheDocument()
  })

  // Display name and slug can differ — e.g. a user with username "jdoe"
  // and first name "Jane Doe" would surface "Jane Doe" as the visible
  // text but link to /users/jdoe. PSY-560.
  it('uses user_username for the link href but user_name for visible text', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'Jane Doe',
        user_username: 'jdoe',
        created_at: new Date().toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={42} />)
    const link = screen.getByText('Jane Doe').closest('a')
    expect(link).toHaveAttribute('href', '/users/jdoe')
  })

  it('shows relative time for recent edits', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'bob',
        user_username: 'bob',
        created_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="venue" entityId={10} />)
    expect(screen.getByText(/3 days ago/)).toBeInTheDocument()
  })

  it('shows "just now" for very recent edits', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'carol',
        user_username: 'carol',
        created_at: new Date(Date.now() - 10 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="festival" entityId={5} />)
    expect(screen.getByText(/just now/)).toBeInTheDocument()
  })

  it('shows hours for edits a few hours ago', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'dave',
        user_username: 'dave',
        created_at: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
      },
    })
    render(<AttributionLine entityType="artist" entityId={1} />)
    expect(screen.getByText(/2 hours ago/)).toBeInTheDocument()
  })

  it('shows date for edits older than 30 days', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'eve',
        user_username: 'eve',
        created_at: '2025-01-15T12:00:00Z',
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
        user_name: 'testuser',
        user_username: 'testuser',
        created_at: new Date().toISOString(),
      },
    })
    const { container } = render(
      <AttributionLine entityType="artist" entityId={1} />
    )
    const p = container.querySelector('p')
    expect(p).toHaveClass('text-xs', 'text-muted-foreground')
  })
})
