import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { NetworkTabBar } from './NetworkTabBar'
import type { RadioStationDetail, RadioSiblingStation } from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makeSibling(overrides: Partial<RadioSiblingStation> = {}): RadioSiblingStation {
  return {
    id: 2,
    slug: 'wfmu-drummer',
    name: 'Drummer',
    broadcast_type: 'internet',
    frequency_mhz: null,
    is_flagship: false,
    ...overrides,
  }
}

function makeStation(overrides: Partial<RadioStationDetail> = {}): RadioStationDetail {
  return {
    id: 1,
    name: 'WFMU',
    slug: 'wfmu',
    description: null,
    city: 'Jersey City',
    state: 'NJ',
    country: 'USA',
    timezone: 'America/New_York',
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
    network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
    sibling_stations: [makeSibling()],
    show_count: 5,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('NetworkTabBar', () => {
  it('renders nothing for a network-less station', () => {
    const { container } = render(
      <NetworkTabBar currentStation={makeStation({ network: null })} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when the network has only the flagship (single tab)', () => {
    const station = makeStation({ sibling_stations: [] })
    const { container } = render(<NetworkTabBar currentStation={station} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders a labelled nav for a multi-station network', () => {
    render(<NetworkTabBar currentStation={makeStation()} />)
    expect(
      screen.getByRole('navigation', { name: 'WFMU network channels' })
    ).toBeInTheDocument()
  })

  it('renders one tab per station (current + siblings)', () => {
    const station = makeStation({
      sibling_stations: [
        makeSibling({ id: 2, slug: 'wfmu-drummer', name: 'Drummer' }),
        makeSibling({ id: 3, slug: 'wfmu-sheena', name: 'Sheena' }),
      ],
    })
    render(<NetworkTabBar currentStation={station} />)
    expect(screen.getAllByRole('link')).toHaveLength(3)
  })

  it('appends the frequency to the flagship tab label', () => {
    render(<NetworkTabBar currentStation={makeStation()} />)
    expect(screen.getByText('WFMU 91.1')).toBeInTheDocument()
  })

  it('marks the current station tab with aria-current=page', () => {
    render(<NetworkTabBar currentStation={makeStation()} />)
    const current = screen.getByText('WFMU 91.1').closest('a')
    expect(current).toHaveAttribute('aria-current', 'page')
  })

  it('links siblings to their network channel URLs', () => {
    render(<NetworkTabBar currentStation={makeStation()} />)
    const sibling = screen.getByText('Drummer').closest('a')
    expect(sibling).toHaveAttribute('href', '/radio/wfmu/channel/drummer')
    expect(sibling).not.toHaveAttribute('aria-current')
  })

  it('orders the flagship tab first regardless of sibling order', () => {
    const station = makeStation({
      sibling_stations: [
        makeSibling({ id: 2, slug: 'wfmu-aardvark', name: 'Aardvark' }),
      ],
    })
    render(<NetworkTabBar currentStation={station} />)
    const nav = screen.getByRole('navigation')
    const links = within(nav).getAllByRole('link')
    // Flagship "WFMU 91.1" sorts ahead of the alphabetically-earlier "Aardvark".
    expect(links[0]).toHaveTextContent('WFMU 91.1')
    expect(links[1]).toHaveTextContent('Aardvark')
  })
})
