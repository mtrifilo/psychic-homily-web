import { describe, it, expect } from 'vitest'
import { outboundRel } from './outboundRel'

describe('outboundRel', () => {
  it('carries the hygiene tokens on an ordinary outbound link', () => {
    expect(outboundRel()).toBe('noopener noreferrer')
    expect(outboundRel({})).toBe('noopener noreferrer')
    expect(outboundRel({ sponsored: false, ugc: false })).toBe(
      'noopener noreferrer'
    )
  })

  // Additive, never a replacement: qualifying a link must not cost it
  // opener/referrer protection.
  it('appends sponsored without dropping the hygiene tokens', () => {
    expect(outboundRel({ sponsored: true })).toBe(
      'noopener noreferrer sponsored'
    )
  })

  // `ugc` is the qualifier for a contributor-chosen destination that earns the
  // site nothing, which is what the free-admission ticket link is.
  it('appends ugc without dropping the hygiene tokens', () => {
    expect(outboundRel({ ugc: true })).toBe('noopener noreferrer ugc')
  })

  // Not mutually exclusive to this function, and the ORDER is pinned so a
  // reordering shows up as a diff rather than as a silent string change.
  it('appends both qualifiers, sponsored first', () => {
    expect(outboundRel({ sponsored: true, ugc: true })).toBe(
      'noopener noreferrer sponsored ugc'
    )
  })

  it('does not mutate the shared token list between calls', () => {
    outboundRel({ sponsored: true, ugc: true })
    expect(outboundRel()).toBe('noopener noreferrer')
  })
})
