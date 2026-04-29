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
  // PSY-349: server provides server-rendered + sanitized HTML alongside raw
  // markdown. Tests use realistic <p>-wrapped output that goldmark would emit.
  description_html: '<p>The best indie bands from AZ</p>',
  is_public: true,
  collaborative: false,
  is_featured: false,
  cover_image_url: null,
  creator_id: 1,
  creator_name: 'testuser',
  contributor_count: 0,
  display_mode: 'unranked',
  item_count: 5,
  subscriber_count: 10,
  forks_count: 0,
  forked_from_collection_id: null,
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
    // PSY-349: card renders description_html (server-sanitized), so an empty
    // description_html means nothing is rendered even if `description` has
    // legacy raw content.
    const collection = {
      ...baseCollection,
      description: '',
      description_html: '',
    }
    render(<CollectionCard collection={collection} />)

    expect(screen.queryByText('The best indie bands from AZ')).not.toBeInTheDocument()
  })

  it('renders creator name', () => {
    render(<CollectionCard collection={baseCollection} />)

    expect(screen.getByText(/testuser/)).toBeInTheDocument()
  })

  // PSY-353: creator attribution links to /users/:username when the
  // creator has a username set; otherwise renders as plain text so we
  // never produce a link to a non-existent profile.
  it('links creator name to /users/:username when creator_username is set', () => {
    const collection = { ...baseCollection, creator_username: 'testuser' }
    render(<CollectionCard collection={collection} />)

    const link = screen.getByRole('link', { name: 'testuser' })
    expect(link).toHaveAttribute('href', '/users/testuser')
  })

  it('does not link creator name when creator_username is null', () => {
    const collection = { ...baseCollection, creator_username: null }
    render(<CollectionCard collection={collection} />)

    expect(
      screen.queryByRole('link', { name: 'testuser' })
    ).not.toBeInTheDocument()
    expect(screen.getByText(/testuser/)).toBeInTheDocument()
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

  // PSY-353: "Built by N contributors" badge surfaces community curation
  // once at least 3 distinct users have added items. Below the threshold
  // the card stays creator-only to avoid noise on solo collections.
  describe('PSY-353 contributor badge', () => {
    it('renders the contributor badge when contributor_count >= 3', () => {
      const collection = { ...baseCollection, contributor_count: 5 }
      render(<CollectionCard collection={collection} />)

      const badge = screen.getByTestId('contributor-badge')
      expect(badge).toBeInTheDocument()
      expect(badge.textContent).toContain('Built by 5 contributors')
    })

    it('renders at the threshold (exactly 3 contributors)', () => {
      const collection = { ...baseCollection, contributor_count: 3 }
      render(<CollectionCard collection={collection} />)

      const badge = screen.getByTestId('contributor-badge')
      expect(badge.textContent).toContain('Built by 3 contributors')
    })

    it('omits the badge when contributor_count is below 3', () => {
      const collection = { ...baseCollection, contributor_count: 2 }
      render(<CollectionCard collection={collection} />)

      expect(screen.queryByTestId('contributor-badge')).not.toBeInTheDocument()
    })

    it('omits the badge when contributor_count is 0', () => {
      render(<CollectionCard collection={baseCollection} />)

      expect(screen.queryByTestId('contributor-badge')).not.toBeInTheDocument()
    })
  })
})
