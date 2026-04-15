import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { TagDetailResponse } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUseTag = vi.fn()
const mockUseTagEntities = vi.fn()
vi.mock('../hooks', () => ({
  useTag: (...args: unknown[]) => mockUseTag(...args),
  useTagEntities: (...args: unknown[]) => mockUseTagEntities(...args),
}))

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

vi.mock('next/navigation', () => ({
  usePathname: () => '/tags/test-tag',
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({
    entityName,
  }: {
    entityType: string
    entityId: number
    entityName: string
  }) => <button data-testid="notify-me-button">Notify {entityName}</button>,
}))

vi.mock('@/components/shared', () => ({
  Breadcrumb: ({
    currentPage,
  }: {
    fallback: { href: string; label: string }
    currentPage: string
  }) => <nav aria-label="Breadcrumb"><span>{currentPage}</span></nav>,
}))

import { TagDetail } from './TagDetail'

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = createQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  )
}

function makeTagDetail(overrides: Partial<TagDetailResponse> = {}): TagDetailResponse {
  return {
    id: 1,
    name: 'Rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 42,
    description: '',
    parent_id: undefined,
    parent_name: undefined,
    child_count: 0,
    aliases: [],
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('TagDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: no entities loaded
    mockUseTagEntities.mockReturnValue({
      data: { entities: [], total: 0 },
      isLoading: false,
    })
  })

  // ── Loading state ──

  it('shows loading spinner while tag is loading', () => {
    mockUseTag.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Error states ──

  it('shows "Tag Not Found" for 404 errors', () => {
    mockUseTag.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Tag not found'),
    })

    renderWithProviders(<TagDetail slug="nonexistent" />)

    expect(screen.getByText('Tag Not Found')).toBeInTheDocument()
    expect(
      screen.getByText("The tag you're looking for doesn't exist.")
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Back to Tags/ })).toHaveAttribute(
      'href',
      '/tags'
    )
  })

  it('shows generic error message for non-404 errors', () => {
    mockUseTag.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Server error'),
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Error Loading Tag')).toBeInTheDocument()
    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows "Tag Not Found" when data is null/undefined (no error)', () => {
    mockUseTag.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="ghost" />)

    expect(screen.getByText('Tag Not Found')).toBeInTheDocument()
  })

  // ── Successful render ──

  it('renders tag name as heading', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ name: 'Rock' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'Rock' })
    ).toBeInTheDocument()
  })

  it('renders category badge', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ category: 'genre' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Genre')).toBeInTheDocument()
  })

  it('renders usage count (plural)', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 42 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('42 uses')).toBeInTheDocument()
  })

  it('renders usage count (singular)', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 1 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('1 use')).toBeInTheDocument()
  })

  it('renders Official badge when is_official', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ is_official: true }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Official')).toBeInTheDocument()
  })

  it('does not render Official badge when not official', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ is_official: false }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Official')).not.toBeInTheDocument()
  })

  it('renders description when present', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ description: 'A genre of popular music.' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('A genre of popular music.')).toBeInTheDocument()
  })

  it('does not render description when empty', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ description: '' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // No description paragraph
    expect(screen.queryByText('A genre of popular music.')).not.toBeInTheDocument()
  })

  // ── Parent tag ──

  it('renders parent tag link when parent exists', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ parent_id: 5, parent_name: 'Music' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Parent Tag')).toBeInTheDocument()
    const parentLink = screen.getByRole('link', { name: /Music/ })
    expect(parentLink).toHaveAttribute('href', '/tags/5')
  })

  it('does not render parent tag section when no parent', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Parent Tag')).not.toBeInTheDocument()
  })

  // ── Child tags ──

  it('renders sub-tag count (plural)', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ child_count: 5 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Sub-tags')).toBeInTheDocument()
    expect(screen.getByText('5 sub-tags')).toBeInTheDocument()
  })

  it('renders sub-tag count (singular)', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ child_count: 1 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('1 sub-tag')).toBeInTheDocument()
  })

  it('does not render sub-tags section when child_count is 0', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ child_count: 0 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Sub-tags')).not.toBeInTheDocument()
  })

  // ── Aliases ──

  it('renders aliases when present', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ aliases: ['rock and roll', 'rock n roll'] }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Also known as')).toBeInTheDocument()
    expect(screen.getByText('rock and roll')).toBeInTheDocument()
    expect(screen.getByText('rock n roll')).toBeInTheDocument()
  })

  it('does not render aliases section when empty', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ aliases: [] }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Also known as')).not.toBeInTheDocument()
  })

  // ── Created by ──

  it('renders "Created by @username" when created_by_username is present', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ created_by_username: 'johndoe' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('@johndoe')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@johndoe' })).toHaveAttribute(
      'href',
      '/users/johndoe'
    )
  })

  it('does not render creator when created_by_username is absent', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText(/Created by/)).not.toBeInTheDocument()
  })

  // ── NotifyMeButton ──

  it('renders NotifyMeButton with correct props', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ id: 7, name: 'Punk' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="punk" />)

    expect(screen.getByTestId('notify-me-button')).toHaveTextContent('Notify Punk')
  })

  // ── Breadcrumb ──

  it('renders breadcrumb with tag name', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ name: 'Jazz' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="jazz" />)

    // "Jazz" appears in both the heading and breadcrumb
    const jazzElements = screen.getAllByText('Jazz')
    expect(jazzElements.length).toBeGreaterThanOrEqual(2)
  })

  // ── Tagged Entities ──

  it('renders tagged entities grouped by type', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 3 }),
      isLoading: false,
      error: null,
    })
    mockUseTagEntities.mockReturnValue({
      data: {
        entities: [
          { entity_type: 'artist', entity_id: 1, name: 'Radiohead', slug: 'radiohead' },
          { entity_type: 'artist', entity_id: 2, name: 'Portishead', slug: 'portishead' },
          { entity_type: 'venue', entity_id: 10, name: 'The Rebel Lounge', slug: 'the-rebel-lounge' },
        ],
        total: 3,
      },
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Tagged Entities')).toBeInTheDocument()
    // "Artists" and "Venues" appear both in the usage breakdown bar and the section headings
    expect(screen.getAllByText('Artists').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Venues').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByRole('link', { name: 'Radiohead' })).toHaveAttribute('href', '/artists/radiohead')
    expect(screen.getByRole('link', { name: 'Portishead' })).toHaveAttribute('href', '/artists/portishead')
    expect(screen.getByRole('link', { name: 'The Rebel Lounge' })).toHaveAttribute('href', '/venues/the-rebel-lounge')
  })

  it('does not render tagged entities section when usage_count is 0', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 0 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Tagged Entities')).not.toBeInTheDocument()
  })

  it('shows loading spinner while entities are loading', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 5 }),
      isLoading: false,
      error: null,
    })
    mockUseTagEntities.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // There should be a spinner in the entities section (separate from the main loading state)
    const spinners = document.querySelectorAll('.animate-spin')
    expect(spinners.length).toBeGreaterThanOrEqual(1)
  })

  // ── Usage breakdown by entity type ──

  it('renders usage breakdown bar when multiple entity types are present', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 4 }),
      isLoading: false,
      error: null,
    })
    mockUseTagEntities.mockReturnValue({
      data: {
        entities: [
          { entity_type: 'artist', entity_id: 1, name: 'Radiohead', slug: 'radiohead' },
          { entity_type: 'artist', entity_id: 2, name: 'Portishead', slug: 'portishead' },
          { entity_type: 'venue', entity_id: 10, name: 'The Rebel Lounge', slug: 'the-rebel-lounge' },
          { entity_type: 'show', entity_id: 20, name: 'Live at Rebel', slug: 'live-at-rebel' },
        ],
        total: 4,
      },
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // The breakdown bar should show counts: "2" for artists, "1" for venue, "1" for show
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('does not render usage breakdown bar with only one entity type', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ usage_count: 2 }),
      isLoading: false,
      error: null,
    })
    mockUseTagEntities.mockReturnValue({
      data: {
        entities: [
          { entity_type: 'artist', entity_id: 1, name: 'Radiohead', slug: 'radiohead' },
          { entity_type: 'artist', entity_id: 2, name: 'Portishead', slug: 'portishead' },
        ],
        total: 2,
      },
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // With only one entity type, no breakdown bar is shown — only the section heading
    expect(screen.getByText('Artists')).toBeInTheDocument()
    // The count "2" should only appear as part of the section heading "(2)"
    expect(screen.getByText('(2)')).toBeInTheDocument()
  })

  // ── Creation date ──

  it('renders creation date from created_at timestamp', () => {
    mockUseTag.mockReturnValue({
      data: makeTagDetail({ created_at: '2025-01-01T00:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // formatRelativeTime will produce a date string (e.g. "Jan 1, 2025" or "X months ago")
    // We just verify a Clock icon container is rendered alongside it
    const clockIcons = document.querySelectorAll('.lucide-clock')
    expect(clockIcons.length).toBeGreaterThanOrEqual(1)
  })
})
