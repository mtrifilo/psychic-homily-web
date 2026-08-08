import Link from 'next/link'
import { ArrowRight } from 'lucide-react'

import type { GraphWeekView as GraphWeekData } from '../graphOverviewApi'
import { formatGraphWeekCounts, formatGraphWeekRange, graphWeekSummary } from '../graphWeek'
import { TEASER_MOTIF, buildGraphWeekMotif, type GraphWeekMotif } from '../graphMotif'

/**
 * The body of `/graph/this-week` — the share page's two states.
 *
 * Deliberately thin. Someone arrives here from a link, reads one fact and
 * leaves for `/graph`, so there is no navigation, no controls and no second
 * call to action. The CARD is what does the work; this is the page that has to
 * exist for the card to have a URL.
 *
 * A view module rather than part of `graphWeekPage`, matching the rest of the
 * feature: the metadata builder next door is an SEO surface, and pairing it
 * with JSX gave the two one change surface and one test file.
 */

/** Shared chrome, so the two states cannot drift into different pages. */
function GraphWeekShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary">
        The Map of the Scene
      </p>
      <h1 className="mt-2 font-display text-4xl font-medium sm:text-5xl">
        This week in the graph
      </h1>
      {children}
      <Link
        href="/graph"
        className="mt-6 inline-flex items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline"
      >
        Open the map of the scene
        <ArrowRight className="size-3.5" aria-hidden="true" />
      </Link>
    </div>
  )
}

/** The week itself: the two numbers, the days they cover, and the map. */
export function GraphWeekContent({ view }: { view: GraphWeekData }) {
  const { map, week } = view
  const motif = buildGraphWeekMotif(map, week, TEASER_MOTIF)

  return (
    <GraphWeekShell>
      <p className="mt-4 font-mono text-sm uppercase tracking-wider text-primary">
        {formatGraphWeekCounts(week)}
      </p>
      <p className="mt-1 font-mono text-xs uppercase tracking-wider text-muted-foreground">
        {formatGraphWeekRange(week.start, week.end)}
      </p>
      <TeaserMotif motif={motif} label={graphWeekSummary(week)} />
    </GraphWeekShell>
  )
}

/**
 * The share URL before there is anything to share.
 *
 * Reached before the first nightly build has ever run, or when a snapshot
 * cannot be dated. Deliberately says WHEN rather than apologising: the URL is
 * permanent and this state resolves itself overnight, so the one useful thing
 * to offer is the map as it stands today.
 */
export function GraphWeekUnbuilt() {
  return (
    <GraphWeekShell>
      <p className="mt-4 max-w-prose text-sm text-muted-foreground">
        The map is built once a night, and this week&rsquo;s numbers come from that build.
        There isn&rsquo;t one yet, so there is nothing to report here until the next one runs.
      </p>
    </GraphWeekShell>
  )
}

/**
 * The same projection the card paints, drawn with theme tokens.
 *
 * A static `<svg>`, not the map canvas: this is a picture of a snapshot, and
 * mounting the interactive canvas here would ship the graph renderer to a page
 * whose only job is to be a link preview with a body. `role="img"` plus the
 * summary is what makes it mean anything without sight of it.
 */
function TeaserMotif({ motif, label }: { motif: GraphWeekMotif; label: string }) {
  const { box, paint } = TEASER_MOTIF
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${box.width} ${box.height}`}
      className="mt-8 w-full rounded-xl border border-border/60 bg-card"
    >
      <g className="stroke-primary/50" strokeWidth={paint.connectorWidth}>
        {motif.connectors.map((line, index) => (
          <line key={index} x1={line.x1} y1={line.y1} x2={line.x2} y2={line.y2} />
        ))}
      </g>
      <g className="fill-muted-foreground/35">
        {motif.dots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={paint.dotRadius} />
        ))}
      </g>
      <g className="fill-primary">
        {motif.newDots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={paint.newDotRadius} />
        ))}
      </g>
    </svg>
  )
}
