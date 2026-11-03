import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * The `--link` token's contract in globals.css: text in it clears WCAG AA
 * (4.5:1) on each surface the token's comment lists as passing. Background,
 * card, muted, secondary and sidebar are asserted in both themes; the muted/20
 * stripe and the primary tints in light only. A token edit that breaks any of
 * them fails here; the comment's failing surfaces (the accent fills, denser
 * tints) are not pinned.
 */

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

const channel = (hex: string, i: number) => parseInt(hex.slice(i, i + 2), 16)

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map(i => {
    const c = channel(hex, i) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

/** `fg` at `alpha` over an opaque `bg`, as the browser composites a `/N` fill. */
function composite(fg: string, bg: string, alpha: number): string {
  return (
    '#' +
    [1, 3, 5]
      .map(i => Math.round(channel(fg, i) * alpha + channel(bg, i) * (1 - alpha)))
      .map(v => v.toString(16).padStart(2, '0'))
      .join('')
  )
}

describe('the link token', () => {
  it('is exposed to Tailwind as the `link` colour', () => {
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

  it.each([
    ['light', 'muted'],
    ['light', 'secondary'],
    ['light', 'sidebar'],
    ['dark', 'muted'],
    ['dark', 'secondary'],
    ['dark', 'sidebar'],
  ])('keeps %s link text at WCAG AA on the %s surface', (theme, surface) => {
    const themeBlock = theme === 'light' ? light : dark
    expect(contrast(tokenHex(themeBlock, 'link'), tokenHex(themeBlock, surface))).toBeGreaterThanOrEqual(4.5)
  })

  it('keeps light link text at WCAG AA on the muted/20 list-row stripe', () => {
    const stripe = composite(tokenHex(light, 'muted'), tokenHex(light, 'background'), 0.2)
    expect(contrast(tokenHex(light, 'link'), stripe)).toBeGreaterThanOrEqual(4.5)
  })

  it.each([
    ['background', 0.1],
    ['background', 0.15],
    ['card', 0.1],
    ['card', 0.15],
  ])('keeps light link text at WCAG AA on a primary tint over %s at %s', (base, alpha) => {
    const tint = composite(tokenHex(light, 'primary'), tokenHex(light, base), alpha)
    expect(contrast(tokenHex(light, 'link'), tint)).toBeGreaterThanOrEqual(4.5)
  })
})
