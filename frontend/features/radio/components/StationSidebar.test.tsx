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

/**
 * Stored values the shared gate refuses, one per reason: a non-http scheme, a
 * data URL, a value that is neither URL nor handle, and userinfo the browser
 * discards. Shared by every column's wiring test so the two cannot drift into
 * asserting different rules.
 */
const REFUSED_VALUES: [string][] = [
  ['javascript:alert(1)'],
  ['data:text/html,<script>alert(1)</script>'],
  ['not a url'],
  ['https://user@wfmu.org/'],
]

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
          social: { twitter: 'https://twitter.com/wfmu' },
        })}
      />
    )

    expect(
      screen.getByRole('link', { name: /^Donate to WFMU\b/ })
    ).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('link', { name: /^WFMU on twitter\b/ })
    ).toHaveAttribute('href', 'https://twitter.com/wfmu')
  })

  // These columns are operator-entered free text, so the box keeps only what
  // the shared gate returns and DROPS the rest. Dropping beats the two
  // alternatives: a live javascript:/data: href, or a permanently greyed
  // bracket that reads as a disabled feature rather than as bad data.
  //
  // The shapes themselves are settled by the gate's own unit tests
  // (lib/socialLinks.test.ts); these assert the WIRING of each column.
  it.each(REFUSED_VALUES)('drops a non-http donation url (%s) entirely', url => {
    render(<StationSidebar station={makeStation({ donation_url: url })} />)

    expect(screen.queryByRole('link', { name: /donate/i })).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /donate/i })
    ).not.toBeInTheDocument()
  })

  // The shared gate repairs a domain-shaped value rather than dropping it, and
  // the href it returns is the one the anchor was checked against, so what the
  // browser resolves and what was judged are the same string. The station
  // columns inherit that with the gate.
  it.each([
    ['givewfmu.org', 'https://givewfmu.org'],
    ['//give.wfmu.org/x', 'https:////give.wfmu.org/x'],
  ])('repairs a scheme-less donation url (%s) to an absolute href', (url, href) => {
    render(<StationSidebar station={makeStation({ donation_url: url })} />)

    const link = screen.getByRole('link', { name: /^Donate to WFMU\b/ })
    expect(link).toHaveAttribute('href', href)
    expect(new URL(link.getAttribute('href') ?? '').protocol).toBe('https:')
  })

  // The key is printed as the link's visible label, so a value under a
  // platform's name is held to that platform's host anchor: "spotify" pointing
  // at another host is a stranger's page wearing a name the reader trusts.
  it('drops a social value whose host is not the platform its key names', () => {
    render(
      <StationSidebar
        station={makeStation({
          social: {
            spotify: 'https://spotify-account-verify.evil.test/',
            instagram: 'https://instagram.com/wfmu',
          },
        })}
      />
    )

    expect(screen.queryByRole('link', { name: /spotify/i })).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /^WFMU on instagram\b/ })
    ).toHaveAttribute('href', 'https://instagram.com/wfmu')
  })

  // A key the shared registry does not know makes a platform claim nothing can
  // check, so it renders nothing. This is a deliberate narrowing: a station
  // whose operator filed a `bluesky` or `mixcloud` key keeps the stored value
  // and shows no bracket for it.
  it('renders nothing for a social key the registry does not know', () => {
    render(
      <StationSidebar
        station={makeStation({
          social: { bluesky: 'https://bsky.app/profile/wfmu' },
        })}
      />
    )

    expect(screen.queryByRole('link', { name: /bluesky/i })).not.toBeInTheDocument()
  })

  // `social` is free-form JSONB with no server-side schema, so its values are
  // string-typed only by assertion. One bad value must skip its own entry, not
  // throw during render and take the whole sidebar to the error boundary.
  it('skips a non-string social value without crashing the sidebar', () => {
    render(
      <StationSidebar
        station={makeStation({
          website: 'https://wfmu.org',
          social: {
            twitter: 123,
            instagram: 'https://instagram.com/wfmu',
          } as never,
        })}
      />
    )

    expect(screen.getByRole('link', { name: /^WFMU website\b/ })).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /^WFMU on instagram\b/ })
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /twitter/i })).not.toBeInTheDocument()
  })

  // The stored value is a legacy bare handle on a column the write path never
  // gated, and the shared tolerance resolves it onto the platform, so the link
  // survives here exactly as it does on an artist page.
  it('resolves a bare handle onto the platform its key names', () => {
    render(
      <StationSidebar station={makeStation({ social: { instagram: 'wfmu' } })} />
    )

    expect(
      screen.getByRole('link', { name: /^WFMU on instagram\b/ })
    ).toHaveAttribute('href', 'https://instagram.com/wfmu')
  })

  // An inherited Object property is a key at runtime on free-form JSONB, and
  // would otherwise resolve to a function rather than to a platform.
  it('does not treat an inherited Object property as a platform', () => {
    render(
      <StationSidebar
        station={makeStation({
          social: { constructor: 'https://instagram.com/x' } as never,
        })}
      />
    )

    expect(
      screen.queryByRole('link', { name: /constructor/i })
    ).not.toBeInTheDocument()
  })

  it.each(REFUSED_VALUES)('drops a website value the gate refuses (%s)', url => {
    render(<StationSidebar station={makeStation({ website: url })} />)

    expect(screen.queryByRole('link', { name: /website/i })).not.toBeInTheDocument()
  })

  // The host label is derived from the gated href rather than the raw column,
  // so the domain a reader sees is the host the click resolves to.
  it('labels the website bracket with the host the href resolves to', () => {
    render(<StationSidebar station={makeStation({ website: 'www.wfmu.org' })} />)

    const site = screen.getByRole('link', { name: /^WFMU website\b/ })
    expect(site).toHaveAttribute('href', 'https://www.wfmu.org')
    expect(site).toHaveTextContent('wfmu.org')
  })

  it('renders no link row when the station has no outbound urls', () => {
    render(<StationSidebar station={makeStation()} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
