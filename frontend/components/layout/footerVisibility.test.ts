import { describe, it, expect } from 'vitest'
import { isFooterHiddenBelowXl } from './footerVisibility'

describe('isFooterHiddenBelowXl', () => {
  it('hides the footer below xl on the Atlas', () => {
    expect(isFooterHiddenBelowXl('/atlas')).toBe(true)
  })

  it.each(['/', '/shows', '/venues', '/graph', '/scenes/phoenix-az', '/atlas/other', '/atlases'])(
    'keeps the footer on %s',
    (pathname) => {
      expect(isFooterHiddenBelowXl(pathname)).toBe(false)
    }
  )

  it('keeps the footer when the pathname is unknown', () => {
    expect(isFooterHiddenBelowXl(null)).toBe(false)
  })
})
