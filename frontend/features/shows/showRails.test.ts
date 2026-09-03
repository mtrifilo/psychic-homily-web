import { describe, expect, it } from 'vitest'
import {
  alsoTonightRailTitle,
  alsoTonightSeeAllHref,
  buildAlsoTonightRail,
  buildMoreAtVenueRail,
  SHOW_RAIL_ROW_CAP,
  VENUE_RAIL_FETCH_LIMIT,
  venueRailShowsUrl,
} from './showRails'
import {
  makeAlsoTonightPayload,
  makeAlsoTonightShow,
  makeRailVenue,
  makeVenueShow,
} from './showRails.fixtures'

describe('buildAlsoTonightRail', () => {
  it('builds nothing without a payload, so a pending rail is an absent rail', () => {
    expect(buildAlsoTonightRail(undefined, 1)).toBeNull()
  })

  it('tolerates the generator-nullable shows array', () => {
    expect(buildAlsoTonightRail(makeAlsoTonightPayload({ shows: null }), 1))
      .toBeNull()
  })

  it('drops the subject show even though the endpoint promises to', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [
          makeAlsoTonightShow({ id: 7, slug: 'seven' }),
          makeAlsoTonightShow({ id: 8, slug: 'eight' }),
        ],
      }),
      7
    )
    expect(rail?.rows.map(row => row.href)).toEqual(['/shows/eight'])
  })

  it('is null when the only listable row was the subject show', () => {
    expect(
      buildAlsoTonightRail(
        makeAlsoTonightPayload({ shows: [makeAlsoTonightShow({ id: 7 })] }),
        7
      )
    ).toBeNull()
  })

  it('caps at the mock’s three rows, keeping the earliest', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [1, 2, 3, 4, 5].map(id =>
          makeAlsoTonightShow({ id, slug: `s${id}` })
        ),
      }),
      99
    )
    expect(rail?.rows.map(row => row.href)).toEqual([
      '/shows/s1',
      '/shows/s2',
      '/shows/s3',
    ])
  })

  it('sets each row’s time on the VENUE’s clock, not the reader’s', () => {
    // 01:00 UTC Aug 13 is 8PM Aug 12 in Chicago. The heading above these rows
    // names the Chicago night, so the rows must be set on Chicago's clock
    // whatever zone the runtime is in.
    const rail = buildAlsoTonightRail(makeAlsoTonightPayload(), 99)
    expect(rail?.rows[0]?.lead).toBe('8PM')
  })

  it('falls back to the scene’s clock for a row with no venue zone', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [
          makeAlsoTonightShow({
            venue_timezone: undefined,
            venue_state: undefined,
          }),
        ],
      }),
      99
    )
    expect(rail?.rows[0]?.lead).toBe('8PM')
  })

  it('leaves the lead null when the payload carries no usable instant', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ starts_at: 'not-a-date' })],
      }),
      99
    )
    expect(rail?.rows[0]?.lead).toBeNull()
  })

  it('carries the room and the price as separate ledger columns', () => {
    const rail = buildAlsoTonightRail(makeAlsoTonightPayload(), 99)
    expect(rail?.rows[0]?.room).toBe('Empty Bottle')
    expect(rail?.rows[0]?.figure).toBe('$15')
    expect(rail?.variant).toBe('night')
  })

  it('leaves an absent cell EMPTY rather than collapsing its column', () => {
    // The difference between a ledger and a list of facts: dropping a cell
    // shifts every cell after it, and the figures stop being a column.
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ venue_name: undefined, price: undefined })],
      }),
      99
    )
    expect(rail?.rows[0]?.room).toBeNull()
    expect(rail?.rows[0]?.figure).toBeNull()
    expect(rail?.variant).toBe('night')
  })

  it('bills the row with the mock’s separator, not the scene views’ comma', () => {
    const rail = buildAlsoTonightRail(makeAlsoTonightPayload(), 99)
    expect(rail?.rows[0]?.title).toBe('Dehd + Lifeguard')
  })

  it('offers see-all when the backend already truncated the night', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({ has_more: true }),
      99
    )
    expect(rail?.seeAllHref).toBe('/scenes/chicago-il/2026-08-12')
  })

  it('offers see-all when the rail’s own cap hid a row the payload carried', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [1, 2, 3, 4].map(id => makeAlsoTonightShow({ id })),
      }),
      99
    )
    expect(rail?.seeAllHref).toBe('/scenes/chicago-il/2026-08-12')
  })

  it('withholds see-all when every listable row is on screen', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [1, 2, 3].map(id => makeAlsoTonightShow({ id })),
      }),
      99
    )
    expect(rail?.seeAllHref).toBeNull()
  })

  it('does not count the subject show as something hidden', () => {
    // Four rows, one of them this show: three are listable and three are
    // drawn, so there is nothing behind a "see all".
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [1, 2, 3, 7].map(id => makeAlsoTonightShow({ id })),
      }),
      7
    )
    expect(rail?.seeAllHref).toBeNull()
  })
})

describe('buildMoreAtVenueRail', () => {
  it('builds nothing for a venue-less show', () => {
    expect(buildMoreAtVenueRail(undefined, [makeVenueShow()], 1, 99)).toBeNull()
  })

  it('excludes the show being read — a page must not recommend itself', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [
        makeVenueShow({ id: 4, slug: 'four' }),
        makeVenueShow({ id: 5, slug: 'five' }),
      ],
      2,
      4
    )
    expect(rail?.rows.map(row => row.href)).toEqual(['/shows/five'])
  })

  it('still fills the rail once the subject show is removed', () => {
    // This is why VENUE_RAIL_FETCH_LIMIT is cap + 1: a full page of rows minus
    // the subject show must still reach the cap.
    const fetched = Array.from({ length: VENUE_RAIL_FETCH_LIMIT }, (_, i) =>
      makeVenueShow({ id: i + 1, slug: `s${i + 1}` })
    )
    const rail = buildMoreAtVenueRail(makeRailVenue(), fetched, 20, 1)
    expect(rail?.rows).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('is null when the room has no other dates', () => {
    expect(
      buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow({ id: 1 })], 1, 1)
    ).toBeNull()
  })

  it('heads the rail with the room’s name', () => {
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 2, 99)
    expect(rail?.title).toBe('More at / Salt Shed')
  })

  it('leads each row with the venue-local date, no weekday', () => {
    // 01:00 UTC Aug 16 is Aug 15 in Chicago.
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 2, 99)
    expect(rail?.rows[0]?.lead).toBe('AUG 15')
  })

  it('lets a row’s own state override the venue’s for the date', () => {
    // The instant has to STRADDLE the two zones' date boundary or the test
    // proves nothing: 05:30 UTC Aug 16 is already Aug 16 in Chicago (00:30)
    // but still Aug 15 in Honolulu (19:30). Reading the venue's IL instead of
    // the row's HI would give AUG 16.
    const straddling = '2026-08-16T05:30:00Z'
    expect(
      buildMoreAtVenueRail(
        makeRailVenue({ timezone: null }),
        [makeVenueShow({ state: 'HI', event_date: straddling })],
        2,
        99
      )?.rows[0]?.lead
    ).toBe('AUG 15')

    // The control: same instant, no row-level override, so the venue's own
    // state decides and the date lands a day later.
    expect(
      buildMoreAtVenueRail(
        makeRailVenue({ timezone: null }),
        [makeVenueShow({ state: null, event_date: straddling })],
        2,
        99
      )?.rows[0]?.lead
    ).toBe('AUG 16')
  })

  it('zero-pads the day so the lead column stays a column', () => {
    // The mock's `SEP 04`. The day is the only variable-width part of the cell.
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ event_date: '2026-09-05T01:00:00Z' })],
      2,
      99
    )
    expect(rail?.rows[0]?.lead).toBe('SEP 04')
  })

  it('dates a row in another year, so an archive page cannot mis-file it', () => {
    // On a past show's page the left rail is headed with ITS year (e.g. 2019)
    // while this rail lists the room's UPCOMING dates. A bare `AUG 15` beside
    // that heading reads as 2019 — the exact inverse of what the row says.
    const nextYear = new Date().getFullYear() + 1
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ event_date: `${nextYear}-09-05T01:00:00Z` })],
      2,
      99
    )
    expect(rail?.rows[0]?.lead).toBe(`SEP 04 '${String(nextYear).slice(-2)}`)
  })

  it('reads the year on the VENUE’s clock, like the date beside it', () => {
    // 02:00 UTC Jan 1 is still 20:00 Dec 31 in Chicago. Taking the year from
    // `new Date(x).getFullYear()` reads the RUNTIME's zone instead, which for
    // a reader east of the venue prints `DEC 31` under a year that has not
    // started there — a date that does not exist, on one of the year's most
    // heavily booked nights.
    const nextYear = new Date().getFullYear() + 1
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ event_date: `${nextYear}-01-01T02:00:00Z` })],
      2,
      99
    )
    // Chicago says Dec 31 of the year that is ending, not Jan 1 of the next.
    expect(rail?.rows[0]?.lead).toBe('DEC 31')
  })

  it('leaves the year off the current one, which needs no disambiguating', () => {
    const thisYear = new Date().getFullYear()
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ event_date: `${thisYear}-09-05T01:00:00Z` })],
      2,
      99
    )
    expect(rail?.rows[0]?.lead).toBe('SEP 04')
  })

  it('leaves the lead null for an unusable instant rather than printing junk', () => {
    // `toLocaleString` does not throw on a bad date — it returns the literal
    // string "Invalid Date", which the uppercased column would happily print.
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ event_date: 'not-a-date' })],
      2,
      99
    )
    expect(rail?.rows[0]?.lead).toBeNull()
  })

  it('names the bill, falling back to the promoter’s title', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ artists: [], title: 'Label Showcase' })],
      2,
      99
    )
    expect(rail?.rows[0]?.title).toBe('Label Showcase')
  })

  it('refuses a whitespace-only title rather than rendering an invisible label', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ artists: [], title: '   ' })],
      2,
      99
    )
    expect(rail?.rows[0]?.title).toBe('Live music')
  })

  it('puts the status in the figure column, superseding the price', () => {
    // The mock gives a sold-out row `SOLD OUT` where its neighbours carry
    // `$45` — one column, not a badge plus a price arguing with each other.
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ price: 45 })],
      2,
      99
    )
    expect(rail?.rows[0]?.figure).toBe('Sold out')
  })

  it('lets CANCELLED outrank SOLD OUT — a called-off ticket status is moot', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ is_cancelled: true, is_sold_out: true })],
      2,
      99
    )
    expect(rail?.rows[0]?.figure).toBe('Cancelled')
    expect(rail?.rows[0]?.isCancelled).toBe(true)
  })

  it('shows the price when there is no status to state', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ is_sold_out: false, price: 45 })],
      2,
      99
    )
    expect(rail?.rows[0]?.figure).toBe('$45')
  })

  it('reserves no room column — the room is the heading', () => {
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 2, 99)
    expect(rail?.variant).toBe('room')
    expect(rail?.rows[0]?.room).toBeNull()
  })

  it('does not offer see-all when the room’s only other show is on screen', () => {
    // total 2 = the one row drawn plus the show being read, which IS in the
    // fetched page, so the subject accounts for the difference.
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow(), makeVenueShow({ id: 99, slug: 'subject' })],
      2,
      99
    )
    expect(rail?.rows).toHaveLength(1)
    expect(rail?.seeAllHref).toBeNull()
  })

  it('offers see-all once rows are hidden', () => {
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 9, 99)
    expect(rail?.seeAllHref).toBe('/venues/salt-shed')
  })

  it('offers see-all for a PAST subject the venue’s upcoming count never held', () => {
    // The regression both adversarial reviewers found. A past show's page is
    // still served, and `total` counts only APPROVED UPCOMING shows, so the
    // subject was never in it and must not be paid back. Four upcoming dates,
    // three drawn, one hidden — the rail has to say so.
    const fetched = [1, 2, 3, 4].map(id =>
      makeVenueShow({ id, slug: `s${id}` })
    )
    const rail = buildMoreAtVenueRail(makeRailVenue(), fetched, 4, 99)
    expect(rail?.rows).toHaveLength(3)
    expect(rail?.seeAllHref).toBe('/venues/salt-shed')
  })

  it('still withholds see-all for a past subject when nothing is hidden', () => {
    // Same population, three upcoming dates, all three drawn.
    const fetched = [1, 2, 3].map(id => makeVenueShow({ id, slug: `s${id}` }))
    const rail = buildMoreAtVenueRail(makeRailVenue(), fetched, 3, 99)
    expect(rail?.rows).toHaveLength(3)
    expect(rail?.seeAllHref).toBeNull()
  })

  it('never links a venue with no slug — an empty slug resolves to the index', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue({ slug: '' }),
      [makeVenueShow()],
      9,
      99
    )
    expect(rail?.rows).toHaveLength(1)
    expect(rail?.seeAllHref).toBeNull()
  })

  it('addresses a row by id when the row carries no slug', () => {
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ id: 200, slug: '' })],
      9,
      99
    )
    expect(rail?.rows[0]?.href).toBe('/shows/200')
  })
})

describe('cross-rail overlap', () => {
  // A room with two bills on one night — an early/late set, or a second stage.
  // Both queries return it: the also-tonight endpoint excludes only the SUBJECT
  // show, and the venue's "upcoming" window includes tonight. Without the
  // exclusion the same bill renders in both columns at once.
  it('drops from the venue rail a show the also-tonight rail already drew', () => {
    const payload = makeAlsoTonightPayload({
      shows: [makeAlsoTonightShow({ id: 500, slug: 'late-set' })],
    })
    const drawn = buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    expect(drawn.has(500)).toBe(true)

    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [
        makeVenueShow({ id: 500, slug: 'late-set' }),
        makeVenueShow({ id: 501, slug: 'next-week' }),
      ],
      2,
      99,
      drawn
    )
    expect(rail?.rows.map(row => row.href)).toEqual(['/shows/next-week'])
  })

  it('hides the venue rail entirely when the other rail drew all of it', () => {
    const payload = makeAlsoTonightPayload({
      shows: [makeAlsoTonightShow({ id: 500 })],
    })
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ id: 500 })],
      1,
      99,
      buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    )
    expect(rail).toBeNull()
  })

  it('does not count a de-duplicated row as something see-all would reveal', () => {
    // total 2 = the drawn row plus the one the other rail already shows. There
    // is nothing behind the bracket the reader has not seen on this screen.
    const payload = makeAlsoTonightPayload({
      shows: [makeAlsoTonightShow({ id: 500 })],
    })
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ id: 500 }), makeVenueShow({ id: 501 })],
      2,
      99,
      buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    )
    expect(rail?.rows).toHaveLength(1)
    expect(rail?.seeAllHref).toBeNull()
  })

  it('still fills the rail when BOTH filters take rows from the fetched page', () => {
    // The regression the fetch limit was resized for. A room with an early and
    // a late set tonight loses the subject AND the sibling the other rail drew;
    // a full page must still reach the cap, or the rails row goes lopsided on
    // exactly the venues the dedup exists to serve.
    const payload = makeAlsoTonightPayload({
      shows: [makeAlsoTonightShow({ id: 500 })],
    })
    const fetched = [
      makeVenueShow({ id: 99, slug: 'subject' }),
      makeVenueShow({ id: 500, slug: 'late-set' }),
      ...Array.from({ length: VENUE_RAIL_FETCH_LIMIT - 2 }, (_, i) =>
        makeVenueShow({ id: 600 + i, slug: `future-${i}` })
      ),
    ]
    expect(fetched).toHaveLength(VENUE_RAIL_FETCH_LIMIT)

    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      fetched,
      20,
      99,
      buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    )
    expect(rail?.rows).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('survives a whole cap’s worth of same-night siblings', () => {
    // The tail case: a multi-stage room with three other bills tonight, all
    // three drawn by the metro rail. The venue rail must still find its rows
    // rather than vanishing from a room with a full calendar.
    const payload = makeAlsoTonightPayload({
      shows: [1, 2, 3].map(id => makeAlsoTonightShow({ id: 500 + id })),
    })
    const fetched = [
      makeVenueShow({ id: 99, slug: 'subject' }),
      makeVenueShow({ id: 501 }),
      makeVenueShow({ id: 502 }),
      makeVenueShow({ id: 503 }),
      ...Array.from({ length: VENUE_RAIL_FETCH_LIMIT - 4 }, (_, i) =>
        makeVenueShow({ id: 700 + i, slug: `later-${i}` })
      ),
    ]
    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      fetched,
      20,
      99,
      buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    )
    expect(rail?.rows).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('excludes only what the other rail actually DREW, not its whole payload', () => {
    // The other rail caps at three; a fourth same-night show at this room is
    // not on screen anywhere and must still be listable here.
    const payload = makeAlsoTonightPayload({
      shows: [1, 2, 3, 4].map(id => makeAlsoTonightShow({ id: 500 + id })),
    })
    const drawn = buildAlsoTonightRail(payload, 99)?.drawnIds ?? new Set()
    expect(drawn.has(504)).toBe(false)

    const rail = buildMoreAtVenueRail(
      makeRailVenue(),
      [makeVenueShow({ id: 504, slug: 'fourth' })],
      5,
      99,
      drawn
    )
    expect(rail?.rows.map(row => row.href)).toEqual(['/shows/fourth'])
  })
})

describe('alsoTonightRailTitle', () => {
  it('says Tonight only when the SCENE says so, never the viewer’s clock', () => {
    expect(alsoTonightRailTitle(makeAlsoTonightPayload())).toBe(
      'Also / Tonight · Chicago'
    )
  })

  it('names the night by its own date otherwise', () => {
    // A show page is read months early and years late; the same rail must not
    // claim "tonight" on either.
    expect(
      alsoTonightRailTitle(makeAlsoTonightPayload({ is_tonight: false }))
    ).toBe('Also / Wed Aug 12 · Chicago')
  })

  it('omits a city it does not have rather than guessing one', () => {
    expect(alsoTonightRailTitle(makeAlsoTonightPayload({ city: undefined })))
      .toBe('Also / Tonight')
  })

  it('dates an archive night with its year, which is not the current one', () => {
    // `Also / Thu Aug 15` on a 2019 page reads as this August to every reader,
    // and the rail carries no other full date to correct the impression.
    expect(
      alsoTonightRailTitle(
        makeAlsoTonightPayload({ is_tonight: false, date: '2019-08-15' })
      )
    ).toBe('Also / Thu Aug 15, 2019 · Chicago')
  })

  it('leaves the year off the current one, which needs no disambiguating', () => {
    const thisYear = new Date().getFullYear()
    const title = alsoTonightRailTitle(
      makeAlsoTonightPayload({ is_tonight: false, date: `${thisYear}-08-12` })
    )
    expect(title).not.toContain(String(thisYear))
  })

  it('degrades to the scope alone rather than heading a date it cannot read', () => {
    // A type is not a runtime guarantee across two independently deployed
    // services, and `parseCalendarDate` turns junk into a confident wrong date.
    expect(
      alsoTonightRailTitle(
        makeAlsoTonightPayload({ is_tonight: false, date: 'not-a-date' })
      )
    ).toBe('Also / Chicago')
    expect(
      alsoTonightRailTitle(makeAlsoTonightPayload({ is_tonight: false, date: '' }))
    ).toBe('Also / Chicago')
  })

  it('survives a date field the payload omitted entirely', () => {
    // Absent, not malformed: this used to throw out of `split` and take the
    // whole show page to its error boundary, not just the rail.
    const payload = makeAlsoTonightPayload({ is_tonight: false })
    delete (payload as { date?: string }).date
    expect(() => alsoTonightRailTitle(payload)).not.toThrow()
    expect(alsoTonightRailTitle(payload)).toBe('Also / Chicago')
  })
})

describe('alsoTonightSeeAllHref', () => {
  it('points at the scene’s own page for that night', () => {
    expect(alsoTonightSeeAllHref(makeAlsoTonightPayload())).toBe(
      '/scenes/chicago-il/2026-08-12'
    )
  })

  it('withholds the link when the backend withheld the slug', () => {
    // The backend drops scene_slug precisely when following it would land on a
    // page that does not list the show it came from.
    expect(
      alsoTonightSeeAllHref(makeAlsoTonightPayload({ scene_slug: undefined }))
    ).toBeNull()
    expect(
      alsoTonightSeeAllHref(makeAlsoTonightPayload({ scene_slug: '' }))
    ).toBeNull()
  })

  it('refuses to build a link from a date the scene route would not route', () => {
    expect(
      alsoTonightSeeAllHref(makeAlsoTonightPayload({ date: 'tonight' }))
    ).toBeNull()
    expect(alsoTonightSeeAllHref(makeAlsoTonightPayload({ date: '' }))).toBeNull()
  })
})

describe('the age column', () => {
  // The rail states the door policy the show page's venue module would state
  // for the same show: the event's own requirement where it has one, the
  // room's house default otherwise.
  const railRow = (over: Parameters<typeof makeAlsoTonightShow>[0]) =>
    buildAlsoTonightRail(
      makeAlsoTonightPayload({ shows: [makeAlsoTonightShow(over)] }),
      99
    )?.rows[0]

  it('is a NIGHT-rail column, filled on its rows and absent from the room rail', () => {
    expect(buildAlsoTonightRail(makeAlsoTonightPayload(), 99)?.rows[0]?.age)
      .toBe('21+')
    expect(
      buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 1, 99)?.rows[0]
        ?.age
    ).toBeNull()
  })

  it('falls back to the room house policy when the event states none', () => {
    expect(railRow({ age_requirement: '', venue_age_policy: '21+' })?.age).toBe(
      '21+'
    )
  })

  it('lets the event override the house default', () => {
    // The case a bare house-policy column would get WRONG: an all-ages matinee
    // in a 21+ room.
    expect(
      railRow({ age_requirement: 'all ages', venue_age_policy: '21+' })?.age
    ).toBe('all ages')
  })

  it('is null when neither half is recorded, leaving the column empty', () => {
    expect(railRow({ age_requirement: '', venue_age_policy: '' })?.age).toBeNull()
    // Whitespace is an absence, not a value: both columns are contributor
    // free text.
    expect(
      railRow({ age_requirement: '   ', venue_age_policy: '  ' })?.age
    ).toBeNull()
  })

  it('leaves the venue rail rows with no age cell to draw', () => {
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 1, 99)
    expect(rail?.rows[0]?.age).toBeNull()
  })
})

describe('live-night ordering', () => {
  // 8PM, 9PM and 10PM Chicago on the fixtures' night.
  const doors8 = makeAlsoTonightShow({
    id: 8,
    slug: 'eight',
    starts_at: '2026-08-13T01:00:00Z',
  })
  const doors9 = makeAlsoTonightShow({
    id: 9,
    slug: 'nine',
    starts_at: '2026-08-13T02:00:00Z',
  })
  const doors10 = makeAlsoTonightShow({
    id: 10,
    slug: 'ten',
    starts_at: '2026-08-13T03:00:00Z',
  })
  const at930 = new Date('2026-08-13T02:30:00Z')

  it('draws the shows still to come before the ones under way', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({ shows: [doors8, doors9, doors10] }),
      99,
      at930
    )
    expect(rail?.rows.map(row => row.href)).toEqual([
      '/shows/ten',
      '/shows/eight',
      '/shows/nine',
    ])
  })

  it('orders BEFORE the cap, so the drawn rows are the ones still to come', () => {
    // Four started sets and one upcoming. Capping first would draw three
    // started shows and hide the only one a reader can still get to.
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [
          makeAlsoTonightShow({
            id: 1,
            slug: 's1',
            starts_at: '2026-08-13T00:00:00Z',
          }),
          makeAlsoTonightShow({
            id: 2,
            slug: 's2',
            starts_at: '2026-08-13T00:30:00Z',
          }),
          makeAlsoTonightShow({
            id: 3,
            slug: 's3',
            starts_at: '2026-08-13T01:00:00Z',
          }),
          makeAlsoTonightShow({
            id: 4,
            slug: 's4',
            starts_at: '2026-08-13T01:30:00Z',
          }),
          doors10,
        ],
      }),
      99,
      at930
    )
    expect(rail?.rows[0]?.href).toBe('/shows/ten')
    expect(rail?.rows).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('keeps an archive or future night earliest-first', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        is_tonight: false,
        shows: [doors8, doors9, doors10],
      }),
      99,
      at930
    )
    expect(rail?.rows.map(row => row.href)).toEqual([
      '/shows/eight',
      '/shows/nine',
      '/shows/ten',
    ])
  })

  it('hides nothing: a night entirely under way still draws its cap', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({ shows: [doors8, doors9, doors10] }),
      99,
      new Date('2026-08-13T04:00:00Z')
    )
    expect(rail?.rows).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('suppresses from the venue rail exactly the rows the reordered rail drew', () => {
    // The invariant the two share: the ids withheld from the room's column are
    // the ones actually on screen in the night's column. A different clock on
    // either side would break it silently.
    const payload = makeAlsoTonightPayload({
      shows: [
        makeAlsoTonightShow({
          id: 1,
          slug: 's1',
          starts_at: '2026-08-13T00:00:00Z',
        }),
        makeAlsoTonightShow({
          id: 2,
          slug: 's2',
          starts_at: '2026-08-13T00:30:00Z',
        }),
        makeAlsoTonightShow({
          id: 3,
          slug: 's3',
          starts_at: '2026-08-13T01:00:00Z',
        }),
        doors10,
      ],
    })
    const rail = buildAlsoTonightRail(payload, 99, at930)
    // The set comes off the rail that was actually built, so it cannot name a
    // row the reader is not looking at — the ids and the hrefs are two readings
    // of one pass.
    expect([...(rail?.drawnIds ?? [])].sort((a, b) => a - b)).toEqual([1, 2, 10])
    expect(rail?.rows.map(row => row.href)).toEqual([
      '/shows/ten',
      '/shows/s1',
      '/shows/s2',
    ])
  })
})

describe('venueRailShowsUrl', () => {
  it('sends the limit the rail hook sends, read from the constant', () => {
    // The server and the hook must ask the same QUESTION. The rows the server
    // reads become the only page the rail ever filters, so a smaller limit
    // restated on the route would seed a page its two filters can empty out.
    expect(venueRailShowsUrl(10)).toContain(`limit=${VENUE_RAIL_FETCH_LIMIT}`)
    expect(venueRailShowsUrl(10)).toContain('time_filter=upcoming')
    expect(venueRailShowsUrl(10)).toContain('/venues/10/shows?')
  })
})
