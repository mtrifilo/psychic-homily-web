import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ProfileFollowing } from './ProfileFollowing'
import { ProfileFieldNotes } from './ProfileFieldNotes'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

const mockUseUserFollowing = vi.fn()
const mockUseUserFieldNotes = vi.fn()

vi.mock('@/features/auth', () => ({
  useUserFollowing: (username: string, opts: unknown) =>
    mockUseUserFollowing(username, opts),
  useUserFieldNotes: (username: string, opts: unknown) =>
    mockUseUserFieldNotes(username, opts),
}))

function notFound(): Error {
  const err = new Error('Not found')
  Object.assign(err, { status: 404 })
  return err
}

beforeEach(() => {
  vi.clearAllMocks()
})

// ============================================================================
// ProfileFollowing
// ============================================================================

describe('ProfileFollowing', () => {
  it('renders followed entities grouped by type with links and counts', () => {
    mockUseUserFollowing.mockReturnValue({
      data: {
        following: [
          {
            entity_type: 'artist',
            entity_id: 1,
            name: 'Just Mustard',
            slug: 'just-mustard',
            followed_at: '2026-01-01T00:00:00Z',
          },
          {
            entity_type: 'artist',
            entity_id: 2,
            name: 'Wisp',
            slug: 'wisp',
            followed_at: '2026-01-02T00:00:00Z',
          },
          {
            entity_type: 'venue',
            entity_id: 3,
            name: 'Valley Bar',
            slug: 'valley-bar',
            followed_at: '2026-01-03T00:00:00Z',
          },
          {
            entity_type: 'tag',
            entity_id: 4,
            name: 'shoegaze',
            slug: 'shoegaze',
            followed_at: '2026-01-04T00:00:00Z',
          },
          {
            entity_type: 'tag',
            entity_id: 5,
            name: 'noise-rock',
            slug: 'noise-rock',
            followed_at: '2026-01-05T00:00:00Z',
          },
        ],
        total: 5,
        limit: 100,
        offset: 0,
      },
      error: null,
    })

    renderWithProviders(<ProfileFollowing username="alice" />)

    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.getByText('Artists')).toBeInTheDocument()
    expect(screen.getByText('Venues')).toBeInTheDocument()
    expect(screen.getByText('Tags')).toBeInTheDocument()
    // No labels/festivals followed → those rows are omitted entirely.
    expect(screen.queryByText('Labels')).not.toBeInTheDocument()
    expect(screen.queryByText('Festivals')).not.toBeInTheDocument()

    expect(
      screen.getByRole('link', { name: 'Just Mustard' })
    ).toHaveAttribute('href', '/artists/just-mustard')
    expect(screen.getByRole('link', { name: 'Valley Bar' })).toHaveAttribute(
      'href',
      '/venues/valley-bar'
    )
    expect(screen.getByRole('link', { name: 'shoegaze' })).toHaveAttribute(
      'href',
      '/tags/shoegaze'
    )
    expect(screen.getByRole('link', { name: 'noise-rock' })).toHaveAttribute(
      'href',
      '/tags/noise-rock'
    )
  })

  it('renders a count line (no names) for count_only privacy', () => {
    mockUseUserFollowing.mockReturnValue({
      data: { following: [], total: 42, limit: 100, offset: 0 },
      error: null,
    })

    renderWithProviders(<ProfileFollowing username="alice" />)
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText(/lists hidden by this member/)).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('renders nothing when hidden by privacy (404)', () => {
    mockUseUserFollowing.mockReturnValue({ data: undefined, error: notFound() })
    const { container } = renderWithProviders(
      <ProfileFollowing username="alice" />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows a Manage action linking to the Library follows view for the owner only', () => {
    mockUseUserFollowing.mockReturnValue({
      data: {
        following: [
          {
            entity_type: 'artist',
            entity_id: 1,
            name: 'Just Mustard',
            slug: 'just-mustard',
          },
        ],
        total: 1,
      },
      error: null,
    })

    const { unmount } = renderWithProviders(
      <ProfileFollowing username="alice" isOwner />
    )
    const manage = screen.getByRole('link', { name: /manage who you follow/i })
    expect(manage).toHaveAttribute('href', '/library?tab=artists')
    unmount()

    renderWithProviders(<ProfileFollowing username="alice" />)
    expect(
      screen.queryByRole('link', { name: /manage who you follow/i })
    ).not.toBeInTheDocument()
  })

  it('renders nothing when a visitor follows nothing', () => {
    mockUseUserFollowing.mockReturnValue({
      data: { following: [], total: 0, limit: 100, offset: 0 },
      error: null,
    })
    const { container } = renderWithProviders(
      <ProfileFollowing username="alice" />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the owner a browse prompt when empty', () => {
    mockUseUserFollowing.mockReturnValue({
      data: { following: [], total: 0, limit: 100, offset: 0 },
      error: null,
    })

    renderWithProviders(<ProfileFollowing username="alice" isOwner />)
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(
      screen.getByText(/Shape your taste graph and get show alerts/)
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^browse$/i })).toHaveAttribute(
      'href',
      '/artists'
    )
    expect(
      screen.queryByRole('link', { name: /manage who you follow/i })
    ).not.toBeInTheDocument()
  })
})

// ============================================================================
// ProfileFieldNotes
// ============================================================================

describe('ProfileFieldNotes', () => {
  it('renders note rows titled by show with a body excerpt (no star ratings)', () => {
    mockUseUserFieldNotes.mockReturnValue({
      data: {
        field_notes: [
          {
            id: 1,
            entity_type: 'show',
            entity_id: 9,
            kind: 'field_note',
            user_id: 5,
            author_name: 'alice',
            author_username: 'alice',
            body: 'A wall of sound that rearranged my ribcage.',
            body_html: '<p>A wall of sound that rearranged my ribcage.</p>',
            created_at: '2026-05-18T00:00:00Z',
            updated_at: '2026-05-18T00:00:00Z',
            show_title: 'Just Mustard at Valley Bar',
            show_slug: 'just-mustard-valley-bar',
          },
        ],
        total: 1,
        limit: 5,
        offset: 0,
      },
      error: null,
    })

    renderWithProviders(<ProfileFieldNotes username="alice" />)

    expect(screen.getByText('Field notes & reviews')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Just Mustard at Valley Bar' })
    ).toHaveAttribute('href', '/shows/just-mustard-valley-bar#field-notes')
    expect(
      screen.getByText(/wall of sound that rearranged my ribcage/)
    ).toBeInTheDocument()
    // No star ratings by design (2026-06-09 decision).
    expect(screen.queryByText(/★/)).not.toBeInTheDocument()
  })

  it('expands in place via "View all", revealing the fetched notes', () => {
    const field_notes = Array.from({ length: 7 }, (_, i) => ({
      id: i + 1,
      show_slug: `show-${i + 1}`,
      show_title: `Show ${i + 1}`,
      body: 'A wall of sound.',
    }))
    mockUseUserFieldNotes.mockReturnValue({
      data: { field_notes, total: 7 },
      error: null,
    })

    renderWithProviders(<ProfileFieldNotes username="alice" />)
    expect(mockUseUserFieldNotes).toHaveBeenLastCalledWith('alice', {
      limit: 100,
    })
    expect(screen.getAllByRole('link')).toHaveLength(5)
    fireEvent.click(
      screen.getByRole('button', { name: /view all 7 field notes/i })
    )
    expect(screen.getAllByRole('link')).toHaveLength(7)
  })

  it('renders nothing when the user has no field notes', () => {
    mockUseUserFieldNotes.mockReturnValue({
      data: { field_notes: [], total: 0, limit: 5, offset: 0 },
      error: null,
    })
    const { container } = renderWithProviders(
      <ProfileFieldNotes username="alice" />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing on error', () => {
    mockUseUserFieldNotes.mockReturnValue({
      data: undefined,
      error: new Error('boom'),
    })
    const { container } = renderWithProviders(
      <ProfileFieldNotes username="alice" />
    )
    expect(container).toBeEmptyDOMElement()
  })
})
