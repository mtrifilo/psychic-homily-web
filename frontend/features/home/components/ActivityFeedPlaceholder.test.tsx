import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ActivityFeedPlaceholder } from './ActivityFeedPlaceholder'

describe('ActivityFeedPlaceholder', () => {
  it('renders the "Across the scene" section heading', () => {
    render(<ActivityFeedPlaceholder />)
    expect(
      screen.getByRole('heading', { name: /across the scene/i })
    ).toBeInTheDocument()
  })

  it('renders the reserved-slot label (placeholder, not a live feed)', () => {
    render(<ActivityFeedPlaceholder />)
    expect(
      screen.getByText('Reserved for the Activity feed')
    ).toBeInTheDocument()
  })

  it('credits the Following Feed project that owns the real feed', () => {
    render(<ActivityFeedPlaceholder />)
    expect(screen.getByText(/PSY-988/)).toBeInTheDocument()
  })

  it('does not render any interactive links (it is a static placeholder)', () => {
    const { container } = render(<ActivityFeedPlaceholder />)
    expect(container.querySelectorAll('a')).toHaveLength(0)
  })
})
