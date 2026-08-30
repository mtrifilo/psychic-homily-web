import { describe, it, expect } from 'vitest'
import { outboundRel } from './outboundRel'

describe('outboundRel', () => {
  it('carries the hygiene tokens on an ordinary outbound link', () => {
    expect(outboundRel()).toBe('noopener noreferrer')
    expect(outboundRel(false)).toBe('noopener noreferrer')
  })

  // Additive, never a replacement: qualifying a paid link must not cost it
  // opener/referrer protection.
  it('appends sponsored without dropping the hygiene tokens', () => {
    expect(outboundRel(true)).toBe('noopener noreferrer sponsored')
  })

  it('does not mutate the shared token list between calls', () => {
    outboundRel(true)
    expect(outboundRel(false)).toBe('noopener noreferrer')
  })
})
