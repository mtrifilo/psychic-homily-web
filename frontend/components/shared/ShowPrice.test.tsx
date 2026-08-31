import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowPrice } from './ShowPrice'

describe('ShowPrice', () => {
  it('renders a lone price bare, with no label to read twice', () => {
    render(<ShowPrice show={{ price: 35, door_price: null }} />)
    const price = screen.getByText('$35')
    expect(price).toBeInTheDocument()
    expect(price).not.toHaveAttribute('title')
  })

  // The a11y half of the register decision, and the reason this is a component
  // rather than a span per surface: a screen reader announces "$35/$40" as
  // punctuation unless the pair is spelled out for it.
  //
  // Asserted through the TEXT a screen reader would reach, not through an
  // attribute. The first implementation used `aria-label`, which a bare span
  // (role `generic`) is forbidden to take — so an attribute assertion passed
  // while browsers went on reading "thirty five slash forty".
  it('spells a split price out for a screen reader and on hover', () => {
    render(<ShowPrice show={{ price: 35, door_price: 40 }} />)
    expect(screen.getByText('$35 advance, $40 at the door')).toBeInTheDocument()
    expect(screen.getByTitle('$35 advance, $40 at the door')).toBeInTheDocument()
    // The glyphs are still what a sighted reader sees, and are hidden from the
    // accessibility tree rather than removed.
    const glyphs = screen.getByText('$35/$40')
    expect(glyphs).toHaveAttribute('aria-hidden', 'true')
  })

  it('renders the fallback when no price is recorded', () => {
    render(<ShowPrice show={{ price: null, door_price: null }} fallback="—" />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  // A meta row wants silence rather than a placeholder holding a column open,
  // so the fallback is optional and defaults to nothing.
  it('renders nothing when no price is recorded and no fallback is given', () => {
    const { container } = render(
      <ShowPrice show={{ price: null, door_price: null }} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  // Zero is a price the site asserts, not an absence, so it must never fall to
  // the fallback.
  it('renders a free show as Free, not as the fallback', () => {
    render(<ShowPrice show={{ price: 0, door_price: null }} fallback="—" />)
    expect(screen.getByText('Free')).toBeInTheDocument()
    expect(screen.queryByText('—')).not.toBeInTheDocument()
  })
})
