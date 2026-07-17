import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import {
  CHART_TOKEN_COUNT,
  clusterColor,
  clusterColorCSS,
  useGraphPalette,
  withHexAlpha,
  type GraphPalette,
} from './graphPalette'
import { EDGE_CSS_VARS, EDGE_TYPES } from './edgeGrammar'

// PSY-1083: theme-resolved palette for canvas paint callbacks. jsdom's
// getComputedStyle never resolves CSS custom properties, so every token
// read falls back to the dark constants — which is exactly the production
// fallback path under test here. The browser-side token resolution is
// verified in the manual repro (light + dark screenshots).

describe('useGraphPalette (jsdom fallback path)', () => {
  it('returns the dark fallback palette when tokens cannot resolve', () => {
    const { result } = renderHook(() => useGraphPalette())
    // Edge fallbacks are the pre-PSY-1083 ArtistGraph EDGE_COLORS values.
    expect(result.current.edges.shared_bills).toBe('#60a5fa')
    expect(result.current.edges.member_of).toBe('#fbbf24')
    expect(result.current.unknownEdge).toBe('#71717a')
    expect(result.current.chart).toHaveLength(CHART_TOKEN_COUNT)
    for (const c of result.current.chart) {
      expect(c).toMatch(/^#[0-9A-Fa-f]{6}$/)
    }
    // Node-label fallbacks are the dark `--foreground` / `--background` (PSY-1091).
    expect(result.current.labelText).toBe('#eee7d9')
    expect(result.current.labelHalo).toBe('#0d0805')
    // Suggested-direction + ego-center fallbacks (PSY-1453).
    expect(result.current.primary).toBe('#e89960')
    expect(result.current.mutedForeground).toBe('#9c8c7c')
  })

  it('covers every canonical edge type', () => {
    const { result } = renderHook(() => useGraphPalette())
    for (const type of EDGE_TYPES) {
      expect(result.current.edges[type]).toMatch(/^#[0-9A-Fa-f]{6}$/)
    }
  })

  it('re-resolves when the <html> class changes (theme toggle)', async () => {
    const { result } = renderHook(() => useGraphPalette())
    const before = result.current
    await act(async () => {
      document.documentElement.classList.add('dark')
      // MutationObserver callbacks are microtask-scheduled.
      await Promise.resolve()
    })
    // jsdom still resolves no tokens, so values match — but the object is a
    // fresh resolve, proving the MutationObserver subscription fired.
    expect(result.current).not.toBe(before)
    expect(result.current.edges.shared_bills).toBe(before.edges.shared_bills)
    document.documentElement.classList.remove('dark')
  })
})

// The fallback constants carry a "MUST stay in sync with the .dark block in
// app/globals.css" contract (they are the canvas palette wherever tokens
// can't resolve — and the dark set is the zero-regression guarantee for the
// pre-PSY-1083 artist graph). Enforce the sync instead of trusting comments.
describe('dark-theme token sync (globals.css ↔ fallback constants)', () => {
  const css = readFileSync(resolve(process.cwd(), 'app/globals.css'), 'utf8')
  // `.dark {` at line start — earlier matches ('@custom-variant dark (.dark *)',
  // prose mentions in comments) are not the token block.
  const darkStart = css.search(/^\.dark \{/m)
  const darkBlock = css.slice(darkStart, css.indexOf('\n}', darkStart))

  function darkToken(name: string): string {
    const m = darkBlock.match(new RegExp(`${name}:\\s*([^;]+);`))
    expect(m, `token ${name} missing from the .dark block`).not.toBeNull()
    return m![1].trim().toLowerCase()
  }

  it('matches every --edge-* fallback to its .dark token', () => {
    const { result } = renderHook(() => useGraphPalette())
    for (const type of EDGE_TYPES) {
      const cssVar = EDGE_CSS_VARS[type]
      expect(result.current.edges[type].toLowerCase(), cssVar).toBe(darkToken(cssVar))
    }
    expect(result.current.unknownEdge.toLowerCase()).toBe(darkToken('--edge-unknown'))
  })

  it('matches every --chart-N fallback to its .dark token', () => {
    const { result } = renderHook(() => useGraphPalette())
    for (let i = 0; i < CHART_TOKEN_COUNT; i++) {
      expect(result.current.chart[i].toLowerCase(), `--chart-${i + 1}`).toBe(
        darkToken(`--chart-${i + 1}`),
      )
    }
  })

  it('matches the node-label fallbacks to their .dark tokens (PSY-1091)', () => {
    const { result } = renderHook(() => useGraphPalette())
    expect(result.current.labelText.toLowerCase(), '--foreground').toBe(darkToken('--foreground'))
    expect(result.current.labelHalo.toLowerCase(), '--background').toBe(darkToken('--background'))
  })

  it('matches the primary + muted-foreground fallbacks to their .dark tokens (PSY-1453)', () => {
    const { result } = renderHook(() => useGraphPalette())
    expect(result.current.primary.toLowerCase(), '--primary').toBe(darkToken('--primary'))
    expect(result.current.mutedForeground.toLowerCase(), '--muted-foreground').toBe(
      darkToken('--muted-foreground'),
    )
  })
})

describe('clusterColor', () => {
  const palette: GraphPalette = {
    edges: {},
    unknownEdge: '#71717a',
    chart: ['#111111', '#222222', '#333333', '#444444', '#555555', '#666666', '#777777', '#888888'],
    otherCluster: '#94A3B8',
    labelText: '#eee7d9',
    labelHalo: '#0d0805',
    primary: '#e89960',
    mutedForeground: '#9c8c7c',
  }

  it('indexes into the resolved chart palette', () => {
    expect(clusterColor(palette, 0)).toBe('#111111')
    expect(clusterColor(palette, 7)).toBe('#888888')
  })

  it('returns the neutral grey for "other" / out-of-range indices', () => {
    expect(clusterColor(palette, -1)).toBe('#94A3B8')
    expect(clusterColor(palette, 8)).toBe('#94A3B8')
    expect(clusterColor(palette, 99)).toBe('#94A3B8')
  })
})

describe('clusterColorCSS', () => {
  it('returns a var() expression per chart token with a dark fallback', () => {
    expect(clusterColorCSS(0)).toBe('var(--chart-1, #e89960)')
    expect(clusterColorCSS(7)).toBe('var(--chart-8, #6db3a6)')
  })

  it('returns the neutral grey for out-of-range indices', () => {
    expect(clusterColorCSS(-1)).toBe('#94A3B8')
    expect(clusterColorCSS(8)).toBe('#94A3B8')
  })
})

describe('withHexAlpha', () => {
  it('appends the alpha pair to 6-digit hex colors', () => {
    expect(withHexAlpha('#60a5fa', '66')).toBe('#60a5fa66')
    expect(withHexAlpha('#D55E00', 'B3')).toBe('#D55E00B3')
  })

  it('passes non-hex color formats through untouched', () => {
    expect(withHexAlpha('rgba(1, 2, 3, 0.5)', '66')).toBe('rgba(1, 2, 3, 0.5)')
    expect(withHexAlpha('oklch(0.7 0.1 50)', '66')).toBe('oklch(0.7 0.1 50)')
    expect(withHexAlpha('#fff', '66')).toBe('#fff')
  })
})
