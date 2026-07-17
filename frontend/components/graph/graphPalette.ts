'use client'

/**
 * Theme-resolved color palette for canvas graph surfaces (PSY-1083).
 *
 * Canvas 2D `fillStyle` / `strokeStyle` can't consume bare `var(--token)`
 * strings (PSY-1079 spike, G13), so paint callbacks need the theme tokens
 * resolved to concrete hex. `useGraphPalette()` resolves them once via
 * `getComputedStyle(document.documentElement)` and re-resolves when the
 * `<html>` class changes (next-themes toggles `.dark` there), watched via
 * MutationObserver through `useSyncExternalStore` — zero per-frame cost,
 * one resolve per theme change, and no setState-inside-effect.
 *
 * Two token families resolve here:
 *   - `--edge-*`   : the typed-edge grammar colors (see ./edgeGrammar.ts)
 *   - `--chart-1..8`: the categorical cluster palette (PSY-947 tokens).
 *     Replaces the hardcoded Okabe-Ito set — Okabe-Ito yellow #ECE133 is
 *     1.21:1 on the newsprint light bg, effectively invisible (PSY-1079
 *     spike measurement).
 *
 * Fallbacks are the DARK values: jsdom (tests) and the SSR pass resolve
 * no tokens, and dark is the pre-PSY-1083 rendering for edges.
 */

import { useMemo, useSyncExternalStore } from 'react'
import {
  EDGE_CSS_VARS,
  FALLBACK_EDGE_COLORS,
  FALLBACK_UNKNOWN_EDGE_COLOR,
  UNKNOWN_EDGE_CSS_VAR,
} from './edgeGrammar'

export interface GraphPalette {
  /** Edge-type → resolved hex (canonical 7 types). */
  edges: Record<string, string>
  /** Resolved hex for edge types outside the grammar. */
  unknownEdge: string
  /** Resolved `--chart-1..8` cluster colors, indexed 0..7. */
  chart: string[]
  /** Neutral grey for the "other" / ungrouped cluster. */
  otherCluster: string
  /** Resolved `--foreground` — node-label text, theme-aware (PSY-1091). */
  labelText: string
  /**
   * Resolved `--background` — halo stroked behind node labels so they stay
   * legible over colored nodes / cluster hulls on either theme (PSY-1091).
   */
  labelHalo: string
  /**
   * Resolved `--primary` — the shared suggested-direction affordance (dashed
   * primary ring + "+" badge, PSY-1453 restyle of the PSY-1273 DOI hint).
   */
  primary: string
  /**
   * Resolved `--muted-foreground` — the ego center node's neutral fill
   * (PSY-1453: center is distinguished by size + ink ring, not a hue).
   */
  mutedForeground: string
}

export const CHART_TOKEN_COUNT = 8

/**
 * Dark-theme `--chart-1..8` hex — MUST stay in sync with the `.dark` block
 * in app/globals.css. Canvas fallback for token-less environments.
 */
const FALLBACK_CHART_COLORS = [
  '#e89960', // --chart-1
  '#7a9d77', // --chart-2
  '#e0b66e', // --chart-3
  '#d74745', // --chart-4
  '#b59880', // --chart-5
  '#80a0c0', // --chart-6
  '#ad8fb0', // --chart-7
  '#6db3a6', // --chart-8
] as const

// Neutral grey for ungrouped nodes — slate-400, unchanged from the
// pre-PSY-1083 ForceGraphView constant. Deliberately NOT a theme token:
// it reads as "no category" on both themes.
const OTHER_CLUSTER_COLOR = '#94A3B8'

// Node-label text + halo — dark-theme `--foreground` / `--background`. Like the
// chart/edge fallbacks, these MUST stay in sync with the `.dark` block in
// app/globals.css; they are the canvas values wherever tokens can't resolve
// (jsdom, SSR). PSY-1091.
const FALLBACK_LABEL_TEXT = '#eee7d9' // --foreground (dark)
const FALLBACK_LABEL_HALO = '#0d0805' // --background (dark)

// Suggested-direction + ego-center tokens (PSY-1453) — dark values, same
// stay-in-sync contract with the `.dark` block as the label tokens above.
const FALLBACK_PRIMARY = '#e89960' // --primary (dark)
const FALLBACK_MUTED_FOREGROUND = '#9c8c7c' // --muted-foreground (dark)

const FALLBACK_PALETTE: GraphPalette = {
  edges: FALLBACK_EDGE_COLORS,
  unknownEdge: FALLBACK_UNKNOWN_EDGE_COLOR,
  chart: [...FALLBACK_CHART_COLORS],
  otherCluster: OTHER_CLUSTER_COLOR,
  labelText: FALLBACK_LABEL_TEXT,
  labelHalo: FALLBACK_LABEL_HALO,
  primary: FALLBACK_PRIMARY,
  mutedForeground: FALLBACK_MUTED_FOREGROUND,
}

/** Cluster fill for a canvas paint callback. -1 / out-of-range = "other". */
export function clusterColor(palette: GraphPalette, colorIndex: number): string {
  if (colorIndex < 0 || colorIndex >= palette.chart.length) {
    return palette.otherCluster
  }
  return palette.chart[colorIndex]
}

/**
 * `var()` expression for a cluster color — for DOM surfaces (the scene
 * graph's cluster legend pills). Theme-reactive with no JS re-resolution.
 */
export function clusterColorCSS(colorIndex: number): string {
  if (colorIndex < 0 || colorIndex >= CHART_TOKEN_COUNT) {
    return OTHER_CLUSTER_COLOR
  }
  return `var(--chart-${colorIndex + 1}, ${FALLBACK_CHART_COLORS[colorIndex]})`
}

/**
 * Append a 2-char hex alpha pair to a 6-digit hex color, passing any other
 * color format through untouched. Guards the `color + '66'` dimming idiom
 * against a future where a token stops being plain hex (e.g. oklch).
 */
export function withHexAlpha(color: string, alphaHexPair: string): string {
  return /^#[0-9a-fA-F]{6}$/.test(color) ? color + alphaHexPair : color
}

function readToken(style: CSSStyleDeclaration, token: string, fallback: string): string {
  const value = style.getPropertyValue(token).trim()
  return value || fallback
}

function resolveGraphPalette(): GraphPalette {
  const style = getComputedStyle(document.documentElement)
  const edges: Record<string, string> = {}
  for (const [type, cssVar] of Object.entries(EDGE_CSS_VARS)) {
    edges[type] = readToken(style, cssVar, FALLBACK_EDGE_COLORS[type])
  }
  const chart = FALLBACK_CHART_COLORS.map((fallback, i) =>
    readToken(style, `--chart-${i + 1}`, fallback),
  )
  return {
    edges,
    unknownEdge: readToken(style, UNKNOWN_EDGE_CSS_VAR, FALLBACK_UNKNOWN_EDGE_COLOR),
    chart,
    otherCluster: OTHER_CLUSTER_COLOR,
    labelText: readToken(style, '--foreground', FALLBACK_LABEL_TEXT),
    labelHalo: readToken(style, '--background', FALLBACK_LABEL_HALO),
    primary: readToken(style, '--primary', FALLBACK_PRIMARY),
    mutedForeground: readToken(style, '--muted-foreground', FALLBACK_MUTED_FOREGROUND),
  }
}

// Theme changes land as a class swap on <html> (next-themes). Subscribe via
// MutationObserver; the snapshot is the className string (referentially
// stable between mutations, as useSyncExternalStore requires).
function subscribeThemeClass(onChange: () => void): () => void {
  if (typeof MutationObserver === 'undefined') return () => {}
  const observer = new MutationObserver(onChange)
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
  return () => observer.disconnect()
}

function getThemeClassSnapshot(): string {
  return document.documentElement.className
}

// Server snapshot: a sentinel that never matches a real class string, so the
// memo below keeps the dark fallback palette during SSR/hydration. The
// palette never reaches server-rendered markup (canvas paints client-side
// only), so there is no hydration-mismatch surface.
function getServerThemeClassSnapshot(): string {
  return ' ssr'
}

/**
 * Resolved graph palette for the active theme. Re-resolves when the
 * `<html>` class changes; paint callbacks should depend on the returned
 * object so they re-close over the fresh colors.
 */
export function useGraphPalette(): GraphPalette {
  const themeClass = useSyncExternalStore(
    subscribeThemeClass,
    getThemeClassSnapshot,
    getServerThemeClassSnapshot,
  )
  return useMemo(() => {
    if (themeClass === ' ssr' || typeof window === 'undefined') {
      return FALLBACK_PALETTE
    }
    return resolveGraphPalette()
  }, [themeClass])
}
