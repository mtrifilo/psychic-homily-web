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

import { useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import { Network } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { useArtistBillComposition } from '../hooks/useArtistBillComposition'
import { ArtistGraphVisualization } from './ArtistGraph'
import type { BillCoArtist } from '../types'

interface BillCompositionProps {
  artistId: number
}

const GRAPH_BREAKPOINT_PX = 640
const MIN_GRAPH_NODES = 3

export function BillComposition({ artistId }: BillCompositionProps) {
  const [months, setMonths] = useState<0 | 12>(0)
  const { data, isLoading } = useArtistBillComposition({ artistId, months, enabled: artistId > 0 })
  const [showGraph, setShowGraph] = useState(false)
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Same ResizeObserver pattern as RelatedArtists — null until measured so
  // the canvas never flashes at the wrong size on first paint.
  useEffect(() => {
    if (!containerRef.current) return
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(containerRef.current)
    return () => observer.disconnect()
  }, [])

  if (isLoading) return null
  if (!data || data.below_threshold) return null

  const graphAvailable =
    data.graph.nodes.length >= MIN_GRAPH_NODES &&
    containerWidth !== null &&
    containerWidth >= GRAPH_BREAKPOINT_PX

  return (
    <div ref={containerRef} className="mt-8 px-4 md:px-0">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-4">
        <h2 className="text-lg font-semibold">Bill composition</h2>
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
              {showGraph ? 'Hide Map' : 'View Map'}
            </Button>
          )}
        </div>
      </div>

      <p className="text-sm text-muted-foreground mb-4">
        {data.stats.total_shows} {data.stats.total_shows === 1 ? 'show' : 'shows'}
        {' · '}
        {data.stats.headliner_count} as headliner
        {' · '}
        {data.stats.opener_count} as opener
      </p>

      {showGraph && graphAvailable && (
        <div className="mb-6">
          <ArtistGraphVisualization
            data={data.graph}
            activeTypes={new Set(['shared_bills'])}
            containerWidth={containerWidth!}
          />
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
