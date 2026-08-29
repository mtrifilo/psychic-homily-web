import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StationSidebar } from './StationSidebar'
import type { RadioStationDetail } from '../types'

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

const mockUseStationEpisodes = vi.fn()
vi.mock('../hooks/useStationEpisodes', () => ({
  useStationEpisodes: (...args: unknown[]) => mockUseStationEpisodes(...args),
}))

const mockUseStationTopArtists = vi.fn()
vi.mock('../hooks/useStationTopArtists', () => ({
  useStationTopArtists: (...args: unknown[]) => mockUseStationTopArtists(...args),
}))

const mockUseStationTopLabels = vi.fn()
vi.mock('../hooks/useStationTopLabels', () => ({
  useStationTopLabels: (...args: unknown[]) => mockUseStationTopLabels(...args),
}))

const mockUseNewReleaseRadar = vi.fn()
vi.mock('../hooks/useNewReleaseRadar', () => ({
  useNewReleaseRadar: (...args: unknown[]) => mockUseNewReleaseRadar(...args),
}))

function makeStation(overrides: Partial<RadioStationDetail> = {}): RadioStationDetail {
  return {
    id: 1,
    name: 'WFMU',
    slug: 'wfmu',
    description: null,
    city: 'Jersey City',
    state: 'NJ',
    country: 'USA',
    timezone: null,
    stream_url: null,
    stream_urls: null,
    website: null,
    donation_url: null,
    donation_embed_url: null,
    logo_url: null,
    social: null,
    broadcast_type: 'both',
    frequency_mhz: 91.1,
    playlist_source: null,
    playlist_config: null,
    last_playlist_fetch_at: null,
    is_active: true,
    network: null,
    sibling_stations: [],
    show_count: 2,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  // Only the STATION info box is under test here; the three data boxes render
  // nothing on empty results.
  mockUseStationEpisodes.mockReturnValue({ data: undefined })
  mockUseStationTopArtists.mockReturnValue({ data: undefined })
  mockUseStationTopLabels.mockReturnValue({ data: undefined })
  mockUseNewReleaseRadar.mockReturnValue({ data: undefined })
})

// These links go through the shared bracket primitive rather than a local
// anchor, so they inherit its announcement and its http(s) floor. A private
// copy of that markup would silently miss both.
describe('StationSidebar — STATION info box outbound links', () => {
  it('renders the website link as an outbound bracket announced once', () => {
    render(<StationSidebar station={makeStation({ website: 'https://wfmu.org' })} />)

    const site = screen.getByRole('link', { name: /^wfmu\.org/ })
    expect(site).toHaveAttribute('href', 'https://wfmu.org')
    expect(site).toHaveAttribute('target', '_blank')
    expect(site).toHaveAttribute('rel', 'noopener noreferrer')
    // The suffix itself is BracketLink's contract; assert only that it is
    // present exactly once, not its wording.
    expect(
      site.getAttribute('aria-label')?.match(/opens in a new tab/g)
    ).toHaveLength(1)
  })

  it('renders donation and social links as outbound brackets too', () => {
    render(
      <StationSidebar
        station={makeStation({
          donation_url: 'https://give.wfmu.org',
          social: { bluesky: 'https://bsky.app/profile/wfmu' },
        })}
      />
    )

    expect(
      screen.getByRole('link', { name: /^donate\b/ })
    ).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('link', { name: /^bluesky/ })
    ).toHaveAttribute('href', 'https://bsky.app/profile/wfmu')
  })

  // Inherited from the bracket primitive's `external` scheme floor: these
  // columns are admin-entered free text, so a non-http value degrades to the
  // disabled fallback here rather than shipping a live javascript:/data: href.
  //
  // Scoped to THIS box on purpose — the same columns are rendered as raw
  // anchors elsewhere on the station page, so this asserts nothing about the
  // field in general.
  it('degrades a non-http donation url to a disabled control, never an anchor', () => {
    render(
      <StationSidebar station={makeStation({ donation_url: 'javascript:alert(1)' })} />
    )

    expect(screen.queryByRole('link', { name: /donate/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'donate' })).toBeDisabled()
  })

  it('renders no link row when the station has no outbound urls', () => {
    render(<StationSidebar station={makeStation()} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
