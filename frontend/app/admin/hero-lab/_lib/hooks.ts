'use client'

/**
 * Hero Lab — React hooks shared by the wordmark effects: theme-token reading,
 * next/font family resolution for canvas, element sizing, in-view gating, the
 * reduced-motion query, and a shared rAF loop.
 */

import { useEffect, useRef, useState, type RefObject } from 'react'

export interface ThemeColors {
  background: string
  foreground: string
  primary: string
  accent: string
  muted: string
  isDark: boolean
}

/** SSR / first-paint default (light "newsprint"); corrected on mount. */
const LIGHT_DEFAULT: ThemeColors = {
  background: '#f4f1ea',
  foreground: '#1a1714',
  primary: '#d2541b',
  accent: '#ebd5a8',
  muted: '#6b5e4f',
  isDark: false,
}

/**
 * Read the live theme tokens off `<html>` and re-read whenever next-themes
 * flips the `.dark` class — so canvas colors track the toggle automatically.
 */
export function useThemeColors(): ThemeColors {
  const [colors, setColors] = useState<ThemeColors>(LIGHT_DEFAULT)
  useEffect(() => {
    const read = (): ThemeColors => {
      const style = getComputedStyle(document.documentElement)
      const isDark = document.documentElement.classList.contains('dark')
      const get = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback
      return {
        background: get('--background', isDark ? '#0d0805' : '#f4f1ea'),
        foreground: get('--foreground', isDark ? '#eee7d9' : '#1a1714'),
        primary: get('--primary', isDark ? '#e89960' : '#d2541b'),
        accent: get('--accent', isDark ? '#dc9064' : '#ebd5a8'),
        muted: get('--muted-foreground', isDark ? '#9c8c7c' : '#6b5e4f'),
        isDark,
      }
    }
    const update = () => setColors(read())
    update()
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    return () => observer.disconnect()
  }, [])
  return colors
}

/**
 * Resolve a next/font CSS variable (e.g. `--font-display`) to the concrete
 * hashed font-family string that Canvas2D's `ctx.font` needs, and wait until
 * the glyphs are actually loaded before reporting it (so sampling isn't blank).
 */
export function useCanvasFont(cssVar = '--font-display', weight = 700): string {
  const [family, setFamily] = useState('sans-serif')
  useEffect(() => {
    let alive = true
    const probe = document.createElement('span')
    probe.style.cssText = `position:absolute;visibility:hidden;font-family:var(${cssVar})`
    document.body.appendChild(probe)
    const resolved = getComputedStyle(probe).fontFamily || 'sans-serif'
    probe.remove()
    const apply = () => {
      if (alive) setFamily(resolved)
    }
    if (typeof document !== 'undefined' && document.fonts) {
      document.fonts
        .load(`${weight} 100px ${resolved}`)
        .then(() => document.fonts.ready)
        .then(apply)
        .catch(apply)
    } else {
      apply()
    }
    return () => {
      alive = false
    }
  }, [cssVar, weight])
  return family
}

/** Track an element's content-box size (CSS px) via ResizeObserver. */
export function useContainerSize(ref: RefObject<HTMLElement | null>): { width: number; height: number } {
  const [size, setSize] = useState({ width: 0, height: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      const rect = entries[0]?.contentRect
      if (rect) setSize({ width: Math.round(rect.width), height: Math.round(rect.height) })
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [ref])
  return size
}

/** True while the element is (near) the viewport — used to pause offscreen rAF. */
export function useInView(ref: RefObject<HTMLElement | null>, rootMargin = '200px'): boolean {
  const [inView, setInView] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const observer = new IntersectionObserver(([entry]) => setInView(entry.isIntersecting), { rootMargin })
    observer.observe(el)
    return () => observer.disconnect()
  }, [ref, rootMargin])
  return inView
}

/** OS-level reduced-motion preference, kept live. */
export function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false)
  useEffect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => setReduced(query.matches)
    update()
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])
  return reduced
}

/**
 * A single rAF loop that runs only while `active`. The callback receives the
 * absolute timestamp and a clamped delta (ms). Latest callback is always used
 * without restarting the loop.
 */
export function useAnimationFrame(callback: (time: number, delta: number) => void, active: boolean): void {
  const cbRef = useRef(callback)
  // Keep the latest callback without restarting the loop — assign in an effect,
  // not during render (react-hooks/refs).
  useEffect(() => {
    cbRef.current = callback
  })
  useEffect(() => {
    if (!active) return
    let raf = 0
    let last = performance.now()
    const loop = (now: number) => {
      const delta = Math.min(now - last, 50)
      last = now
      cbRef.current(now, delta)
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(raf)
  }, [active])
}

/** Pointer position in device px relative to the effect container. */
export interface PointerSample {
  x: number
  y: number
  active: boolean
}
