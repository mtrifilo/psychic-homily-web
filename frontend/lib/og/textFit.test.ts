import { describe, it, expect } from 'vitest'
import {
  DESCENT_RATIO,
  fitFontSize,
  fitItemList,
  measureData,
  measureSans,
  dataBaselineLift,
} from './textFit'
import {
  CITY_SIZE_MAX,
  CITY_SIZE_MIN,
  CONTENT_WIDTH,
  FOOTER_GAP,
  FOOTER_SIZE,
  ROOMS_MAX_WIDTH,
  WORDMARK,
  cityMaxWidth,
} from '@/features/scenes/sceneWeekOgLayout'

/**
 * Budgets come from the card's own layout module rather than being copied here,
 * so a change to the padding or the footer gap moves the assertions with it.
 */
const CITY_BUDGET = cityMaxWidth('IL')

const roomsOverflow = (n: number) => `${n} ${n === 1 ? 'room' : 'rooms'} tracked`

describe('measureSans', () => {
  // The tables are only worth having if they agree with the real font. 524.6 is
  // the table's own value; the blessed Figma card reported 511 for the same
  // string, drawn in Inter. The ~2.7% gap is the face difference, not an error.
  it('matches the width the card was designed against', () => {
    expect(measureSans('Chicago', 'schibstedBold', 132)).toBeCloseTo(524.6, 0)
    // The mock reported 511px for the same string drawn in Inter. Expressed as
    // a relative tolerance rather than a fixed one so a legitimate table update
    // doesn't fail a check that is really about the two faces being close.
    expect(Math.abs(measureSans('Chicago', 'schibstedBold', 132) - 511) / 511).toBeLessThan(
      0.03
    )
  })

  // A golden value, not a design figure: it exists so that regenerating the
  // regular table incorrectly fails here rather than silently reflowing the
  // footer on every card.
  it('pins the regular face against its table', () => {
    expect(measureSans('Empty Bottle', 'schibstedRegular', 34)).toBeCloseTo(202.2, 1)
  })

  // Accents are stripped before lookup because almost every accented glyph in
  // these faces carries its base letter's advance. If that ever stops being
  // true the tables need regenerating, and this test is what catches it.
  it('measures accented names as their base letters', () => {
    expect(measureSans('Montréal', 'schibstedBold', 132)).toBeCloseTo(
      measureSans('Montreal', 'schibstedBold', 132),
      5
    )
  })

  // The exception: the body face draws the caron on `d l t` as a raised comma
  // beside the letter, so folding them onto their base would UNDER-measure a
  // Czech or Slovak name, the unsafe direction.
  it('measures ď ľ ť at their own, wider advance', () => {
    for (const [carón, base] of [
      ['ď', 'd'],
      ['ľ', 'l'],
      ['ť', 't'],
    ]) {
      expect(measureSans(carón, 'schibstedBold', 1000)).toBeGreaterThan(
        measureSans(base, 'schibstedBold', 1000)
      )
    }
    expect(measureSans('ď', 'schibstedBold', 1000)).toBeCloseTo(705, 0)
  })

  // The property that matters is the DIRECTION of the error, not the constant.
  // Unknown glyphs must over-measure: the subsets are Latin-only, the scene
  // table is worldwide, and an under-measured headline runs off a canvas that
  // has no reflow. The widest glyph in the brand subsets is 1136/1000 em.
  it('over-measures glyphs it has no table entry for', () => {
    const widestKnown = measureSans('W', 'schibstedBold', 1000)
    const unknown = measureSans('東', 'schibstedBold', 1000)
    expect(unknown).toBeGreaterThan(widestKnown)
    expect(unknown).toBeGreaterThanOrEqual(1000)
  })

  // Left to the unknown-glyph fallback, `·` would be charged several times its
  // real width: a systematic over-estimate on every multi-room footer.
  it('measures the list separator and en dash exactly', () => {
    expect(measureSans('·', 'schibstedRegular', 1000)).toBeCloseTo(280, 0)
    expect(measureSans('·', 'schibstedBold', 1000)).toBeCloseTo(304, 0)
    expect(measureSans('–', 'schibstedRegular', 1000)).toBeCloseTo(500, 0)
  })

  it('adds letter spacing per character when asked', () => {
    const plain = measureSans('Chicago', 'schibstedBold', 132)
    expect(measureSans('Chicago', 'schibstedBold', 132, -3)).toBeCloseTo(plain - 21, 5)
  })

  it('measures the empty string as zero', () => {
    expect(measureSans('', 'schibstedBold', 132)).toBe(0)
  })
})

describe('measureData', () => {
  // A golden value, like the regular face's above: a wrongly regenerated data
  // table fails here rather than silently moving every footer.
  it('pins the wordmark width against its table', () => {
    expect(measureData('psychichomily.com', 34)).toBeCloseTo(314.7, 0)
  })

  // The data face is proportional, so the separator and en dash in the eyebrow
  // and the week range are measured exactly rather than charged the fallback.
  it('measures the separator and en dash in the data face exactly', () => {
    expect(measureData('·', 1000)).toBeCloseTo(300, 0)
    expect(measureData('–', 1000)).toBeCloseTo(530, 0)
  })

  it('counts letter spacing per character', () => {
    const plain = measureData('IL', 40)
    expect(measureData('IL', 40, 3)).toBeCloseTo(plain + 6, 5)
  })
})

describe('dataBaselineLift', () => {
  // The descent ratios predict 25.3px between a 132px city and a 40px state.
  // Rendering "Phoenix AZ" through the weekly card with this lift put the two
  // runs' lowest ink rows 2px apart, which is the round glyphs' overshoot.
  it('predicts the drift between the city and the state', () => {
    expect(dataBaselineLift(132, 40)).toBeCloseTo(25.26, 1)
  })

  // The real invariant: the lift is exactly the difference in descender depth,
  // so it vanishes at the sizes where the two descenders coincide.
  it('vanishes when the two descenders land at the same depth', () => {
    const dataSize = (100 * DESCENT_RATIO.sans) / DESCENT_RATIO.data
    expect(dataBaselineLift(100, dataSize)).toBeCloseTo(0, 10)
  })
})

describe('fitFontSize', () => {
  // Every common metro still renders at the blessed display size — the fit is
  // there for outliers, not as a general shrink.
  it('leaves ordinary metro names untouched', () => {
    for (const city of ['Chicago', 'Minneapolis', 'San Francisco', 'Oklahoma City']) {
      expect(
        fitFontSize(city, 'schibstedBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
      ).toBe(CITY_SIZE_MAX)
    }
  })

  // The mock was only ever drawn with "Chicago" (525px). These are the names
  // that would actually have clipped, which is why this function exists.
  it('steps genuinely long city names down until they fit', () => {
    for (const city of ['Colorado Springs', 'Saint Petersburg']) {
      const size = fitFontSize(
        city,
        'schibstedBold',
        CITY_BUDGET,
        CITY_SIZE_MAX,
        CITY_SIZE_MIN
      )
      expect(size).toBeLessThan(CITY_SIZE_MAX)
      expect(measureSans(city, 'schibstedBold', size)).toBeLessThanOrEqual(CITY_BUDGET)
    }
  })

  it('never returns below the floor, even for an absurd name', () => {
    expect(
      fitFontSize('A'.repeat(80), 'schibstedBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
    ).toBe(CITY_SIZE_MIN)
  })

  // "Fort Lauderdale" is the case that only fails once the state is beside it:
  // 1034px fits the 1056px content box but not the ~995px the city is left
  // with. A fit that ignored the state's width would pass the first assertion
  // and fail the second.
  it('accounts for space already taken by the state', () => {
    const city = 'Fort Lauderdale'
    expect(
      fitFontSize(city, 'schibstedBold', CONTENT_WIDTH, CITY_SIZE_MAX, CITY_SIZE_MIN)
    ).toBe(CITY_SIZE_MAX)
    expect(
      fitFontSize(city, 'schibstedBold', CITY_BUDGET, CITY_SIZE_MAX, CITY_SIZE_MIN)
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
      fitItemList([], 'schibstedRegular', FOOTER_SIZE, ROOMS_MAX_WIDTH, roomsOverflow)
    ).toBeNull()
  })

  it('names what fits and counts the rest', () => {
    const line = fitItemList(
      chicago,
      'schibstedRegular',
      FOOTER_SIZE,
      ROOMS_MAX_WIDTH,
      roomsOverflow
    )
    expect(line).not.toBeNull()
    expect(measureSans(line!, 'schibstedRegular', FOOTER_SIZE)).toBeLessThanOrEqual(
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
        'schibstedRegular',
        FOOTER_SIZE,
        ROOMS_MAX_WIDTH,
        roomsOverflow
      )
    ).toBe('Valley Bar · Crescent')
  })

  it('never truncates a room name mid-word', () => {
    const line = fitItemList(
      chicago,
      'schibstedRegular',
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
        'schibstedRegular',
        FOOTER_SIZE,
        200,
        roomsOverflow
      )
    ).toBe('2 rooms tracked')
  })

  it('uses the caller label verbatim, including its singular', () => {
    expect(
      fitItemList(['A'.repeat(100)], 'schibstedRegular', FOOTER_SIZE, 200, roomsOverflow)
    ).toBe('1 room tracked')
  })
})

describe('card layout budgets', () => {
  // Not a change-detector on the constants — it asserts the relationship the
  // footer depends on: the room list gets whatever the wordmark and the gap
  // between them leave, and that has to be enough to name at least one room.
  it('leaves the room list room for a real venue name', () => {
    expect(ROOMS_MAX_WIDTH).toBe(
      CONTENT_WIDTH - measureData(WORDMARK, FOOTER_SIZE) - FOOTER_GAP
    )
    expect(ROOMS_MAX_WIDTH).toBeGreaterThan(
      measureSans('Salt Shed Fairgrounds +9', 'schibstedRegular', FOOTER_SIZE)
    )
  })
})
