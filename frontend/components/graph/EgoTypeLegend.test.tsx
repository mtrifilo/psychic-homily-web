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
  // near-term window "playing soon" implied. Pinned here (not only through
  // ArtistGraph's concatenated-string assertion) so the wording cannot drift
  // back on the component that owns it.
  it('states the upcoming-show marker as a predicate, not a time window', () => {
    render(<EgoTypeLegend families={['bills']} showUpcomingDot />)
    const legend = screen.getByTestId('ego-type-legend')
    expect(legend.textContent).toBe('billshas upcoming shows')
    expect(legend.textContent).not.toMatch(/soon/i)
  })

  it('omits each marker key when its marker is absent from the canvas', () => {
    render(<EgoTypeLegend families={['bills']} />)
    const legend = screen.getByTestId('ego-type-legend')
    expect(legend.textContent).toBe('bills')
  })
})
