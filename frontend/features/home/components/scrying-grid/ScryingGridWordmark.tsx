'use client'

/**
 * Scrying Grid hero wordmark (PSY-1137).
 *
 * Renders "PSYCHIC HOMILY" as a field of light-cells that shimmer at rest and
 * ignite + lean toward the cursor — the Andi Watson "fill in the gaps" model
 * (see docs/features/hero-wordmark-animation.md). This is the Canvas2D
 * implementation reviewed + approved on /admin/hero-lab, promoted to production:
 * same sampling, physics, and draw, wrapped with the production concerns below.
 *
 * Progressive enhancement + a11y: a real <h1> "Psychic Homily" is always in the
 * DOM. It is the SSR paint (the LCP element), the no-JS fallback, and stays in
 * the a11y tree + DOM (SEO) after the canvas takes over. On prefers-reduced-
 * motion the field is drawn once, static, with no animation or pointer reaction.
 */

import { useEffect, useRef, useState } from 'react'
import { mix, readWordmarkColors, resolveFontFamily, rgba, sampleWordmark, type RGB } from './sampleWordmark'

const LINES = ['PSYCHIC', 'HOMILY'] as const

interface Particle {
  hx: number
  hy: number
  x: number
  y: number
  vx: number
  vy: number
  phase: number
}

export function ScryingGridWordmark({ className }: { className?: string }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [enhanced, setEnhanced] = useState(false)

  useEffect(() => {
    const container = containerRef.current
    const canvas = canvasRef.current
    if (!container || !canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const baseDot = Math.max(1.1, 1.35 * dpr)
    const pointer = { x: 0, y: 0, active: false }
    let particles: Particle[] = []
    let { foreground: fg, primary, isDark } = readWordmarkColors()
    let radius = 1
    let running = false
    let raf = 0
    let resizeRaf = 0
    let disposed = false
    const HOT: RGB = [255, 244, 224]
    // Pre-rendered grain tile to break gradient banding + add organic texture
    // (the trick behind the Vercel-style glow — noise composited over the pool).
    const noiseTile = document.createElement('canvas')
    noiseTile.width = 96
    noiseTile.height = 96
    const noiseCtx = noiseTile.getContext('2d')
    if (noiseCtx) {
      const grain = noiseCtx.createImageData(96, 96)
      for (let i = 0; i < grain.data.length; i += 4) {
        const v = 110 + Math.floor(Math.random() * 145)
        grain.data[i] = v
        grain.data[i + 1] = v
        grain.data[i + 2] = v
        grain.data[i + 3] = 255
      }
      noiseCtx.putImageData(grain, 0, 0)
    }
    // Offscreen for the HDR-style bloom pass (downscale → bright-pass → additive
    // blurred upscale). Sized to a fraction of the scene in rebuild().
    const bloomCanvas = document.createElement('canvas')
    const bloomCtx = bloomCanvas.getContext('2d')
    const supportsFilter = !!bloomCtx && 'filter' in bloomCtx

    const draw = (time: number, isStatic: boolean) => {
      const w = canvas.width
      const h = canvas.height
      ctx.clearRect(0, 0, w, h)
      const pt = pointer
      // Dark-mode hover: a layered, grain-textured spotlight over the bulging
      // dots — warm-white hot core → terracotta → soft tail, additive, with a
      // grain pass that kills banding. Tracks the cursor directly (no lag).
      if (isDark && !isStatic && pt.active) {
        const sx = pt.x
        const sy = pt.y
        const sr = radius * 1.25
        ctx.globalCompositeOperation = 'lighter'
        const halo = ctx.createRadialGradient(sx, sy, 0, sx, sy, sr)
        halo.addColorStop(0, rgba(primary, 0.13))
        halo.addColorStop(0.3, rgba(primary, 0.06))
        halo.addColorStop(0.65, rgba(primary, 0.02))
        halo.addColorStop(1, rgba(primary, 0))
        ctx.fillStyle = halo
        ctx.beginPath()
        ctx.arc(sx, sy, sr, 0, Math.PI * 2)
        ctx.fill()
        const cr = sr * 0.42
        const core = ctx.createRadialGradient(sx, sy, 0, sx, sy, cr)
        core.addColorStop(0, rgba(HOT, 0.24))
        core.addColorStop(0.4, rgba(primary, 0.12))
        core.addColorStop(1, rgba(primary, 0))
        ctx.fillStyle = core
        ctx.beginPath()
        ctx.arc(sx, sy, cr, 0, Math.PI * 2)
        ctx.fill()
        // Grain pass — source-atop textures only the lit pool (not the empty
        // background), breaking the radial-gradient banding.
        if (noiseCtx) {
          const pattern = ctx.createPattern(noiseTile, 'repeat')
          if (pattern) {
            ctx.globalCompositeOperation = 'source-atop'
            ctx.globalAlpha = 0.07
            ctx.fillStyle = pattern
            ctx.fillRect(sx - sr, sy - sr, sr * 2, sr * 2)
            ctx.globalAlpha = 1
          }
        }
        ctx.globalCompositeOperation = 'source-over'
      }
      for (let i = 0; i < particles.length; i++) {
        const p = particles[i]
        const shimmer = isStatic ? 0.72 : 0.45 + 0.4 * Math.sin(time * 0.0016 + p.phase)
        let ignite = 0
        if (!isStatic && pt.active) {
          const dx = pt.x - p.hx
          const dy = pt.y - p.hy
          const d = Math.hypot(dx, dy)
          if (d < radius) {
            ignite = 1 - d / radius
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
        const bright = Math.max(0, Math.min(1, shimmer + ignite * 0.8))
        const col = mix(fg, primary, ignite)
        const r = baseDot * (0.7 + 0.7 * bright) * (1 + ignite * 0.6)
        ctx.fillStyle = rgba(col, isDark ? 0.32 + 0.68 * bright : 0.5 + 0.5 * bright)
        ctx.beginPath()
        ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
        ctx.fill()
      }

      // HDR-style bloom: downscale the scene, square it for a cheap bright-pass,
      // then additively draw it back blurred — the spotlight + ignited dots bleed
      // a luminous glow. Dark mode only (light mode would wash out).
      if (isDark && !isStatic && supportsFilter && bloomCtx && bloomCanvas.width > 1) {
        const bw = bloomCanvas.width
        const bh = bloomCanvas.height
        bloomCtx.globalCompositeOperation = 'source-over'
        bloomCtx.clearRect(0, 0, bw, bh)
        bloomCtx.drawImage(canvas, 0, 0, bw, bh)
        bloomCtx.globalCompositeOperation = 'multiply'
        bloomCtx.drawImage(bloomCanvas, 0, 0)
        bloomCtx.globalCompositeOperation = 'source-over'
        ctx.save()
        ctx.globalCompositeOperation = 'lighter'
        ctx.globalAlpha = 0.85
        ctx.filter = `blur(${Math.round(7 * dpr)}px)`
        ctx.drawImage(bloomCanvas, 0, 0, bw, bh, 0, 0, canvas.width, canvas.height)
        ctx.restore()
      }
    }

    const rebuild = (): boolean => {
      const rect = container.getBoundingClientRect()
      if (rect.width < 8 || rect.height < 8) return false
      const w = Math.round(rect.width * dpr)
      const h = Math.round(rect.height * dpr)
      canvas.width = w
      canvas.height = h
      canvas.style.width = `${rect.width}px`
      canvas.style.height = `${rect.height}px`
      radius = Math.min(w, h) * 0.3
      bloomCanvas.width = Math.max(1, Math.round(w / 3))
      bloomCanvas.height = Math.max(1, Math.round(h / 3))
      const points = sampleWordmark(w, h, {
        lines: LINES,
        fontFamily: resolveFontFamily('--font-display'),
        gapDev: Math.round(6 * dpr),
      })
      if (points.length === 0) return false
      particles = points.map((p) => ({
        hx: p.x,
        hy: p.y,
        x: p.x,
        y: p.y,
        vx: 0,
        vy: 0,
        phase: Math.random() * Math.PI * 2,
      }))
      return true
    }

    const frame = (now: number) => {
      if (disposed || !running) return
      draw(now, false)
      raf = requestAnimationFrame(frame)
    }
    const startLoop = () => {
      if (running || disposed || reduced || particles.length === 0) return
      running = true
      raf = requestAnimationFrame(frame)
    }
    const stopLoop = () => {
      running = false
      cancelAnimationFrame(raf)
    }

    const begin = () => {
      if (disposed || !rebuild()) return
      setEnhanced(true)
      if (reduced) draw(0, true)
      else startLoop()
    }
    // Fonts must be loaded before sampling, or we sample fallback glyphs.
    if (document.fonts?.ready) document.fonts.ready.then(begin).catch(begin)
    else begin()

    const onMove = (e: PointerEvent) => {
      const rect = container.getBoundingClientRect()
      pointer.x = (e.clientX - rect.left) * dpr
      pointer.y = (e.clientY - rect.top) * dpr
      pointer.active = true
    }
    const onLeave = () => {
      pointer.active = false
    }
    if (!reduced) {
      container.addEventListener('pointermove', onMove)
      container.addEventListener('pointerleave', onLeave)
    }

    // Recolor on theme flip; the running loop picks it up next frame, otherwise redraw.
    const applyColors = () => {
      ;({ foreground: fg, primary, isDark } = readWordmarkColors())
      if (!running && particles.length > 0) draw(0, true)
    }
    const themeObs = new MutationObserver(applyColors)
    themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })

    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(resizeRaf)
      resizeRaf = requestAnimationFrame(() => {
        if (disposed || !rebuild()) return
        setEnhanced(true)
        if (!running) draw(0, true)
      })
    })
    ro.observe(container)

    // Pause the loop when the hero scrolls out of view.
    const io = new IntersectionObserver(
      ([entry]) => (entry.isIntersecting ? startLoop() : stopLoop()),
      { rootMargin: '120px' },
    )
    io.observe(container)

    return () => {
      disposed = true
      stopLoop()
      cancelAnimationFrame(resizeRaf)
      container.removeEventListener('pointermove', onMove)
      container.removeEventListener('pointerleave', onLeave)
      themeObs.disconnect()
      ro.disconnect()
      io.disconnect()
    }
  }, [])

  return (
    <div
      ref={containerRef}
      className={`relative isolate w-full select-none ${className ?? ''}`}
      style={{ touchAction: 'pan-y' }}
    >
      <h1
        id="home-hero-heading"
        className={`m-0 text-center transition-opacity duration-700 ${enhanced ? 'opacity-0' : 'opacity-100'}`}
      >
        <span className="sr-only">Psychic Homily</span>
        <span
          aria-hidden
          className="block font-display text-[clamp(2.75rem,10vw,7rem)] font-bold leading-[0.9] tracking-tight text-foreground"
        >
          PSYCHIC
          <br />
          HOMILY
        </span>
      </h1>
      <canvas
        ref={canvasRef}
        aria-hidden
        className={`pointer-events-none absolute inset-0 h-full w-full transition-opacity duration-700 ${
          enhanced ? 'opacity-100' : 'opacity-0'
        }`}
      />
    </div>
  )
}
