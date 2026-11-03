import { describe, expect, it } from 'vitest'
import { ENTITY_LINK_CLASS } from './entityLink'

// The link token's own contract (its contrast on the passing surfaces listed
// beside `--link` in globals.css) is pinned in app/linkToken.test.ts.
describe('ENTITY_LINK_CLASS', () => {
  it('draws the accent tier in the solid link token, with no opacity and no hover recolour', () => {
    const tokens = ENTITY_LINK_CLASS.accent.split(' ')
    expect(tokens).toContain('text-link')
    expect(tokens.some(t => t.startsWith('text-link/'))).toBe(false)
    expect(tokens.some(t => t.startsWith('hover:text-'))).toBe(false)
    expect(tokens).toContain('hover:underline')
  })

  it('recolours the restrained tier to the link token on hover, never to primary', () => {
    const tokens = ENTITY_LINK_CLASS.restrained.split(' ')
    expect(tokens).toContain('hover:text-link')
    expect(tokens).not.toContain('hover:text-primary')
  })
})
