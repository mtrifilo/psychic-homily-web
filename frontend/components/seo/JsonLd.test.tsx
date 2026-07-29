import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { JsonLd } from './JsonLd'

const markupFor = (data: object) => renderToStaticMarkup(<JsonLd data={data} />)

/** The payload as a crawler's JSON parser sees it. */
const parsePayload = (markup: string) => {
  const body = markup.replace(/^.*?>/s, '').replace(/<\/script>$/, '')
  return JSON.parse(body)
}

describe('JsonLd', () => {
  it('renders the data as an ld+json script', () => {
    const markup = markupFor({ '@type': 'Thing', name: 'The Rebel Lounge' })

    expect(markup).toContain('type="application/ld+json"')
    expect(parsePayload(markup)).toEqual({
      '@type': 'Thing',
      name: 'The Rebel Lounge',
    })
  })

  // Entity names are user-contributed: `POST /shows` accepts a free-text artist
  // name from any authenticated account and auto-approves the show, so the name
  // reaches this component via `GET /artists`. `JSON.stringify` escapes `"` and
  // `\` and NOT `<`, so without an escape here a name containing `</script>`
  // closes the block and the rest parses as HTML — stored XSS, which the CSP
  // would not catch because `script-src` includes `'unsafe-inline'`.
  it('escapes a name that tries to close the script element', () => {
    const name = 'Evil Band</script><script>alert(1)</script>'
    const markup = markupFor({ '@type': 'Thing', name })

    // Exactly one closing tag: the element's own.
    expect(markup.match(/<\/script>/g)).toHaveLength(1)
    expect(markup).not.toContain('<script>alert(1)')
    // The crawler still reads the original name back.
    expect(parsePayload(markup).name).toBe(name)
  })

  it('escapes the JavaScript line terminators that are legal in JSON', () => {
    const LS = String.fromCharCode(0x2028)
    const PS = String.fromCharCode(0x2029)
    const name = `Line${LS}Break${PS}Band`
    const markup = markupFor({ '@type': 'Thing', name })

    expect(markup).not.toContain(LS)
    expect(markup).not.toContain(PS)
    expect(parsePayload(markup).name).toBe(name)
  })

  it('escapes ampersands so an entity reference cannot be decoded first', () => {
    const markup = markupFor({ '@type': 'Thing', name: 'Sturm & Drang' })

    expect(markup).not.toContain('&')
    expect(parsePayload(markup).name).toBe('Sturm & Drang')
  })
})
