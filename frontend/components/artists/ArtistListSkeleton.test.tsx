import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ArtistListSkeleton } from './ArtistListSkeleton'

describe('ArtistListSkeleton', () => {
  it('renders without crashing', () => {
    const { container } = render(<ArtistListSkeleton />)
    expect(container.firstChild).toBeInTheDocument()
  })

  it('has animate-pulse class for loading animation', () => {
    const { container } = render(<ArtistListSkeleton />)
    expect(container.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders 12 card skeleton placeholders', () => {
    const { container } = render(<ArtistListSkeleton />)
    const cards = container.querySelectorAll('.rounded-lg.border')
    expect(cards).toHaveLength(12)
  })

  it('renders 3 filter chip skeleton placeholders', () => {
    const { container } = render(<ArtistListSkeleton />)
    const chips = container.querySelectorAll('.rounded-full.bg-muted')
    expect(chips).toHaveLength(3)
  })

  it('renders search bar skeleton placeholder', () => {
    const { container } = render(<ArtistListSkeleton />)
    const searchBar = container.querySelector('.h-9.w-full.max-w-sm.rounded-md.bg-muted')
    expect(searchBar).toBeInTheDocument()
  })
})
