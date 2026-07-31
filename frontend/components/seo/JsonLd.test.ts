import { describe, it, expect } from 'vitest'
import { serializeJsonLd } from './JsonLd'

// The data in these blocks is community-submitted — show titles, venue names,
// artist names — and it is written into a <script> with dangerouslySetInnerHTML,
// which React does not escape. `JSON.stringify` alone leaves `<` and `>` intact,
// so a `</script>` in a band name ends the element and the rest becomes markup.
describe('serializeJsonLd', () => {
  it('cannot be closed early by a </script> in the data', () => {
    const out = serializeJsonLd({
      name: 'Band </script><img src=x onerror=alert(1)>',
    })
    expect(out).not.toContain('</script')
    expect(out).not.toContain('<')
    expect(out).not.toContain('>')
  })

  it('escapes the ampersand that HTML entity tricks need', () => {
    expect(serializeJsonLd({ name: 'Tom & Jerry' })).not.toContain('&')
  })

  // Legal in JSON, but JavaScript line terminators — they break any consumer
  // that evaluates the block rather than parsing it.
  it('escapes U+2028 and U+2029', () => {
    const out = serializeJsonLd({ name: 'a\u2028b\u2029c' })
    expect(out).not.toContain('\u2028')
    expect(out).not.toContain('\u2029')
  })

  // The escapes are plain JSON string escapes, so a crawler still reads exactly
  // what we meant to publish. Escaping that changed the data would be a bug.
  it('round-trips to the original values', () => {
    const data = {
      '@context': 'https://schema.org',
      name: 'Band </script> & <friends>\u2028',
      nested: { url: 'https://psychichomily.com/shows/a?b=1&c=2' },
    }
    expect(JSON.parse(serializeJsonLd(data))).toEqual(data)
  })
})
