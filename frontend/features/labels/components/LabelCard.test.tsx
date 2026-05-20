import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LabelCard } from './LabelCard'
import type { LabelListItem } from '../types'

// Mock next/link so hrefs render as plain anchors.
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

function makeLabel(overrides: Partial<LabelListItem> = {}): LabelListItem {
  return {
    id: 1,
    name: 'Sub Pop',
    slug: 'sub-pop',
    city: 'Seattle',
    state: 'WA',
    status: 'active',
    artist_count: 12,
    release_count: 340,
    ...overrides,
  }
}

describe('LabelCard', () => {
  it('renders as an article element', () => {
    render(<LabelCard label={makeLabel()} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('links the label name to its detail page', () => {
    render(<LabelCard label={makeLabel()} />)
    const link = screen.getByRole('link', { name: 'Sub Pop' })
    expect(link).toHaveAttribute('href', '/labels/sub-pop')
  })

  it('renders the formatted location', () => {
    render(<LabelCard label={makeLabel()} />)
    expect(screen.getByText('Seattle, WA')).toBeInTheDocument()
  })

  it('omits the location when neither city nor state is set', () => {
    render(<LabelCard label={makeLabel({ city: null, state: null })} />)
    expect(screen.queryByText(/,/)).not.toBeInTheDocument()
  })

  it('renders the status badge', () => {
    render(<LabelCard label={makeLabel({ status: 'defunct' })} />)
    expect(screen.getByText('Defunct')).toBeInTheDocument()
  })

  it('pluralizes artist and release counts', () => {
    render(<LabelCard label={makeLabel({ artist_count: 12, release_count: 340 })} />)
    expect(screen.getByText('12 artists')).toBeInTheDocument()
    expect(screen.getByText('340 releases')).toBeInTheDocument()
  })

  it('uses singular artist and release labels for a count of 1', () => {
    render(<LabelCard label={makeLabel({ artist_count: 1, release_count: 1 })} />)
    expect(screen.getByText('1 artist')).toBeInTheDocument()
    expect(screen.getByText('1 release')).toBeInTheDocument()
  })

  describe('compact density', () => {
    it('renders the name link and status but only the artist count', () => {
      render(<LabelCard label={makeLabel()} density="compact" />)
      expect(screen.getByRole('link', { name: 'Sub Pop' })).toHaveAttribute(
        'href',
        '/labels/sub-pop'
      )
      expect(screen.getByText('Active')).toBeInTheDocument()
      expect(screen.getByText('12 artists')).toBeInTheDocument()
      // Compact view does not surface the release count.
      expect(screen.queryByText('340 releases')).not.toBeInTheDocument()
    })
  })

  describe('expanded density', () => {
    it('renders name, status, location, and both counts', () => {
      render(<LabelCard label={makeLabel()} density="expanded" />)
      expect(screen.getByText('Sub Pop')).toBeInTheDocument()
      expect(screen.getByText('Active')).toBeInTheDocument()
      expect(screen.getByText('Seattle, WA')).toBeInTheDocument()
      expect(screen.getByText('12 artists')).toBeInTheDocument()
      expect(screen.getByText('340 releases')).toBeInTheDocument()
    })
  })
})
