import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: undefined }),
}))

// The save bracket reads the router for its login redirect; there is no app
// router mounted in a render-only test.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/shows/test-show',
}))

// The planted-tag warning is a Sentry side effect; this file asserts that the
// row FIRES it, and plantedTagTelemetry's own tests cover dedupe and scrubbing.
const reportPlantedTicketTag = vi.fn()
vi.mock('@/lib/tickets/plantedTagTelemetry', () => ({
  reportPlantedTicketTag: (...args: unknown[]) => reportPlantedTicketTag(...args),
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

// The collection affordance needs a QueryClientProvider this render-only
// test doesn't mount, and the share affordance self-hides in jsdom (no
// clipboard/share capability); both behaviours are covered in their own
// test files.
vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
  AddToCollectionButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="add-to-collection" data-entity-name={entityName}>
      [Add to collection]
    </button>
  ),
  ShareButton: ({ path }: { path: string | null }) => (
    <button data-testid="share-button" data-path={path ?? ''}>
      [Share]
    </button>
  ),
}))

import { ShowTicketRow } from './ShowTicketRow'
import { ticketLineSegments } from './showTicketLine'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: '',
    // 8 PM Aug 12 in Chicago.
    event_date: '2026-08-13T01:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 1,
        slug: 'salt-shed',
        name: 'Salt Shed',
        city: 'Chicago',
        state: 'IL',
        timezone: 'America/Chicago',
        verified: true,
      },
    ],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}

describe('ticketLineSegments', () => {
  // The start time leads (user decision): for the common show with no
  // announced doors/music this line is the only clock on the page. It is
  // rendered in the STRIPE's register ("8PM"), so one page never spells the
  // same clock two ways.
  it('leads with the venue-local start time in the stripe register', () => {
    expect(ticketLineSegments(makeShow(), 'upcoming')[0]).toBe('8PM')
  })

  // Same refusal rule as the stripe and the venue facts line: a venue whose
  // timezone is a guess gets no confidently-wrong hour.
  it('prints no clock when the venue timezone is a guess', () => {
    const segments = ticketLineSegments(makeShow({
        venues: [
          {
            id: 1,
            slug: 'berlin-venue',
            name: 'Berlin Venue',
            city: 'Berlin',
            state: 'Berlin',
            timezone: null,
            verified: true,
          },
        ],
      }), 'upcoming')
    expect(segments.some(segment => /\dPM|\dAM/.test(segment))).toBe(false)
  })

  it('claims ON SALE only when there is somewhere to buy', () => {
    expect(ticketLineSegments(makeShow(), 'upcoming')).not.toContain('ON SALE')
    expect(
      ticketLineSegments(makeShow({ ticket_url: 'https://tix.example/1' }), 'upcoming')
    ).toContain('ON SALE')
  })

  it('swaps the sale state for SOLD OUT', () => {
    const segments = ticketLineSegments(makeShow({ ticket_url: 'https://tix.example/1', is_sold_out: true }), 'upcoming')
    expect(segments).toContain('SOLD OUT')
    expect(segments).not.toContain('ON SALE')
  })

  // The stripe says CANCELLED at the top of the page; this line must not
  // argue with it.
  it('never claims ON SALE for a cancelled show', () => {
    expect(
      ticketLineSegments(makeShow({ ticket_url: 'https://tix.example/1', is_cancelled: true }), 'upcoming')
    ).not.toContain('ON SALE')
  })

  // Cancellation outranks sold-out: "SOLD OUT" asserts the event is
  // happening, and the stripe above says it is not.
  it('makes no sale claim at all for a cancelled show that is also sold out', () => {
    const segments = ticketLineSegments(
      makeShow({
        ticket_url: 'https://tix.example/1',
        is_cancelled: true,
        is_sold_out: true,
      }),
      'upcoming'
    )
    expect(segments).not.toContain('SOLD OUT')
    expect(segments).not.toContain('ON SALE')
  })

  // Both sale-state claims are present tense; the archive is most of the
  // corpus and stale flags/urls survive the date.
  it('makes no sale claim on a past show, sold out included', () => {
    expect(
      ticketLineSegments(makeShow({ ticket_url: 'https://tix.example/1' }), 'past')
    ).not.toContain('ON SALE')
    expect(
      ticketLineSegments(makeShow({ is_sold_out: true }), 'past')
    ).not.toContain('SOLD OUT')
  })

  // The locked PAST mock's line: what entry cost, then the closed door,
  // in that order.
  it('closes a past line with NO LONGER AVAILABLE after the price', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 35, ticket_url: 'https://tix.example/1' }),
      'past'
    )
    expect(segments).toEqual(['8PM', '$35', 'NO LONGER AVAILABLE'])
  })

  // NO LONGER AVAILABLE is the past tense of ON SALE, so it is said when a
  // past line has a purchase to close out and stays silent when it does not:
  // a line that never said ON SALE has nothing to un-say. Every row's reason
  // is its label.
  const SOLD = { price: 35, ticket_url: 'https://tix.example/1' }
  it.each<[string, Partial<ShowResponse>, ShowLifecycleState, boolean]>([
    ['a stored ticket url alone is a purchase', { ticket_url: 'https://tix.example/1' }, 'past', true],
    ['a price alone is a purchase', { price: 35 }, 'past', true],
    ['a free show with no link never opened a sale', { price: 0 }, 'past', false],
    // A link outranks a zero price: a free RSVP or guestlist IS a reservation
    // to close out, so this is the one "Free" line that closes.
    ['a free show with an rsvp link did open one', { price: 0, ticket_url: 'https://rsvp.example/1' }, 'past', true],
    ['neither field means no commerce to close', {}, 'past', false],
    // The flag this test matrix most easily forgets: an ingested show an
    // admin marked sold out can carry no price and no link, and its line read
    // `8PM · SOLD OUT` until the show ended. It must close, not fall silent.
    ['a sold-out flag alone is a sale to close', { is_sold_out: true }, 'past', true],
    // getShowLifecycleState returns 'past' for an unreadable date. The stripe
    // renders nothing at all for that show, so this line must not announce a
    // closed door the page cannot date.
    ['an unreadable date is not evidence of pastness', { ...SOLD, event_date: 'not-a-date' }, 'past', false],
    ['an empty date is not evidence of pastness', { ...SOLD, event_date: '' }, 'past', false],
    ['a whitespace-only url is storable, and is not a purchase', { ticket_url: '   ' }, 'past', false],
    // Cancellation outranks the past register exactly as it outranks the
    // present-tense pair: the stripe says CANCELLED and never PAST SHOW, so
    // this line must not answer in the other state's words.
    ['cancellation outranks the past register', { ...SOLD, is_cancelled: true }, 'past', false],
    // The closing statement is the PAST register's alone; both live states
    // still have a sale state of their own to make.
    ['an upcoming show still has a live sale state', SOLD, 'upcoming', false],
    ['a show tonight still has a live sale state', SOLD, 'today', false],
  ])('%s', (_reason, overrides, lifecycle, saysIt) => {
    const segments = ticketLineSegments(makeShow(overrides), lifecycle)
    expect(segments.includes('NO LONGER AVAILABLE')).toBe(saysIt)
  })

  // The backend stores the field untrimmed and ingest skips the validator,
  // so a whitespace-only url is storable — and is not somewhere to buy.
  it('never claims ON SALE on a whitespace-only ticket url', () => {
    expect(
      ticketLineSegments(makeShow({ ticket_url: '   ' }), 'upcoming')
    ).not.toContain('ON SALE')
  })

  it('renders the single price, whole dollars without cents', () => {
    expect(ticketLineSegments(makeShow({ price: 35 }), 'upcoming')).toContain('$35')
    expect(
      ticketLineSegments(makeShow({ price: 12.5 }), 'upcoming')
    ).toContain('$12.50')
  })

  it('renders a zero price as Free', () => {
    expect(ticketLineSegments(makeShow({ price: 0 }), 'upcoming')).toContain('Free')
  })

  it('omits the price segment when no price is known', () => {
    const segments = ticketLineSegments(
      makeShow({ price: null, door_price: null }),
      'upcoming'
    )
    expect(segments.join(' ')).not.toContain('$')
  })

  // PSY-1864: the advance/door split. ADV and DOOR are disambiguators, so
  // they appear only when there are two numbers to tell apart.
  it('qualifies the pair as ADV and DOOR when both prices are known', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 35, door_price: 40 }),
      'upcoming'
    )
    expect(segments).toContain('$35 ADV')
    expect(segments).toContain('DOOR $40')
    // The mock's order: advance leads, door follows.
    expect(segments.indexOf('$35 ADV')).toBeLessThan(
      segments.indexOf('DOOR $40')
    )
  })

  it('leaves a lone advance price bare', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 35, door_price: null }),
      'upcoming'
    )
    expect(segments).toContain('$35')
    expect(segments.join(' ')).not.toContain('ADV')
    expect(segments.join(' ')).not.toContain('DOOR')
  })

  it('leaves a lone door price bare', () => {
    const segments = ticketLineSegments(
      makeShow({ price: null, door_price: 40 }),
      'upcoming'
    )
    expect(segments).toContain('$40')
    expect(segments.join(' ')).not.toContain('DOOR')
  })

  // Zero is a price, not silence, on either side of the split.
  it('spells a free advance against a paid door', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 0, door_price: 10 }),
      'upcoming'
    )
    expect(segments).toContain('Free ADV')
    expect(segments).toContain('DOOR $10')
  })

  it('spells a free door against a paid advance', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 10, door_price: 0 }),
      'upcoming'
    )
    expect(segments).toContain('$10 ADV')
    expect(segments).toContain('DOOR Free')
  })

  // Nothing stops a curator (or an importer) recording the same number twice.
  // `$35 ADV · DOOR $35` spends two qualifiers to say one thing.
  it('collapses an equal advance and door price to one bare segment', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 35, door_price: 35 }),
      'upcoming'
    )
    expect(segments).toContain('$35')
    expect(segments.join(' ')).not.toContain('ADV')
    expect(segments.join(' ')).not.toContain('DOOR')
  })

  it('collapses an equal free advance and free door', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 0, door_price: 0 }),
      'upcoming'
    )
    expect(segments).toContain('Free')
    expect(segments.join(' ')).not.toContain('ADV')
  })

  it('drops the cents on both halves of the split', () => {
    const segments = ticketLineSegments(
      makeShow({ price: 12.5, door_price: 15 }),
      'upcoming'
    )
    expect(segments).toContain('$12.50 ADV')
    expect(segments).toContain('DOOR $15')
  })

  it('renders the split into the ticket line the page shows', () => {
    render(
      <ShowTicketRow
        show={makeShow({ price: 35, door_price: 40 })}
        lifecycle="upcoming"
      />
    )
    expect(screen.getByTestId('ticket-line')).toHaveTextContent(
      '$35 ADV · DOOR $40'
    )
  })

  // The venue facts line owns the age fact, but a venue-less show never
  // mounts that module — the line falls back so "21+" cannot vanish from
  // the page.
  it('carries the age requirement only for a venue-less show', () => {
    expect(
      ticketLineSegments(makeShow({ venues: [], age_requirement: '21+' }), 'upcoming')
    ).toContain('21+')
    expect(
      ticketLineSegments(makeShow({ age_requirement: '21+' }), 'upcoming')
    ).not.toContain('21+')
  })
})

// A stored value that names no HOST is not somewhere to buy: there is no
// anchor to render and no vendor to name, so the line must not claim ON SALE
// and then point the reader nowhere. `ticket_url` is open contribution, and
// `repairTicketUrl` turns a script-bearing value into an absolute-looking one.
describe('ticketLineSegments with a hostless ticket url', () => {
  it.each(['https://', '/', 'javascript:alert(1)'])(
    'says nothing about a sale for %s',
    ticket_url => {
      const segments = ticketLineSegments(
        makeShow({ ticket_url, price: 25 }),
        'upcoming'
      )
      expect(segments).not.toContain('ON SALE')
      expect(segments).toEqual(['8PM', '$25'])
    }
  )
})

describe('ShowTicketRow', () => {
  /** The middot line above the verb row, read as flat text. */
  function ticketLine(): string {
    return screen.getByTestId('ticket-line').textContent ?? ''
  }

  // THE PAID-REFERRAL RULE, with no partner ID configured: an outbound vendor
  // anchor is withheld and the vendor is NAMED on the line instead, so the
  // reader still knows where the ticket is sold.
  it('names a known vendor and renders no link without a partner ID', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 25, ticket_url: 'https://www.ticketweb.com/event/2' })}
      />
    )

    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
    expect(ticketLine()).toContain('$25')
    expect(ticketLine()).toContain('TicketWeb')
  })

  // An unrecognized host has no name written down, so the line states the
  // hostname, which is a fact about the URL rather than a claim about a
  // company. `www.` is not part of how a reader names a site.
  it('names an unknown vendor by hostname', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 25, ticket_url: 'https://www.tix.example/1' })}
      />
    )

    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
    expect(ticketLine()).toContain('tix.example')
  })

  // FREE ADMISSION IS THE EXEMPTION: an RSVP link on a show that states a
  // price of zero is not a ticket referral, nobody is paid either way, and the
  // click is the reader's only route in. It links whatever the vendor is.
  it('keeps a free show RSVP link for any vendor', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, ticket_url: 'https://rsvp.example/1' })}
      />
    )

    const buy = screen.getByRole('link', { name: /^Buy tickets\b/i })
    expect(buy).toHaveAttribute('href', 'https://rsvp.example/1')
    expect(buy).toHaveAttribute('target', '_blank')
    // `ugc`, not `sponsored`: the destination is contributor-chosen and earns
    // the site nothing, and this is the only outbound ticket link that renders
    // on a build with no partner ID.
    expect(buy).toHaveAttribute('rel', 'noopener noreferrer ugc')
    // Anchored above so the ↗ cannot drift into the announced name, and the
    // new-tab claim must be present exactly once.
    expect(
      buy.getAttribute('aria-label')?.match(/opens in a new tab/g)
    ).toHaveLength(1)
    // The vendor name is what an unlinked referral leaves behind; a linked one
    // already names the vendor by being clickable.
    expect(ticketLine()).not.toContain('rsvp.example')
  })

  // Zero is a price, but an unpriced show is not a free one: with no price
  // column stated at all the referral is a referral like any other.
  it('withholds the link for an unpriced show', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ ticket_url: 'https://rsvp.example/1' })}
      />
    )

    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // A zero advance price beside a real door price still charges for entry.
  it('withholds the link when only the advance price is zero', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, door_price: 15, ticket_url: 'https://rsvp.example/1' })}
      />
    )

    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // Affiliate tagging is a config flip, not a markup change: the same row
  // withholds the anchor today and renders a tagged, qualified one once the
  // environment carries a partner ID.
  describe('with an affiliate partner ID configured', () => {
    beforeEach(() => {
      process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID = '1234567'
      reportPlantedTicketTag.mockClear()
    })
    afterEach(() => {
      delete process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID
    })

    it('tags a configured vendor on its own domain and qualifies the link', () => {
      render(
        <ShowTicketRow
          lifecycle="upcoming"
          show={makeShow({ ticket_url: 'https://www.ticketweb.com/event/2' })}
        />
      )

      const buy = screen.getByRole('link', { name: /^Buy tickets\b/i })
      expect(buy).toHaveAttribute(
        'href',
        'https://www.ticketweb.com/event/2?irmp=1234567'
      )
      expect(buy).toHaveAttribute('rel', 'noopener noreferrer sponsored')
      expect(ticketLine()).not.toContain('TicketWeb')
    })

    // The stored value carried the tag, and we only ever append at render, so
    // it was planted by whoever submitted the show. The link still renders as
    // stored; the report is what makes the row findable.
    // A planted tag credits somebody else, so `ticketLink` refuses to append
    // ours and the click is not paid for: the anchor is withheld on a vendor
    // that is otherwise configured. The report still fires, because it is
    // about the stored value rather than about what this row renders.
    it('withholds the link for a planted tag and still reports it', () => {
      render(
        <ShowTicketRow
          lifecycle="upcoming"
          show={makeShow({
            id: 4242,
            ticket_url: 'https://www.ticketweb.com/event/2?irmp=9999999',
          })}
        />
      )

      expect(
        screen.queryByRole('link', { name: /Buy tickets/i })
      ).not.toBeInTheDocument()
      expect(ticketLine()).toContain('TicketWeb')
      expect(reportPlantedTicketTag).toHaveBeenCalledWith({
        entityType: 'show',
        entityId: 4242,
        tag: {
          param: 'irmp',
          host: 'www.ticketweb.com',
          matchesConfiguredPartner: false,
        },
      })
    })

    it('reports nothing for a link this build tagged itself', () => {
      render(
        <ShowTicketRow
          lifecycle="upcoming"
          show={makeShow({ ticket_url: 'https://www.ticketweb.com/event/2' })}
        />
      )
      expect(reportPlantedTicketTag).not.toHaveBeenCalled()
    })

    // The partner ID is per NETWORK, and a vendor outside every program can
    // never carry one: a configured build still withholds its link.
    it('withholds the link for a vendor with no affiliate entry', () => {
      render(
        <ShowTicketRow
          lifecycle="upcoming"
          show={makeShow({ ticket_url: 'https://tix.example/1' })}
        />
      )

      expect(
        screen.queryByRole('link', { name: /Buy tickets/i })
      ).not.toBeInTheDocument()
      expect(ticketLine()).toContain('tix.example')
    })
  })

  // The backend stores ticket urls as typed; the repair is scheme-anchored
  // and case-insensitive, not a bare prefix test. Asserted on FREE shows,
  // which are the case where an arbitrary host still renders an anchor to
  // read the repaired href off.
  it('repairs a scheme-less ticket url to https', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, ticket_url: 'tix.example/1' })}
      />
    )

    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://tix.example/1'
    )
  })

  it('leaves an uppercase scheme alone and repairs protocol-relative urls', () => {
    const { rerender } = render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, ticket_url: 'HTTPS://tix.example/1' })}
      />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'HTTPS://tix.example/1'
    )

    rerender(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, ticket_url: '//tix.example/1' })}
      />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://tix.example/1'
    )
  })

  // A prefix-passing non-scheme would otherwise ship as a RELATIVE href
  // resolving under /shows/.
  it('prefixes a value that merely starts with the letters http', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ price: 0, ticket_url: 'httpfoo.example/1' })}
      />
    )
    expect(screen.getByRole('link', { name: /Buy tickets/i })).toHaveAttribute(
      'href',
      'https://httpfoo.example/1'
    )
  })

  it('renders no Buy Tickets bracket without a ticket url', () => {
    render(<ShowTicketRow lifecycle="upcoming" show={makeShow()} />)
    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // Shared derivation with the sale-state segment: the bracket must not
  // offer tickets the line above just said are gone or moot.
  it.each([
    ['a sold-out show', { is_sold_out: true }],
    ['a cancelled show', { is_cancelled: true }],
    ['a whitespace-only ticket url', { ticket_url: '   ' }],
  ])('renders no Buy Tickets bracket for %s', (_label, overrides) => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({ ticket_url: 'https://tix.example/1', ...overrides })}
      />
    )
    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // The affordance is the half a stale claim costs a reader money on.
  it('renders no Buy Tickets bracket on a past show', () => {
    render(
      <ShowTicketRow
        lifecycle="past"
        show={makeShow({ ticket_url: 'https://tix.example/1' })}
      />
    )
    expect(
      screen.queryByRole('link', { name: /Buy tickets/i })
    ).not.toBeInTheDocument()
  })

  // PSY-1666 coupling: the calendar affordance (which saves as a side
  // effect) and the save bracket share this row and the same query key.
  it('renders the full mock action row: calendar, save, collection, share', () => {
    render(<ShowTicketRow lifecycle="upcoming" show={makeShow()} />)

    expect(screen.getByText('Add to calendar')).toBeInTheDocument()
    expect(screen.getByText('Save')).toBeInTheDocument()
    expect(screen.getByTestId('add-to-collection')).toBeInTheDocument()
    expect(screen.getByTestId('share-button')).toHaveAttribute(
      'data-path',
      '/shows/test-show'
    )
  })

  // The past register's row: the forward-looking verb goes, the archive verbs
  // stay. Attendance is out of scope and its absence is deliberately NOT
  // asserted — nothing in this tree renders it, so the query could not fail.
  it('drops the calendar verb on a past show and keeps the archive row', () => {
    // Priced, so the rendered line actually carries the past register rather
    // than testing the archive row beside an empty one.
    render(<ShowTicketRow lifecycle="past" show={makeShow({ price: 35 })} />)

    expect(screen.getByTestId('ticket-line')).toHaveTextContent(
      'NO LONGER AVAILABLE'
    )
    expect(screen.queryByText('Add to calendar')).not.toBeInTheDocument()
    expect(screen.getByText('Save')).toBeInTheDocument()
    expect(screen.getByTestId('add-to-collection')).toBeInTheDocument()
    expect(screen.getByTestId('share-button')).toBeInTheDocument()
  })

  it('names the collection entry from the bill when the show has no title', () => {
    render(
      <ShowTicketRow
        lifecycle="upcoming"
        show={makeShow({
          title: '',
          artists: [
            {
              id: 1,
              slug: 'modest-mouse',
              name: 'Modest Mouse',
              set_type: 'headliner',
              position: 0,
              socials: {},
            },
          ],
        })}
      />
    )

    expect(screen.getByTestId('add-to-collection')).toHaveAttribute(
      'data-entity-name',
      'Modest Mouse'
    )
  })
})
