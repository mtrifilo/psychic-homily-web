'use client'

/**
 * Spotlight comparison (PSY-1137) — three ways to fix the hard rectangular
 * cutoff of the dark-mode cursor glow. All use the chosen Variant 1 density
 * (gapFactor=5). Hover each near its top/left/right edges to compare how the
 * glow behaves at a boundary. Admin-only dev tool (/admin/hero-lab/smoke, gated
 * by AdminGuard) — comparison scratch, not a user-facing surface. The A–E hero
 * gallery is the parent route, /admin/hero-lab.
 *
 *  1. Full-bleed glow layer — glow lives on a hero-spanning layer (here a CSS
 *     radial-gradient on the whole box), so it has no nearby edge to clip.
 *  2. Oversized canvas — the pool stays on the wordmark canvas, but the letters
 *     are inset so the pool fully fades inside the canvas over the letters.
 *  3. Cells only — no pool at all; only the dots ignite/lean toward the cursor.
 *
 * Switch to dark mode (nav toggle) to see the glow — it's dark-mode only.
 */

import { useEffect, useRef } from 'react'
import { ScryingGridWordmark } from '@/features/home/components/scrying-grid/ScryingGridWordmark'
import { RadialGodRays } from './RadialGodRays'
import { VolumetricSmoke } from './VolumetricSmoke'

const BOX =
  'relative isolate flex min-h-[520px] items-center justify-center overflow-hidden rounded-lg border border-border/40 bg-background'

/** Option 1 — pool on a full-bleed CSS layer spanning the whole box. */
function FullBleedMock() {
  const ref = useRef<HTMLDivElement>(null)
  const onMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const el = ref.current
    if (!el) return
    const r = el.getBoundingClientRect()
    el.style.setProperty('--mx', `${e.clientX - r.left}px`)
    el.style.setProperty('--my', `${e.clientY - r.top}px`)
    el.style.setProperty('--glow', '1')
  }
  const onLeave = () => ref.current?.style.setProperty('--glow', '0')
  return (
    <div
      ref={ref}
      onPointerMove={onMove}
      onPointerLeave={onLeave}
      className={BOX}
      style={{ '--mx': '50%', '--my': '50%', '--glow': '0' } as React.CSSProperties}
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 z-20"
        style={
          {
            background:
              'radial-gradient(circle 200px at var(--mx) var(--my), rgba(232,153,96,0.20), rgba(232,153,96,0.06) 40%, transparent 72%)',
            opacity: 'var(--glow)',
            mixBlendMode: 'screen',
            transition: 'opacity 220ms ease-out',
          } as React.CSSProperties
        }
      />
      <ScryingGridWordmark spotlight="cells" gapFactor={5} className="h-[260px] w-full max-w-[720px]" />
    </div>
  )
}

/**
 * Option 3 + photoreal stage smoke. The "smoke machine on a stage" look: real
 * smoke footage (white smoke on black) composited with `mix-blend-mode: screen`
 * so the black drops out and only the haze drifts through the lit wordmark.
 * Footage: Mixkit free license (no attribution). A slow drift behind the
 * letters; a fainter layer in front for depth (haze catching the lights).
 */
function SmokeStageMock() {
  const back = useRef<HTMLVideoElement>(null)
  const front = useRef<HTMLVideoElement>(null)
  // Slow the footage right down so the haze drifts lazily (subtle stage smoke,
  // not a fast-rolling plume). playbackRate has no HTML attribute — set it live.
  useEffect(() => {
    // Back layer uses an ffmpeg-interpolated pre-slowed clip (setpts=2.2 +
    // minterpolate=fps=30), so it's slow AND smooth at 1× — no choppiness.
    if (back.current) back.current.playbackRate = 1
    if (front.current) front.current.playbackRate = 0.55
  }, [])
  return (
    <div className={`${BOX} bg-black`}>
      {/* Atmospheric haze behind the lit wordmark — kept subtle so the lights
          dominate (a stage with a little haze, not a smoke-filled room). */}
      <video
        ref={back}
        aria-hidden
        className="pointer-events-none absolute inset-0 z-0 h-full w-full object-cover"
        style={{ mixBlendMode: 'screen', opacity: 0.3 }}
        src="/hero-lab/smoke-white-slow.mp4"
        autoPlay
        loop
        muted
        playsInline
      />
      <ScryingGridWordmark
        spotlight="cells"
        gapFactor={5}
        className="relative z-10 h-[260px] w-full max-w-[720px]"
      />
      {/* A whisper of haze drifting in front of the lights for depth. */}
      <video
        ref={front}
        aria-hidden
        className="pointer-events-none absolute inset-0 z-20 h-full w-full object-cover"
        style={{ mixBlendMode: 'screen', opacity: 0.07 }}
        src="/hero-lab/smoke-soft.mp4"
        autoPlay
        loop
        muted
        playsInline
      />
    </div>
  )
}

/**
 * Option 3 + interactive 2D fluid smoke (webgl-fluid, Navier–Stokes on WebGL2).
 * Cursor stirs the haze (TRIGGER:'hover'); AUTO splats give ambient life. Tuned
 * monochrome-warm (COLORFUL:false + warm SPLAT_COLOR, no bloom/sunrays) so it
 * reads as smoke, not rainbow dye. Lazy-imported. The wordmark is made
 * pointer-transparent so the cursor reaches the fluid canvas behind it.
 */
function FluidSmokeMock() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const inited = useRef(false)
  useEffect(() => {
    if (inited.current || !canvasRef.current) return
    inited.current = true
    let cancelled = false
    void import('webgl-fluid').then(({ default: WebGLFluid }) => {
      if (cancelled || !canvasRef.current) return
      WebGLFluid(canvasRef.current, {
        TRIGGER: 'hover',
        IMMEDIATE: false,
        AUTO: true,
        INTERVAL: 4000, // sparse ambient puffs
        SPLAT_COUNT: 2,
        DENSITY_DISSIPATION: 2.6, // higher = dye fades fast → wispy haze, not an accumulating fluid
        VELOCITY_DISSIPATION: 0.5,
        PRESSURE: 0.8,
        CURL: 9,
        SPLAT_RADIUS: 0.18,
        SPLAT_FORCE: 3200,
        SHADING: true,
        COLORFUL: false,
        SPLAT_COLOR: { r: 0.34, g: 0.31, b: 0.29 }, // dim near-neutral gray
        TRANSPARENT: true,
        BLOOM: false,
        SUNRAYS: false,
      })
    })
    return () => {
      cancelled = true
    }
  }, [])
  return (
    <div className={`${BOX} bg-black`}>
      <canvas
        ref={canvasRef}
        aria-hidden
        className="absolute inset-0 z-0 h-full w-full"
        style={{ opacity: 0.5 }}
      />
      <ScryingGridWordmark
        spotlight="cells"
        gapFactor={5}
        className="pointer-events-none relative z-10 h-[260px] w-full max-w-[720px]"
      />
    </div>
  )
}

export default function HeroLabPage() {
  return (
    <main className="mx-auto max-w-[1200px] px-6 py-10">
      <header className="mb-8">
        <a
          href="/hero-lab"
          className="font-mono text-xs text-primary underline-offset-4 hover:underline"
        >
          ← Hero Lab (A–E gallery)
        </a>
        <h1 className="mt-2 font-display text-2xl font-bold text-foreground">
          Hero spotlight + stage-smoke exploration
        </h1>
        <p className="mt-1 max-w-[760px] text-sm text-muted-foreground">
          Spotlight cutoff fixes (mocks 1–3) and the stage-smoke / god-ray atmosphere mocks (4–6), all at the
          chosen Variant 1 density. Switch to <strong>dark mode</strong> (nav toggle) and hover each near its
          edges. The WebGL mocks (fluid, volumetric) need a real browser — see the README.
        </p>
      </header>

      <div className="flex flex-col gap-12">
        {/* 1 — Full-bleed glow layer */}
        <section aria-label="Full-bleed glow layer">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">1</span> Full-bleed glow layer
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            The glow lives on a layer spanning the whole hero, so it has no nearby edge to clip — it fades
            naturally and spills softly past the letters like real light.
          </p>
          <FullBleedMock />
        </section>

        {/* 2 — Oversized canvas */}
        <section aria-label="Oversized canvas">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">2</span> Oversized canvas (padded)
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            The pool stays on the wordmark canvas, but the letters are inset so the pool fully fades inside the
            canvas when you hover the letters. Hero footprint grows; pool spills into the padding.
          </p>
          <div className={BOX}>
            <ScryingGridWordmark spotlight="oversized" gapFactor={5} className="h-[480px] w-full max-w-[1040px]" />
          </div>
        </section>

        {/* 3 — Cells only */}
        <section aria-label="Cells only">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">3</span> Cells only (no pool)
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            No floating gradient at all — only the dots ignite and lean toward the cursor. Nothing to clip,
            ever; most &ldquo;light-cells&rdquo;-true, but no atmospheric pool.
          </p>
          <div className={BOX}>
            <ScryingGridWordmark spotlight="cells" gapFactor={5} className="h-[260px] w-full max-w-[720px]" />
          </div>
        </section>

        {/* 4 — Cells + photoreal stage smoke */}
        <section aria-label="Cells plus stage smoke">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">4</span> Cells + photoreal stage smoke
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            Option 3, but on a &ldquo;stage&rdquo;: real smoke footage composited with screen blend so haze
            billows through the lit wordmark like a smoke machine is running. Photoreal (it&rsquo;s footage);
            ambient, not cursor-driven.
          </p>
          <SmokeStageMock />
        </section>

        {/* 5 — Cells + interactive 2D fluid smoke */}
        <section aria-label="Cells plus fluid smoke">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">5</span> Cells + interactive fluid smoke
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            A real Navier&ndash;Stokes fluid sim (webgl-fluid). Move the cursor over it: the haze genuinely
            swirls and advects where you drag. Tuned monochrome-warm. Real physics + interactive, but reads as
            a 2D ink/haze sheet rather than photoreal volumetric smoke.
          </p>
          <FluidSmokeMock />
        </section>

        {/* 6 — Cells + volumetric smoke billowing in from both sides */}
        <section aria-label="Cells plus volumetric smoke">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">6</span> Cells + volumetric stage fog (rolling, full-frame)
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            A raymarched 3D noise volume (custom shader) — neutral-gray fog that FILLS the frame and rolls /
            billows (domain-warped, settles low like a stage), drifting in front of the lit wordmark so the
            lights shine through and the logo emerges from the haze (the Sunn O))) look). Cursor disturbs it.
          </p>
          <div className={`${BOX} bg-black`}>
            <ScryingGridWordmark
              spotlight="cells"
              gapFactor={5}
              className="relative z-10 h-[260px] w-full max-w-[720px]"
            />
            <VolumetricSmoke className="pointer-events-none absolute inset-0 z-20 h-full w-full opacity-70" />
          </div>
        </section>

        {/* 7 — Cells + screen-space radial god-rays */}
        <section aria-label="Cells plus god-rays">
          <h2 className="mb-1 font-display text-lg font-bold text-foreground">
            <span className="text-primary">7</span> Cells + screen-space god-rays (crisp beams)
          </h2>
          <p className="mb-3 max-w-[760px] text-sm text-muted-foreground">
            Post-process radial light-scattering (GPU Gems 3) — the bright wordmark + a backlight throw crisp
            beam shafts; the gaps between light-cells stripe them. The light follows the cursor — sweep it to
            rake the beams. This is the crisp-beam counterpart to mock 6&rsquo;s soft volumetric fog.
          </p>
          <div className={`${BOX} bg-black`}>
            <RadialGodRays className="absolute inset-0 z-0 h-full w-full" />
            <ScryingGridWordmark
              spotlight="cells"
              gapFactor={5}
              className="pointer-events-none relative z-10 h-[260px] w-full max-w-[720px]"
            />
          </div>
        </section>
      </div>
    </main>
  )
}
