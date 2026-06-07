import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type {
  TagEnrichedDetailResponse,
  TagIntersectionResponse,
} from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUseTagDetail = vi.fn()
const mockUseTagIntersection = vi.fn()
const mockUseSearchTags = vi.fn()
vi.mock('../hooks', () => ({
  useTagDetail: (...args: unknown[]) => mockUseTagDetail(...args),
  useTagIntersection: (...args: unknown[]) => mockUseTagIntersection(...args),
  useSearchTags: (...args: unknown[]) => mockUseSearchTags(...args),
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

// Router + search params are driven by a module-level mutable so individual
// tests can seed the ?with= pivot state and assert router.replace calls.
const mockReplace = vi.fn()
let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  usePathname: () => '/tags/test-tag',
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="notify-me-button">Notify {entityName}</button>
  ),
}))

vi.mock('@/components/shared', () => ({
  Breadcrumb: ({
    fallback,
    intermediate,
    currentPage,
  }: {
    fallback: { href: string; label: string }
    intermediate?: { href: string; label: string }[]
    currentPage: string
  }) => (
    <nav aria-label="Breadcrumb" data-testid="breadcrumb-stub">
      <a href={fallback.href}>{fallback.label}</a>
      {(intermediate ?? []).map((c) => (
        <a key={c.href} href={c.href} data-testid="breadcrumb-intermediate">
          {c.label}
        </a>
      ))}
      <span>{currentPage}</span>
    </nav>
  ),
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
    id: 1,
    name: 'Shoegaze',
    slug: 'shoegaze',
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

function makeIntersection(
  overrides: Partial<TagIntersectionResponse> = {}
): TagIntersectionResponse {
  return {
    tags: [
      {
        id: 1,
        name: 'Shoegaze',
        slug: 'shoegaze',
        category: 'genre',
        is_official: false,
        usage_count: 42,
      },
    ],
    tag_match: 'all',
    // Canonical backend order: artist, release, label, show, venue, festival,
    // collection. Component reorders to the design's display order.
    groups: [
      { entity_type: 'artist', count: 0, preview: [] },
      { entity_type: 'release', count: 0, preview: [] },
      { entity_type: 'label', count: 0, preview: [] },
      { entity_type: 'show', count: 0, preview: [] },
      { entity_type: 'venue', count: 0, preview: [] },
      { entity_type: 'festival', count: 0, preview: [] },
      { entity_type: 'collection', count: 0, preview: [] },
    ],
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mockSearchParams = new URLSearchParams()
  // Default: tag with no entities loaded.
  mockUseTagIntersection.mockReturnValue({
    data: makeIntersection(),
    isLoading: false,
  })
  mockUseSearchTags.mockReturnValue({ data: { tags: [] }, isLoading: false })
})

describe('TagDetail', () => {
  // ── Loading / error states ──

  it('shows loading spinner while tag is loading', () => {
    mockUseTagDetail.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows "Tag Not Found" for 404 errors', () => {
    mockUseTagDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Tag not found'),
    })
    renderWithProviders(<TagDetail slug="nonexistent" />)
    expect(screen.getByText('Tag Not Found')).toBeInTheDocument()
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
    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(screen.getByText('Error Loading Tag')).toBeInTheDocument()
  })

  // ── Thin metadata band ──

  it('renders tag name as heading, category chip, and demoted meta', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ name: 'Shoegaze', usage_count: 42 }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'Shoegaze' })
    ).toBeInTheDocument()
    expect(screen.getByText('Genre')).toBeInTheDocument()
    expect(screen.getByText('42 uses')).toBeInTheDocument()
  })

  it('renders singular "use" for a 1-use tag', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 1 }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(screen.getByText('1 use')).toBeInTheDocument()
  })

  it('renders the Filter shows action pointing at /shows?tags={slug}', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(
      screen.getByRole('link', { name: 'Filter shows' })
    ).toHaveAttribute('href', '/shows?tags=shoegaze')
  })

  it('renders NotifyMeButton with the tag name', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ name: 'Punk' }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="punk" />)
    expect(screen.getByTestId('notify-me-button')).toHaveTextContent(
      'Notify Punk'
    )
  })

  it('renders description HTML when present', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        description_html: '<p>Dreamy <strong>guitar</strong> textures.</p>',
      }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    const desc = screen.getByTestId('tag-description')
    expect(desc.innerHTML).toContain('<strong>guitar</strong>')
  })

  it('includes the parent tag as an intermediate breadcrumb when present', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        category: 'genre',
        parent: {
          id: 5,
          name: 'post-punk',
          slug: 'post-punk',
          category: 'genre',
          is_official: false,
          usage_count: 12,
        },
      }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    const intermediates = screen.getAllByTestId('breadcrumb-intermediate')
    expect(intermediates).toHaveLength(1)
    expect(intermediates[0]).toHaveAttribute('href', '/tags/post-punk')
  })

  // ── Co-visible sections (not tabs) ──

  it('renders co-visible sections in the fixed display order, suppressing empties', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 100 }),
      isLoading: false,
      error: null,
    })
    mockUseTagIntersection.mockReturnValue({
      data: makeIntersection({
        groups: [
          {
            entity_type: 'artist',
            count: 12,
            preview: [
              {
                entity_type: 'artist',
                entity_id: 1,
                name: 'Whirr',
                slug: 'whirr',
                city: 'Oakland',
                state: 'CA',
                upcoming_show_count: 8,
              },
            ],
          },
          { entity_type: 'release', count: 0, preview: [] },
          { entity_type: 'label', count: 3, preview: [
            {
              entity_type: 'label',
              entity_id: 5,
              name: 'Slumberland Records',
              slug: 'slumberland',
              city: 'Oakland',
              state: 'CA',
              release_count: 34,
            },
          ] },
          {
            entity_type: 'show',
            count: 2,
            preview: [
              {
                entity_type: 'show',
                entity_id: 9,
                name: 'Whirr Show',
                slug: 'whirr-show',
                headliner_name: 'Whirr',
                venue_name: 'The Rebel Lounge',
                city: 'Phoenix',
                state: 'AZ',
                event_date: '2026-07-28T03:00:00Z',
              },
            ],
          },
          { entity_type: 'venue', count: 0, preview: [] },
          { entity_type: 'festival', count: 0, preview: [] },
          { entity_type: 'collection', count: 0, preview: [] },
        ],
      }),
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)

    // Non-empty sections rendered; empty ones suppressed.
    expect(screen.getByTestId('tag-section-artist')).toBeInTheDocument()
    expect(screen.getByTestId('tag-section-show')).toBeInTheDocument()
    expect(screen.getByTestId('tag-section-label')).toBeInTheDocument()
    expect(screen.queryByTestId('tag-section-release')).not.toBeInTheDocument()
    expect(screen.queryByTestId('tag-section-venue')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('tag-section-festival')
    ).not.toBeInTheDocument()

    // Display order: artist (Artists) before show (Upcoming shows) before label.
    const sections = within(screen.getByTestId('tag-sections')).getAllByRole(
      'heading',
      { level: 2 }
    )
    const order = sections.map((h) => h.textContent)
    expect(order[0]).toContain('Artists')
    expect(order[1]).toContain('Upcoming shows')
    expect(order[2]).toContain('Labels')

    // Dense rows surface name + metric (scope to the artist section because
    // "Whirr" is also the headliner of the show row).
    const artistSection = screen.getByTestId('tag-section-artist')
    expect(
      within(artistSection).getByRole('link', { name: 'Whirr' })
    ).toHaveAttribute('href', '/artists/whirr')
    expect(within(artistSection).getByText('8 shows')).toBeInTheDocument()
  })

  it('builds "Show all N" links to the existing per-type browse filtered by tag', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 100 }),
      isLoading: false,
      error: null,
    })
    mockUseTagIntersection.mockReturnValue({
      data: makeIntersection({
        groups: [
          {
            entity_type: 'artist',
            count: 142,
            preview: [
              {
                entity_type: 'artist',
                entity_id: 1,
                name: 'Whirr',
                slug: 'whirr',
                upcoming_show_count: 8,
              },
            ],
          },
          { entity_type: 'release', count: 0, preview: [] },
          { entity_type: 'label', count: 0, preview: [] },
          { entity_type: 'show', count: 0, preview: [] },
          { entity_type: 'venue', count: 0, preview: [] },
          { entity_type: 'festival', count: 0, preview: [] },
          { entity_type: 'collection', count: 0, preview: [] },
        ],
      }),
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(
      screen.getByTestId('tag-section-showall-artist')
    ).toHaveAttribute('href', '/artists?tags=shoegaze')
  })

  // ── Sparse state (frame 437:7) ──

  it('renders the "Help grow this tag" CTA for a single-item sparse tag', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ name: 'dungeon synth', usage_count: 1 }),
      isLoading: false,
      error: null,
    })
    mockUseTagIntersection.mockReturnValue({
      data: makeIntersection({
        groups: [
          {
            entity_type: 'artist',
            count: 1,
            preview: [
              {
                entity_type: 'artist',
                entity_id: 1,
                name: 'Old Sorcery',
                slug: 'old-sorcery',
                city: 'Tasmania',
                state: 'AU',
                upcoming_show_count: 0,
              },
            ],
          },
          { entity_type: 'release', count: 0, preview: [] },
          { entity_type: 'label', count: 0, preview: [] },
          { entity_type: 'show', count: 0, preview: [] },
          { entity_type: 'venue', count: 0, preview: [] },
          { entity_type: 'festival', count: 0, preview: [] },
          { entity_type: 'collection', count: 0, preview: [] },
        ],
      }),
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="dungeon-synth" />)

    expect(screen.getByTestId('tag-section-artist')).toBeInTheDocument()
    expect(screen.queryByTestId('tag-section-release')).not.toBeInTheDocument()
    expect(screen.getByTestId('help-grow-cta')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Suggest something/ })
    ).toBeInTheDocument()
  })

  it('does NOT show the Help-grow CTA when the tag has multiple populated sections', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 50 }),
      isLoading: false,
      error: null,
    })
    mockUseTagIntersection.mockReturnValue({
      data: makeIntersection({
        groups: [
          {
            entity_type: 'artist',
            count: 12,
            preview: [
              { entity_type: 'artist', entity_id: 1, name: 'Whirr', slug: 'whirr' },
            ],
          },
          {
            entity_type: 'release',
            count: 5,
            preview: [
              { entity_type: 'release', entity_id: 2, name: 'Loveless', slug: 'loveless' },
            ],
          },
          { entity_type: 'label', count: 0, preview: [] },
          { entity_type: 'show', count: 0, preview: [] },
          { entity_type: 'venue', count: 0, preview: [] },
          { entity_type: 'festival', count: 0, preview: [] },
          { entity_type: 'collection', count: 0, preview: [] },
        ],
      }),
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(screen.queryByTestId('help-grow-cta')).not.toBeInTheDocument()
  })

  // ── Related tags rail + "+ add another tag" pivot ──

  it('renders related-tag chips that add the tag to the intersection in place', async () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({
        related_tags: [
          {
            id: 20,
            name: 'ambient',
            slug: 'ambient',
            category: 'genre',
            is_official: false,
            usage_count: 9,
          },
        ],
      }),
      isLoading: false,
      error: null,
    })

    const user = userEvent.setup()
    renderWithProviders(<TagDetail slug="shoegaze" />)

    const rail = screen.getByTestId('related-tags')
    expect(within(rail).getByText('Related tags')).toBeInTheDocument()

    await user.click(screen.getByTestId('related-tag-ambient'))
    // Clicking a related tag narrows in place by writing ?with=ambient.
    expect(mockReplace).toHaveBeenCalledWith(
      '/tags/shoegaze?with=ambient',
      { scroll: false }
    )
  })

  it('exposes the "+ add another tag to filter" pivot trigger', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail(),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="shoegaze" />)
    expect(screen.getByTestId('add-tag-pivot-trigger')).toHaveTextContent(
      '+ add another tag to filter'
    )
  })

  it('reflects an active pivot from ?with= and re-queries the intersection', () => {
    mockSearchParams = new URLSearchParams('with=ambient')
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 100 }),
      isLoading: false,
      error: null,
    })
    mockUseTagIntersection.mockReturnValue({
      data: makeIntersection({
        tags: [
          {
            id: 1,
            name: 'Shoegaze',
            slug: 'shoegaze',
            category: 'genre',
            is_official: false,
            usage_count: 42,
          },
          {
            id: 2,
            name: 'Ambient',
            slug: 'ambient',
            category: 'genre',
            is_official: false,
            usage_count: 30,
          },
        ],
        groups: [
          {
            entity_type: 'artist',
            count: 3,
            preview: [
              { entity_type: 'artist', entity_id: 1, name: 'Whirr', slug: 'whirr' },
            ],
          },
          { entity_type: 'release', count: 0, preview: [] },
          { entity_type: 'label', count: 0, preview: [] },
          { entity_type: 'show', count: 0, preview: [] },
          { entity_type: 'venue', count: 0, preview: [] },
          { entity_type: 'festival', count: 0, preview: [] },
          { entity_type: 'collection', count: 0, preview: [] },
        ],
      }),
      isLoading: false,
    })

    renderWithProviders(<TagDetail slug="shoegaze" />)

    // The hook is called with BOTH slugs.
    const calledSlugs = mockUseTagIntersection.mock.calls.at(-1)?.[0]
    expect(calledSlugs).toEqual(['shoegaze', 'ambient'])

    // Active-filter bar shows the added tag + a remove control.
    const bar = screen.getByTestId('active-filter-bar')
    expect(within(bar).getByText('Ambient')).toBeInTheDocument()
    expect(
      within(bar).getByRole('button', { name: /Remove Ambient filter/ })
    ).toBeInTheDocument()

    // "Show all" deep-links to the multi-tag browse with tag_match=all.
    expect(
      screen.getByTestId('tag-section-showall-artist')
    ).toHaveAttribute('href', '/artists?tags=shoegaze%2Cambient&tag_match=all')
  })

  it('does not fire the intersection query for a tag with zero usage', () => {
    mockUseTagDetail.mockReturnValue({
      data: makeTagDetail({ usage_count: 0 }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<TagDetail slug="empty-tag" />)
    // enabled=false is passed through as the 3rd arg.
    const lastCall = mockUseTagIntersection.mock.calls.at(-1)
    expect(lastCall?.[2]).toEqual({ enabled: false })
  })
})
