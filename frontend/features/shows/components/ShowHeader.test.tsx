import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ArtistResponse, SetType, ShowResponse } from '../types'

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

      expect(supportLineText()).toMatch(
        /First Support.*Second Support.*Third Support/s
      )
    })

    it('renders multiple headliners in position order', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 2, name: 'Co Headliner', slug: 'co', set_type: 'headliner', position: 1 }),
          makeArtist({ id: 1, name: 'Main Headliner', slug: 'main', set_type: 'headliner', position: 0 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.getByRole('heading', { level: 1 }).textContent ?? '').toMatch(
        /Main Headliner.*Co Headliner/s
      )
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

    // Ties are produced by show merges, which re-point a loser show's
    // show_artists rows onto the winner carrying their own positions. The
    // backend's ORDER BY has no tiebreaker, so the client must impose one.
    it('breaks position ties on id so tied artists render deterministically', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Alpha', slug: 'alpha', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 3, name: 'Charlie', slug: 'charlie', position: 0 }),
          makeArtist({ id: 2, name: 'Bravo', slug: 'bravo', position: 0 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(supportLineText()).toMatch(/Bravo.*Charlie/s)
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
    // `opener` is the backend's default for every non-headliner, not a
    // distinguishing role, so annotating it would mark nearly every support
    // act. Locked in so it is not "fixed" back the other way by accident.
    it('leaves openers unannotated', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'The Opener', slug: 'the-opener', set_type: 'opener', position: 1 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(screen.queryByText('(opener)')).not.toBeInTheDocument()
      expect(supportLineText()).toContain('The Opener')
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

    // `set_type` is a bare string on the wire over an unconstrained VARCHAR
    // column, so a hostile/corrupt value must not resolve through the
    // prototype chain and crash the render.
    it('renders no annotation for a set_type that is not in the label map', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({
            id: 2,
            name: 'Odd One',
            slug: 'odd',
            set_type: '__proto__' as SetType,
            position: 1,
          }),
        ],
      })

      expect(() => render(<ShowHeader show={show} />)).not.toThrow()
      expect(supportLineText()).toContain('Odd One')
      expect(supportLineText()).not.toMatch(/\(/)
    })

    it('leaves generic performers unannotated', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 2, name: 'Just A Band', slug: 'band', set_type: 'performer', position: 1 }),
        ],
      })

      render(<ShowHeader show={show} />)

      expect(supportLineText()).toContain('Just A Band')
      expect(supportLineText()).not.toMatch(/\(/)
    })
  })
})
