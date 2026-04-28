import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string; [key: string]: unknown }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

// Mock formatRelativeTime
vi.mock('@/lib/formatRelativeTime', () => ({
  formatRelativeTime: (date: string) => `relative(${date})`,
}))

import { CollectionCard } from './CollectionCard'
import type { Collection } from '../types'

const baseCollection: Collection = {
  id: 1,
  title: 'Arizona Indie Essentials',
  slug: 'arizona-indie-essentials',
  description: 'The best indie bands from AZ',
  is_public: true,
  collaborative: false,
  is_featured: false,
  cover_image_url: null,
  creator_id: 1,
  creator_name: 'testuser',
  contributor_count: 0,
  item_count: 5,
  subscriber_count: 10,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
}

describe('CollectionCard', () => {
  it('renders collection title as a link', () => {
    render(<CollectionCard collection={baseCollection} />)

    const link = screen.getByRole('link', { name: 'Arizona Indie Essentials' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/collections/arizona-indie-essentials')
  })

  it('renders description when present', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.getByText('The best indie bands from AZ')).toBeInTheDocument()
  })

  it('does not render description when absent', () => {
    const collection = { ...baseCollection, description: null as unknown as string }
    render(<CollectionCard collection={collection} />)

    expect(screen.queryByText('The best indie bands from AZ')).not.toBeInTheDocument()
  })

  it('renders creator name', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.getByText('by testuser')).toBeInTheDocument()
  })

  it('renders item count (plural)', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.getByText('5 items')).toBeInTheDocument()
  })

  it('renders singular item count', () => {
    const collection = { ...baseCollection, item_count: 1 }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByText('1 item')).toBeInTheDocument()
  })

  it('renders subscriber count when > 0', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.getByText('10 subscribers')).toBeInTheDocument()
  })

  it('renders singular subscriber count', () => {
    const collection = { ...baseCollection, subscriber_count: 1 }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByText('1 subscriber')).toBeInTheDocument()
  })

  it('does not render subscriber count when 0', () => {
    const collection = { ...baseCollection, subscriber_count: 0 }
    render(<CollectionCard collection={collection} />)

    expect(screen.queryByText('0 subscribers')).not.toBeInTheDocument()
    expect(screen.queryByText('subscribers')).not.toBeInTheDocument()
  })

  it('shows Featured badge when is_featured', () => {
    const collection = { ...baseCollection, is_featured: true }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByText('Featured')).toBeInTheDocument()
  })

  it('does not show Featured badge when not featured', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.queryByText('Featured')).not.toBeInTheDocument()
  })

  it('shows Collaborative badge when collaborative', () => {
    const collection = { ...baseCollection, collaborative: true }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByText('Collaborative')).toBeInTheDocument()
  })

  it('does not show Collaborative badge when not collaborative', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.queryByText('Collaborative')).not.toBeInTheDocument()
  })

  it('renders cover image when URL is provided', () => {
    const collection = { ...baseCollection, cover_image_url: 'https://example.com/cover.jpg' }
    render(<CollectionCard collection={collection} />)

    const img = screen.getByRole('img', { name: 'Arizona Indie Essentials cover' })
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg')
  })

  // PSY-350: "N new since last visit" badge
  it('renders the N-new badge when new_since_last_visit > 0', () => {
    const collection = { ...baseCollection, new_since_last_visit: 3 }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByText('3 new')).toBeInTheDocument()
    expect(
      screen.getByLabelText('3 new since your last visit')
    ).toBeInTheDocument()
  })

  it('omits the N-new badge when new_since_last_visit is 0', () => {
    const collection = { ...baseCollection, new_since_last_visit: 0 }
    render(<CollectionCard collection={collection} />)

    expect(screen.queryByText(/new$/)).not.toBeInTheDocument()
  })

  it('omits the N-new badge when new_since_last_visit is undefined', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.queryByText(/new$/)).not.toBeInTheDocument()
  })
})
