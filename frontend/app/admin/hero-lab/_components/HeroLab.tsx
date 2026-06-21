'use client'

/**
 * Hero Lab — review surface for five candidate "PSYCHIC HOMILY" hero
 * treatments (PSY hero-animation exploration). Move the cursor over each stage;
 * toggle theme + reduced-motion from the sticky control bar. The reduced-motion
 * toggle forces every effect to its static fallback (also what real users with
 * `prefers-reduced-motion: reduce` get).
 */

import { useState, type ReactNode } from 'react'
import { useTheme } from 'next-themes'
import { StageFrame, type StageMeta } from './StageFrame'
import { ScryingGrid } from './ScryingGrid'
import { MirageHaze } from './MirageHaze'
import { MagneticLetters } from './MagneticLetters'
import { ScryingPool } from './ScryingPool'
import { DustDissolve } from './DustDissolve'
import { usePrefersReducedMotion } from '../_lib/hooks'

const STAGES: StageMeta[] = [
  {
    id: 'scrying-grid',
    badge: 'A',
    short: 'Scrying Grid',
    title: 'Scrying Grid',
    technique: 'Wordmark as a field of light-cells that ignite and lean toward the cursor. The Andi Watson "fill in the gaps" reference, made interactive.',
    library: 'Canvas2D MVP · Regl / OGL flow-field for production (Vercel Ship technique)',
    difficulty: 'Medium · Watson-fit 10/10',
  },
  {
    id: 'mirage-haze',
    badge: 'B',
    short: 'Mirage Haze',
    title: 'Mirage Haze',
    technique: 'Real, legible wordmark shimmering like desert air / scrying water; the cursor warms the haze locally. Literal payoff of "This is not a mirage."',
    library: 'Canvas2D displacement · VFX-JS or OGL fragment shader for production',
    difficulty: 'Low–Medium · Watson-fit 9/10',
  },
  {
    id: 'magnetic-letters',
    badge: 'C',
    short: 'Magnetic',
    title: 'Magnetic Letters',
    technique: 'Real DOM text; letters gently repel and scale near the cursor. Tasteful, tiny, fully accessible. The low-risk floor option.',
    library: 'DOM + rAF · GSAP SplitText (free since 2025) or Motion for production',
    difficulty: 'Low · Watson-fit 6/10',
  },
  {
    id: 'scrying-pool',
    badge: 'D',
    short: 'Scrying Pool',
    title: 'Scrying Pool',
    technique: 'Wordmark through refracting liquid glass; a loupe follows the cursor and clicks drop ripples. Canvas2D approximation of a real fluid sim.',
    library: 'Canvas2D approx · react-fluid-distortion (R3F, Navier-Stokes) for production — heavy',
    difficulty: 'High · Watson-fit 8/10',
  },
  {
    id: 'dust-dissolve',
    badge: 'E',
    short: 'Dust',
    title: 'Dust Dissolve',
    technique: 'Particles assemble into the wordmark, then scatter into rising dust where the cursor passes before reforming. Canvas2D approximation.',
    library: 'Canvas2D approx · Three.js WebGPU + TSL MSDF dissolve for production — bleeding-edge',
    difficulty: 'High · Watson-fit 7/10',
  },
]

const EFFECTS: Record<string, (p: { reducedMotion: boolean }) => ReactNode> = {
  'scrying-grid': (p) => <ScryingGrid {...p} />,
  'mirage-haze': (p) => <MirageHaze {...p} />,
  'magnetic-letters': (p) => <MagneticLetters {...p} />,
  'scrying-pool': (p) => <ScryingPool {...p} />,
  'dust-dissolve': (p) => <DustDissolve {...p} />,
}

function ControlButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-md border border-border bg-card px-3 py-1.5 font-mono text-xs text-foreground transition-colors hover:border-primary hover:text-primary"
    >
      {children}
    </button>
  )
}

export function HeroLab() {
  const prefersReduced = usePrefersReducedMotion()
  const [forceReduced, setForceReduced] = useState(false)
  const { resolvedTheme, setTheme } = useTheme()
  const reducedMotion = prefersReduced || forceReduced

  return (
    <div className="min-h-screen w-full bg-background">
      {/* Control bar — tucks under the global TopBar. */}
      <div className="sticky top-[var(--topbar-height)] z-20 border-b border-border bg-background/85 backdrop-blur">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-5 gap-y-2 px-4 py-3">
          <span className="font-display text-sm font-bold text-foreground">
            Hero Lab · <span className="text-primary">PSYCHIC HOMILY</span>
          </span>
          <div className="flex items-center gap-2">
            <ControlButton onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}>
              Theme: {resolvedTheme ?? '…'}
            </ControlButton>
            <ControlButton onClick={() => setForceReduced((v) => !v)}>
              Reduced-motion: {reducedMotion ? 'on' : 'off'}
              {prefersReduced ? ' (OS)' : ''}
            </ControlButton>
          </div>
          <nav className="ml-auto flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs text-muted-foreground">
            {STAGES.map((s) => (
              <a key={s.id} href={`#${s.id}`} className="transition-colors hover:text-primary">
                <span className="text-primary">{s.badge}</span> {s.short}
              </a>
            ))}
          </nav>
        </div>
      </div>

      <header className="mx-auto max-w-6xl px-4 pt-8">
        <p className="max-w-3xl font-mono text-xs leading-relaxed text-muted-foreground">
          Five candidate hero treatments for the wordmark, each pointer-reactive. Hover (or drag on touch)
          over a stage to drive it. Compare in light + dark, and flip reduced-motion to preview the static
          fallback every option ships with. <span className="text-foreground/70">Option A (Scrying Grid) shipped
          to the homepage (PSY-1137); B–E are preserved here for future seasonal / section heroes.</span>
        </p>
        <p className="mt-3 max-w-3xl font-mono text-xs text-muted-foreground">
          → Stage-smoke / god-ray atmosphere exploration:{' '}
          <a href="/admin/hero-lab/smoke" className="text-primary underline-offset-4 hover:underline">
            /admin/hero-lab/smoke
          </a>
        </p>
      </header>

      {STAGES.map((s) => (
        <StageFrame key={s.id} meta={s}>
          {EFFECTS[s.id]({ reducedMotion })}
        </StageFrame>
      ))}

      <footer className="mx-auto max-w-6xl border-t border-border/60 px-4 py-12">
        <p className="max-w-3xl font-mono text-xs leading-relaxed text-muted-foreground">
          A, B and C reflect their real production approach. D and E are dependency-free Canvas2D
          approximations of heavier techniques (react-fluid-distortion / WebGPU+TSL) — close enough to judge
          the aesthetic, lighter than the real thing. All five keep a static fallback for reduced-motion and
          would ship the real <code className="text-foreground/70">&lt;h1&gt;</code> text in the DOM for SEO + screen readers.
        </p>
      </footer>
    </div>
  )
}
