'use client'

import { useEffect, useRef, useState } from 'react'

/**
 * Lazy-mount a below-the-fold graph section on scroll intent (PSY-1347,
 * extracted from InlineGraph PSY-837 + HomeSceneGraph PSY-1344, which carried
 * near-verbatim copies — a fix to this observer logic previously meant editing
 * both).
 *
 * Returns a `containerRef` to attach to the section's outer element and an
 * `isMounted` flag that flips true the first time that element intersects the
 * viewport (pre-loaded by `rootMargin`, default 200px, so data fetching kicks
 * off just before the placeholder is fully on-screen). Once mounted it never
 * tears down — the visitor has shown intent by scrolling here.
 *
 * When `IntersectionObserver` is unavailable (SSR, very old browsers) it falls
 * back to an immediate mount. React 19.2: the fallback defers its `setState` to
 * a microtask so it lands AFTER the effect returns rather than synchronously in
 * the effect body (react-hooks/set-state-in-effect / a cascading render); the
 * two-phase render (placeholder → mounted) is preserved exactly.
 */
export function useLazyGraphMount(rootMargin = '200px'): {
  containerRef: React.RefObject<HTMLDivElement | null>
  isMounted: boolean
} {
  const containerRef = useRef<HTMLDivElement>(null)
  const [isMounted, setIsMounted] = useState(false)

  useEffect(() => {
    if (isMounted) return
    const node = containerRef.current
    if (!node || typeof IntersectionObserver === 'undefined') {
      let cancelled = false
      Promise.resolve().then(() => {
        if (!cancelled) setIsMounted(true)
      })
      return () => {
        cancelled = true
      }
    }
    const observer = new IntersectionObserver(
      entries => {
        if (entries.some(entry => entry.isIntersecting)) {
          setIsMounted(true)
          observer.disconnect()
        }
      },
      { rootMargin },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [isMounted, rootMargin])

  return { containerRef, isMounted }
}
