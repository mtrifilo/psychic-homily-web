import { describe, expect, it } from 'vitest'
import { getSavedReleasePageBounds } from './savedReleasePagination'

describe('getSavedReleasePageBounds', () => {
  it('clamps a final page after its only saved release is removed', () => {
    expect(getSavedReleasePageBounds(2, 50, 50)).toEqual({
      page: 2,
      totalPages: 1,
      targetPage: 1,
    })
  })

  it('normalizes invalid page input', () => {
    expect(getSavedReleasePageBounds(Number.NaN, 0, 50)).toEqual({
      page: 1,
      totalPages: 1,
      targetPage: 1,
    })
  })
})
