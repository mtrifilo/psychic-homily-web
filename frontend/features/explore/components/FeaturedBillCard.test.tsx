import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FeaturedBillCard } from './FeaturedBillCard'
import type { ExploreFeaturedBill } from '../types'

const baseBill: ExploreFeaturedBill = {
  id: 42,
  slug: 'featured-bill',
  title: 'Big Show',
  event_date: '2026-06-15T03:00:00Z',
  headliner_name: 'Cool Band',
  venue_name: 'The Trunk Space',
  venue_city: 'Phoenix',
  venue_state: 'AZ',
  curator_note_html: '<p>Why this bill matters.</p>',
}

describe('FeaturedBillCard', () => {
  it('renders the headliner, venue line, and a view-show link', () => {
    render(<FeaturedBillCard bill={baseBill} />)
    expect(screen.getByText('Cool Band')).toBeInTheDocument()
    // venue line
    expect(screen.getByText(/The Trunk Space/)).toBeInTheDocument()
    // view-show CTA points to the show detail page
    const cta = screen.getByRole('link', { name: /view show/i })
    expect(cta).toHaveAttribute('href', '/shows/featured-bill')
  })

  it('renders the curator note as sanitized HTML', () => {
    const { container } = render(<FeaturedBillCard bill={baseBill} />)
    expect(container.querySelector('p')?.textContent).toBe(
      'Why this bill matters.',
    )
  })

  it('omits the curator note block when no HTML is provided', () => {
    const { container } = render(
      <FeaturedBillCard bill={{ ...baseBill, curator_note_html: undefined }} />,
    )
    expect(container.querySelector('p')).toBeNull()
  })

  it('renders the thumbnail when image_url is set', () => {
    render(
      <FeaturedBillCard bill={{ ...baseBill, image_url: '/big.jpg' }} />,
    )
    const img = screen.getByRole('img', { name: 'Big Show' })
    expect(img).toBeInTheDocument()
  })

  it('omits the thumbnail when image_url is absent', () => {
    render(<FeaturedBillCard bill={baseBill} />)
    expect(screen.queryByRole('img')).toBeNull()
  })
})
