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
 * optimization and minification on, as a production build runs it. Candidate
 * scanning points at an empty directory, so only the stylesheet itself is
 * compiled; base rules do not depend on scanned classes.
 *
 * The rule must stay in `@layer base`: there a cursor utility on an element
 * (`cursor-grab`, `disabled:cursor-not-allowed`) still overrides it.
 */

// The full list, not a sample: dropping or narrowing one selector silently
// returns that control to the arrow, and the compiled form is what ships.
// The roles are mirrored by POINTER_ROLES in e2e/pages/cursor.spec.ts.
const EXPECTED_SELECTORS = [
  'button:not(:disabled)',
  'select:not(:disabled)',
  'input:where([type="checkbox"],[type="radio"],[type="file"],[type="submit"],[type="button"],[type="range"]):not(:disabled)',
  'summary',
  'label[for]',
  ':where([role="button"],[role="link"],[role="tab"],[role="menuitem"],[role="menuitemcheckbox"],[role="menuitemradio"],[role="option"],[role="radio"],[role="checkbox"],[role="switch"],[role="combobox"]:not(input)):not(:disabled,[aria-disabled="true"])',
]

/** Quotes and whitespace are the minifier's choice, not the contract. */
const normalize = (selector: string) => selector.replace(/['"\s]/g, '')

let scanDir: string
let pointerRules: Rule[]

beforeAll(async () => {
  scanDir = mkdtempSync(join(tmpdir(), 'pointer-cursor-scan-'))
  const from = resolve(process.cwd(), 'app/globals.css')
  const result = await postcss([
    tailwindcss({ base: scanDir, optimize: true }),
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
  it('emits one cursor:pointer rule in @layer base, with every interactive selector', () => {
    const baseRules = pointerRules.filter(rule => enclosingLayers(rule).join() === 'base')
    expect(baseRules).toHaveLength(1)
    expect(baseRules[0].selectors.map(normalize)).toEqual(EXPECTED_SELECTORS.map(normalize))
  })
})
