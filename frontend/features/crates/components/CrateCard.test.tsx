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

import { CrateCard } from './CrateCard'
import type { Crate } from '../types'

const baseCrate: Crate = {
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
  item_count: 5,
  subscriber_count: 10,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
}

describe('CrateCard', () => {
  it('renders crate title as a link', () => {
    render(<CrateCard crate={baseCrate} />)

    const link = screen.getByRole('link', { name: 'Arizona Indie Essentials' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/crates/arizona-indie-essentials')
  })

  it('renders description when present', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.getByText('The best indie bands from AZ')).toBeInTheDocument()
  })

  it('does not render description when absent', () => {
    const crate = { ...baseCrate, description: null as unknown as string }
    render(<CrateCard crate={crate} />)

    expect(screen.queryByText('The best indie bands from AZ')).not.toBeInTheDocument()
  })

  it('renders creator name', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.getByText('by testuser')).toBeInTheDocument()
  })

  it('renders item count (plural)', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.getByText('5 items')).toBeInTheDocument()
  })

  it('renders singular item count', () => {
    const crate = { ...baseCrate, item_count: 1 }
    render(<CrateCard crate={crate} />)

    expect(screen.getByText('1 item')).toBeInTheDocument()
  })

  it('renders subscriber count when > 0', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.getByText('10 subscribers')).toBeInTheDocument()
  })

  it('renders singular subscriber count', () => {
    const crate = { ...baseCrate, subscriber_count: 1 }
    render(<CrateCard crate={crate} />)

    expect(screen.getByText('1 subscriber')).toBeInTheDocument()
  })

  it('does not render subscriber count when 0', () => {
    const crate = { ...baseCrate, subscriber_count: 0 }
    render(<CrateCard crate={crate} />)

    expect(screen.queryByText('0 subscribers')).not.toBeInTheDocument()
    expect(screen.queryByText('subscribers')).not.toBeInTheDocument()
  })

  it('shows Featured badge when is_featured', () => {
    const crate = { ...baseCrate, is_featured: true }
    render(<CrateCard crate={crate} />)

    expect(screen.getByText('Featured')).toBeInTheDocument()
  })

  it('does not show Featured badge when not featured', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.queryByText('Featured')).not.toBeInTheDocument()
  })

  it('shows Collaborative badge when collaborative', () => {
    const crate = { ...baseCrate, collaborative: true }
    render(<CrateCard crate={crate} />)

    expect(screen.getByText('Collaborative')).toBeInTheDocument()
  })

  it('does not show Collaborative badge when not collaborative', () => {
    render(<CrateCard crate={baseCrate} />)

    expect(screen.queryByText('Collaborative')).not.toBeInTheDocument()
  })

  it('renders cover image when URL is provided', () => {
    const crate = { ...baseCrate, cover_image_url: 'https://example.com/cover.jpg' }
    render(<CrateCard crate={crate} />)

    const img = screen.getByRole('img', { name: 'Arizona Indie Essentials cover' })
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg')
  })
})
