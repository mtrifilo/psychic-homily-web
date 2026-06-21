'use client'

/**
 * Effect A — "Scrying Grid".
 *
 * The wordmark is rendered as a field of light-cells (sampled points) that
 * shimmer at rest and IGNITE near the cursor, leaning toward it before
 * springing home. This is the Andi Watson reference made interactive — his
 * monograph cover renders the wordmark as glowing cells over LED battens, and
 * his "fill in the gaps" spaced-strip technique is exactly this: the eye
 * completes the letters out of discrete points. Production path: Canvas2D for
 * the MVP, Regl/OGL flow-field for the lush version (the Vercel Ship hero
 * technique).
 */

import { useEffect, useRef } from 'react'
import {
  clamp,
  getDpr,
  mix,
  parseHex,
  rgba,
  sampleWordmark,
  type RGB,
} from '../_lib/canvas'
import {
  useAnimationFrame,
  useCanvasFont,
  useContainerSize,
  useInView,
  useThemeColors,
  type PointerSample,
} from '../_lib/hooks'
import { WORDMARK_LINES } from '../_lib/constants'

interface Particle {
  hx: number
  hy: number
  x: number
  y: number
  vx: number
  vy: number
  phase: number
}

export function ScryingGrid({ reducedMotion }: { reducedMotion: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const particles = useRef<Particle[]>([])
  const pointer = useRef<PointerSample>({ x: 0, y: 0, active: false })
  const drawRef = useRef<(time: number, isStatic: boolean) => void>(() => {})

  const size = useContainerSize(containerRef)
  const colors = useThemeColors()
  const font = useCanvasFont()
  const inView = useInView(containerRef)
  const dpr = getDpr()
  const active = inView && !reducedMotion

  const fg: RGB = parseHex(colors.foreground)
  const primary: RGB = parseHex(colors.primary)
  const isDark = colors.isDark

  const draw = (time: number, isStatic: boolean) => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const w = canvas.width
    const h = canvas.height
    ctx.clearRect(0, 0, w, h)

    const radius = Math.min(w, h) * 0.3
    const pt = pointer.current
    const baseDot = Math.max(1.1, 1.35 * dpr)
    const list = particles.current

    for (let i = 0; i < list.length; i++) {
      const p = list[i]
      const shimmer = isStatic ? 0.72 : 0.45 + 0.4 * Math.sin(time * 0.0016 + p.phase)
      let ignite = 0

      if (!isStatic && pt.active) {
        const dx = pt.x - p.hx
        const dy = pt.y - p.hy
        const dist = Math.hypot(dx, dy)
        if (dist < radius) {
          ignite = 1 - dist / radius
          p.vx += dx * ignite * 0.0012
          p.vy += dy * ignite * 0.0012
        }
      }

      if (!isStatic) {
        p.vx += (p.hx - p.x) * 0.02
        p.vy += (p.hy - p.y) * 0.02
        p.vx *= 0.86
        p.vy *= 0.86
        p.x += p.vx
        p.y += p.vy
      } else {
        p.x = p.hx
        p.y = p.hy
      }

      const brightness = clamp(shimmer + ignite * 0.8, 0, 1)
      const color = mix(fg, primary, ignite)
      const r = baseDot * (0.7 + 0.7 * brightness) * (1 + ignite * 0.6)

      if (isDark && ignite > 0.05) {
        ctx.fillStyle = rgba(color, 0.16 * ignite)
        ctx.beginPath()
        ctx.arc(p.x, p.y, r * 3.4, 0, Math.PI * 2)
        ctx.fill()
      }

      ctx.fillStyle = rgba(color, isDark ? 0.32 + 0.68 * brightness : 0.5 + 0.5 * brightness)
      ctx.beginPath()
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
      ctx.fill()
    }
  }

  // Keep the latest draw closure in a ref so effects/rAF never go stale —
  // assigned in an effect, not during render (react-hooks/refs).
  useEffect(() => {
    drawRef.current = draw
  })

  // Build (or rebuild) the point field whenever size / font / dpr change.
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !size.width || !size.height) return
    const w = size.width * dpr
    const h = size.height * dpr
    canvas.width = w
    canvas.height = h
    canvas.style.width = `${size.width}px`
    canvas.style.height = `${size.height}px`
    const { points } = sampleWordmark(w, h, {
      lines: WORDMARK_LINES,
      fontFamily: font,
      gapDev: 7 * dpr,
    })
    particles.current = points.map((p) => ({
      hx: p.x,
      hy: p.y,
      x: p.x,
      y: p.y,
      vx: 0,
      vy: 0,
      phase: Math.random() * Math.PI * 2,
    }))
    drawRef.current(0, true)
  }, [size.width, size.height, font, dpr])

  // Repaint a static frame when paused (out of view / reduced motion / theme flip).
  useEffect(() => {
    if (!active) drawRef.current(0, true)
  }, [active, colors.foreground, colors.primary, isDark, size.width, size.height, font])

  useAnimationFrame((time) => drawRef.current(time, false), active)

  return (
    <div
      ref={containerRef}
      className="relative h-full w-full cursor-crosshair touch-none"
      onPointerMove={(e) => {
        const rect = e.currentTarget.getBoundingClientRect()
        pointer.current = {
          x: (e.clientX - rect.left) * dpr,
          y: (e.clientY - rect.top) * dpr,
          active: true,
        }
      }}
      onPointerLeave={() => {
        pointer.current.active = false
      }}
    >
      <canvas ref={canvasRef} className="block h-full w-full" />
    </div>
  )
}
