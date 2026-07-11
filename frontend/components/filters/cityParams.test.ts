import { describe, it, expect } from 'vitest'
import {
  parseCitiesParam,
  buildCitiesParam,
  citiesEqual,
  citiesParser,
  ALL_CITIES,
} from './cityParams'

describe('parseCitiesParam', () => {
  it('parses a single city,state pair', () => {
    expect(parseCitiesParam('Phoenix,AZ')).toEqual([{ city: 'Phoenix', state: 'AZ' }])
  })

  it('parses multiple pipe-separated pairs and trims whitespace', () => {
    expect(parseCitiesParam('Phoenix, AZ|Mesa,AZ')).toEqual([
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Mesa', state: 'AZ' },
    ])
  })

  it('returns [] for null/undefined/empty', () => {
    expect(parseCitiesParam(null)).toEqual([])
    expect(parseCitiesParam(undefined)).toEqual([])
    expect(parseCitiesParam('')).toEqual([])
  })

  it('drops malformed segments (extra commas or a blank half)', () => {
    expect(parseCitiesParam('Phoenix,AZ|garbage|New York,,NY|,AZ|Mesa,AZ')).toEqual([
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Mesa', state: 'AZ' },
    ])
  })
})

describe('buildCitiesParam', () => {
  it('serializes to the pipe/comma wire format', () => {
    expect(
      buildCitiesParam([
        { city: 'Phoenix', state: 'AZ' },
        { city: 'Mesa', state: 'AZ' },
      ])
    ).toBe('Phoenix,AZ|Mesa,AZ')
  })

  it('round-trips with parseCitiesParam', () => {
    const cities = [
      { city: 'Los Angeles', state: 'CA' },
      { city: 'Brooklyn', state: 'NY' },
    ]
    expect(parseCitiesParam(buildCitiesParam(cities))).toEqual(cities)
  })
})

describe('citiesEqual', () => {
  it('is order-insensitive', () => {
    const a = [
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Mesa', state: 'AZ' },
    ]
    const b = [
      { city: 'Mesa', state: 'AZ' },
      { city: 'Phoenix', state: 'AZ' },
    ]
    expect(citiesEqual(a, b)).toBe(true)
  })

  it('distinguishes different lengths and members', () => {
    expect(citiesEqual([{ city: 'Phoenix', state: 'AZ' }], [])).toBe(false)
    expect(
      citiesEqual(
        [{ city: 'Phoenix', state: 'AZ' }],
        [{ city: 'Tucson', state: 'AZ' }]
      )
    ).toBe(false)
  })
})

describe('citiesParser', () => {
  it('parses the ALL_CITIES sentinel', () => {
    expect(citiesParser.parse(ALL_CITIES)).toBe(ALL_CITIES)
  })

  it('parses a valid selection into typed pairs', () => {
    expect(citiesParser.parse('Phoenix,AZ|Mesa,AZ')).toEqual([
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Mesa', state: 'AZ' },
    ])
  })

  it('resolves empty / all-malformed values to null (falls back to default)', () => {
    expect(citiesParser.parse('')).toBeNull()
    expect(citiesParser.parse('garbage')).toBeNull()
    expect(citiesParser.parse(',AZ|New York,,NY')).toBeNull()
  })

  it('serializes the sentinel and selections', () => {
    expect(citiesParser.serialize(ALL_CITIES)).toBe(ALL_CITIES)
    expect(
      citiesParser.serialize([
        { city: 'Phoenix', state: 'AZ' },
        { city: 'Mesa', state: 'AZ' },
      ])
    ).toBe('Phoenix,AZ|Mesa,AZ')
  })

  it('round-trips selections and the sentinel through serialize→parse', () => {
    const cities = [{ city: 'Seattle', state: 'WA' }]
    expect(citiesParser.parse(citiesParser.serialize(cities))).toEqual(cities)
    expect(citiesParser.parse(citiesParser.serialize(ALL_CITIES))).toBe(ALL_CITIES)
  })

  it('eq compares the sentinel and selections correctly', () => {
    expect(citiesParser.eq!(ALL_CITIES, ALL_CITIES)).toBe(true)
    expect(citiesParser.eq!(ALL_CITIES, [{ city: 'Phoenix', state: 'AZ' }])).toBe(false)
    expect(
      citiesParser.eq!(
        [{ city: 'Phoenix', state: 'AZ' }, { city: 'Mesa', state: 'AZ' }],
        [{ city: 'Mesa', state: 'AZ' }, { city: 'Phoenix', state: 'AZ' }]
      )
    ).toBe(true)
  })
})
