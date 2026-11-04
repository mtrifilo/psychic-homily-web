import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import postcss, { type AtRule, type Node, type Rule } from 'postcss'
import tailwindcss from '@tailwindcss/postcss'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'

/**
 * The pointer-cursor base rule in globals.css, read from the CSS Tailwind
 * actually emits: globals.css is compiled through the project's PostCSS
 * plugin (`@tailwindcss/postcss`, the one postcss.config.mjs loads) with
 * optimization on, as a production build runs it. Candidate scanning points at
 * an empty directory, so only the stylesheet itself is compiled; base rules do
 * not depend on scanned classes.
 *
 * The rule must stay in `@layer base`: there a cursor utility on an element
 * (`cursor-grab`, `disabled:cursor-not-allowed`) still overrides it.
 */

const EXPECTED_SELECTORS = [
  'button:not(:disabled)',
  '[role="button"]:not([aria-disabled="true"])',
  '[role="link"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="menuitemcheckbox"]',
  '[role="menuitemradio"]',
  '[role="option"]',
  '[role="radio"]',
  '[role="checkbox"]',
  '[role="switch"]',
  '[role="combobox"]:not(input)',
  'summary',
  'select:not(:disabled)',
  'label[for]',
  'input:where([type="checkbox"],[type="radio"],[type="file"],[type="submit"],[type="button"],[type="range"]):not(:disabled)',
]

/** Quote style and whitespace are the optimizer's choice, not the contract. */
const normalize = (selector: string) =>
  selector.replace(/'/g, '"').replace(/\s+/g, '').trim()

let scanDir: string
let pointerRules: Rule[]

beforeAll(async () => {
  scanDir = mkdtempSync(join(tmpdir(), 'pointer-cursor-scan-'))
  const from = resolve(process.cwd(), 'app/globals.css')
  const result = await postcss([
    tailwindcss({ base: scanDir, optimize: { minify: false } }),
  ]).process(readFileSync(from, 'utf8'), { from })

  pointerRules = []
  result.root.walkRules(rule => {
    if (rule.nodes.some(n => n.type === 'decl' && n.prop === 'cursor' && n.value === 'pointer')) {
      pointerRules.push(rule)
    }
  })
}, 30_000)

afterAll(() => {
  rmSync(scanDir, { recursive: true, force: true })
})

/** The `@layer` names enclosing a rule, outermost first. */
function enclosingLayers(rule: Rule): string[] {
  const layers: string[] = []
  for (let node: Node | undefined = rule.parent; node; node = node.parent) {
    if (node.type === 'atrule' && (node as AtRule).name === 'layer') {
      layers.unshift((node as AtRule).params)
    }
  }
  return layers
}

describe('pointer-cursor base rule (compiled globals.css)', () => {
  it('emits exactly one rule with every interactive selector', () => {
    const baseRules = pointerRules.filter(rule => enclosingLayers(rule).includes('base'))
    expect(baseRules).toHaveLength(1)
    const selectors = baseRules[0].selectors.map(normalize)
    expect(selectors).toEqual(EXPECTED_SELECTORS.map(normalize))
  })

  it('keeps the rule in @layer base, below the utilities that override it', () => {
    const rule = pointerRules.find(r =>
      r.selectors.map(normalize).includes(normalize('label[for]'))
    )
    expect(rule, 'pointer rule missing from compiled CSS').toBeDefined()
    expect(enclosingLayers(rule!)).toEqual(['base'])
  })
})
