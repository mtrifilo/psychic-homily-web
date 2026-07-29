import { describe, it, expect } from 'vitest'
import { measureSans } from '@/lib/og/textFit'
import {
  CONTENT_WIDTH,
  DATE_SIZE,
  DOMAIN_SIZE,
  SOLD_OUT_SIZE,
  SUPPORT_SIZE,
  TITLE_SIZE_MAX,
  TITLE_SIZE_MIN,
  VENUE_MAX_WIDTH,
  VENUE_SIZE,
  buildVenueLine,
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
