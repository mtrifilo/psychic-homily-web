import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowPrice } from './ShowPrice'

describe('ShowPrice', () => {
  it('renders a lone price bare, with no label to read twice', () => {
    render(<ShowPrice show={{ price: 35, door_price: null }} />)
    const price = screen.getByText('$35')
    expect(price).toBeInTheDocument()
    expect(price).not.toHaveAttribute('aria-label')
    expect(price).not.toHaveAttribute('title')
  })

  // The a11y half of the register decision, and the reason this is a component
  // rather than a span per surface: a screen reader announces "$35/$40" as
  // punctuation unless the pair is spelled out for it.
  it('spells a split price out for a screen reader and on hover', () => {
    render(<ShowPrice show={{ price: 35, door_price: 40 }} />)
    const price = screen.getByText('$35/$40')
    expect(price).toHaveAttribute('aria-label', '$35 advance, $40 at the door')
    expect(price).toHaveAttribute('title', '$35 advance, $40 at the door')
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
