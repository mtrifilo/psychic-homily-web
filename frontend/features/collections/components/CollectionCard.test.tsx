import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

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

// PSY-352: auth + like mutation mocks. Default: anonymous viewer + no-op
// mutations. Individual tests override these via mockIsAuthenticated and
// the mutation spies below.
let mockIsAuthenticated = false
const mockLikeMutate = vi.fn()
const mockUnlikeMutate = vi.fn()
let mockIsLikePending = false

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: mockIsAuthenticated, user: null }),
}))

vi.mock('../hooks', () => ({
  useLikeCollection: () => ({
    mutate: mockLikeMutate,
    isPending: mockIsLikePending,
  }),
  useUnlikeCollection: () => ({
    mutate: mockUnlikeMutate,
    isPending: mockIsLikePending,
  }),
}))

beforeEach(() => {
  mockIsAuthenticated = false
  mockIsLikePending = false
  mockLikeMutate.mockReset()
  mockUnlikeMutate.mockReset()
})

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
  like_count: 0,
  user_likes_this: false,
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

  // PSY-554: when the cover URL 404s, the tile must not stay blank — the
  // existing entity-type mosaic / Library fallback used for null URLs
  // should also render after onError fires.
  it('falls back to the entity-type mosaic when the cover image fails to load', () => {
    const collection = {
      ...baseCollection,
      cover_image_url: 'https://example.com/missing.jpg',
      entity_type_counts: { artist: 3, release: 1 },
    }
    render(<CollectionCard collection={collection} />)

    const img = screen.getByRole('img', { name: 'Arizona Indie Essentials cover' })
    fireEvent.error(img)
    expect(
      screen.queryByRole('img', { name: 'Arizona Indie Essentials cover' })
    ).not.toBeInTheDocument()
    // Mosaic icons replace the broken image; we don't assert on the
    // specific Lucide markup since that's an implementation detail of
    // CollectionCoverImage's fallback prop. Sanity-check is that the
    // image is gone but the surrounding card is still intact.
    expect(screen.getByText('Arizona Indie Essentials')).toBeInTheDocument()
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

  // ──────────────────────────────────────────────
  // PSY-352: like toggle
  // ──────────────────────────────────────────────

  it('renders a clickable heart with count for authenticated viewers', () => {
    mockIsAuthenticated = true
    const collection = { ...baseCollection, like_count: 4, user_likes_this: false }
    render(<CollectionCard collection={collection} />)

    const btn = screen.getByTestId('collection-like-button')
    expect(btn).toBeInTheDocument()
    expect(btn).toHaveTextContent('4')
    expect(btn).toHaveAttribute('aria-pressed', 'false')
    expect(btn).toHaveAttribute('aria-label', 'Like collection')
  })

  it('renders a non-interactive heart + count for anonymous viewers when likes exist', () => {
    mockIsAuthenticated = false
    const collection = { ...baseCollection, like_count: 7 }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByTestId('collection-like-count')).toHaveTextContent('7')
    expect(screen.queryByTestId('collection-like-button')).not.toBeInTheDocument()
  })

  it('hides the heart entirely for anonymous viewers when like_count is 0', () => {
    mockIsAuthenticated = false
    const collection = { ...baseCollection, like_count: 0 }
    render(<CollectionCard collection={collection} />)

    expect(screen.queryByTestId('collection-like-count')).not.toBeInTheDocument()
    expect(screen.queryByTestId('collection-like-button')).not.toBeInTheDocument()
  })

  it('still renders the heart for authenticated viewers when like_count is 0', () => {
    mockIsAuthenticated = true
    const collection = { ...baseCollection, like_count: 0 }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByTestId('collection-like-button')).toHaveTextContent('0')
  })

  it('marks the heart as pressed when user_likes_this is true', () => {
    mockIsAuthenticated = true
    const collection = { ...baseCollection, like_count: 1, user_likes_this: true }
    render(<CollectionCard collection={collection} />)

    const btn = screen.getByTestId('collection-like-button')
    expect(btn).toHaveAttribute('aria-pressed', 'true')
    expect(btn).toHaveAttribute('aria-label', 'Unlike collection')
  })

  it('calls likeCollection when an unliked heart is clicked', () => {
    mockIsAuthenticated = true
    const collection = { ...baseCollection, like_count: 0, user_likes_this: false }
    render(<CollectionCard collection={collection} />)

    fireEvent.click(screen.getByTestId('collection-like-button'))
    expect(mockLikeMutate).toHaveBeenCalledWith({ slug: 'arizona-indie-essentials' })
    expect(mockUnlikeMutate).not.toHaveBeenCalled()
  })

  it('calls unlikeCollection when an already-liked heart is clicked', () => {
    mockIsAuthenticated = true
    const collection = { ...baseCollection, like_count: 1, user_likes_this: true }
    render(<CollectionCard collection={collection} />)

    fireEvent.click(screen.getByTestId('collection-like-button'))
    expect(mockUnlikeMutate).toHaveBeenCalledWith({ slug: 'arizona-indie-essentials' })
    expect(mockLikeMutate).not.toHaveBeenCalled()
  })

  it('disables the heart while a like mutation is pending', () => {
    mockIsAuthenticated = true
    mockIsLikePending = true
    const collection = { ...baseCollection, user_likes_this: false }
    render(<CollectionCard collection={collection} />)

    expect(screen.getByTestId('collection-like-button')).toBeDisabled()
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

  // PSY-354: tag chip rendering on the card.
  describe('tag chips (PSY-354)', () => {
    it('does not render the chip row when tags is empty', () => {
      render(<CollectionCard collection={baseCollection} />)
      expect(
        screen.queryByTestId('collection-card-tags')
      ).not.toBeInTheDocument()
    })

    it('renders a chip per tag, linking to /collections?tag=<slug>', () => {
      const collection: Collection = {
        ...baseCollection,
        tags: [
          {
            id: 1,
            name: 'phoenix',
            slug: 'phoenix',
            category: 'locale',
            is_official: false,
            usage_count: 4,
          },
          {
            id: 2,
            name: 'best-of-2026',
            slug: 'best-of-2026',
            category: 'other',
            is_official: false,
            usage_count: 1,
          },
        ],
      }
      render(<CollectionCard collection={collection} />)
      const row = screen.getByTestId('collection-card-tags')
      expect(row).toBeInTheDocument()
      const phoenix = screen.getByRole('link', { name: 'phoenix' })
      expect(phoenix).toHaveAttribute(
        'href',
        '/collections?tag=phoenix'
      )
      const bestOf = screen.getByRole('link', { name: 'best-of-2026' })
      expect(bestOf).toHaveAttribute(
        'href',
        '/collections?tag=best-of-2026'
      )
    })

    it('caps visible chips at 5 and shows the overflow count', () => {
      const collection: Collection = {
        ...baseCollection,
        tags: Array.from({ length: 7 }, (_, i) => ({
          id: i + 1,
          name: `tag-${i + 1}`,
          slug: `tag-${i + 1}`,
          category: 'other',
          is_official: false,
          usage_count: 1,
        })),
      }
      render(<CollectionCard collection={collection} />)
      const row = screen.getByTestId('collection-card-tags')
      expect(row.querySelectorAll('a').length).toBe(5)
      expect(row.textContent).toContain('+2')
    })
  })
})
