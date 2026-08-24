import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EgoTypeLegend } from './EgoTypeLegend'

describe('EgoTypeLegend (PSY-1453)', () => {
  it('renders a swatch row per fill family present, in display order', () => {
    render(<EgoTypeLegend families={['radio', 'bills', 'label', 'bills']} />)
    const legend = screen.getByTestId('ego-type-legend')
    expect(legend.textContent).toBe('billslabelradio')
  })

  it('appends a neutral "other" row when neutral-filled nodes are present', () => {
    render(<EgoTypeLegend families={['bills', null]} />)
    expect(screen.getByTestId('ego-type-legend').textContent).toBe('billsother')
  })

  it('renders nothing for an empty graph', () => {
    render(<EgoTypeLegend families={[]} />)
    expect(screen.queryByTestId('ego-type-legend')).toBeNull()
  })

  // PSY-1914: the green dot fires on `upcoming_show_count > 0` — any future
  // show, at any distance — so the key states that predicate rather than the
  // near-term window "playing soon" implied. Pinned on the component that
  // renders it, not only through ArtistGraph's concatenated-string assertion.
  // The three exact-textContent tests above already pin marker-key ABSENCE,
  // so only the present case needs its own case here.
  it('states the upcoming-show marker as a predicate, not a time window', () => {
    render(<EgoTypeLegend families={['bills']} showUpcomingDot />)
    expect(screen.getByTestId('ego-type-legend').textContent).toBe('billshas upcoming shows')
  })

  // The MIXED case is the only one that discriminates: with both flags set or
  // both clear, collapsing the two gates into one `showUpcomingDot ||
  // showPlayableRing` reads identically, and every other spec (here and in
  // ArtistGraph.palette) renders one of those two. Without this, a canvas
  // carrying only violet rings could grow a bogus upcoming-show key.
  it('shows only the playable key when that is the only marker on the canvas', () => {
    render(<EgoTypeLegend families={['bills']} showPlayableRing />)
    expect(screen.getByTestId('ego-type-legend').textContent).toBe('billsplayable audio')
  })

  it('frames the keys as a named group, since every swatch is aria-hidden', () => {
    render(<EgoTypeLegend families={['bills']} showUpcomingDot />)
    expect(screen.getByRole('group', { name: 'Graph legend' })).toBeInTheDocument()
  })
})
