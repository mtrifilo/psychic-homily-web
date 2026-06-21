'use client'

/**
 * Effect B — "Mirage Haze".
 *
 * The real wordmark, rendered crisp, then displaced per scanline by a
 * sinusoidal field so it shimmers like desert air / scrying water — the literal
 * payoff of the site's "This is not a mirage." line. The cursor warms the haze
 * locally (amplitude swells around the pointer's vertical band). The text stays
 * fully legible, which is the point. Production path: VFX-JS (shader effects on
 * real DOM text) or a small OGL fragment-shader pass; lbebber/HeatDistortionEffect
 * is the canonical WebGL version.
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

export function MirageHaze({ reducedMotion }: { reducedMotion: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const sourceRef = useRef<HTMLCanvasElement | null>(null)
  const pointer = useRef<PointerSample>({ x: 0, y: 0, active: false })
  const drawRef = useRef<(time: number, isStatic: boolean) => void>(() => {})

  const size = useContainerSize(containerRef)
  const colors = useThemeColors()
  const font = useCanvasFont()
  const inView = useInView(containerRef)
  const dpr = getDpr()
  const active = inView && !reducedMotion

  const primary = parseHex(colors.primary)

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
    const baseAmp = h * 0.012
    const strip = Math.max(1, Math.round(2 * dpr))
    const sigma = h * 0.16

    for (let y = 0; y < h; y += strip) {
      const boost = pt.active ? gauss(y - pt.y, sigma) : 0
      const amp = baseAmp * (0.55 + 1.9 * boost)
      const dx = amp * Math.sin(y * 0.018 + time * 0.0042) + amp * 0.5 * Math.sin(y * 0.05 - time * 0.0026)
      ctx.drawImage(src, 0, y, w, strip, dx, y, w, strip)
    }

    // A whisper of warm chromatic shimmer that follows the cursor band.
    if (pt.active) {
      ctx.globalAlpha = 0.12
      ctx.globalCompositeOperation = 'source-atop'
      ctx.fillStyle = rgba(primary, 1)
      const band = h * 0.22
      ctx.fillRect(0, pt.y - band, w, band * 2)
      ctx.globalAlpha = 1
      ctx.globalCompositeOperation = 'source-over'
    }
  }

  // Keep the latest draw closure in a ref so effects/rAF never go stale —
  // assigned in an effect, not during render (react-hooks/refs).
  useEffect(() => {
    drawRef.current = draw
  })

  // Rebuild the crisp source image on size / font / color changes.
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
    >
      <canvas ref={canvasRef} className="block h-full w-full" />
    </div>
  )
}
