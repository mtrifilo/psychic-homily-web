'use client'

/**
 * Hero Lab — a labeled "stage" wrapping one effect: badge + title, a mono
 * technique/library caption (Space Mono, on-brand for data), and a tall canvas
 * area that paints on the live theme background.
 */

import type { ReactNode } from 'react'

export interface StageMeta {
  id: string
  badge: string
  short: string
  title: string
  technique: string
  library: string
  difficulty: string
}

export function StageFrame({ meta, children }: { meta: StageMeta; children: ReactNode }) {
  return (
    <section id={meta.id} className="scroll-mt-28 border-t border-border/60">
      <div className="mx-auto max-w-6xl px-4 pt-8">
        <div className="flex items-baseline justify-between gap-4">
          <div className="flex items-baseline gap-3">
            <span className="font-mono text-sm font-bold text-primary">{meta.badge}</span>
            <h2 className="font-display text-xl font-bold text-foreground sm:text-2xl">{meta.title}</h2>
          </div>
          <span className="hidden shrink-0 font-mono text-[11px] uppercase tracking-wide text-muted-foreground sm:block">
            {meta.difficulty}
          </span>
        </div>
        <p className="mt-2 max-w-3xl font-mono text-xs leading-relaxed text-muted-foreground">
          {meta.technique} <span className="text-foreground/70">→ {meta.library}</span>
        </p>
      </div>
      <div className="mt-5 h-[58vh] min-h-[360px] w-full px-4 pb-12">{children}</div>
    </section>
  )
}
