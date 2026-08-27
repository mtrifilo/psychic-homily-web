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
  // The two exact-textContent cases above already pin marker-key ABSENCE.
  //
  // Both marker cases below are also the MIXED ones (each flag set with the
  // other defaulting false), which is what makes them worth a case each: with
  // both flags set or both clear, collapsing the two gates into a single
  // `showUpcomingDot || showPlayableRing` renders identically, and
  // ArtistGraph.palette only ever renders both. Without these, a canvas
  // carrying one marker could grow the other one's key.
  it('states the upcoming-show marker as a predicate, not a time window', () => {
    render(<EgoTypeLegend families={['bills']} showUpcomingDot />)
    expect(screen.getByTestId('ego-type-legend').textContent).toBe('billshas upcoming shows')
  })

  it('shows only the playable key when that is the only marker on the canvas', () => {
    render(<EgoTypeLegend families={['bills']} showPlayableRing />)
    expect(screen.getByTestId('ego-type-legend').textContent).toBe('billsplayable audio')
  })

  // The keys carry the whole meaning: every swatch is decorative, so a reader
  // that surfaced them would hear the color twice and the meaning never.
  it('hides every swatch from assistive tech', () => {
    const { container } = render(
      <EgoTypeLegend families={['bills', null]} showUpcomingDot showPlayableRing />
    )
    const swatches = container.querySelectorAll('span > span')
    expect(swatches.length).toBe(4)
    for (const swatch of swatches) {
      expect(swatch).toHaveAttribute('aria-hidden', 'true')
    }
  })
})
