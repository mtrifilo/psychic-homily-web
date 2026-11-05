'use client'

/**
 * Hero Lab: the production wordmark at the signed-in home's two fixed sizes,
 * each in the box the boards draw: the bar mark 306x30 in a 390px
 * phone bar, and the nameplate 1084x100 in the 1088px content column. Both run
 * `motion="settle"`; "Replay intro" forgets this tab's played intro (it plays
 * once per tab session) and remounts them. The presets' values are
 * recorded on WORDMARK_PRESETS. The control bar's reduced-motion toggle does
 * not reach these (the component reads the OS setting only).
 */

import { useState } from 'react'
import { Search } from 'lucide-react'
import {
  forgetSettleIntroPlayed,
  ScryingGridWordmark,
} from '@/features/home/components/scrying-grid/ScryingGridWordmark'
import {
  WORDMARK_ONE_LINE,
  WORDMARK_PRESETS,
} from '@/features/home/components/scrying-grid/wordmarkPresets'

function presetCaption(name: keyof typeof WORDMARK_PRESETS): string {
  const preset = WORDMARK_PRESETS[name]
  return `gapFactor ${preset.gapFactor} · bloom ${preset.bloom ? 'on (dark)' : 'off'}`
}

export function SignedInMarks() {
  const [run, setRun] = useState(0)

  return (
    <section id="signed-in-marks" className="scroll-mt-28 border-t border-border/60">
      <div className="mx-auto max-w-6xl px-4 pt-8">
        <div className="flex items-baseline justify-between gap-4">
          <div className="flex items-baseline gap-3">
            <span className="font-data text-sm font-bold text-link">A2</span>
            <h2 className="font-display text-xl font-bold text-foreground sm:text-2xl">
              Signed-in home: bar mark and nameplate
            </h2>
          </div>
          <button
            type="button"
            onClick={() => {
              forgetSettleIntroPlayed()
              setRun(count => count + 1)
            }}
            className="rounded-md border border-border bg-card px-3 py-1.5 font-data text-xs text-foreground transition-colors hover:border-primary hover:text-link"
          >
            Replay intro
          </button>
        </div>
        <p className="mt-2 max-w-3xl font-data text-xs leading-relaxed text-muted-foreground">
          One intro then static; hover or tap ignites and settles within a second of leaving.
        </p>

        <h3 className="mt-6 font-data text-xs uppercase text-muted-foreground">
          Bar mark · 306x30 · {presetCaption('bar')}
        </h3>
        <div className="mt-2 flex h-14 w-[390px] max-w-full items-center justify-between border border-border/50 bg-background/95 px-4">
          <ScryingGridWordmark
            key={`bar-${run}`}
            preset="bar"
            lines={WORDMARK_ONE_LINE}
            heading={false}
            motion="settle"
            className="shrink-0"
          />
          <Search className="size-5 text-foreground" aria-hidden />
        </div>

        <h3 className="mt-8 font-data text-xs uppercase text-muted-foreground">
          Nameplate · 1084x100 · {presetCaption('nameplate')}
        </h3>
        <div className="mt-2 overflow-x-auto pb-12">
          <div className="w-[1088px] border border-border/50 py-8">
            <ScryingGridWordmark
              key={`nameplate-${run}`}
              preset="nameplate"
              lines={WORDMARK_ONE_LINE}
              heading={false}
              motion="settle"
              className="mx-auto"
            />
          </div>
        </div>
      </div>
    </section>
  )
}
