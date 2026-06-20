'use client'

/**
 * Hero background lightning (PSY-1137 follow-on) — Canvas2D base of the hybrid
 * renderer (a lazy WebGL-bloom upgrade swaps in over this where supported).
 *
 * Forked bolts shoot outward from behind the wordmark into the hero's empty
 * space — the "charged third-eye" motif. Bolt geometry uses the canonical
 * jitter-and-fork method (midpoint displacement + recursive branches; cf.
 * drilian.com lightning-bolts + tutsplus "shockingly good 2D lightning"), which
 * is what reads as crackling lightning rather than a smooth streak.
 *
 * Theme: additive amber glow + bright core on the near-black dark theme; on the
 * cream light theme the glow can't read, so bolts become solid dark-terracotta
 * "ink cracks" (source-over, no halo).
 *
 * Accessibility (HARD constraints — WCAG 2.3.1): bolts spawn ~once per 2-4s
 * (far under 3 flashes/sec), fade smoothly with no flicker, never a full-screen
 * flash, and the whole layer is disabled under prefers-reduced-motion.
 * Decorative + aria-hidden.
 */

import { useEffect, useRef } from 'react'
import { mix, readWordmarkColors } from './sampleWordmark'

interface Segment {
  ax: number
  ay: number
  bx: number
  by: number
  gen: number
}

interface Bolt {
  segments: Segment[]
  origin: { x: number; y: number }
  maxDist: number
  born: number
  life: number
  growth: number
}

const SUBDIVISIONS = 6

export function HeroLightning({ className }: { className?: string }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const container = containerRef.current
    const canvas = canvasRef.current
    if (!container || !canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    let { primary, foreground, isDark } = readWordmarkColors()
    // Dark: warm-white hot core. Light: a darker terracotta-brown "ink".
    let crack = mix(primary, foreground, 0.4)
    let w = 0
    let h = 0
    let bolts: Bolt[] = []
    let raf = 0
    let spawnTimer = 0
    let running = false
    let visible = true
    let disposed = false

    const resize = () => {
      const rect = container.getBoundingClientRect()
      w = Math.max(1, Math.round(rect.width * dpr))
      h = Math.max(1, Math.round(rect.height * dpr))
      canvas.width = w
      canvas.height = h
      canvas.style.width = `${rect.width}px`
      canvas.style.height = `${rect.height}px`
    }

    // Jitter-and-fork: midpoint-displace the main bolt while spawning branches
    // that veer off and get subdivided (jagged) by later passes.
    const makeBolt = (): Bolt => {
      const ox = w * (0.4 + Math.random() * 0.2)
      const oy = h * (0.32 + Math.random() * 0.36)
      const angle = Math.random() * Math.PI * 2
      const len = Math.max(w, h) * (0.45 + Math.random() * 0.4)
      let segments: Segment[] = [
        { ax: ox, ay: oy, bx: ox + Math.cos(angle) * len, by: oy + Math.sin(angle) * len, gen: 0 },
      ]
      let amp = len * 0.16
      for (let pass = 0; pass < SUBDIVISIONS; pass++) {
        const next: Segment[] = []
        for (const s of segments) {
          const dx = s.bx - s.ax
          const dy = s.by - s.ay
          const nlen = Math.hypot(dx, dy) || 1
          const mx = (s.ax + s.bx) / 2
          const my = (s.ay + s.by) / 2
          const off = (Math.random() - 0.5) * amp * (s.gen === 0 ? 1 : 0.6)
          const jx = mx + (-dy / nlen) * off
          const jy = my + (dx / nlen) * off
          next.push({ ax: s.ax, ay: s.ay, bx: jx, by: jy, gen: s.gen })
          next.push({ ax: jx, ay: jy, bx: s.bx, by: s.by, gen: s.gen })
          // Fork: only in early-mid passes so the branch still gets jagged.
          const forkChance = s.gen === 0 ? 0.5 : s.gen === 1 ? 0.28 : 0
          if (pass >= 1 && pass <= 3 && Math.random() < forkChance) {
            const branchAngle =
              Math.atan2(dy, dx) + (Math.random() < 0.5 ? -1 : 1) * (0.3 + Math.random() * 0.5)
            const flen = nlen * (0.6 + Math.random() * 0.5)
            next.push({
              ax: jx,
              ay: jy,
              bx: jx + Math.cos(branchAngle) * flen,
              by: jy + Math.sin(branchAngle) * flen,
              gen: s.gen + 1,
            })
          }
        }
        segments = next
        amp *= 0.55
      }
      let maxDist = 1
      for (const s of segments) maxDist = Math.max(maxDist, Math.hypot(s.bx - ox, s.by - oy))
      return {
        segments,
        origin: { x: ox, y: oy },
        maxDist,
        born: performance.now(),
        life: 480 + Math.random() * 260,
        growth: 80 + Math.random() * 70,
      }
    }

    const strokeSeg = (s: Segment) => {
      ctx.beginPath()
      ctx.moveTo(s.ax, s.ay)
      ctx.lineTo(s.bx, s.by)
      ctx.stroke()
    }

    const drawBolt = (b: Bolt, now: number) => {
      const age = now - b.born
      if (age >= b.life) return
      const grow = Math.min(1, age / b.growth)
      const fade =
        age < b.growth ? age / b.growth : 1 - (age - b.growth) / (b.life - b.growth)
      if (fade <= 0) return
      const revealDist = grow * b.maxDist
      ctx.lineJoin = 'round'
      ctx.lineCap = 'round'
      for (const s of b.segments) {
        if (Math.hypot(s.ax - b.origin.x, s.ay - b.origin.y) > revealDist) continue
        const gen = s.gen === 0 ? 1 : s.gen === 1 ? 0.55 : 0.32
        const alpha = Math.max(0, Math.min(1, fade * gen))
        if (alpha <= 0.01) continue
        if (isDark) {
          ctx.strokeStyle = `rgba(${primary[0]}, ${primary[1]}, ${primary[2]}, ${0.11 * alpha})`
          ctx.lineWidth = dpr * (s.gen === 0 ? 4.5 : 2.6)
          strokeSeg(s)
          ctx.strokeStyle = `rgba(255, 238, 214, ${0.62 * alpha})`
          ctx.lineWidth = dpr * (s.gen === 0 ? 1.1 : 0.8)
          strokeSeg(s)
        } else {
          ctx.strokeStyle = `rgba(${Math.round(crack[0])}, ${Math.round(crack[1])}, ${Math.round(crack[2])}, ${(0.38 + 0.24 * gen) * alpha})`
          ctx.lineWidth = dpr * (s.gen === 0 ? 1.5 : 1)
          strokeSeg(s)
        }
      }
    }

    const frame = (now: number) => {
      if (disposed) return
      ctx.clearRect(0, 0, w, h)
      bolts = bolts.filter((b) => now - b.born < b.life)
      ctx.globalCompositeOperation = isDark ? 'lighter' : 'source-over'
      for (const b of bolts) drawBolt(b, now)
      ctx.globalCompositeOperation = 'source-over'
      if (bolts.length > 0) {
        raf = requestAnimationFrame(frame)
      } else {
        running = false
      }
    }
    const ensureRunning = () => {
      if (!running && !disposed && visible) {
        running = true
        raf = requestAnimationFrame(frame)
      }
    }

    const scheduleSpawn = () => {
      spawnTimer = window.setTimeout(
        () => {
          if (disposed) return
          if (visible && w > 1) {
            bolts.push(makeBolt())
            if (Math.random() < 0.12) bolts.push(makeBolt())
            ensureRunning()
          }
          scheduleSpawn()
        },
        3200 + Math.random() * 3800,
      )
    }

    resize()
    const themeObs = new MutationObserver(() => {
      ;({ primary, foreground, isDark } = readWordmarkColors())
      crack = mix(primary, foreground, 0.4)
    })
    themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    const ro = new ResizeObserver(resize)
    ro.observe(container)
    const io = new IntersectionObserver(
      ([entry]) => {
        visible = entry.isIntersecting
        if (visible) ensureRunning()
      },
      { rootMargin: '0px' },
    )
    io.observe(container)
    scheduleSpawn()

    return () => {
      disposed = true
      cancelAnimationFrame(raf)
      clearTimeout(spawnTimer)
      themeObs.disconnect()
      ro.disconnect()
      io.disconnect()
    }
  }, [])

  return (
    <div
      ref={containerRef}
      aria-hidden
      className={`pointer-events-none absolute inset-0 -z-10 overflow-hidden ${className ?? ''}`}
    >
      <canvas ref={canvasRef} className="h-full w-full" />
    </div>
  )
}
