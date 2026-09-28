import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ENTITY_LINK_CLASS } from './entityLink'

const css = readFileSync(resolve(process.cwd(), 'app/globals.css'), 'utf8')

/** A theme's token block: `:root {` for light, `.dark {` for dark. */
function block(opener: RegExp): string {
  const start = css.search(opener)
  expect(start, `block ${opener} missing from globals.css`).toBeGreaterThan(-1)
  return css.slice(start, css.indexOf('\n}', start))
}

const light = block(/^:root \{/m)
const dark = block(/^\.dark \{/m)

/** A token's hex, following one level of `var(--other)` within the block. */
function tokenHex(themeBlock: string, name: string): string {
  const m = themeBlock.match(new RegExp(`\\n\\s*--${name}:\\s*([^;]+);`))
  expect(m, `--${name} missing`).not.toBeNull()
  const value = m![1].trim()
  const alias = value.match(/^var\(--([\w-]+)\)$/)
  return alias ? tokenHex(themeBlock, alias[1]) : value.toLowerCase()
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map(i => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe('ENTITY_LINK_CLASS', () => {
  it('draws the accent tier in the solid link token, with no opacity and no hover recolour', () => {
    const tokens = ENTITY_LINK_CLASS.accent.split(' ')
    expect(tokens).toContain('text-link')
    expect(tokens.some(t => t.startsWith('text-link/'))).toBe(false)
    expect(tokens.some(t => t.startsWith('hover:text-'))).toBe(false)
    expect(tokens).toContain('hover:underline')
  })

  it('exposes the link token to Tailwind', () => {
    expect(css).toMatch(/--color-link:\s*var\(--link\);/)
  })

  it.each([
    ['light', light],
    ['dark', dark],
  ])('keeps link text at WCAG AA on the %s background and card', (_theme, themeBlock) => {
    const link = tokenHex(themeBlock, 'link')
    expect(contrast(link, tokenHex(themeBlock, 'background'))).toBeGreaterThanOrEqual(4.5)
    expect(contrast(link, tokenHex(themeBlock, 'card'))).toBeGreaterThanOrEqual(4.5)
  })
})
