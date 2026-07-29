import { describe, it, expect } from 'vitest'
import {
  fitFontSize,
  fitItemList,
  measureMono,
  measureSans,
  monoBaselineLift,
} from './textFit'
import {
  CITY_SIZE_MAX,
  CITY_SIZE_MIN,
  CONTENT_WIDTH,
  FOOTER_SIZE,
  ROOMS_MAX_WIDTH,
  STATE_SIZE,
  cityMaxWidth,
} from '@/features/scenes/sceneWeekOgLayout'

/**
 * Budgets come from the card's own layout module rather than being copied here,
 * so a change to the padding or the footer gap moves the assertions with it.
 */
const CITY_BUDGET = cityMaxWidth('IL')

const roomsOverflow = (n: number) => `${n} ${n === 1 ? 'room' : 'rooms'} tracked`

describe('measureSans', () => {
  // The tables are only worth having if they agree with the real font. 515.3 is
  // the table's own value; the blessed Figma card reported 511 for the same
  // string, drawn in Inter — the ~1% gap is the face difference, not an error.
  it('matches the width the card was designed against', () => {
    expect(measureSans('Chicago', 'satoshiBold', 132)).toBeCloseTo(515.3, 0)
    expect(measureSans('Chicago', 'satoshiBold', 132)).toBeCloseTo(511, -1)
  })

  // A golden value, not a design figure: it exists so that regenerating the
  // regular table incorrectly fails here rather than silently reflowing the
  // footer on every card.
  it('pins the regular face against its table', () => {
    expect(measureSans('Empty Bottle', 'satoshiRegular', 34)).toBeCloseTo(189.96, 1)
  })

  // Accents are stripped before lookup because every accented glyph in these
  // faces carries its base letter's advance. If that ever stops being true the
  // tables need regenerating, and this test is what catches it.
  it('measures accented names as their base letters', () => {
    expect(measureSans('Montréal', 'satoshiBold', 132)).toBeCloseTo(
      measureSans('Montreal', 'satoshiBold', 132),
      5
    )
  })

  it('falls back to a known advance outside the table', () => {
    expect(measureSans('東京', 'satoshiBold', 132)).toBeCloseTo((2 * 584 * 132) / 1000, 5)
  })

  it('measures the empty string as zero', () => {
    expect(measureSans('', 'satoshiBold', 132)).toBe(0)
  })
})

describe('measureMono', () => {
  it('matches the wordmark width the card was designed against', () => {
    expect(measureMono('psychichomily.com', 34)).toBeCloseTo(353.7, 0)
  })

  it('counts letter spacing per character', () => {
    const plain = measureMono('IL', 40)
    expect(measureMono('IL', 40, 3)).toBeCloseTo(plain + 6, 5)
  })
})

describe('monoBaselineLift', () => {
  // Satori's own `alignItems: 'baseline'` left the state 16px below the city;
  // the descent ratios predict 17px, and rendering with this correction
  // measured the gap back to 1px.
  it('predicts the drift measured on the rendered card', () => {
    expect(monoBaselineLift(132, 40)).toBeCloseTo(17.24, 1)
  })

  it('is zero when the two runs share a descender depth', () => {
    expect(monoBaselineLift(0, 0)).toBe(0)
  })
})

describe('fitFontSize', () => {
  // Every common metro still renders at the blessed display size — the fit is
  // there for outliers, not as a general shrink.
  it('leaves ordinary metro names untouched', () => {
    for (const city of ['Chicago', 'Minneapolis', 'San Francisco', 'Oklahoma City']) {
      expect(
        fitFontSize(city, 'satoshiBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
      ).toBe(CITY_SIZE_MAX)
    }
  })

  // The mock was only ever drawn with "Chicago" (515px). These are the names
  // that would actually have clipped, which is why this function exists.
  it('steps genuinely long city names down until they fit', () => {
    for (const city of ['Colorado Springs', 'Saint Petersburg']) {
      const size = fitFontSize(
        city,
        'satoshiBold',
        CITY_BUDGET,
        CITY_SIZE_MAX,
        CITY_SIZE_MIN
      )
      expect(size).toBeLessThan(CITY_SIZE_MAX)
      expect(measureSans(city, 'satoshiBold', size)).toBeLessThanOrEqual(CITY_BUDGET)
    }
  })

  it('never returns below the floor, even for an absurd name', () => {
    expect(
      fitFontSize('A'.repeat(80), 'satoshiBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
    ).toBe(CITY_SIZE_MIN)
  })

  // "Saint Petersburg" is the case that only fails once the state is beside it:
  // 1022px fits the 1056px content box but not the ~981px the city is left
  // with. A fit that ignored the state's width would pass the first assertion
  // and fail the second.
  it('accounts for space already taken by the state', () => {
    const city = 'Saint Petersburg'
    expect(
      fitFontSize(city, 'satoshiBold', CONTENT_WIDTH, CITY_SIZE_MAX, CITY_SIZE_MIN)
    ).toBe(CITY_SIZE_MAX)
    expect(
      fitFontSize(city, 'satoshiBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
    ).toBeLessThan(CITY_SIZE_MAX)
  })

  it('gives the headline the whole box when the scene has no state', () => {
    expect(cityMaxWidth('')).toBe(CONTENT_WIDTH)
    expect(cityMaxWidth('IL')).toBeLessThan(CONTENT_WIDTH)
  })
})

describe('fitItemList', () => {
  const chicago = [
    'Cobra Lounge',
    'Douglas Park',
    'Empty Bottle',
    'Lincoln Hall',
    'Salt Shed',
    'Salt Shed Fairgrounds',
    'Schubas Tavern',
    'Sleeping Village',
    'Tack Room',
    'Thalia Hall',
    'United Center',
  ]

  it('returns null when there are no rooms to name', () => {
    expect(
      fitItemList([], 'satoshiRegular', FOOTER_SIZE, ROOMS_MAX_WIDTH, roomsOverflow)
    ).toBeNull()
  })

  it('names what fits and counts the rest', () => {
    const line = fitItemList(
      chicago,
      'satoshiRegular',
      FOOTER_SIZE,
      ROOMS_MAX_WIDTH,
      roomsOverflow
    )
    expect(line).not.toBeNull()
    expect(measureSans(line!, 'satoshiRegular', FOOTER_SIZE)).toBeLessThanOrEqual(
      ROOMS_MAX_WIDTH
    )
    // Whatever it drops, it must say so — a card implying full coverage would
    // be false, and a local would notice.
    expect(line).toMatch(/ \+\d+$/)
    const shown = line!.replace(/ \+\d+$/, '').split(' · ').length
    const dropped = Number(line!.match(/\+(\d+)$/)![1])
    expect(shown + dropped).toBe(chicago.length)
  })

  it('names every room and omits the remainder when they all fit', () => {
    expect(
      fitItemList(
        ['Valley Bar', 'Crescent'],
        'satoshiRegular',
        FOOTER_SIZE,
        ROOMS_MAX_WIDTH,
        roomsOverflow
      )
    ).toBe('Valley Bar · Crescent')
  })

  it('never truncates a room name mid-word', () => {
    const line = fitItemList(
      chicago,
      'satoshiRegular',
      FOOTER_SIZE,
      ROOMS_MAX_WIDTH,
      roomsOverflow
    )!
    for (const name of line.replace(/ \+\d+$/, '').split(' · ')) {
      expect(chicago).toContain(name)
    }
  })

  // The degenerate case: a single name too long for the footer. Clipping it
  // would look broken, so the line falls back to the caller's honest summary.
  it('falls back to the overflow label when not even one name fits', () => {
    expect(
      fitItemList(
        ['A Venue With An Extremely Long Name That Cannot Possibly Fit', 'Another'],
        'satoshiRegular',
        FOOTER_SIZE,
        200,
        roomsOverflow
      )
    ).toBe('2 rooms tracked')
  })

  it('uses the caller label verbatim, including its singular', () => {
    expect(
      fitItemList(['A'.repeat(100)], 'satoshiRegular', FOOTER_SIZE, 200, roomsOverflow)
    ).toBe('1 room tracked')
  })
})

describe('card layout budgets', () => {
  // Guards the geometry the fit tests above lean on: if the padding or the
  // footer gap moves, these are the numbers that move with it.
  it('leaves the room list a usable share of the footer', () => {
    expect(CONTENT_WIDTH).toBe(1056)
    expect(ROOMS_MAX_WIDTH).toBeGreaterThan(0)
    expect(ROOMS_MAX_WIDTH).toBeLessThan(CONTENT_WIDTH - measureMono('psychichomily.com', FOOTER_SIZE))
  })

  it('keeps the state subordinate to the city', () => {
    expect(STATE_SIZE).toBeLessThan(CITY_SIZE_MIN)
  })
})
