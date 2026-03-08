import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowStatusBadge } from './ShowStatusBadge'
import type { ShowResponse } from '@/lib/types/show'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'approved',
    venues: [],
    artists: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowStatusBadge', () => {
  it('renders nothing when show is neither cancelled nor sold out', () => {
    const { container } = render(
      <ShowStatusBadge show={makeShow()} />
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders CANCELLED badge when show is cancelled', () => {
    render(<ShowStatusBadge show={makeShow({ is_cancelled: true })} />)
    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
  })

  it('renders SOLD OUT badge when show is sold out', () => {
    render(<ShowStatusBadge show={makeShow({ is_sold_out: true })} />)
    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  it('renders both badges when show is cancelled and sold out', () => {
    render(
      <ShowStatusBadge
        show={makeShow({ is_cancelled: true, is_sold_out: true })}
      />
    )
    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  it('applies custom className to wrapper span', () => {
    const { container } = render(
      <ShowStatusBadge
        show={makeShow({ is_cancelled: true })}
        className="my-custom-class"
      />
    )
    const wrapper = container.querySelector('span.my-custom-class')
    expect(wrapper).toBeInTheDocument()
  })
})
