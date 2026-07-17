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
})
