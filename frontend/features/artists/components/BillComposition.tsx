'use client'

/**
 * BillComposition (PSY-364)
 *
 * Renders an artist's bill-slot history: opens-with / closes-with tables plus a
 * mini-graph scoped to `shared_bills` edges. Time-filterable (all-time vs. last 12
 * months). Hidden entirely when the artist has fewer than 3 approved shows.
 *
 * Reuses ArtistGraphVisualization unchanged — the backend pre-scopes the payload
 * to bill relationships, and we pass `activeTypes={new Set(['shared_bills'])}` so
 * the legend/filter grammar matches the project's typed-edge palette (PSY-362).
 */

import { useState } from 'react'
import Link from 'next/link'
import { Network } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import { useContainerWidth, GRAPH_BREAKPOINT_PX } from '@/components/graph/useContainerWidth'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import { useArtistBillComposition } from '../hooks/useArtistBillComposition'
import { ArtistGraphVisualization } from './ArtistGraph'
import type { BillCoArtist } from '../types'

interface BillCompositionProps {
  artistId: number
  /**
   * Start collapsed (PSY-644 dense main column). Renders only the
   * `<h2>Bill composition</h2>` header with a `[Show]`/`[Hide]` toggle until
   * expanded. The data fetch stays eager so the existing
   * "hide-when-below-threshold" behavior still applies — collapsed-but-empty
   * artists still disappear entirely.
   */
  defaultCollapsed?: boolean
}

const MIN_GRAPH_NODES = 3

export function BillComposition({ artistId, defaultCollapsed = false }: BillCompositionProps) {
  const [open, setOpen] = useState(!defaultCollapsed)
  const [months, setMonths] = useState<0 | 12>(0)
  const { data, isLoading } = useArtistBillComposition({ artistId, months, enabled: artistId > 0 })
  const [showGraph, setShowGraph] = useState(false)
  // Shared callback-ref measurement (PSY-1305; rationale in useContainerWidth.ts).
  const { refCallback: containerRefCallback, containerWidth } = useContainerWidth()

  // Loading reserves a placeholder box (shared GraphSkeleton, PSY-1347)
  // instead of returning null — a null here shifts the sections below when
  // the tables land. Deliberately headerless: below-threshold artists never
  // get this section, so a labeled header that then vanishes would read as
  // broken; an anonymous pulse quietly collapsing is the lesser jank. The
  // height stands in for the section's typical header + stats + tables mass,
  // not a canvas (the graph itself is toggle-mounted after data arrives).
  if (isLoading) {
    return (
      <div className="mt-8 px-4 md:px-0">
        <GraphSkeleton className="h-[240px]" />
      </div>
    )
  }
  // Self-hide on missing data (error included) is intentional: this is a
  // supplementary, toggle-gated section — not an unconditional graph surface,
  // so the PSY-1446 error-card convention doesn't apply here.
  if (!data || data.below_threshold) return null

  const graphAvailable =
    data.graph.nodes.length >= MIN_GRAPH_NODES &&
    containerWidth !== null &&
    containerWidth >= GRAPH_BREAKPOINT_PX

  return (
    <div ref={containerRefCallback} className="mt-8 px-4 md:px-0">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-4">
        <div className="flex items-baseline gap-3">
          <h2 className="text-lg font-semibold">Bill composition</h2>
          {defaultCollapsed && (
            <BracketLink
              label={open ? 'Hide' : 'Show'}
              onClick={() => setOpen(!open)}
            />
          )}
        </div>
        {open && (
          <div className="flex items-center gap-2">
            <Tabs
              value={months === 0 ? 'all' : '12m'}
              onValueChange={value => setMonths(value === 'all' ? 0 : 12)}
            >
              <TabsList>
                <TabsTrigger value="all">All time</TabsTrigger>
                <TabsTrigger value="12m">Last 12 months</TabsTrigger>
              </TabsList>
            </Tabs>
            {graphAvailable && (
              <Button
                variant={showGraph ? 'default' : 'outline'}
                size="sm"
                onClick={() => setShowGraph(!showGraph)}
              >
                <Network className="h-4 w-4 mr-1.5" />
                {showGraph ? 'Hide graph' : 'Explore graph'}
              </Button>
            )}
          </div>
        )}
      </div>

      {open && (
        <>
          <p className="text-sm text-muted-foreground mb-4">
            {data.stats.total_shows} {data.stats.total_shows === 1 ? 'show' : 'shows'}
            {' · '}
            {data.stats.headliner_count} as headliner
            {' · '}
            {data.stats.opener_count} as opener
          </p>

          {showGraph && graphAvailable && (
            <div className="mb-6">
              {/*
                PSY-1371: ArtistGraphVisualization loads react-force-graph-2d as a
                MODULE-SCOPE dynamic(ssr:false) chunk shared with the ego-graph
                dialog. A failed chunk fetch throws to the nearest boundary (App
                Router); this inline mount had none, so it would crash the whole
                artist page — AND, because the lazy is a singleton whose rejection
                is cached, this second mount re-throws even after the dialog's
                boundary caught the first failure. Contain it here too.
              */}
              <GraphSectionErrorBoundary
                sentryTag="artist-bill-composition"
                fallback={
                  <p role="alert" className="text-sm text-muted-foreground py-8 text-center">
                    The bill-composition graph couldn’t load.
                  </p>
                }
              >
                <ArtistGraphVisualization
                  data={data.graph}
                  activeTypes={new Set(['shared_bills'])}
                  containerWidth={containerWidth!}
                />
              </GraphSectionErrorBoundary>
            </div>
          )}

          <div className="grid md:grid-cols-2 gap-6">
            <BillCoArtistTable
              title="Opens with"
              rows={data.opens_with}
              emptyText={
                months === 12
                  ? 'No opening acts in the last 12 months.'
                  : "Hasn't shared a bill with an opening act yet."
              }
            />
            <BillCoArtistTable
              title="Closes with"
              rows={data.closes_with}
              emptyText={
                months === 12
                  ? "Hasn't opened for anyone in the last 12 months."
                  : "Hasn't opened for anyone yet."
              }
            />
          </div>
        </>
      )}
    </div>
  )
}

interface BillCoArtistTableProps {
  title: string
  rows: BillCoArtist[]
  emptyText: string
}

function BillCoArtistTable({ title, rows, emptyText }: BillCoArtistTableProps) {
  return (
    <div>
      <h3 className="text-sm font-semibold mb-2 text-muted-foreground uppercase tracking-wide">
        {title}
      </h3>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{emptyText}</p>
      ) : (
        <ul className="space-y-1.5">
          {rows.map(row => (
            <li
              key={row.artist.id}
              className="flex items-baseline justify-between gap-2 text-sm"
            >
              <Link
                href={`/artists/${row.artist.slug}`}
                className="font-medium hover:underline truncate"
              >
                {row.artist.name}
              </Link>
              <span className="text-xs text-muted-foreground whitespace-nowrap">
                {row.shared_count} {row.shared_count === 1 ? 'show' : 'shows'}
                {' · '}
                last: {row.last_shared}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
