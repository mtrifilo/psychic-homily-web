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
 * Progressive enhancement + a11y: the text "Psychic Homily" is always in the
 * DOM. With `heading` (the default) it is a real <h1>: the SSR paint (the LCP
 * element), the no-JS fallback, and in the a11y tree + DOM (SEO) after the
 * canvas takes over. Without it the container is an image named "Psychic
 * Homily", for a page whose h1 is elsewhere. On prefers-reduced-motion the
 * field is drawn once, static, with no animation or pointer reaction.
 */

import { Fragment, useEffect, useRef, useState } from 'react'
import { useReducedMotion } from '@/lib/hooks/common/useReducedMotion'
import { cn } from '@/lib/utils'
import { readWordmarkColors, resolveFontFamily, rgba, sampleWordmark, type RGB } from './sampleWordmark'

const DEFAULT_LINES: readonly string[] = ['PSYCHIC', 'HOMILY']
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'
const ACCESSIBLE_NAME = 'Psychic Homily'

/** A cell's brightness in the resting frame (0..1). */
const REST_BRIGHTNESS = 0.72
/** `settle`: length of the arrival intro. At most 1.5 s by design, and far
 *  under WCAG 2.2.2's five seconds, so it needs no pause control. */
export const SETTLE_INTRO_MS = 1200
/** `settle`: the longest the loop runs after the pointer leaves before it
 *  snaps to the resting frame and stops. */
export const SETTLE_RELEASE_MS = 1000
/** `settle`: the intro's opening opacity. The field rises from faint cells and
 *  only ever brightens: a fade, never a flash (WCAG 2.3.1; the nameplate is
 *  over the flash-area threshold). */
const INTRO_FLOOR = 0.12

/** A fixed-size layout of the field, tuned on /admin/hero-lab against the
 *  Product Designs components (bar mark 2240:3336, nameplate 2240:3359). */
export interface WordmarkPreset {
  /** Sampling pitch in CSS px. */
  gapFactor: number
  /** Dot radius in CSS px before the shimmer and ignite scaling; a resting
   *  dot is about 0.6 x the pitch across, as on the boards. */
  dotRadius: number
  /** Glyph weight sampled (the boards set Hanken Grotesk Black). */
  fontWeight: number
  /** Share of the box the letters' ink fills, centred (see `fillInk` in
   *  `sampleWordmark`). Under 1 so the edge cells are not cut in half. */
  inkFill: number
  /** Pointer reach as a multiple of the container height. */
  igniteReach: number
  /** Dark-mode bloom pass. */
  bloom: boolean
  /** Size of the text fallback (no JS, or the canvas could not sample). */
  fallbackClassName: string
}

/**
 * - `bar`: one line in a 306x30 box, a 28px cap. Pitch 3px (about 9 to 10
 *   cells per letter height). No bloom: at a 3px pitch the halo fills the
 *   gaps between cells and blurs the dot texture, adding no visible glow.
 * - `nameplate`: one line in a 1084x100 box, a 100px cap. Pitch 4px. Bloom
 *   in dark mode.
 *
 * Measured on /admin/hero-lab at dpr 2: the bar's cells span about 298x26
 * and the nameplate's about 1062x95, each centred, at exactly the pitch.
 */
export const WORDMARK_PRESETS = {
  bar: {
    gapFactor: 3,
    dotRadius: 0.75,
    fontWeight: 900,
    inkFill: 0.98,
    igniteReach: 1.2,
    bloom: false,
    fallbackClassName: 'whitespace-nowrap text-[22px] font-black leading-none',
  },
  nameplate: {
    gapFactor: 4,
    dotRadius: 1,
    fontWeight: 900,
    inkFill: 0.98,
    igniteReach: 1.2,
    bloom: true,
    fallbackClassName: 'whitespace-nowrap text-[88px] font-black leading-none',
  },
} as const satisfies Record<string, WordmarkPreset>

export type WordmarkPresetName = keyof typeof WORDMARK_PRESETS

const HERO_FALLBACK_CLASS =
  'text-[clamp(3.25rem,12vw,10rem)] font-bold leading-[0.9] tracking-tight'

function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = Math.max(0, Math.min(1, (x - edge0) / (edge1 - edge0)))
  return t * t * (3 - 2 * t)
}

interface Particle {
  hx: number
  hy: number
  x: number
  y: number
  vx: number
  vy: number
  phase: number
}

export function ScryingGridWordmark({
  className,
  headingId,
  lines = DEFAULT_LINES,
  heading = true,
  motion = 'rest',
  preset,
  gapFactor = preset ? WORDMARK_PRESETS[preset].gapFactor : 6,
  spotlight = 'cells',
}: {
  className?: string
  /** id for the rendered <h1> — the owning page sets this when a section labels itself by it. */
  headingId?: string
  /** One entry per line of the wordmark. */
  lines?: readonly string[]
  /** When false no <h1> renders: the container is an image named "Psychic
   *  Homily", for a page whose h1 is elsewhere. */
  heading?: boolean
  /**
   * - `rest` (default): shimmers continuously while on screen.
   * - `settle`: one intro of SETTLE_INTRO_MS on first visibility, then the
   *   loop STOPS on the resting frame. The pointer (hover, touch) ignites the
   *   cells while it is over the field; within SETTLE_RELEASE_MS of leaving,
   *   the field is back at rest and the loop stops again. Paused off-screen
   *   and in a hidden tab. Nothing restarts on its own.
   */
  motion?: 'rest' | 'settle'
  /** A fixed-size layout (see WORDMARK_PRESETS). The container's size comes
   *  from `className` and must match the preset's box. */
  preset?: WordmarkPresetName
  /** Sampling pitch in CSS px (× dpr) — smaller = denser dot field, cleaner letterforms. */
  gapFactor?: number
  /**
   * Cursor-glow treatment (dark mode). Production (`HomeHero`) uses `cells`;
   * `pool`/`oversized` exist for the /admin/hero-lab comparison:
   * - `cells`     — no pool; only the dots ignite/lean (the chosen production mode).
   * - `pool`      — radial pool drawn on the wordmark canvas (clips at canvas edges).
   * - `oversized` — same pool, but the wordmark is inset so the pool fits over the letters.
   */
  spotlight?: 'pool' | 'oversized' | 'cells'
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [enhanced, setEnhanced] = useState(false)
  // The visible fallback is hidden for JS users so the solid-font wordmark
  // never flashes before the canvas fades in. It's only revealed if enhancement
  // fails (`failed`), or for no-JS via the <noscript> style below.
  const [failed, setFailed] = useState(false)
  const reducedMotion = useReducedMotion()
  // A string, so a caller's inline array literal does not rebuild the field
  // on every render.
  const linesKey = lines.join('\n')

  useEffect(() => {
    const container = containerRef.current
    const canvas = canvasRef.current
    if (!container || !canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    // The live query as well as the hook: during hydration the hook still
    // answers its server value (false), and this effect runs once with it.
    const reduced =
      reducedMotion || window.matchMedia(REDUCED_MOTION_QUERY).matches
    const layout = preset ? WORDMARK_PRESETS[preset] : null
    const settle = motion === 'settle'
    const bloomOn = layout ? layout.bloom : true
    const baseDot = layout ? layout.dotRadius * dpr : Math.max(1.1, 1.35 * dpr)
    const pointer = { x: 0, y: 0, active: false }
    // settle state. The intro's clock advances by frame timestamps, so a
    // pause (off-screen, hidden tab) resumes it where it stopped.
    let introElapsed = settle && !reduced ? 0 : SETTLE_INTRO_MS
    let lastFrameAt: number | null = null
    let releasing = false
    let releaseStart: number | null = null
    let inView = true
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
      if (isDark && !isStatic && pt.active && spotlight !== 'cells') {
        const sx = pt.x
        const sy = pt.y
        // Pool radius is capped so it stays contained on the large wordmark.
        const sr = Math.min(radius * 1.2, 150 * dpr)
        // Edge feather: the pool is a circle, the canvas is a rectangle, so a
        // pool that overruns an edge gets sliced into a hard straight line. Fade
        // the whole pool to nothing as the cursor approaches any boundary so the
        // cutoff is always soft. (Distance to nearest edge, normalized by sr.)
        const edgeFade = Math.max(0, Math.min(1, Math.min(sx, sy, w - sx, h - sy) / sr))
        if (edgeFade > 0.002) {
          ctx.globalCompositeOperation = 'lighter'
          const halo = ctx.createRadialGradient(sx, sy, 0, sx, sy, sr)
          halo.addColorStop(0, rgba(primary, 0.13 * edgeFade))
          halo.addColorStop(0.3, rgba(primary, 0.06 * edgeFade))
          halo.addColorStop(0.65, rgba(primary, 0.02 * edgeFade))
          halo.addColorStop(1, rgba(primary, 0))
          ctx.fillStyle = halo
          ctx.beginPath()
          ctx.arc(sx, sy, sr, 0, Math.PI * 2)
          ctx.fill()
          const cr = sr * 0.42
          const core = ctx.createRadialGradient(sx, sy, 0, sx, sy, cr)
          core.addColorStop(0, rgba(HOT, 0.24 * edgeFade))
          core.addColorStop(0.4, rgba(primary, 0.12 * edgeFade))
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
      }
      // settle: the intro rises from INTRO_FLOOR to full opacity over its
      // first 60% while the shimmer converges on the resting value, so its
      // last frame IS the resting frame and stopping changes no pixel.
      const introT = introElapsed / SETTLE_INTRO_MS
      const reveal = settle ? INTRO_FLOOR + (1 - INTRO_FLOOR) * smoothstep(0, 0.6, introT) : 1
      const settleMix = settle ? smoothstep(0.35, 1, introT) : 0
      for (let i = 0; i < particles.length; i++) {
        const p = particles[i]
        let shimmer = REST_BRIGHTNESS
        if (!isStatic) {
          const live = 0.45 + 0.4 * Math.sin(time * 0.0016 + p.phase)
          shimmer = live + (REST_BRIGHTNESS - live) * settleMix
        }
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
        // Inline the fg→primary lerp + rgba — avoids a per-particle array
        // allocation (and helper call) in this hot loop (~N particles × 60fps).
        const cr = Math.round(fg[0] + (primary[0] - fg[0]) * ignite)
        const cg = Math.round(fg[1] + (primary[1] - fg[1]) * ignite)
        const cb = Math.round(fg[2] + (primary[2] - fg[2]) * ignite)
        const alpha = (isDark ? 0.32 + 0.68 * bright : 0.5 + 0.5 * bright) * reveal
        const r = baseDot * (0.7 + 0.7 * bright) * (1 + ignite * 0.6)
        ctx.fillStyle = `rgba(${cr}, ${cg}, ${cb}, ${alpha})`
        ctx.beginPath()
        ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
        ctx.fill()
      }

      // Subtle halo: downscale the scene, square it for a cheap bright-pass, then
      // additively draw it back blurred — each lit dot gets a soft glow without
      // the heavy bloom washing out the letterforms. Dark mode only (light mode
      // would wash out). Tuned restrained per the chosen "subtle halo" direction.
      // In settle mode the resting frame is the designed frame, halo included.
      if (isDark && bloomOn && (settle || !isStatic) && supportsFilter && bloomCtx && bloomCanvas.width > 1) {
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
        ctx.globalAlpha = 0.32
        ctx.filter = `blur(${Math.round(4 * dpr)}px)`
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
      radius = layout ? h * layout.igniteReach : Math.min(w, h) * 0.3
      bloomCanvas.width = Math.max(1, Math.round(w / 3))
      bloomCanvas.height = Math.max(1, Math.round(h / 3))
      const points = sampleWordmark(w, h, {
        lines: linesKey.split('\n'),
        fontFamily: resolveFontFamily(),
        gapDev: Math.round(gapFactor * dpr),
        // Oversized: inset the wordmark so the pool has room to fully fade
        // inside the canvas instead of getting sliced at the edge.
        padScale: spotlight === 'oversized' ? 0.6 : (layout?.inkFill ?? 1),
        fillInk: !!layout,
        fontWeight: layout?.fontWeight,
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

    // settle: every particle home and still, so the static frame is the one
    // the next animated frame would have drawn.
    const isAtRest = () => {
      for (let i = 0; i < particles.length; i++) {
        const p = particles[i]
        if (Math.abs(p.x - p.hx) > 0.1 || Math.abs(p.y - p.hy) > 0.1) return false
        if (Math.abs(p.vx) > 0.05 || Math.abs(p.vy) > 0.05) return false
      }
      return true
    }
    const needsFrames = () =>
      !settle || introElapsed < SETTLE_INTRO_MS || pointer.active || releasing

    const frame = (now: number) => {
      if (disposed || !running) return
      if (settle) {
        // The first frame after a (re)start adds no time, so a pause is never
        // billed to the intro.
        introElapsed = Math.min(
          SETTLE_INTRO_MS,
          introElapsed + (lastFrameAt === null ? 0 : now - lastFrameAt),
        )
        lastFrameAt = now
        if (releasing && !pointer.active) {
          releaseStart ??= now
          if (now - releaseStart >= SETTLE_RELEASE_MS || isAtRest()) releasing = false
        }
        if (!needsFrames()) {
          stopLoop()
          for (const p of particles) {
            p.vx = 0
            p.vy = 0
          }
          draw(now, true)
          return
        }
      }
      draw(now, false)
      raf = requestAnimationFrame(frame)
    }
    const startLoop = () => {
      if (running || disposed || reduced || particles.length === 0 || !needsFrames()) return
      running = true
      lastFrameAt = null
      raf = requestAnimationFrame(frame)
    }
    const stopLoop = () => {
      running = false
      cancelAnimationFrame(raf)
    }
    // settle runs frames only on screen and in a visible tab; rest is gated by
    // the IntersectionObserver alone.
    const resume = () => {
      if (!settle || (inView && !document.hidden)) startLoop()
    }

    const begin = () => {
      if (disposed) return
      if (!rebuild()) {
        // Canvas couldn't sample (e.g. zero-size container) — reveal the text.
        setFailed(true)
        return
      }
      setEnhanced(true)
      if (!reduced) resume()
      if (!running) draw(0, true)
    }
    // Fonts must be loaded before sampling, or we sample fallback glyphs.
    if (document.fonts?.ready) document.fonts.ready.then(begin).catch(begin)
    else begin()

    const onMove = (e: PointerEvent) => {
      const rect = container.getBoundingClientRect()
      pointer.x = (e.clientX - rect.left) * dpr
      pointer.y = (e.clientY - rect.top) * dpr
      pointer.active = true
      if (settle) {
        releasing = false
        releaseStart = null
        resume()
      }
    }
    const onLeave = () => {
      pointer.active = false
      if (settle) {
        releasing = true
        releaseStart = null
      }
    }
    if (!reduced) {
      container.addEventListener('pointermove', onMove)
      container.addEventListener('pointerleave', onLeave)
      // A tap ignites without a move; a scroll that takes the touch over
      // cancels it.
      if (settle) {
        container.addEventListener('pointerdown', onMove)
        container.addEventListener('pointercancel', onLeave)
      }
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
        // A container with no size at mount (display:none at this breakpoint)
        // took the text fallback; the canvas replaces it now.
        setFailed(false)
        if (!running) {
          draw(0, true)
          if (settle && !reduced) resume()
        }
      })
    })
    ro.observe(container)

    // Pause the loop when the wordmark scrolls out of view.
    const io = new IntersectionObserver(
      ([entry]) => {
        inView = entry.isIntersecting
        if (inView) resume()
        else stopLoop()
      },
      { rootMargin: '120px' },
    )
    io.observe(container)

    const onVisibility = () => (document.hidden ? stopLoop() : resume())
    if (settle) document.addEventListener('visibilitychange', onVisibility)

    return () => {
      disposed = true
      stopLoop()
      cancelAnimationFrame(resizeRaf)
      container.removeEventListener('pointermove', onMove)
      container.removeEventListener('pointerleave', onLeave)
      container.removeEventListener('pointerdown', onMove)
      container.removeEventListener('pointercancel', onLeave)
      document.removeEventListener('visibilitychange', onVisibility)
      themeObs.disconnect()
      ro.disconnect()
      io.disconnect()
    }
  }, [gapFactor, spotlight, reducedMotion, linesKey, motion, preset])

  // Visible fallback, hidden by default so it never flashes before the
  // canvas; revealed on enhancement failure, or for no-JS (noscript).
  const fallback = (
    <span
      aria-hidden
      data-hero-fallback
      className={`block font-display text-foreground transition-opacity duration-700 ${
        preset ? WORDMARK_PRESETS[preset].fallbackClassName : HERO_FALLBACK_CLASS
      } ${failed ? 'opacity-100' : 'opacity-0'}`}
    >
      {linesKey.split('\n').map((line, index) => (
        <Fragment key={index}>
          {index > 0 && <br />}
          {line}
        </Fragment>
      ))}
    </span>
  )

  return (
    <div
      ref={containerRef}
      role={heading ? undefined : 'img'}
      aria-label={heading ? undefined : ACCESSIBLE_NAME}
      className={cn('relative isolate flex w-full items-center justify-center select-none', className)}
      style={{ touchAction: 'pan-y' }}
    >
      {heading ? (
        <h1 id={headingId} className="m-0 text-center">
          <span className="sr-only">{ACCESSIBLE_NAME}</span>
          {fallback}
        </h1>
      ) : (
        <span className="m-0 text-center">{fallback}</span>
      )}
      <canvas
        ref={canvasRef}
        aria-hidden
        className={`pointer-events-none absolute inset-0 h-full w-full transition-opacity duration-700 ${
          enhanced ? 'opacity-100' : 'opacity-0'
        }`}
      />
      {/* No-JS: the canvas never enhances, so reveal the text fallback. */}
      <noscript>
        <style>{`[data-hero-fallback]{opacity:1!important}`}</style>
      </noscript>
    </div>
  )
}
