import { describe, expect, it } from 'vitest'
import {
  alsoTonightRailTitle,
  alsoTonightSeeAllHref,
  buildAlsoTonightRail,
  buildMoreAtVenueRail,
  SHOW_RAIL_ROW_CAP,
  VENUE_RAIL_FETCH_LIMIT,
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
    expect(rail?.rows[0]?.lead).toBe('8:00 PM')
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
    expect(rail?.rows[0]?.lead).toBe('8:00 PM')
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

  it('carries the room and the price as the row’s facts', () => {
    const rail = buildAlsoTonightRail(makeAlsoTonightPayload(), 99)
    expect(rail?.rows[0]?.facts).toEqual(['Empty Bottle', '$15.00'])
  })

  it('drops a fact it does not have rather than rendering a gap', () => {
    const rail = buildAlsoTonightRail(
      makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ venue_name: undefined, price: undefined })],
      }),
      99
    )
    expect(rail?.rows[0]?.facts).toEqual([])
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
    const rail = buildMoreAtVenueRail(
      makeRailVenue({ timezone: null }),
      [makeVenueShow({ state: 'HI' })],
      2,
      99
    )
    // 01:00 UTC Aug 16 is still Aug 15 in Hawaii, two hours further back.
    expect(rail?.rows[0]?.lead).toBe('AUG 15')
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

  it('carries the sold-out flag through to the row', () => {
    const rail = buildMoreAtVenueRail(makeRailVenue(), [makeVenueShow()], 2, 99)
    expect(rail?.rows[0]?.isSoldOut).toBe(true)
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
