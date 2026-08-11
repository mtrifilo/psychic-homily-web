import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistListItem } from '../types'

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockGet = vi.fn()
// `toString` backs the pager's href builder, which edits the LIVE params rather
// than rebuilding from a key list. Derived from the same `mockGet` so a test
// that stubs one param cannot end up with an address bar that disagrees.
const mockSearchParamKeys = ['cities', 'tags', 'tag_match', 'page', 'utm_source']
const mockSearchParamsToString = () => {
  const params = new URLSearchParams()
  for (const key of mockSearchParamKeys) {
    const value = mockGet(key)
    if (value != null) params.set(key, String(value))
  }
  return params.toString()
}
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => ({
    get: mockGet,
    toString: mockSearchParamsToString,
  }),
}))

// nuqs `useQueryState` bridged to the same mocked searchParams (single URL
// source of truth); the real citiesParser runs. The setters are asserted on.
const mockSetCities = vi.fn()
const mockSetPage = vi.fn()
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (
      key: string,
      parser: { parse: (v: string) => unknown; defaultValue?: unknown },
    ) => {
      const raw = mockGet(key)
      const value = raw != null ? parser.parse(raw) : (parser.defaultValue ?? null)
      return [value, key === 'page' ? mockSetPage : mockSetCities]
    },
  }
})

// Mock hooks
const mockUseArtists = vi.fn()
const mockUseArtistCities = vi.fn()
vi.mock('../hooks/useArtists', () => ({
  useArtists: (opts: unknown) => mockUseArtists(opts),
  useArtistCities: () => mockUseArtistCities(),
}))

const mockUseDensity = vi.fn()
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: (key: string) => mockUseDensity(key),
}))

// Mock child components that are complex
vi.mock('./ArtistSearch', () => ({
  ArtistSearch: () => <div data-testid="artist-search">ArtistSearch</div>,
}))

vi.mock('@/components/filters', () => ({
  CityFilters: ({
    onFilterChange,
    selectedCities,
  }: {
    onFilterChange: (cities: { city: string; state: string }[]) => void
    selectedCities: { city: string; state: string }[]
    cities: unknown[]
  }) => (
    <div data-testid="city-filters">
      <span data-testid="selected-count">{selectedCities.length}</span>
      <button
        data-testid="clear-filters"
        onClick={() => onFilterChange([])}
      >
        Clear
      </button>
    </div>
  ),
}))

// Partial mock: `Pagination` is deliberately the REAL component, because the
// page links and their hrefs are what these tests assert. Stubbing it would
// leave the pager's URL shape unverified, which is the half most likely to
// break (PSY-1754/1755 both shipped a builder that dropped foreign params).
vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
  DensityToggle: ({ density }: { density: string; onDensityChange: (v: string) => void }) => (
    <div data-testid="density-toggle">{density}</div>
  ),
  EntityCardTitle: ({
    name,
    href,
    ariaLabel,
  }: {
    name: string
    href: string
    ariaLabel?: string
  }) => (
    <a href={href} aria-label={ariaLabel ?? name}>
      <h3 title={name}>{name}</h3>
    </a>
  ),
}))

vi.mock('@/features/tags', () => ({
  // PSY-1001: surface the `layout` prop as `data-layout` so the test can
  // assert the desktop facet panel renders as the top bar (not the old rail).
  TagFacetPanel: ({
    layout,
    onToggle,
  }: {
    layout?: 'rail' | 'bar'
    onToggle: (slugs: string[]) => void
  }) => (
    <div data-testid="tag-facet-panel" data-layout={layout ?? 'rail'}>
      <button data-testid="toggle-tag" onClick={() => onToggle(['shoegaze'])}>
        shoegaze
      </button>
    </div>
  ),
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

import { ArtistList } from './ArtistList'

function makeArtist(overrides: Partial<ArtistListItem> = {}): ArtistListItem {
  return {
    id: 1,
    slug: 'test-artist',
    name: 'Test Artist',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    upcoming_show_count: 3,
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: null,
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtistList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
    mockUseDensity.mockReturnValue({ density: 'comfortable', setDensity: vi.fn() })
    mockUseArtistCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })
    mockUseArtists.mockReturnValue({
      data: { artists: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
  })

  it('shows loading spinner on initial load', () => {
    mockUseArtists.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      error: null,
      refetch: vi.fn(),
    })
    mockUseArtistCities.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('renders artist search component', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('artist-search')).toBeInTheDocument()
  })

  it('renders density toggle with current density', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('density-toggle')).toHaveTextContent('comfortable')
  })

  // PSY-1001: the desktop tag facet is a full-width top bar above a
  // full-width list (Variant A), not the old left rail. The shared
  // TagFacetPanel must receive layout="bar".
  it('renders the desktop tag facet as a top bar (layout="bar")', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('tag-facet-panel')).toHaveAttribute(
      'data-layout',
      'bar'
    )
  })

  it('renders empty state when no artists', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByText('No artists available at this time.')).toBeInTheDocument()
  })

  it('renders filtered empty state when cities selected', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'cities' ? 'Phoenix,AZ' : null
    )
    mockUseArtists.mockReturnValue({
      data: { artists: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(
      screen.getByText('No artists match the current filters.')
    ).toBeInTheDocument()
  })

  it('shows "Clear filters" link when filtered and empty', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'cities' ? 'Phoenix,AZ' : null
    )
    mockUseArtists.mockReturnValue({
      data: { artists: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByText('Clear filters')).toBeInTheDocument()
  })

  it('renders artist cards when data available', () => {
    const artists = [
      makeArtist({ id: 1, name: 'Artist One', slug: 'artist-one' }),
      makeArtist({ id: 2, name: 'Artist Two', slug: 'artist-two' }),
    ]
    mockUseArtists.mockReturnValue({
      data: { artists, total: 2, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByText('Artist One')).toBeInTheDocument()
    expect(screen.getByText('Artist Two')).toBeInTheDocument()
  })

  it('shows error state with retry button', () => {
    const refetch = vi.fn()
    mockUseArtists.mockReturnValue({
      data: { artists: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch,
    })

    renderWithProviders(<ArtistList />)
    expect(
      screen.getByText('Failed to load artists. Please try again later.')
    ).toBeInTheDocument()
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('calls refetch on retry button click', async () => {
    const user = userEvent.setup()
    const refetch = vi.fn()
    mockUseArtists.mockReturnValue({
      data: { artists: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch,
    })

    renderWithProviders(<ArtistList />)
    await user.click(screen.getByText('Retry'))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('renders city filters when cities data available', () => {
    mockUseArtistCities.mockReturnValue({
      data: {
        cities: [
          { city: 'Phoenix', state: 'AZ', artist_count: 5 },
          { city: 'Mesa', state: 'AZ', artist_count: 3 },
        ],
      },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('city-filters')).toBeInTheDocument()
  })

  it('does not render city filters when no cities', () => {
    mockUseArtistCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.queryByTestId('city-filters')).not.toBeInTheDocument()
  })

  it('parses cities from URL search params', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'cities' ? 'Phoenix,AZ|Mesa,AZ' : null
    )
    mockUseArtistCities.mockReturnValue({
      data: {
        cities: [{ city: 'Phoenix', state: 'AZ', artist_count: 5 }],
      },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    // useArtists should be called with the parsed cities (+ new tag fields)
    expect(mockUseArtists).toHaveBeenCalledWith(
      expect.objectContaining({
        cities: [
          { city: 'Phoenix', state: 'AZ' },
          { city: 'Mesa', state: 'AZ' },
        ],
      })
    )
  })

  it('passes no cities filter when no search params', () => {
    mockGet.mockReturnValue(null)

    renderWithProviders(<ArtistList />)
    expect(mockUseArtists).toHaveBeenCalledWith(
      expect.objectContaining({
        cities: undefined,
      })
    )
  })

  it('parses tags from URL and passes them to useArtists', () => {
    mockGet.mockImplementation((key: string) => {
      if (key === 'tags') return 'post-punk,shoegaze'
      return null
    })

    renderWithProviders(<ArtistList />)
    expect(mockUseArtists).toHaveBeenCalledWith(
      expect.objectContaining({
        tags: ['post-punk', 'shoegaze'],
        tagMatch: 'all',
      })
    )
  })

  it('honors tag_match=any in URL', () => {
    mockGet.mockImplementation((key: string) => {
      if (key === 'tags') return 'post-punk,shoegaze'
      if (key === 'tag_match') return 'any'
      return null
    })

    renderWithProviders(<ArtistList />)
    expect(mockUseArtists).toHaveBeenCalledWith(
      expect.objectContaining({
        tags: ['post-punk', 'shoegaze'],
        tagMatch: 'any',
      })
    )
  })

  // PSY-496: city filter must be page-scoped. Arriving at /artists from
  // another entity page (e.g. /shows) without a `cities` URL param should
  // render the unfiltered list. The shared city filter had been auto-applying
  // the user's profile favorite_cities on mount, which made users land on
  // /artists?cities=Phoenix%2CAZ after clicking the sidebar link — most
  // artists have city: null, so the list rendered "0 artists".
  describe('PSY-496: city filter is page-scoped (no cross-page persistence)', () => {
    it('does not call router.replace to append cities param on mount (no URL param)', () => {
      // Simulate cross-page navigation: arriving at /artists with no cities
      // URL param. Even if the user previously selected Phoenix on /shows,
      // /artists should render unfiltered and NOT mutate the URL.
      mockGet.mockReturnValue(null)

      renderWithProviders(<ArtistList />)

      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockUseArtists).toHaveBeenCalledWith(
        expect.objectContaining({ cities: undefined })
      )
    })

    it('respects an explicit cities URL param (still supports direct/bookmark nav)', () => {
      // Manual/bookmark nav: /artists?cities=Phoenix,AZ must still filter.
      mockGet.mockImplementation((key: string) =>
        key === 'cities' ? 'Phoenix,AZ' : null
      )

      renderWithProviders(<ArtistList />)

      // URL must not be mutated by the component.
      expect(mockReplace).not.toHaveBeenCalled()
      // Filter drives the list as expected.
      expect(mockUseArtists).toHaveBeenCalledWith(
        expect.objectContaining({
          cities: [{ city: 'Phoenix', state: 'AZ' }],
        })
      )
    })

    it('clearing the city filter writes null via nuqs (bare URL — no sentinel on /artists)', async () => {
      const user = userEvent.setup()
      mockUseArtistCities.mockReturnValue({
        data: { cities: [{ city: 'Phoenix', state: 'AZ', artist_count: 5 }] },
        isLoading: false,
        isFetching: false,
      })
      mockGet.mockImplementation((key: string) =>
        key === 'cities' ? 'Phoenix,AZ' : null
      )

      renderWithProviders(<ArtistList />)
      await user.click(screen.getByTestId('clear-filters'))

      // No derived default on /artists → empty selection clears the param
      // (null), unlike /shows' explicit ALL_CITIES sentinel (PSY-1390).
      expect(mockSetCities).toHaveBeenCalledWith(null)
    })
  })

  // PSY-1774: the browse list is paged, because unbounded it answered with the
  // whole 6,200-artist catalogue and 502'd through the proxy.
  describe('pagination', () => {
    const pagedArtists = [makeArtist({ id: 1, name: 'Artist One', slug: 'artist-one' })]

    function mockPage(total: number, offset: number) {
      mockUseArtists.mockReturnValue({
        data: { artists: pagedArtists, total, limit: 50, offset },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
    }

    it('requests the first page with the browse page size when no ?page= is set', () => {
      mockPage(120, 0)

      renderWithProviders(<ArtistList />)

      expect(mockUseArtists).toHaveBeenCalledWith(
        expect.objectContaining({ limit: 50, offset: 0 })
      )
    })

    it('turns ?page=3 into the matching offset', () => {
      mockGet.mockImplementation((key: string) => (key === 'page' ? '3' : null))
      mockPage(300, 100)

      renderWithProviders(<ArtistList />)

      expect(mockUseArtists).toHaveBeenCalledWith(
        expect.objectContaining({ limit: 50, offset: 100 })
      )
    })

    it('counts the whole matching set, not the rows on this page', () => {
      mockPage(6200, 0)

      renderWithProviders(<ArtistList />)

      // `artists.length` here would report "1 artist" over a catalogue of 6,200.
      expect(screen.getByTestId('artist-count')).toHaveTextContent('6200 artists')
    })

    it('links page two from the current params, preserving ones it does not own', () => {
      mockGet.mockImplementation((key: string) => {
        if (key === 'tags') return 'post-punk'
        if (key === 'utm_source') return 'newsletter'
        return null
      })
      mockPage(120, 0)

      renderWithProviders(<ArtistList />)

      const pageTwo = screen.getAllByRole('link', { name: 'Page 2' })[0]
      const href = pageTwo.getAttribute('href') ?? ''
      expect(href).toContain('page=2')
      // The params this component does not own must survive a page change —
      // a from-scratch href builder drops them at the first click.
      expect(href).toContain('utm_source=newsletter')
      expect(href).toContain('tags=post-punk')
    })

    it('writes no ?page= for page one, so the head of the list has one URL', () => {
      mockGet.mockImplementation((key: string) => (key === 'page' ? '3' : null))
      mockPage(300, 100)

      renderWithProviders(<ArtistList />)

      const pageOne = screen.getAllByRole('link', { name: 'Page 1' })[0]
      expect(pageOne.getAttribute('href')).toBe('/artists')
    })

    it('renders no pager when everything fits on one page', () => {
      mockPage(1, 0)

      renderWithProviders(<ArtistList />)

      expect(screen.queryByTestId('pagination')).not.toBeInTheDocument()
    })

    it('resets the pager when the city filter changes', async () => {
      const user = userEvent.setup()
      mockUseArtistCities.mockReturnValue({
        data: { cities: [{ city: 'Phoenix', state: 'AZ', artist_count: 5 }] },
        isLoading: false,
        isFetching: false,
      })
      mockGet.mockImplementation((key: string) => (key === 'page' ? '4' : null))
      mockPage(300, 150)

      renderWithProviders(<ArtistList />)
      await user.click(screen.getByTestId('clear-filters'))

      // A filter that narrows the set while `?page=4` survives lands the reader
      // on an empty page they did not ask for.
      expect(mockSetPage).toHaveBeenCalledWith(null)
    })

    it('drops ?page= when the tag filter changes', async () => {
      const user = userEvent.setup()
      mockGet.mockImplementation((key: string) => {
        if (key === 'page') return '4'
        if (key === 'tags') return 'post-punk'
        return null
      })
      mockPage(300, 150)

      renderWithProviders(<ArtistList />)
      await user.click(screen.getByTestId('toggle-tag'))

      const [href] = mockPush.mock.calls[0]
      expect(href).toContain('tags=shoegaze')
      expect(href).not.toContain('page=')
    })

    it('reports a page past the end as such, not as an empty catalogue', () => {
      mockGet.mockImplementation((key: string) => (key === 'page' ? '99' : null))
      mockUseArtists.mockReturnValue({
        data: { artists: [], total: 120, limit: 50, offset: 4900 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })

      renderWithProviders(<ArtistList />)

      // Regex, not an exact string: the sentence, the link and the closing
      // period are three sibling nodes, so no single element's own text equals
      // the whole line.
      expect(
        screen.getByText(/That page is past the end of the list\./)
      ).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: 'Back to the first page' })
      ).toHaveAttribute('href', '/artists')
      expect(
        screen.queryByText('No artists available at this time.')
      ).not.toBeInTheDocument()
    })
  })
})
