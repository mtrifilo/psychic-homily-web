import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ExplorePage } from './ExplorePage'

vi.mock('./UpcomingShowsList', () => ({
  UpcomingShowsList: () => <div data-testid="upcoming-shows-list" />,
}))
vi.mock('./ShuffleCta', () => ({
  ShuffleCta: () => <div data-testid="shuffle-cta" />,
}))

describe('ExplorePage', () => {
  it('renders the heading + upcoming shows + shuffle CTA', () => {
    render(<ExplorePage />)
    expect(
      screen.getByRole('heading', { level: 1, name: /explore/i }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('upcoming-shows-list')).toBeInTheDocument()
    expect(screen.getByTestId('shuffle-cta')).toBeInTheDocument()
  })
})
