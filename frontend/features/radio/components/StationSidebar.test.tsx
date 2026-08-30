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

    // Named for the STATION, not the host label: "wfmu.org" alone says nothing
    // in a list, and the ↗ must stay visual rather than being read aloud.
    const site = screen.getByRole('link', { name: /^WFMU website\b/ })
    expect(site).toHaveAttribute('href', 'https://wfmu.org')
    expect(site).toHaveAttribute('target', '_blank')
    expect(site).toHaveAttribute('rel', 'noopener noreferrer')
    expect(site.getAttribute('aria-label')).not.toMatch(/↗/)
    // The suffix wording is BracketLink's contract; assert only that it is
    // present exactly once.
    expect(
      site.getAttribute('aria-label')?.match(/opens in a new tab/g)
    ).toHaveLength(1)
    // ...and that the ↗ is still VISIBLE.
    expect(site).toHaveTextContent('↗')
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
      screen.getByRole('link', { name: /^Donate to WFMU\b/ })
    ).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('link', { name: /^WFMU on bluesky\b/ })
    ).toHaveAttribute('href', 'https://bsky.app/profile/wfmu')
  })

  // These columns are operator-entered free text, so the box keeps only
  // absolute http(s) and DROPS the rest. Dropping beats the two alternatives:
  // a live javascript:/data: href, or a permanently greyed bracket that reads
  // as a disabled feature rather than as bad data.
  //
  // Scoped to THIS box on purpose — the same columns are rendered as raw
  // anchors elsewhere on the station page (StationDetail's header buttons), so
  // this asserts nothing about the field in general. Validating those columns
  // on write, which is the real fix, is tracked in PSY-1953.
  it.each([
    ['javascript:alert(1)'],
    ['data:text/html,<script>alert(1)</script>'],
    ['//evil.example/give'],
    ['givewfmu.org'],
  ])('drops a non-http donation url (%s) entirely', url => {
    render(<StationSidebar station={makeStation({ donation_url: url })} />)

    expect(screen.queryByRole('link', { name: /donate/i })).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /donate/i })
    ).not.toBeInTheDocument()
  })

  // `social` is free-form JSONB with no server-side schema, so its values are
  // string-typed only by assertion. One bad value must skip its own entry, not
  // throw during render and take the whole sidebar to the error boundary.
  it('skips a non-string social value without crashing the sidebar', () => {
    render(
      <StationSidebar
        station={makeStation({
          website: 'https://wfmu.org',
          social: { twitter: 123, bluesky: 'https://bsky.app/profile/wfmu' } as never,
        })}
      />
    )

    expect(screen.getByRole('link', { name: /^WFMU website\b/ })).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /^WFMU on bluesky\b/ })
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /twitter/i })).not.toBeInTheDocument()
  })

  it('renders no link row when the station has no outbound urls', () => {
    render(<StationSidebar station={makeStation()} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
