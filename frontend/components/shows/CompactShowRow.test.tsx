import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CompactShowRow } from './CompactShowRow'

const baseShow = {
  id: 1,
  slug: 'test-show',
  event_date: '2026-04-15T20:00:00Z',
  price: 15,
  artists: [
    { id: 1, name: 'Artist One', slug: 'artist-one' },
    { id: 2, name: 'Artist Two', slug: null },
  ],
}

const baseVenue = {
  name: 'The Venue',
  slug: 'the-venue',
  city: 'Phoenix',
  state: 'AZ',
}

describe('CompactShowRow', () => {
  it('renders artist names', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    expect(screen.getByText('Artist One')).toBeInTheDocument()
    expect(screen.getByText('Artist Two')).toBeInTheDocument()
  })

  it('links artists with slugs', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    const link = screen.getByText('Artist One').closest('a')
    expect(link).toHaveAttribute('href', '/artists/artist-one')
  })

  it('renders artist without slug as plain text', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    const artistTwo = screen.getByText('Artist Two')
    expect(artistTwo.closest('a')).toBeNull()
    expect(artistTwo.tagName).toBe('SPAN')
  })

  it('renders TBA when no artists', () => {
    render(
      <CompactShowRow show={{ ...baseShow, artists: [] }} state="AZ" />
    )
    expect(screen.getByText('TBA')).toBeInTheDocument()
  })

  it('renders price when provided', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    expect(screen.getByText('$15.00')).toBeInTheDocument()
  })

  it('does not render price when null', () => {
    render(
      <CompactShowRow
        show={{ ...baseShow, price: null }}
        state="AZ"
      />
    )
    expect(screen.queryByText(/\$/)).not.toBeInTheDocument()
  })

  it('renders a details link by default', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    expect(screen.getByText('Details')).toBeInTheDocument()
    const link = screen.getByText('Details').closest('a')
    expect(link).toHaveAttribute('href', '/shows/test-show')
  })

  it('hides details link when showDetailsLink is false', () => {
    render(
      <CompactShowRow show={baseShow} state="AZ" showDetailsLink={false} />
    )
    expect(screen.queryByText('Details')).not.toBeInTheDocument()
  })

  it('uses show ID in link when slug is missing', () => {
    render(
      <CompactShowRow
        show={{ ...baseShow, slug: null }}
        state="AZ"
      />
    )
    const link = screen.getByText('Details').closest('a')
    expect(link).toHaveAttribute('href', '/shows/1')
  })

  it('renders venue line when showVenueLine is true and venue is provided', () => {
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        showVenueLine
        venue={baseVenue}
      />
    )
    expect(screen.getByText('The Venue')).toBeInTheDocument()
  })

  it('does not render venue line by default', () => {
    render(
      <CompactShowRow show={baseShow} state="AZ" venue={baseVenue} />
    )
    // Venue name should not appear since showVenueLine defaults to false
    expect(screen.queryByText('The Venue')).not.toBeInTheDocument()
  })

  it('renders venue as link when venue has slug', () => {
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        showVenueLine
        venue={baseVenue}
      />
    )
    const link = screen.getByText('The Venue').closest('a')
    expect(link).toHaveAttribute('href', '/venues/the-venue')
  })

  it('renders venue as plain text when venue has no slug', () => {
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        showVenueLine
        venue={{ ...baseVenue, slug: null }}
      />
    )
    const venueText = screen.getByText('The Venue')
    expect(venueText.closest('a')).toBeNull()
  })

  it('shows venue as primary line when primaryLine is venue', () => {
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        primaryLine="venue"
        venue={baseVenue}
      />
    )
    // Venue should be shown as primary (bold) text
    const venueLink = screen.getByText('The Venue')
    expect(venueLink).toBeInTheDocument()
  })

  it('shows Venue TBA when primaryLine is venue but no venue', () => {
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        primaryLine="venue"
        venue={null}
      />
    )
    expect(screen.getByText('Venue TBA')).toBeInTheDocument()
  })

  it('renders secondary artists with prefix', () => {
    const secondaryArtists = [
      { id: 10, name: 'Opener Act', slug: 'opener-act' },
    ]
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        secondaryArtists={secondaryArtists}
        secondaryArtistsPrefix="with"
      />
    )
    expect(screen.getByText('with')).toBeInTheDocument()
    expect(screen.getByText('Opener Act')).toBeInTheDocument()
  })

  it('uses default secondaryArtistsPrefix of w/', () => {
    const secondaryArtists = [
      { id: 10, name: 'Opener Act', slug: null },
    ]
    render(
      <CompactShowRow
        show={baseShow}
        state="AZ"
        secondaryArtists={secondaryArtists}
      />
    )
    expect(screen.getByText('w/')).toBeInTheDocument()
  })

  it('does not render secondary artists section when empty', () => {
    render(
      <CompactShowRow show={baseShow} state="AZ" secondaryArtists={[]} />
    )
    expect(screen.queryByText('w/')).not.toBeInTheDocument()
  })

  it('renders date badge link pointing to show detail', () => {
    render(<CompactShowRow show={baseShow} state="AZ" />)
    // The date badge is a link to the show detail
    const links = screen.getAllByRole('link')
    const dateBadgeLink = links.find(l => l.getAttribute('href') === '/shows/test-show')
    expect(dateBadgeLink).toBeDefined()
  })
})
