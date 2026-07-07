import { describe, it, expect } from 'vitest'
import type { GraphPalette } from '@/components/graph/graphPalette'
import { GENRE_FAMILIES, genreFamilyColor } from './genreFamilies'

const palette: GraphPalette = {
  edges: {},
  unknownEdge: '#000000',
  chart: ['c0', 'c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7'],
  otherCluster: '#94A3B8',
  labelText: '#ffffff',
  labelHalo: '#000000',
}

describe('genreFamilyColor', () => {
  it('resolves a family key to its palette slot hex', () => {
    // punk_hardcore -> colorIndex 3, folk_country -> colorIndex 0 (see genreFamilies.ts)
    expect(genreFamilyColor(palette, 'punk_hardcore')).toBe('c3')
    expect(genreFamilyColor(palette, 'folk_country')).toBe('c0')
    expect(genreFamilyColor(palette, 'pop_soul')).toBe('c7')
  })

  it('returns undefined for an absent key so the caller uses the neutral base', () => {
    expect(genreFamilyColor(palette, undefined)).toBeUndefined()
    expect(genreFamilyColor(palette, null)).toBeUndefined()
    expect(genreFamilyColor(palette, '')).toBeUndefined()
  })

  it('returns undefined for a key the frontend does not know (backend/FE drift)', () => {
    expect(genreFamilyColor(palette, 'nonexistent_family')).toBeUndefined()
  })
})

describe('GENRE_FAMILIES taxonomy integrity', () => {
  it('has eight families with unique keys and unique palette slots in range', () => {
    expect(GENRE_FAMILIES).toHaveLength(8)

    const keys = new Set(GENRE_FAMILIES.map((f) => f.key))
    expect(keys.size).toBe(8)

    const indices = GENRE_FAMILIES.map((f) => f.colorIndex)
    expect(new Set(indices).size).toBe(8) // no two families share a color
    for (const i of indices) {
      expect(i).toBeGreaterThanOrEqual(0)
      expect(i).toBeLessThan(8) // within --chart-1..8
    }
  })

  it('pins the cross-layer key contract with the backend genre-family map', () => {
    // These keys MUST match the genreFamily* constants in
    // backend/internal/services/catalog/genre_families.go — a mismatch leaves a
    // backend-emitted dominant_genre un-tinted with no legend entry.
    expect(GENRE_FAMILIES.map((f) => f.key).sort()).toEqual(
      [
        'electronic',
        'folk_country',
        'hip_hop',
        'jazz_experimental',
        'metal',
        'pop_soul',
        'punk_hardcore',
        'rock_indie',
      ].sort(),
    )
  })

  it('keeps the common families off the orange chart-1 slot', () => {
    // chart-1 (colorIndex 0) reads close to the no-data DOT_COLOR_BASE orange, so
    // the catalog's common families must not land there (genreFamilies.ts doc).
    const onChart1 = GENRE_FAMILIES.filter((f) => f.colorIndex === 0).map((f) => f.key)
    expect(onChart1).not.toContain('punk_hardcore')
    expect(onChart1).not.toContain('rock_indie')
  })
})
