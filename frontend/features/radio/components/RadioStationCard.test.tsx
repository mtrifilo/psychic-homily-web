import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioStationCard } from './RadioStationCard'
import type { RadioStationListItem, RadioSiblingStation } from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// next/image is mocked the same way TopBar.test.tsx does it: render
// the underlying <img> with the same src/alt so DOM-level assertions
// (getByAltText, src equality) keep meaningful coverage of the
// caller-passed URL — without spinning up the image optimizer.
vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => {
    const { priority, ...rest } = props
    return <img {...rest} data-priority={priority ? 'true' : undefined} />
  },
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

function makeStation(overrides: Partial<RadioStationListItem> = {}): RadioStationListItem {
  return {
    id: 1,
    name: 'KEXP',
    slug: 'kexp',
    city: 'Seattle',
    state: 'WA',
    country: 'USA',
    broadcast_type: 'terrestrial',
    frequency_mhz: 90.3,
    logo_url: null,
    is_active: true,
    network: null,
    sibling_stations: [],
    show_count: 5,
    ...overrides,
  }
}

describe('RadioStationCard', () => {
  it('renders the station name as a link to the detail page', () => {
    render(<RadioStationCard station={makeStation()} />)
    const link = screen.getByText('KEXP').closest('a')
    expect(link).toHaveAttribute('href', '/radio/kexp')
  })

  it('renders the broadcast-type label', () => {
    render(<RadioStationCard station={makeStation({ broadcast_type: 'terrestrial' })} />)
    expect(screen.getByText('FM/AM')).toBeInTheDocument()
  })

  it('renders the frequency when present', () => {
    render(<RadioStationCard station={makeStation({ frequency_mhz: 90.3 })} />)
    expect(screen.getByText('90.3 MHz')).toBeInTheDocument()
  })

  it('omits the frequency for internet-only stations', () => {
    render(<RadioStationCard station={makeStation({ frequency_mhz: null })} />)
    expect(screen.queryByText(/MHz/)).not.toBeInTheDocument()
  })

  it('renders city + state location', () => {
    render(<RadioStationCard station={makeStation()} />)
    expect(screen.getByText('Seattle, WA')).toBeInTheDocument()
  })

  it('pluralizes the show count', () => {
    render(<RadioStationCard station={makeStation({ show_count: 5 })} />)
    expect(screen.getByText('5 shows')).toBeInTheDocument()
  })

  it('singularizes a single show', () => {
    render(<RadioStationCard station={makeStation({ show_count: 1 })} />)
    expect(screen.getByText('1 show')).toBeInTheDocument()
  })

  it('hides the show count when zero', () => {
    render(<RadioStationCard station={makeStation({ show_count: 0 })} />)
    expect(screen.queryByText(/show/)).not.toBeInTheDocument()
  })

  it('advertises sibling channels only when the station is the network flagship', () => {
    const station = makeStation({
      slug: 'wfmu',
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
      sibling_stations: [makeSibling({ id: 2 }), makeSibling({ id: 3 })],
    })
    render(<RadioStationCard station={station} />)
    expect(screen.getByText('+ 2 channels')).toBeInTheDocument()
  })

  it('singularizes a single sibling channel', () => {
    const station = makeStation({
      slug: 'wfmu',
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
      sibling_stations: [makeSibling({ id: 2 })],
    })
    render(<RadioStationCard station={station} />)
    expect(screen.getByText('+ 1 channel')).toBeInTheDocument()
  })

  it('does not advertise channels for a non-flagship station even with siblings', () => {
    const station = makeStation({
      slug: 'wfmu-drummer',
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
      sibling_stations: [makeSibling({ id: 2 })],
    })
    render(<RadioStationCard station={station} />)
    expect(screen.queryByText(/channel/)).not.toBeInTheDocument()
  })

  it('links a non-flagship station to its network channel URL', () => {
    const station = makeStation({
      name: 'Drummer',
      slug: 'wfmu-drummer',
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
    })
    render(<RadioStationCard station={station} />)
    const link = screen.getByText('Drummer').closest('a')
    expect(link).toHaveAttribute('href', '/radio/wfmu/channel/drummer')
  })

  it('renders the logo with alt text when logo_url is set', () => {
    // URL uses a real allowlisted host (`www.kexp.org`) from
    // next.config.ts so the test asserts the URL that would actually
    // pass `next/image`'s remotePatterns gate in production.
    const logoUrl = 'https://www.kexp.org/static/img/kexp-logo.png'
    render(
      <RadioStationCard
        station={makeStation({ logo_url: logoUrl })}
      />
    )
    const img = screen.getByAltText('KEXP logo')
    expect(img).toHaveAttribute('src', logoUrl)
  })
})
