import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { stubAllImagesLoadState } from '@/test/stubAllImagesLoadState'
import type { ArtistResponse, SetType, ShowResponse } from '../types'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: undefined }),
}))

vi.mock('../hooks/useSavedShows', () => ({
  useSaveShow: () => ({ mutate: vi.fn(), isPending: false }),
  useShowSaveCount: () => ({ data: undefined }),
  useSaveShowToggle: () => ({
    isLoading: false,
    toggle: vi.fn(),
    error: null,
  }),
}))

// The ticket row's save bracket reads the router for its login redirect;
// there is no app router mounted in a render-only test.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/shows/test-show',
}))

// The venue module and ticket row pull in follow / notify / collection
// affordances whose hooks need a QueryClientProvider these render-only tests
// don't mount. Mocked at the component boundary, same convention as
// VenueDetail.test / ArtistDetail.test; each primitive's own behaviour is
// covered in its own test file.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ bracketLabel }: { bracketLabel?: string }) => (
    <button data-testid="follow-venue">[{bracketLabel ?? 'Follow'}]</button>
  ),
}))

// See ShowDetail.test: the `@/features/notifications` mock here served only
// ShowVenueModule's retired [Notify me] bracket (PSY-1905).

vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
  AddToCollectionButton: () => (
    <button data-testid="add-to-collection">[Add to collection]</button>
  ),
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

/** The grid that decides whether the flyer column exists at all. */
function layoutGrid(): HTMLElement {
  const grid = screen.getByRole('heading', { level: 1 }).closest('.grid')
  if (!grid) throw new Error('layout grid not found')
  return grid as HTMLElement
}

const TWO_COLUMN_CLASS = 'md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]'

describe('ShowHeader layout', () => {
  // The locked decision: no flyer means no reserved gutter. The earlier
  // always-on placeholder promised an image that was never coming.
  it('collapses to one full-width column when the show has no flyer', () => {
    render(<ShowHeader lifecycle="upcoming" show={makeShow({ image_url: null })} />)

    expect(screen.queryByTestId('show-flyer-plate')).not.toBeInTheDocument()
    expect(layoutGrid()).not.toHaveClass(TWO_COLUMN_CLASS)
  })

  it('renders the flyer beside the bill when the show has one', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/poster.jpg' })}
      />
    )

    expect(screen.getByTestId('show-flyer-image')).toHaveAttribute(
      'src',
      'https://flyers.example/poster.jpg'
    )
    expect(layoutGrid()).toHaveClass(TWO_COLUMN_CLASS)
  })

  // The plate is last in the DOM so a phone gets the bill before the poster
  // and a screen reader hears the content before the supplement. Desktop puts
  // it back in the mock's left column with `order`, not with markup order.
  it('reads after the bill in the document and renders left of it on desktop', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/poster.jpg' })}
      />
    )

    const plate = screen.getByTestId('show-flyer-plate')
    expect(plate).toHaveClass('md:order-first')
    expect(
      screen.getByRole('heading', { level: 1 }).compareDocumentPosition(plate) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  // The backend stores `image_url` untrimmed, so the padded form genuinely
  // reaches the client; the rendered src is the normalised URL, not the input.
  it('normalises a padded flyer url instead of putting whitespace in the src', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: '  https://flyers.example/poster.jpg  ' })}
      />
    )

    expect(screen.getByTestId('show-flyer-image')).toHaveAttribute(
      'src',
      'https://flyers.example/poster.jpg'
    )
  })

  // `image_url` is writable by any email-verified submitter, so an unusable
  // value must collapse the column rather than emit an <img src> built from it.
  it.each([
    ['a whitespace-only url', '   '],
    ['a relative path', '/uploads/flyer.jpg'],
    ['a non-http scheme', 'data:image/png;base64,AAAA'],
    ['an unparseable value', 'not a url at all'],
  ])('collapses the column for %s', (_label, imageUrl) => {
    render(<ShowHeader lifecycle="upcoming" show={makeShow({ image_url: imageUrl })} />)

    expect(screen.queryByTestId('show-flyer-plate')).not.toBeInTheDocument()
    expect(layoutGrid()).not.toHaveClass(TWO_COLUMN_CLASS)
  })

  // A dead hotlink is the same situation as no flyer: collapse, rather than
  // leave a broken-image glyph in a reserved 18rem gutter.
  it('collapses the column when the flyer fails to load', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/gone.jpg' })}
      />
    )

    fireEvent.error(screen.getByTestId('show-flyer-image'))

    expect(screen.queryByTestId('show-flyer-plate')).not.toBeInTheDocument()
    expect(layoutGrid()).not.toHaveClass(TWO_COLUMN_CLASS)
  })

  // The page is server-rendered, so a dead hotlink can 404 while the HTML is
  // still parsing, before React attaches its onError handler. Verified in a
  // browser against a 404 flyer: without the mount-time check the column
  // stayed reserved and the bill sat in the second column beside a blank
  // gutter, because that error event was fired at nobody.
  it('collapses the column for a flyer that already failed before hydration', () => {
    stubAllImagesLoadState({ complete: true, naturalWidth: 0 })

    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/gone.jpg' })}
      />
    )

    expect(screen.queryByTestId('show-flyer-plate')).not.toBeInTheDocument()
    expect(layoutGrid()).not.toHaveClass(TWO_COLUMN_CLASS)
  })

  // An admin can fix a bad `image_url` from the edit drawer on this very page.
  // The failure is keyed to the URL that failed, so a new one is not suppressed.
  it('shows a replacement flyer after the previous one failed', () => {
    const { rerender } = render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/gone.jpg' })}
      />
    )
    fireEvent.error(screen.getByTestId('show-flyer-image'))

    rerender(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({ image_url: 'https://flyers.example/fixed.jpg' })}
      />
    )

    expect(screen.getByTestId('show-flyer-image')).toHaveAttribute(
      'src',
      'https://flyers.example/fixed.jpg'
    )
  })

  // The only provenance a show row carries is the venue slug a discovery run
  // scraped the listing from; the credit prints that venue's display NAME.
  it('credits the venue the listing was scraped from', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({
          image_url: 'https://flyers.example/poster.jpg',
          source_venue: 'the-venue',
        })}
      />
    )

    expect(screen.getByTestId('show-flyer-credit')).toHaveTextContent(
      'flyer via The Venue'
    )
  })

  it.each([
    ['a user-submitted show with no source', undefined],
    ['a source venue that matches nothing on the show', 'some-other-venue'],
  ])('credits nobody for %s', (_label, sourceVenue) => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({
          image_url: 'https://flyers.example/poster.jpg',
          source_venue: sourceVenue,
        })}
      />
    )

    expect(screen.queryByTestId('show-flyer-credit')).not.toBeInTheDocument()
  })

  // A matched venue whose name is blank credits nobody rather than printing
  // "flyer via " with nothing after it.
  it('credits nobody when the matched venue has a blank name', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({
          image_url: 'https://flyers.example/poster.jpg',
          source_venue: 'the-venue',
          venues: [
            {
              id: 1,
              slug: 'the-venue',
              name: '   ',
              city: 'Phoenix',
              state: 'AZ',
              verified: true,
            },
          ],
        })}
      />
    )

    expect(screen.queryByTestId('show-flyer-credit')).not.toBeInTheDocument()
  })

  // The venue line goes through the same location helper as the bill above it.
  // The hand-written template it replaced printed "Phoenix, " for a venue with
  // no state.
  it('renders a stateless venue location without a trailing comma', () => {
    render(
      <ShowHeader
        lifecycle="upcoming"
        show={makeShow({
          venues: [
            {
              id: 1,
              slug: 'the-venue',
              name: 'The Venue',
              city: 'Berlin',
              state: '',
              verified: true,
            },
          ],
        })}
      />
    )

    expect(screen.getByTestId('venue-location')).toHaveTextContent('Berlin')
    expect(
      screen.getByTestId('venue-location').textContent?.trim()
    ).toBe('Berlin')
  })

  // The venue is where the show happens, so its state decides the calendar the
  // date is read on. The status stripe resolves the zone the same way; a header
  // that used the denormalized show row could print a different day inches
  // away from the band.
  it('dates the show on the venue calendar, not the show row', () => {
    const show = makeShow({
      // 12:30 AM Aug 15 in Chicago, still 10:30 PM Aug 14 in Phoenix. The
      // venue has no resolved timezone, so the state is what decides.
      event_date: '2026-08-15T05:30:00Z',
      state: 'AZ',
      venues: [
        {
          id: 1,
          slug: 'the-venue',
          name: 'The Venue',
          city: 'Chicago',
          state: 'IL',
          verified: true,
        },
      ],
    })

    render(<ShowHeader lifecycle="upcoming" show={show} />)

    expect(screen.getByText(/Sat, Aug 15/)).toBeInTheDocument()
    expect(screen.queryByText(/Fri, Aug 14/)).not.toBeInTheDocument()
  })
})

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

      render(<ShowHeader lifecycle="upcoming" show={show} />)

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

      render(<ShowHeader lifecycle="upcoming" show={show} />)

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

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Opening Slot Zero')
      expect(supportLineText()).toContain('Later Act')
    })

    // Nothing enforces one position per show (the index is non-unique) and the
    // backend's ORDER BY has no tiebreaker, so the client must impose one.
    it('breaks position ties on id so tied artists render deterministically', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Alpha', slug: 'alpha', set_type: 'headliner', position: 0 }),
          makeArtist({ id: 3, name: 'Charlie', slug: 'charlie', position: 0 }),
          makeArtist({ id: 2, name: 'Bravo', slug: 'bravo', position: 0 }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(supportLineText()).toMatch(/Bravo.*Charlie/s)
    })

    it('does not mutate the artists array it was handed', () => {
      const artists = [
        makeArtist({ id: 2, name: 'Second', slug: 'second', position: 2 }),
        makeArtist({ id: 1, name: 'First', slug: 'first', position: 1 }),
      ]
      const show = makeShow({ artists })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(artists.map(a => a.name)).toEqual(['Second', 'First'])
    })
  })

  describe('label and hometown annotations', () => {
    /** The headliner line, read as flat text. */
    function headlineText(): string {
      return screen.getByRole('heading', { level: 1 }).textContent ?? ''
    }

    it('annotates the headliner with its labels and hometown', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Modest Mouse',
            slug: 'modest-mouse',
            set_type: 'headliner',
            position: 0,
            city: 'Issaquah',
            state: 'WA',
            country: 'USA',
            labels: [{ id: 10, name: 'Epic', slug: 'epic' }],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      // Exact, so the mock's locked SEQUENCE is pinned: name, then labels,
      // then hometown, with nothing interposed.
      expect(headlineText()).toBe('Modest Mouse on [Epic] from Issaquah, WA')
      // Locked location rule: the country is suppressed for a US state.
      expect(headlineText()).not.toContain('USA')
      // The accessible name is the assertion that actually pins the a11y
      // decisions: it honours `aria-hidden` (so the brackets vanish) and keeps
      // `sr-only` (so the connectives stay). Deleting either would leave the
      // textContent assertion above green.
      expect(screen.getByRole('heading', { level: 1 })).toHaveAccessibleName(
        'Modest Mouse on Epic from Issaquah, WA'
      )
      expect(screen.getByRole('link', { name: 'Epic' })).toHaveAttribute(
        'href',
        '/labels/epic'
      )
    })

    // Two labels are ONE fact about one band, so they share one bracket pair
    // with a middot between them, not a bracket each.
    it('renders multiple labels middot-separated inside a single bracket', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({
            id: 2,
            name: 'Califone',
            slug: 'califone',
            position: 1,
            city: 'Chicago',
            state: 'IL',
            labels: [
              { id: 20, name: 'Jealous Butcher', slug: 'jealous-butcher' },
              { id: 21, name: 'Dead Oceans', slug: 'dead-oceans' },
            ],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(supportLineText()).toContain(
        '[Jealous Butcher · Dead Oceans]'
      )
      expect(supportLineText()).toContain('Chicago, IL')
      expect(
        screen.getByRole('link', { name: 'Dead Oceans' })
      ).toHaveAttribute('href', '/labels/dead-oceans')
    })

    // A co-headline bill runs two annotated names through one heading. The
    // bullet between them is decoration and must not be announced, while the
    // spaces around it must survive.
    it('annotates every headliner on a co-headline bill', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'First Name',
            slug: 'first',
            set_type: 'headliner',
            position: 0,
            city: 'Chicago',
            state: 'IL',
            labels: [{ id: 10, name: 'Label One', slug: 'label-one' }],
          }),
          makeArtist({
            id: 2,
            name: 'Second Name',
            slug: 'second',
            set_type: 'headliner',
            position: 1,
            city: 'Melbourne',
            country: 'Australia',
            labels: [{ id: 11, name: 'Label Two', slug: 'label-two' }],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      // textContent ignores aria-hidden, so the bullet is in this string; what
      // matters is that the SPACES around it are their own text nodes, outside
      // the hidden span, and that the span itself is hidden.
      expect(headlineText()).toBe(
        'First Name on [Label One] from Chicago, IL • ' +
          'Second Name on [Label Two] from Melbourne, Australia'
      )
      expect(screen.getByText('•')).toHaveAttribute('aria-hidden', 'true')
      expect(screen.getByRole('link', { name: 'Label Two' })).toHaveAttribute(
        'href',
        '/labels/label-two'
      )
    })

    it('renders an unsigned artist with no brackets at all', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'No Label Band',
            slug: 'no-label',
            set_type: 'headliner',
            position: 0,
            city: 'Phoenix',
            state: 'AZ',
            labels: [],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(headlineText()).toBe('No Label Band from Phoenix, AZ')
    })

    // The list endpoints omit the key entirely (absent means "not looked up").
    it('renders no brackets when labels were never looked up', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Unknown Signing', slug: 'unknown', set_type: 'headliner', position: 0 }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(headlineText()).toBe('Unknown Signing')
    })

    // `labels.slug` is nullable in the database and flattened to "" on the
    // wire; linking it would build a link to `/labels/`.
    it('renders a label with no slug as plain text', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Top Bill',
            slug: 'top',
            set_type: 'headliner',
            position: 0,
            labels: [{ id: 30, name: 'Unslugged Records', slug: '' }],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      // Also the labels-without-hometown case: pins the spacing when
      // BillLabels is the last thing on the line.
      expect(headlineText()).toBe('Top Bill on [Unslugged Records]')
      expect(
        screen.queryByRole('link', { name: 'Unslugged Records' })
      ).not.toBeInTheDocument()
    })

    it('keeps the country for an artist outside the US', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Rolling Blackouts',
            slug: 'rbcf',
            set_type: 'headliner',
            position: 0,
            city: 'Melbourne',
            country: 'Australia',
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(headlineText()).toBe('Rolling Blackouts from Melbourne, Australia')
    })

    // `formatLocation`'s placeholder is designed to stand alone in a location
    // field, not to be read mid-line as part of the bill.
    it('prints no placeholder when an artist has no placeable location', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Placeless',
            slug: 'placeless',
            set_type: 'headliner',
            position: 0,
            city: null,
            state: null,
            country: null,
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(headlineText()).toBe('Placeless')
    })

    // The drop is decided on the PARTS. A string comparison against the
    // formatter's placeholder would also silence a band whose city is
    // literally that placeholder, which is what an extraction run writes when
    // it does not know.
    it('still prints a hometown for a city named like the unknown placeholder', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Edge Case',
            slug: 'edge-case',
            set_type: 'headliner',
            position: 0,
            city: 'Location Unknown',
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(headlineText()).toBe('Edge Case from Location Unknown')
    })
  })

  describe('set_type annotations', () => {
    // The locked mock renders NO role labels on the bill: support acts are
    // `w/` lines and nothing more. `set_type` is curated and authoritative
    // since PSY-1673, so the data exists and adding annotations is now a
    // DESIGN decision rather than a data-quality one. This table is what makes
    // that decision cost a failing test instead of a one-line map entry.
    it.each([
      ['opener', 'opener'],
      ['direct_support', 'direct_support'],
      ['dj', 'dj'],
      ['performer', 'performer'],
    ])('leaves a %s unannotated', (_label, setType) => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({
            id: 2,
            name: 'The Support',
            slug: 'the-support',
            set_type: setType as SetType,
            position: 1,
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(supportLineText()).toBe('w/ The Support')
    })

    // The one exception, and it predates the mock. Kept because removing a
    // shipped annotation is as much a design change as adding one, but pinned
    // LAST on the line so the mock's name-labels-hometown sequence survives.
    it('annotates special guests, after the labels and hometown', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Top Bill', slug: 'top', set_type: 'headliner', position: 0 }),
          makeArtist({
            id: 2,
            name: 'The Guest',
            slug: 'guest',
            set_type: 'special_guest',
            position: 1,
            city: 'Chicago',
            state: 'IL',
            labels: [{ id: 40, name: 'Dead Oceans', slug: 'dead-oceans' }],
          }),
        ],
      })

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(screen.getByText('(special guest)')).toBeInTheDocument()
      expect(supportLineText()).toBe(
        'w/ The Guest on [Dead Oceans] from Chicago, IL (special guest)'
      )
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

      expect(() => render(<ShowHeader lifecycle="upcoming" show={show} />)).not.toThrow()
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

      render(<ShowHeader lifecycle="upcoming" show={show} />)

      expect(supportLineText()).toContain('Just A Band')
      expect(supportLineText()).not.toMatch(/\(/)
    })
  })
})
