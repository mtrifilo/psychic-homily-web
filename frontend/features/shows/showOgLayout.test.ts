import { describe, it, expect } from 'vitest'
import { measureSans } from '@/lib/og/textFit'
import {
  CONTENT_WIDTH,
  DATE_SIZE,
  DOMAIN_SIZE,
  PLATE_BOX_HEIGHT,
  PLATE_BOX_WIDTH,
  SOLD_OUT_SIZE,
  SUPPORT_SIZE,
  TEXT_WIDTH_WITH_PLATE,
  TITLE_SIZE_MAX,
  TITLE_SIZE_MIN,
  VENUE_MAX_WIDTH,
  VENUE_SIZE,
  buildVenueLine,
  dateRowWidth,
  fitPlate,
  fitTitleSize,
  fitVenueSize,
  venueOverflows,
} from './showOgLayout'

/** The card is consumed at ~300px — a 4× downscale. */
const SHARE_SCALE = 1 / 4
/** PSY-1576's standing rule for the whole card family. */
const LEGIBILITY_FLOOR = 8

describe('legibility at the share size', () => {
  // This is the entire point of the ticket: the previous tuning left three
  // elements at 5.5–6.5px effective.
  it('keeps every meaning-carrying element above the floor', () => {
    const elements = {
      date: DATE_SIZE,
      title: TITLE_SIZE_MAX,
      titleAtFloor: TITLE_SIZE_MIN,
      support: SUPPORT_SIZE,
      venue: VENUE_SIZE,
      domain: DOMAIN_SIZE,
      soldOut: SOLD_OUT_SIZE,
    }
    for (const [name, px] of Object.entries(elements)) {
      expect(px * SHARE_SCALE, `${name} at ${px}px`).toBeGreaterThanOrEqual(LEGIBILITY_FLOOR)
    }
  })

  // The badge says "you cannot get in" — the card's most actionable fact, and
  // at its old 22px it was an unlabelled red pill, confusable with the red
  // CANCELLED wash, which means the opposite.
  it('holds the sold-out badge to the floor too', () => {
    expect(SOLD_OUT_SIZE * SHARE_SCALE).toBeGreaterThanOrEqual(LEGIBILITY_FLOOR)
  })
})

describe('fitTitleSize', () => {
  it('leaves an ordinary title at the blessed display size', () => {
    expect(fitTitleSize('Pearly Drops at Sleeping Village')).toBe(TITLE_SIZE_MAX)
  })

  it('steps a long bill down rather than letting it grow the box', () => {
    const long =
      'Slightly Stoopid, The Elovaters, Bumpin Uglies and Artikal Sound System at Salt Shed Fairgrounds'
    expect(fitTitleSize(long)).toBeLessThan(TITLE_SIZE_MAX)
  })

  // The area budget alone cannot represent a token wider than one line: no
  // amount of wrapping makes a word narrower than itself, so a long one-word
  // title satisfied "fits in two lines" and was still cut mid-glyph.
  it('shrinks for an unbreakable token wider than the content box', () => {
    const token = 'Supercalifragilisticexpialidocious'
    // At the display size this single word is far wider than the content box,
    // and wrapping cannot help it.
    expect(measureSans(token, 'satoshiBold', TITLE_SIZE_MAX)).toBeGreaterThan(CONTENT_WIDTH)
    const size = fitTitleSize(token)
    expect(size).toBeLessThan(TITLE_SIZE_MAX)
    expect(measureSans(token, 'satoshiBold', size)).toBeLessThanOrEqual(CONTENT_WIDTH)
  })

  // Beyond the floor there is nothing shrinking can do — the size clamps and
  // the container clips, rather than the text being scaled into illegibility.
  it('clamps at the floor for a token no size can fit', () => {
    expect(fitTitleSize('Supercalifragilisticexpialidocious'.repeat(2))).toBe(TITLE_SIZE_MIN)
  })

  it('never returns below its floor, even for an absurd title', () => {
    expect(fitTitleSize('A'.repeat(500))).toBe(TITLE_SIZE_MIN)
  })
})

describe('fitVenueSize', () => {
  it('leaves an ordinary venue line at the blessed size', () => {
    expect(fitVenueSize('Sleeping Village · Chicago, IL')).toBe(VENUE_SIZE)
    expect(venueOverflows('Sleeping Village · Chicago, IL')).toBe(false)
  })

  // Folding the city in made this line much longer. "The Fillmore New Orleans ·
  // New Orleans, LA" wrapped at full size, dropping the state onto its own line
  // and pushing the wordmark off the edge.
  it('steps a long venue+city down so it stays on one line', () => {
    const line = buildVenueLine('The Fillmore New Orleans', 'New Orleans', 'LA')
    const size = fitVenueSize(line)
    expect(size).toBeLessThan(VENUE_SIZE)
    expect(measureSans(line, 'satoshiMedium', size)).toBeLessThanOrEqual(VENUE_MAX_WIDTH)
    expect(venueOverflows(line)).toBe(false)
  })

  // Real production venue names come from an automated pipeline that never
  // reviews length. Past the floor the line must be KNOWN to overflow, so the
  // card can clip it instead of letting it run under the wordmark.
  it('reports an overflow it cannot solve by shrinking', () => {
    const line = buildVenueLine(
      'Hollywood Casino Amphitheatre at Maryland Heights',
      'Maryland Heights',
      'MO'
    )
    expect(venueOverflows(line)).toBe(true)
  })
})

describe('buildVenueLine', () => {
  it('joins name, city and state', () => {
    expect(buildVenueLine('Sleeping Village', 'Chicago', 'IL')).toBe(
      'Sleeping Village · Chicago, IL'
    )
  })

  // Interpolating produced a trailing "Phoenix, " when state was empty.
  it('drops empty parts instead of leaving dangling punctuation', () => {
    expect(buildVenueLine('Crescent Ballroom', 'Phoenix', '')).toBe(
      'Crescent Ballroom · Phoenix'
    )
    expect(buildVenueLine('Crescent Ballroom', '', '')).toBe('Crescent Ballroom')
    expect(buildVenueLine('Crescent Ballroom', null, null)).toBe('Crescent Ballroom')
  })
})

describe('fitPlate', () => {
  // The decision this ticket encodes: a gig poster is a designed object, so it
  // is letterboxed whole rather than cropped to fill a slot. Cropping cuts the
  // headline off the poster, which is the one thing on it worth sharing.
  it('never crops — the fitted box always has the source ratio', () => {
    const sources: Array<[number, number]> = [
      [1080, 1350], // Instagram portrait, the common flyer
      [1000, 1000], // square
      [2000, 1000], // landscape
      [1275, 1650], // 8.5×11 scan
      [612, 792],
    ]
    for (const [w, h] of sources) {
      const fitted = fitPlate(w, h)
      expect(fitted.width / fitted.height, `${w}×${h}`).toBeCloseTo(w / h, 1)
    }
  })

  it('never exceeds the plate box on either axis', () => {
    const sources: Array<[number, number]> = [
      [4000, 3000],
      [1080, 1350],
      [1, 4000], // a sliver
      [4000, 1],
      [300, 300],
    ]
    for (const [w, h] of sources) {
      const fitted = fitPlate(w, h)
      expect(fitted.width, `${w}×${h} width`).toBeLessThanOrEqual(PLATE_BOX_WIDTH)
      expect(fitted.height, `${w}×${h} height`).toBeLessThanOrEqual(PLATE_BOX_HEIGHT)
    }
  })

  // The box is portrait because posters are: a 2:3 poster should be using the
  // full height rather than floating in the middle of it.
  it('fills the height for a 2:3 poster', () => {
    expect(fitPlate(1000, 1500).height).toBe(PLATE_BOX_HEIGHT)
  })

  // The 4:5 Instagram crop is squarer than the box, so it is the WIDTH that
  // binds — it letterboxes vertically rather than being cropped to fill.
  it('binds on width for a 4:5 Instagram flyer', () => {
    const fitted = fitPlate(1080, 1350)
    expect(fitted.width).toBe(PLATE_BOX_WIDTH)
    expect(fitted.height).toBeLessThan(PLATE_BOX_HEIGHT)
  })

  it('fills the width for a landscape flyer', () => {
    expect(fitPlate(2000, 1000).width).toBe(PLATE_BOX_WIDTH)
  })

  // An extreme ratio rounds toward zero on its short axis, and Satori draws a
  // zero-height element as nothing at all — a silently blank plate.
  it('keeps a degenerate ratio at least one pixel on both axes', () => {
    for (const [w, h] of [
      [4000, 1],
      [1, 4000],
    ] as const) {
      const fitted = fitPlate(w, h)
      expect(fitted.width).toBeGreaterThanOrEqual(1)
      expect(fitted.height).toBeGreaterThanOrEqual(1)
    }
  })

  // `0 * Infinity` is NaN, and Satori draws a NaN-sized element as nothing.
  it('stays bounded for zero and nonsense dimensions', () => {
    for (const [w, h] of [
      [0, 0],
      [0, 500],
      [500, 0],
      [-10, 20],
      [Number.NaN, 100],
    ] as const) {
      const fitted = fitPlate(w, h)
      expect(Number.isFinite(fitted.width), `${w}×${h}`).toBe(true)
      expect(Number.isFinite(fitted.height), `${w}×${h}`).toBe(true)
      expect(fitted.width).toBeGreaterThanOrEqual(1)
      expect(fitted.height).toBeGreaterThanOrEqual(1)
    }
  })
})

describe('the text column beside a plate', () => {
  // The whole reason the fit functions take a width: measured against the FULL
  // content width, a title that fits the wide card would overrun the narrow one
  // and be clipped mid-word.
  it('steps the title down further than it would on the full-width card', () => {
    const title = 'Militarie Gun, MSPAINT and Pool Kids at Sleeping Village'
    expect(fitTitleSize(title, TEXT_WIDTH_WITH_PLATE)).toBeLessThan(
      fitTitleSize(title, CONTENT_WIDTH)
    )
  })

  // The footer STACKS beside a plate. Kept inline it would leave the venue
  // ~267px of a 640px column — so the ordinary case, not some pathological
  // venue name, would clip on every single flyer card.
  it('cannot fit the venue beside the wordmark in the narrow column', () => {
    const inlineBudget = TEXT_WIDTH_WITH_PLATE - (CONTENT_WIDTH - VENUE_MAX_WIDTH)
    expect(venueOverflows(buildVenueLine('Sleeping Village', 'Chicago', 'IL'), inlineBudget))
      .toBe(true)
  })

  // Stacked, the venue line gets the whole column — which is the budget the
  // route passes as `textWidth`.
  it('fits an ordinary venue line once the footer stacks', () => {
    const line = buildVenueLine('Sleeping Village', 'Chicago', 'IL')
    expect(venueOverflows(line, TEXT_WIDTH_WITH_PLATE)).toBe(false)
    // And at full display size, not shrunk to squeeze in.
    expect(fitVenueSize(line, TEXT_WIDTH_WITH_PLATE)).toBe(VENUE_SIZE)
  })

  // The date row has no fit function, so nothing shrinks it — over budget Yoga
  // WRAPS, breaking the date mid-phrase and stacking the pill to "SOLD / OUT".
  // The long form does not fit beside a plate, which is exactly why the plate
  // card abbreviates; both halves are asserted so the reason cannot be lost.
  it('cannot fit the long date form beside a plate when sold out', () => {
    expect(dateRowWidth('Wednesday, September 30, 2026', true)).toBeGreaterThan(
      TEXT_WIDTH_WITH_PLATE
    )
  })

  it('fits the abbreviated date and the sold-out pill on one line', () => {
    // Longest abbreviated en-US forms: every weekday/month abbreviates to three
    // characters, so a two-digit day and a four-digit year is the widest case.
    for (const date of ['Wed, Sep 30, 2026', 'Sat, Nov 28, 2026', 'Sun, Aug 2, 2026']) {
      expect(dateRowWidth(date, true), date).toBeLessThanOrEqual(TEXT_WIDTH_WITH_PLATE)
    }
  })

  // The full-width card keeps the long form, and must keep fitting it.
  it('still fits the long date form on the full-width card', () => {
    expect(dateRowWidth('Wednesday, September 30, 2026', true)).toBeLessThanOrEqual(CONTENT_WIDTH)
  })

  // A guessed day carries a leading `~`, one more mono glyph on the row that
  // has no fit function and therefore wraps rather than shrinks. Both budgets
  // are re-asserted with it.
  it('still fits the marked abbreviated date beside a plate when sold out', () => {
    for (const date of ['~Wed, Sep 30, 2026', '~Sat, Nov 28, 2026']) {
      expect(dateRowWidth(date, true), date).toBeLessThanOrEqual(TEXT_WIDTH_WITH_PLATE)
    }
  })

  it('still fits the marked long date form on the full-width card', () => {
    expect(
      dateRowWidth('~Wednesday, September 30, 2026', true)
    ).toBeLessThanOrEqual(CONTENT_WIDTH)
  })

  // Every element is still consumed at a 4× downscale, plate or no plate.
  it('holds the narrow column to the same legibility floor', () => {
    expect(
      fitTitleSize('A'.repeat(500), TEXT_WIDTH_WITH_PLATE) * SHARE_SCALE
    ).toBeGreaterThanOrEqual(LEGIBILITY_FLOOR)
  })

  // A flyer-less show must render exactly the card it rendered before.
  it('defaults to the full content width when no plate is present', () => {
    const title = 'Pearly Drops at Sleeping Village'
    expect(fitTitleSize(title)).toBe(fitTitleSize(title, CONTENT_WIDTH))
    const line = buildVenueLine('Sleeping Village', 'Chicago', 'IL')
    expect(fitVenueSize(line)).toBe(fitVenueSize(line, VENUE_MAX_WIDTH))
    expect(venueOverflows(line)).toBe(venueOverflows(line, VENUE_MAX_WIDTH))
  })
})
