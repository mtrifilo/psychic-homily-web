'use client'

/**
 * Effect D — "Scrying Pool".
 *
 * The wordmark seen through refracting liquid glass: a slow two-direction wave
 * warp, a glassy loupe that follows the cursor, and concentric ripples on click
 * (a direct nod to the rippling-water motif in Watson's content library). This
 * Canvas2D version APPROXIMATES the look of a real fluid sim — the production
 * path is react-fluid-distortion (React Three Fiber, Pavel Dobryakov's
 * Navier-Stokes solver), which is heavier (~150kb Three.js) but truly fluid.
 */

import { useEffect, useRef } from 'react'
import { gauss, getDpr, makeWordmarkCanvas, parseHex, rgba } from '../_lib/canvas'
import {
  useAnimationFrame,
  useCanvasFont,
  useContainerSize,
  useInView,
  useThemeColors,
  type PointerSample,
} from '../_lib/hooks'
import { WORDMARK_LINES } from '../_lib/constants'

interface Ripple {
  x: number
  y: number
  born: number
}

export function ScryingPool({ reducedMotion }: { reducedMotion: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const sourceRef = useRef<HTMLCanvasElement | null>(null)
  const pointer = useRef<PointerSample>({ x: 0, y: 0, active: false })
  const ripples = useRef<Ripple[]>([])
  const drawRef = useRef<(time: number, isStatic: boolean) => void>(() => {})

  const size = useContainerSize(containerRef)
  const colors = useThemeColors()
  const font = useCanvasFont()
  const inView = useInView(containerRef)
  const dpr = getDpr()
  const active = inView && !reducedMotion

  const primary = parseHex(colors.primary)
  const accent = parseHex(colors.accent)

  const draw = (time: number, isStatic: boolean) => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    const src = sourceRef.current
    if (!canvas || !ctx || !src) return
    const w = canvas.width
    const h = canvas.height
    ctx.clearRect(0, 0, w, h)

    if (isStatic) {
      ctx.drawImage(src, 0, 0)
      return
    }

    const pt = pointer.current
    const amp = h * 0.02
    const strip = Math.max(1, Math.round(2 * dpr))
    ripples.current = ripples.current.filter((r) => time - r.born < 2200)

    for (let y = 0; y < h; y += strip) {
      let dx =
        amp * Math.sin(y * 0.016 + time * 0.0016) +
        amp * 0.6 * Math.sin(y * 0.045 - time * 0.0011)
      // Cursor loupe: vertical band swells like a lens.
      if (pt.active) dx += amp * 1.6 * gauss(y - pt.y, h * 0.14) * Math.sin(time * 0.006)
      // Expanding ripples nudge the strips they pass over.
      for (const r of ripples.current) {
        const age = time - r.born
        const ringRadius = (age / 1000) * h * 0.9
        const d = Math.abs(Math.abs(y - r.y) - ringRadius)
        if (d < h * 0.06) dx += amp * 2 * (1 - d / (h * 0.06)) * (1 - age / 2200) * Math.sign(y - r.y || 1)
      }
      ctx.drawImage(src, 0, y, w, strip, dx, y, w, strip)
    }

    // Glassy highlight following the cursor sells the "liquid lens".
    if (pt.active) {
      const lens = ctx.createRadialGradient(pt.x, pt.y, 0, pt.x, pt.y, h * 0.34)
      lens.addColorStop(0, rgba(colors.isDark ? primary : accent, colors.isDark ? 0.16 : 0.22))
      lens.addColorStop(1, rgba(primary, 0))
      ctx.fillStyle = lens
      ctx.fillRect(0, 0, w, h)
    }

    // Faint ripple rings.
    for (const r of ripples.current) {
      const age = time - r.born
      const ringRadius = (age / 1000) * h * 0.9
      ctx.strokeStyle = rgba(primary, 0.18 * (1 - age / 2200))
      ctx.lineWidth = Math.max(1, dpr)
      ctx.beginPath()
      ctx.arc(r.x, r.y, ringRadius, 0, Math.PI * 2)
      ctx.stroke()
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
    sourceRef.current = makeWordmarkCanvas(w, h, {
      lines: WORDMARK_LINES,
      fontFamily: font,
      color: colors.foreground,
    })
    drawRef.current(0, true)
  }, [size.width, size.height, font, colors.foreground, dpr])

  useEffect(() => {
    if (!active) drawRef.current(0, true)
  }, [active, colors.foreground, size.width, size.height, font])

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
      onPointerDown={(e) => {
        const rect = e.currentTarget.getBoundingClientRect()
        ripples.current.push({
          x: (e.clientX - rect.left) * dpr,
          y: (e.clientY - rect.top) * dpr,
          born: performance.now(),
        })
      }}
    >
      <canvas ref={canvasRef} className="block h-full w-full" />
    </div>
  )
}
