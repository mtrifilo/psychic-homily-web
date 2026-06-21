'use client'

/**
 * Effect E — "Dust Dissolve".
 *
 * The wordmark is a cloud of particles that assembles on load, drifts at rest,
 * and SCATTERS into dust where the cursor passes (rising slightly, like embers)
 * before reforming. Distinct from the Scrying Grid: those cells stay put and
 * brighten; these physically blow apart and re-coalesce. This Canvas2D version
 * APPROXIMATES the production path — Three.js WebGPU + TSL dissolving MSDF text
 * (Codrops "Gommage", Jan 2026) — which is bleeding-edge and needs a fallback.
 */

import { useEffect, useRef } from 'react'
import { clamp, getDpr, mix, parseHex, rgba, sampleWordmark, type RGB } from '../_lib/canvas'
import {
  useAnimationFrame,
  useCanvasFont,
  useContainerSize,
  useInView,
  useThemeColors,
  type PointerSample,
} from '../_lib/hooks'
import { WORDMARK_LINES } from '../_lib/constants'

interface Mote {
  hx: number
  hy: number
  x: number
  y: number
  vx: number
  vy: number
  r: number
  seed: number
}

export function DustDissolve({ reducedMotion }: { reducedMotion: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const motes = useRef<Mote[]>([])
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

    const pt = pointer.current
    const radius = Math.min(w, h) * 0.26
    const list = motes.current

    for (let i = 0; i < list.length; i++) {
      const m = list[i]

      if (isStatic) {
        m.x = m.hx
        m.y = m.hy
        ctx.fillStyle = rgba(fg, isDark ? 0.85 : 0.82)
        ctx.beginPath()
        ctx.arc(m.x, m.y, m.r, 0, Math.PI * 2)
        ctx.fill()
        continue
      }

      let fx = (m.hx - m.x) * 0.012
      let fy = (m.hy - m.y) * 0.012
      // Idle turbulence so the cloud breathes.
      fx += Math.sin(time * 0.001 + m.seed) * 0.02
      fy += Math.cos(time * 0.0012 + m.seed * 1.3) * 0.02

      let dust = 0
      if (pt.active) {
        const dx = m.x - pt.x
        const dy = m.y - pt.y
        const dist = Math.hypot(dx, dy) || 1
        if (dist < radius) {
          dust = 1 - dist / radius
          const force = dust * dust * 1.4
          fx += (dx / dist) * force
          fy += (dy / dist) * force - dust * 0.35 // bias upward, like embers
        }
      }

      m.vx = (m.vx + fx) * 0.9
      m.vy = (m.vy + fy) * 0.9
      m.x += m.vx
      m.y += m.vy

      const displacement = Math.hypot(m.x - m.hx, m.y - m.hy)
      const alpha = clamp(1 - displacement / (radius * 1.1), 0.12, 1)
      const color = mix(fg, primary, Math.min(1, dust * 1.6))
      ctx.fillStyle = rgba(color, alpha * (isDark ? 0.9 : 0.82))
      ctx.beginPath()
      ctx.arc(m.x, m.y, m.r, 0, Math.PI * 2)
      ctx.fill()
    }
  }

  // Keep the latest draw closure in a ref so effects/rAF never go stale —
  // assigned in an effect, not during render (react-hooks/refs).
  useEffect(() => {
    drawRef.current = draw
  })

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !size.width || !size.height) return
    const w = size.width * dpr
    const h = size.height * dpr
    canvas.width = w
    canvas.height = h
    canvas.style.width = `${size.width}px`
    canvas.style.height = `${size.height}px`
    const baseDot = Math.max(1, 1.1 * dpr)
    const { points } = sampleWordmark(w, h, {
      lines: WORDMARK_LINES,
      fontFamily: font,
      gapDev: 7 * dpr,
    })
    motes.current = points.map((p) => ({
      hx: p.x,
      hy: p.y,
      // Start scattered so the cloud visibly assembles into the wordmark.
      x: p.x + (Math.random() - 0.5) * w * 0.5,
      y: p.y + (Math.random() - 0.5) * h * 0.7,
      vx: 0,
      vy: 0,
      r: baseDot * (0.6 + Math.random() * 0.9),
      seed: Math.random() * Math.PI * 2,
    }))
    drawRef.current(0, true)
  }, [size.width, size.height, font, dpr])

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
