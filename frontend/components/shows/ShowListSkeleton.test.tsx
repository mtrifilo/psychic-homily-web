import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowListSkeleton } from './ShowListSkeleton'

describe('ShowListSkeleton', () => {
  it('renders a section element', () => {
    render(<ShowListSkeleton />)
    const section = document.querySelector('section')
    expect(section).toBeInTheDocument()
  })

  it('renders 6 show card skeletons', () => {
    const { container } = render(<ShowListSkeleton />)
    // Each show card skeleton has a border/rounded-lg container
    const cardSkeletons = container.querySelectorAll('.border.border-border\\/50.rounded-lg')
    expect(cardSkeletons).toHaveLength(6)
  })

  it('renders city filter skeletons', () => {
    const { container } = render(<ShowListSkeleton />)
    // City filter skeleton area has 3 rounded-full skeleton pills
    const filterSkeletons = container.querySelectorAll('.rounded-full')
    expect(filterSkeletons).toHaveLength(3)
  })
})
