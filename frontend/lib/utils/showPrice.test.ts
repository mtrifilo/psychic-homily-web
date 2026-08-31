import { describe, it, expect } from 'vitest'
import { statedShowPrices, showPriceLabel } from './showPrice'

describe('statedShowPrices', () => {
  it('says nothing when neither price is recorded', () => {
    expect(statedShowPrices({ price: null, door_price: null })).toEqual([])
    expect(statedShowPrices({})).toEqual([])
  })

  it('returns the single price a show records, advance or door', () => {
    expect(statedShowPrices({ price: 35, door_price: null })).toEqual([35])
    expect(statedShowPrices({ price: null, door_price: 40 })).toEqual([40])
  })

  it('returns a genuine pair advance-first', () => {
    expect(statedShowPrices({ price: 35, door_price: 40 })).toEqual([35, 40])
  })

  // Nothing constrains a curator or an importer from entering the same number
  // twice, and two slots saying one thing reads as a rendering bug.
  it('collapses equal prices to one', () => {
    expect(statedShowPrices({ price: 35, door_price: 35 })).toEqual([35])
  })

  // Zero is a price the site spells "Free", so it must survive as a value
  // rather than falling to the empty case with a truthiness guard.
  it('treats zero as a price, not as silence', () => {
    expect(statedShowPrices({ price: 0, door_price: null })).toEqual([0])
    expect(statedShowPrices({ price: 0, door_price: 10 })).toEqual([0, 10])
  })

  // A door price cheaper than the advance price is unusual but not impossible,
  // and the order stays advance-first so the pair never has to be re-sorted to
  // be read.
  it('keeps advance first even when the door is cheaper', () => {
    expect(statedShowPrices({ price: 40, door_price: 30 })).toEqual([40, 30])
  })
})

describe('showPriceLabel', () => {
  it('is null when no price is recorded', () => {
    expect(showPriceLabel({ price: null, door_price: null })).toBeNull()
  })

  it('renders a lone price bare, with no qualifier to disambiguate from', () => {
    expect(showPriceLabel({ price: 35, door_price: null })).toEqual({ text: '$35' })
    expect(showPriceLabel({ price: null, door_price: 40 })).toEqual({ text: '$40' })
    expect(showPriceLabel({ price: 0, door_price: null })).toEqual({ text: 'Free' })
  })

  it('renders a pair in the dense-list register with a spelled-out title', () => {
    expect(showPriceLabel({ price: 35, door_price: 40 })).toEqual({
      text: '$35/$40',
      title: '$35 advance, $40 at the door',
    })
  })

  // The title is what a screen reader announces instead of "35 slash 40", so
  // its absence on the single-price case is deliberate rather than an omission:
  // there is nothing to disambiguate.
  it('carries no title when there is only one number', () => {
    expect(showPriceLabel({ price: 35, door_price: 35 })?.title).toBeUndefined()
  })

  it('spells cents when a price really has them', () => {
    expect(showPriceLabel({ price: 12.5, door_price: null })).toEqual({ text: '$12.50' })
  })
})
