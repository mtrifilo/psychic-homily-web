'use client'

/**
 * Effect C — "Magnetic Letters".
 *
 * Pure DOM: each glyph is a real <span>; near the cursor letters are gently
 * pushed away and scaled up, easing back when it leaves. Tasteful, tiny, and
 * fully accessible (the heading is real selectable text). Production path: GSAP
 * SplitText (free since 2025) or Motion per-character — no canvas, no WebGL,
 * best LCP. This is the low-risk floor option.
 */

import { useEffect, useRef } from 'react'
import { WORDMARK_LINES } from '../_lib/constants'

interface LetterState {
  el: HTMLSpanElement
  cx: number
  cy: number
  x: number
  y: number
  s: number
}

export function MagneticLetters({ reducedMotion }: { reducedMotion: boolean }) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const letters = useRef<LetterState[]>([])
  const pointer = useRef({ x: 0, y: 0, active: false })

  // Register each glyph span and seed its rest state.
  const register = (el: HTMLSpanElement | null) => {
    if (!el) return
    if (!letters.current.some((l) => l.el === el)) {
      letters.current.push({ el, cx: 0, cy: 0, x: 0, y: 0, s: 1 })
    }
  }

  useEffect(() => {
    if (reducedMotion) {
      for (const l of letters.current) l.el.style.transform = ''
      return
    }
    let raf = 0
    let alive = true

    // Cache each glyph's rest-center relative to the wrapper (transform-free).
    const measure = () => {
      for (const l of letters.current) {
        l.cx = l.el.offsetLeft + l.el.offsetWidth / 2
        l.cy = l.el.offsetTop + l.el.offsetHeight / 2
      }
    }
    measure()
    const onResize = () => measure()
    window.addEventListener('resize', onResize)

    const loop = () => {
      if (!alive) return
      const wrap = wrapRef.current
      if (wrap) {
        const rect = wrap.getBoundingClientRect()
        const px = pointer.current.x
        const py = pointer.current.y
        const act = pointer.current.active
        const radius = Math.min(rect.width, rect.height) * 0.55 + 90

        for (const l of letters.current) {
          let tx = 0
          let ty = 0
          let ts = 1
          if (act) {
            const dx = l.cx - px
            const dy = l.cy - py
            const dist = Math.hypot(dx, dy) || 1
            if (dist < radius) {
              const force = 1 - dist / radius
              tx = (dx / dist) * force * 28
              ty = (dy / dist) * force * 28
              ts = 1 + force * 0.2
            }
          }
          // Critically damped-ish easing toward target.
          l.x += (tx - l.x) * 0.16
          l.y += (ty - l.y) * 0.16
          l.s += (ts - l.s) * 0.16
          l.el.style.transform = `translate(${l.x.toFixed(2)}px, ${l.y.toFixed(2)}px) scale(${l.s.toFixed(3)})`
        }
      }
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)

    return () => {
      alive = false
      cancelAnimationFrame(raf)
      window.removeEventListener('resize', onResize)
    }
  }, [reducedMotion])

  return (
    <div
      ref={wrapRef}
      className="relative flex h-full w-full select-none flex-col items-center justify-center"
      role="img"
      aria-label="Psychic Homily"
      onPointerMove={(e) => {
        const rect = e.currentTarget.getBoundingClientRect()
        pointer.current = { x: e.clientX - rect.left, y: e.clientY - rect.top, active: true }
      }}
      onPointerLeave={() => {
        pointer.current.active = false
      }}
    >
      {WORDMARK_LINES.map((line) => (
        <div key={line} className="flex font-display text-[clamp(2.5rem,11vw,8rem)] font-bold leading-[0.95] tracking-tight text-foreground">
          {line.split('').map((char, i) => (
            <span
              key={`${line}-${i}`}
              ref={register}
              aria-hidden
              className="inline-block will-change-transform"
            >
              {char}
            </span>
          ))}
        </div>
      ))}
    </div>
  )
}
