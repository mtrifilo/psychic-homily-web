import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ArtistResponse, ShowResponse } from '../types'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: undefined }),
}))

vi.mock('../hooks/useSavedShows', () => ({
  useSaveShow: () => ({ mutate: vi.fn(), isPending: false }),
  useShowSaveCount: () => ({ data: undefined }),
}))

import { ShowHeader } from './ShowHeader'

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist',
    name: 'Artist',
    set_type: 'performer',
    position: 0,
    socials: {},
    ...overrides,
  }
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-08-15T03:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    price: 12,
    age_requirement: '21+',
    description: null,
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 1,
        slug: 'the-venue',
        name: 'The Venue',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

/** The "w/ ..." support line, read as flat text for order assertions. */
function supportLineText(): string {
  const marker = screen.getByText('w/')
  return marker.parentElement?.textContent ?? ''
}

describe('ShowHeader bill rendering', () => {
  describe('bill order', () => {
    it('renders support artists in position order, not API array order', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          // Deliberately out of order relative to `position`.
          makeArtist({ id: 4, name: 'Third Support', slug: 'third', position: 3 }),
          makeArtist({ id: 2, name: 'First Support', slug: 'first', position: 1 }),
          makeArtist({ id: 3, name: 'Second Support', slug: 'second', position: 2 }),
        ],
      })

      render(<ShowHeader show={show} />)

      const text = supportLineText()
      expect(text.indexOf('First Support')).toBeLessThan(text.indexOf('Second Support'))
      expect(text.indexOf('Second Support')).toBeLessThan(text.indexOf('Third Support'))
    })

    it('renders multiple headliners in position order', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 2, name: 'Co Headliner', slug: 'co', set_type: 'headliner', position: 1 }),
          makeArtist({ id: 1, name: 'Main Headliner', slug: 'main', set_type: 'headliner', position: 0 }),
        ],
      })

      render(<ShowHeader show={show} />)

      const heading = screen.getByRole('heading', { level: 1 }).textContent ?? ''
      expect(heading.indexOf('Main Headliner')).toBeLessThan(heading.indexOf('Co Headliner'))
    })

    it('picks the lowest-position artist as the implicit headliner when none is flagged', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 2, name: 'Later Act', slug: 'later', position: 1 }),
          makeArtist({ id: 1, name: 'Opening Slot Zero', slug: 'zero', position: 0 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Opening Slot Zero')
      expect(supportLineText()).toContain('Later Act')
    })

    it('keeps API order for artists sharing a position (legacy all-zero rows)', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Alpha', slug: 'alpha', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'Bravo', slug: 'bravo', position: 0 }),
          makeArtist({ id: 3, name: 'Charlie', slug: 'charlie', position: 0 }),
        ],
      })

      render(<ShowHeader show={show} />)

      const text = supportLineText()
      expect(text.indexOf('Bravo')).toBeLessThan(text.indexOf('Charlie'))
    })

    it('does not mutate the artists array it was handed', () => {
      const artists = [
        makeArtist({ id: 2, name: 'Second', slug: 'second', position: 2 }),
        makeArtist({ id: 1, name: 'First', slug: 'first', position: 1 }),
      ]
      const show = makeShow({ artists })

      render(<ShowHeader show={show} />)

      expect(artists.map(a => a.name)).toEqual(['Second', 'First'])
    })
  })

  describe('set_type annotations', () => {
    it('annotates openers', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'The Opener', slug: 'the-opener', set_type: 'opener', position: 1 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.getByText('(opener)')).toBeInTheDocument()
    })

    it('annotates special guests', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'The Guest', slug: 'guest', set_type: 'special_guest', position: 1 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.getByText('(special guest)')).toBeInTheDocument()
    })

    it('leaves generic performers unannotated', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'Just A Band', slug: 'band', set_type: 'performer', position: 1 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.queryByText('(opener)')).not.toBeInTheDocument()
      expect(screen.queryByText('(special guest)')).not.toBeInTheDocument()
      expect(screen.queryByText('(performer)')).not.toBeInTheDocument()
    })
  })
})
