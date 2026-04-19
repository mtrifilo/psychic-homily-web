import React, { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { TagEnrichedDetailResponse } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUseTagDetail = vi.fn()
const mockUseTagEntities = vi.fn()
vi.mock('../hooks', () => ({
  useTagDetail: (...args: unknown[]) => mockUseTagDetail(...args),
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

function makeTagDetail(
  overrides: Partial<TagEnrichedDetailResponse> = {}
): TagEnrichedDetailResponse {
  return {
    // thin fields
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
    // enriched fields
    description_html: '',
    parent: null,
    children: [],
    usage_breakdown: {
      artist: 0,
      venue: 0,
      show: 0,
      release: 0,
      label: 0,
      festival: 0,
    },
    top_contributors: [],
    created_by: null,
    related_tags: [],
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
    mockUseTagDetail.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Regression: loading → success transition (PSY-447) ──
  // Rules of Hooks violation: earlier versions called useMemo below the
  // early returns for loading/error/!tag, so the hook count changed when
  // data arrived (tag went from undefined → populated). In production
  // React logs "change in the order of Hooks" / "Rendered more hooks than
  // during the previous render" and the error boundary renders a 500.
  //
  // The other tests all pass with the broken code because the mocked
  // `useTagDetail` does not call any real React hooks, so when the
  // component body goes from "0 hooks (early return)" to "1 hook (useMemo)",
  // React has nothing to compare against. In production the real
  // `useTagDetail` (via TanStack Query's `useQuery`) calls several real
  // hooks before the component's own early return, so the mismatch is
  // detected.
  //
  // This regression test makes the mock call a real React hook (`useState`)
  // so that the 0-to-1 transition in the component body becomes a
  // 1-to-2 transition, which is what React's hook-tracker can detect.
  it('renders without hook-order errors during the loading → success transition', () => {
    // Custom mock implementation that calls a real React hook. This
    // mimics TanStack Query's internal hook calls so React's hook-order
    // tracker sees the true mismatch introduced by hooks below an
    // early return.
    let dataState: TagEnrichedDetailResponse | undefined = undefined
    let isLoadingState = true
    mockUseTagDetail.mockImplementation(() => {
      // Real React hook — ensures the number of hooks this "replacement"
      // contributes is stable across renders.
      useState(0)
      return { data: dataState, isLoading: isLoadingState, error: null }
    })

    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    // Initial render: loading
    const queryClient = createQueryClient()
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <TagDetail slug="shoegaze" />
      </QueryClientProvider>
    )

    // Transition to populated data — this is what triggered the
    // hook-order violation in production.
    dataState = makeTagDetail({
      name: 'Shoegaze',
      usage_count: 18,
      usage_breakdown: {
        artist: 15,
        venue: 0,
        show: 3,
        release: 0,
        label: 0,
        festival: 0,
      },
    })
    isLoadingState = false

    let threwDuringRerender: Error | null = null
    try {
      rerender(
        <QueryClientProvider client={queryClient}>
          <TagDetail slug="shoegaze" />
        </QueryClientProvider>
      )
    } catch (e) {
      threwDuringRerender = e as Error
    }

    // A hook-order violation throws during render with a message like
    // "Rendered more hooks than during the previous render." or
    // "change in the order of Hooks". React also logs a dev-only
    // console.error about it.
    const allErrorOutput = [
      ...(threwDuringRerender ? [threwDuringRerender.message] : []),
      ...errorSpy.mock.calls.map(([msg]) =>
        typeof msg === 'string' ? msg : ''
      ),
    ]
    const hookErrors = allErrorOutput.filter(
      (msg) =>
        msg.includes('change in the order of Hooks') ||
        msg.includes('Rendered more hooks than during the previous render') ||
        msg.includes('Rendered fewer hooks than expected')
    )
    expect(hookErrors).toEqual([])
    expect(threwDuringRerender).toBeNull()

    // Sanity check: populated content actually renders after the transition.
    expect(
      screen.getByRole('heading', { level: 1, name: 'Shoegaze' })
    ).toBeInTheDocument()

    errorSpy.mockRestore()
  })

  // ── Error states ──

  it('shows "Tag Not Found" for 404 errors', () => {
    mockUseTagDetail.mockReturnValue({
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
    mockUseTagDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Server error'),
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Error Loading Tag')).toBeInTheDocument()
    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows "Tag Not Found" when data is null/undefined (no error)', () => {
    mockUseTagDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="ghost" />)

    expect(screen.getByText('Tag Not Found')).toBeInTheDocument()
  })

  // ── Core header ──

  it('renders tag name as heading', () => {
    mockUseTagDetail.mockReturnValue({
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
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ category: 'genre' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Genre')).toBeInTheDocument()
  })

  it('renders usage count (plural)', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 42 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('42 uses')).toBeInTheDocument()
  })

  it('renders usage count (singular)', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 1 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('1 use')).toBeInTheDocument()
  })

  it('renders Official badge when is_official', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ is_official: true }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Official')).toBeInTheDocument()
  })

  it('does not render Official badge when not official', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ is_official: false }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Official')).not.toBeInTheDocument()
  })

  // ── Description (markdown) ──

  it('renders description HTML when description_html is present', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        description_html: '<p>A genre of <strong>popular</strong> music.</p>',
        description: 'A genre of **popular** music.',
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const desc = screen.getByTestId('tag-description')
    expect(desc).toBeInTheDocument()
    expect(desc.innerHTML).toContain('<strong>popular</strong>')
  })

  it('falls back to plain description when description_html is empty', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        description: 'Plain text fallback.',
        description_html: '',
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('Plain text fallback.')).toBeInTheDocument()
    expect(screen.queryByTestId('tag-description')).not.toBeInTheDocument()
  })

  it('does not render description when empty', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ description: '', description_html: '' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByTestId('tag-description')).not.toBeInTheDocument()
  })

  // ── Parent / children hierarchy (genre only) ──

  it('renders parent tag pill for genre tags with a parent', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        category: 'genre',
        parent: {
          id: 5,
          name: 'Music',
          slug: 'music',
          category: 'genre',
          is_official: false,
          usage_count: 100,
        },
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByTestId('tag-hierarchy')).toBeInTheDocument()
    expect(screen.getByText('Parent')).toBeInTheDocument()
    const parentLink = screen.getByRole('link', { name: /Music/ })
    expect(parentLink).toHaveAttribute('href', '/tags/music')
  })

  it('renders children pills for genre tags with children', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        category: 'genre',
        children: [
          { id: 11, name: 'dream-pop', slug: 'dream-pop', category: 'genre', is_official: false, usage_count: 4 },
          { id: 12, name: 'nu-gaze', slug: 'nu-gaze', category: 'genre', is_official: false, usage_count: 2 },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)

    expect(screen.getByText('Children (2)')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /dream-pop/ })).toHaveAttribute(
      'href',
      '/tags/dream-pop'
    )
    expect(screen.getByRole('link', { name: /nu-gaze/ })).toHaveAttribute(
      'href',
      '/tags/nu-gaze'
    )
  })

  it('hides hierarchy block for non-genre categories even with parent/children', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        category: 'locale',
        parent: {
          id: 5,
          name: 'West Coast',
          slug: 'west-coast',
          category: 'locale',
          is_official: false,
          usage_count: 10,
        },
        children: [
          { id: 11, name: 'Phoenix', slug: 'phoenix', category: 'locale', is_official: false, usage_count: 4 },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="arizona" />)

    expect(screen.queryByTestId('tag-hierarchy')).not.toBeInTheDocument()
  })

  it('hides hierarchy block entirely when no parent and no children', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ category: 'genre' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByTestId('tag-hierarchy')).not.toBeInTheDocument()
  })

  // ── Aliases ──

  it('renders aliases when present', () => {
    mockUseTagDetail.mockReturnValue({
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
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ aliases: [] }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Also known as')).not.toBeInTheDocument()
  })

  // ── Creator attribution ──

  it('renders "Created by @username" from the enriched created_by field', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        created_by: { id: 42, username: 'johndoe' },
      }),
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

  it('falls back to legacy created_by_username when created_by is null', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        created_by: null,
        created_by_username: 'legacyuser',
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByText('@legacyuser')).toBeInTheDocument()
  })

  it('does not render creator when no creator info is available', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText(/Created by/)).not.toBeInTheDocument()
  })

  // ── Usage breakdown summary row ──

  it('renders non-zero breakdown counts in the header', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        usage_count: 18,
        usage_breakdown: {
          artist: 15,
          venue: 0,
          show: 3,
          release: 0,
          label: 0,
          festival: 0,
        },
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const summary = screen.getByTestId('usage-breakdown-summary')
    expect(summary).toBeInTheDocument()
    // 15 artists · 3 shows — zero-count types are hidden
    expect(summary).toHaveTextContent('15')
    expect(summary).toHaveTextContent('artists')
    expect(summary).toHaveTextContent('3')
    expect(summary).toHaveTextContent('shows')
    expect(summary).not.toHaveTextContent('0 venues')
  })

  it('renders singular label when breakdown count is 1', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        usage_count: 1,
        usage_breakdown: {
          artist: 1,
          venue: 0,
          show: 0,
          release: 0,
          label: 0,
          festival: 0,
        },
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const summary = screen.getByTestId('usage-breakdown-summary')
    expect(summary).toHaveTextContent('1')
    expect(summary).toHaveTextContent('artist')
    expect(summary).not.toHaveTextContent('artists')
  })

  it('hides the breakdown summary when all counts are zero', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(
      screen.queryByTestId('usage-breakdown-summary')
    ).not.toBeInTheDocument()
  })

  // ── Top contributors ──

  it('renders top contributors with handles and counts', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        top_contributors: [
          { user: { id: 1, username: 'alice' }, count: 8 },
          { user: { id: 2, username: 'bob' }, count: 5 },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByTestId('top-contributors')).toBeInTheDocument()
    expect(screen.getByText('Top contributors')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@alice' })).toHaveAttribute(
      'href',
      '/users/alice'
    )
    expect(screen.getByRole('link', { name: '@bob' })).toHaveAttribute(
      'href',
      '/users/bob'
    )
    expect(screen.getByText('(8)')).toBeInTheDocument()
    expect(screen.getByText('(5)')).toBeInTheDocument()
  })

  it('hides anonymous contributors and hides the section when all are anonymous (PSY-450)', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        top_contributors: [{ user: { id: 99 }, count: 3 }],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    // Never leak the internal DB id as a fallback label.
    expect(screen.queryByText(/user #\d+/)).not.toBeInTheDocument()
    // When every contributor is anonymous the section must be hidden entirely.
    expect(screen.queryByTestId('top-contributors')).not.toBeInTheDocument()
  })

  it('shows only named contributors when the list is mixed (PSY-450)', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        top_contributors: [
          { user: { id: 1, username: 'alice' }, count: 8 },
          { user: { id: 42 }, count: 6 },
          { user: { id: 2, username: 'bob' }, count: 2 },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.getByTestId('top-contributors')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@alice' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@bob' })).toBeInTheDocument()
    expect(screen.queryByText(/user #\d+/)).not.toBeInTheDocument()
  })

  it('hides top contributors section when empty', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ top_contributors: [] }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByTestId('top-contributors')).not.toBeInTheDocument()
  })

  // ── Related tags ──

  it('renders related tags pills', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        related_tags: [
          { id: 20, name: 'post-rock', slug: 'post-rock', category: 'genre', is_official: false, usage_count: 5 },
          { id: 21, name: 'dream-pop', slug: 'dream-pop', category: 'genre', is_official: true, usage_count: 9 },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)

    const section = screen.getByTestId('related-tags')
    expect(section).toBeInTheDocument()
    expect(screen.getByText('Related tags')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /post-rock/ })).toHaveAttribute(
      'href',
      '/tags/post-rock'
    )
    expect(screen.getByRole('link', { name: /dream-pop/ })).toHaveAttribute(
      'href',
      '/tags/dream-pop'
    )
  })

  it('hides related tags section when empty', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ related_tags: [] }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByTestId('related-tags')).not.toBeInTheDocument()
  })

  // ── NotifyMeButton + breadcrumb ──

  it('renders NotifyMeButton with correct props', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ id: 7, name: 'Punk' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="punk" />)

    expect(screen.getByTestId('notify-me-button')).toHaveTextContent(
      'Notify Punk'
    )
  })

  it('renders breadcrumb with tag name', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ name: 'Jazz' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="jazz" />)

    const jazzElements = screen.getAllByText('Jazz')
    expect(jazzElements.length).toBeGreaterThanOrEqual(2)
  })

  // ── Tagged Entities (preserved grouped list) ──

  it('renders tagged entities grouped by type', () => {
    mockUseTagDetail.mockReturnValue({
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
    expect(screen.getAllByText('Artists').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Venues').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByRole('link', { name: 'Radiohead' })).toHaveAttribute(
      'href',
      '/artists/radiohead'
    )
    expect(screen.getByRole('link', { name: 'Portishead' })).toHaveAttribute(
      'href',
      '/artists/portishead'
    )
    expect(screen.getByRole('link', { name: 'The Rebel Lounge' })).toHaveAttribute(
      'href',
      '/venues/the-rebel-lounge'
    )
  })

  it('does not render tagged entities section when usage_count is 0', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 0 }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    expect(screen.queryByText('Tagged Entities')).not.toBeInTheDocument()
  })

  it('shows loading spinner while entities are loading', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 5 }),
      isLoading: false,
      error: null,
    })
    mockUseTagEntities.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const spinners = document.querySelectorAll('.animate-spin')
    expect(spinners.length).toBeGreaterThanOrEqual(1)
  })

  // ── Creation date ──

  it('renders creation date from created_at timestamp', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ created_at: '2025-01-01T00:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagDetail slug="rock" />)

    const clockIcons = document.querySelectorAll('.lucide-clock')
    expect(clockIcons.length).toBeGreaterThanOrEqual(1)
  })
})
