import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ContributionTimeline } from './ContributionTimeline'
import type { ContributionEntry } from '@/features/auth'

// Mock next/link
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

function makeEntry(overrides: Partial<ContributionEntry> = {}): ContributionEntry {
  return {
    id: 1,
    action: 'created',
    entity_type: 'show',
    entity_id: 100,
    entity_name: 'Test Show',
    created_at: new Date().toISOString(),
    source: 'web',
    ...overrides,
  }
}

describe('ContributionTimeline', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-19T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows empty state when contributions is empty', () => {
    render(<ContributionTimeline contributions={[]} />)
    expect(screen.getByText('No recent contributions.')).toBeInTheDocument()
  })

  it('renders a contribution entry with entity name', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ entity_name: 'Valley Bar', entity_type: 'venue' }),
        ]}
      />
    )
    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
  })

  it('formats unknown action text with capitalization', () => {
    render(
      <ContributionTimeline
        contributions={[makeEntry({ action: 'venue_edit_submitted' })]}
      />
    )
    expect(screen.getByText('Venue Edit Submitted')).toBeInTheDocument()
  })

  it('uses friendly labels for known actions', () => {
    render(
      <ContributionTimeline
        contributions={[makeEntry({ action: 'submit_show' })]}
      />
    )
    expect(screen.getByText('Submitted show')).toBeInTheDocument()
  })

  it('maps suggest_edit to user-friendly label', () => {
    render(
      <ContributionTimeline
        contributions={[makeEntry({ action: 'suggest_edit' })]}
      />
    )
    expect(screen.getByText('Suggested edit')).toBeInTheDocument()
  })

  it('links to entity for known entity types', () => {
    const entityTypes = ['show', 'venue', 'artist', 'release', 'label', 'festival'] as const
    for (const entityType of entityTypes) {
      const { unmount } = render(
        <ContributionTimeline
          contributions={[
            makeEntry({
              id: Math.random(),
              entity_type: entityType,
              entity_id: 42,
              entity_name: `Test ${entityType}`,
            }),
          ]}
        />
      )
      const link = screen.getByText(`Test ${entityType}`)
      expect(link.closest('a')).toHaveAttribute('href', `/${entityType}s/42`)
      unmount()
    }
  })

  it('renders entity name without link for unknown entity types', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({
            entity_type: 'unknown_type',
            entity_name: 'Some Entity',
          }),
        ]}
      />
    )
    const entityText = screen.getByText('Some Entity')
    expect(entityText.closest('a')).toBeNull()
    expect(entityText.tagName).toBe('SPAN')
  })

  it('links requests to /requests/:id', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({
            entity_type: 'request',
            entity_id: 5,
            entity_name: 'Add artist Foo',
          }),
        ]}
      />
    )
    const link = screen.getByText('Add artist Foo')
    expect(link.closest('a')).toHaveAttribute('href', '/requests/5')
  })

  it('links collections to /collection/:id', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({
            entity_type: 'collection',
            entity_id: 8,
            entity_name: 'My Favorites',
          }),
        ]}
      />
    )
    const link = screen.getByText('My Favorites')
    expect(link.closest('a')).toHaveAttribute('href', '/collection/8')
  })

  it('shows fallback label with link when entity_name is missing for a known type', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({
            entity_name: undefined,
            entity_type: 'show',
            entity_id: 55,
          }),
        ]}
      />
    )
    const fallback = screen.getByText('a show')
    expect(fallback).toBeInTheDocument()
    expect(fallback.closest('a')).toHaveAttribute('href', '/shows/55')
  })

  it('shows raw entity type when entity_name is missing for an unknown type', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({
            entity_name: undefined,
            entity_type: 'something_else',
            entity_id: 99,
          }),
        ]}
      />
    )
    expect(screen.getByText('something_else')).toBeInTheDocument()
  })

  it('formats "just now" for very recent timestamps', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-03-19T11:59:45Z' }),
        ]}
      />
    )
    expect(screen.getByText(/just now/)).toBeInTheDocument()
  })

  it('formats minutes ago', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-03-19T11:30:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/30m ago/)).toBeInTheDocument()
  })

  it('formats hours ago', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-03-19T09:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/3h ago/)).toBeInTheDocument()
  })

  it('formats days ago', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-03-17T12:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/2d ago/)).toBeInTheDocument()
  })

  it('formats weeks ago', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-03-05T12:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/2w ago/)).toBeInTheDocument()
  })

  it('formats older dates as month/day', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2026-01-15T12:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/Jan 15/)).toBeInTheDocument()
  })

  it('includes year for dates in a different year', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ created_at: '2025-06-15T12:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/Jun 15, 2025/)).toBeInTheDocument()
  })

  it('shows source when source is not "web"', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ source: 'cli', created_at: '2026-03-19T11:00:00Z' }),
        ]}
      />
    )
    expect(screen.getByText(/via cli/)).toBeInTheDocument()
  })

  it('does not show source when source is "web"', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ source: 'web', created_at: '2026-03-19T11:00:00Z' }),
        ]}
      />
    )
    expect(screen.queryByText(/via web/)).not.toBeInTheDocument()
  })

  it('does not show source when source is "audit_log"', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ source: 'audit_log', created_at: '2026-03-19T11:00:00Z' }),
        ]}
      />
    )
    expect(screen.queryByText(/via audit_log/)).not.toBeInTheDocument()
  })

  it('does not show source when source is "submission"', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ source: 'submission', created_at: '2026-03-19T11:00:00Z' }),
        ]}
      />
    )
    expect(screen.queryByText(/via submission/)).not.toBeInTheDocument()
  })

  it('renders multiple entries', () => {
    render(
      <ContributionTimeline
        contributions={[
          makeEntry({ id: 1, entity_name: 'Show A' }),
          makeEntry({ id: 2, entity_name: 'Venue B', entity_type: 'venue' }),
          makeEntry({ id: 3, entity_name: 'Artist C', entity_type: 'artist' }),
        ]}
      />
    )
    expect(screen.getByText('Show A')).toBeInTheDocument()
    expect(screen.getByText('Venue B')).toBeInTheDocument()
    expect(screen.getByText('Artist C')).toBeInTheDocument()
  })
})
