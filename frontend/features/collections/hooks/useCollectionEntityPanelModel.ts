'use client'

/**
 * useCollectionEntityPanelModel (PSY-1473) — builds the EntityContextPanel
 * view-model for a selected non-artist collection-graph node.
 *
 * Data audit (collection-graph payload + cheap existing entity endpoints):
 *   - "N in this graph" / bill·roster name lists → client-side from links
 *   - venue next show + upcoming count → GET /venues/{slug}/shows?upcoming
 *   - release embed + artist·year + label → GET /releases/{slug}
 *   - show bill (full) → GET /shows/{slug} (graph names as fallback)
 *   - label release_count + latest → GET /labels/{slug} + /releases
 *   - festival dates / location / catalog count → GET /festivals/{slug}
 *
 * No dedicated non-artist graph-card endpoint exists; these are the existing
 * detail/list endpoints the entity pages already hit. Track count and doors
 * time are NOT on those payloads — omitted (disclosed in the PR).
 */

import { useMemo } from 'react'

import { formatShowDate } from '@/lib/utils/formatters'
import { useVenueShows } from '@/features/venues/hooks/useVenues'
import { useRelease } from '@/features/releases/hooks/useReleases'
import { useShow } from '@/features/shows/hooks/useShows'
import {
  useLabel,
  useLabelCatalog,
} from '@/features/labels/hooks/useLabels'
import { useFestival } from '@/features/festivals/hooks/useFestivals'
import type {
  CollectionGraphLink,
  CollectionGraphNode,
} from '@/features/collections/types'
import type { ReleaseDetail } from '@/features/releases/types'
import type {
  EntityPanelEntityType,
  EntityPanelPrimary,
} from '@/components/graph/EntityContextPanel'
import { isEntityPanelType } from '@/components/graph/EntityContextPanel'
import {
  FESTIVAL_ARTIST_EDGE_TYPES,
  LABEL_ARTIST_EDGE_TYPES,
  RELEASE_ARTIST_EDGE_TYPES,
  SHOW_ARTIST_EDGE_TYPES,
  VENUE_ARTIST_EDGE_TYPES,
  collectionGraphNeighbors,
  formatArtistNameList,
  indexCollectionNodes,
} from '../lib/collectionGraphNeighbors'

// ReleaseExternalLink lives on releases/types — re-import the finders locally
// rather than coupling to ReleaseDetail.tsx's private helpers.
function findBandcampEmbedUrl(
  links: { platform: string; url: string }[],
): string | null {
  const link = links.find(
    l =>
      l.platform.toLowerCase() === 'bandcamp' &&
      (l.url.includes('/album/') || l.url.includes('/track/')),
  )
  return link?.url ?? null
}

function findSpotifyEmbedUrl(
  links: { platform: string; url: string }[],
): string | null {
  const link = links.find(l => l.platform.toLowerCase() === 'spotify')
  return link?.url ?? null
}

function locationMeta(node: CollectionGraphNode): string | null {
  const parts = [node.city, node.state].filter(Boolean)
  return parts.length > 0 ? parts.join(', ') : null
}

export interface CollectionEntityPanelModel {
  entityType: EntityPanelEntityType
  name: string
  slug: string
  meta: string | null
  primary: EntityPanelPrimary | null
  facts: string[]
  isLoading: boolean
  isError: boolean
}

export function useCollectionEntityPanelModel(opts: {
  selected: CollectionGraphNode | null
  nodes: CollectionGraphNode[]
  links: CollectionGraphLink[]
}): CollectionEntityPanelModel | null {
  const { selected, nodes, links } = opts
  const entityType = selected?.entity_type
  const enabledType =
    selected && entityType && isEntityPanelType(entityType) ? entityType : null
  const slug = selected?.slug ?? ''

  const nodesById = useMemo(() => indexCollectionNodes(nodes), [nodes])

  const venueShows = useVenueShows({
    venueId: slug,
    limit: 20,
    timeFilter: 'upcoming',
    enabled: enabledType === 'venue' && slug.length > 0,
  })

  const releaseQuery = useRelease({
    idOrSlug: slug,
    enabled: enabledType === 'release' && slug.length > 0,
  })

  const showQuery = useShow(enabledType === 'show' ? slug : '')

  const labelQuery = useLabel({
    idOrSlug: slug,
    enabled: enabledType === 'label' && slug.length > 0,
  })

  const labelCatalog = useLabelCatalog({
    labelIdOrSlug: slug,
    enabled: enabledType === 'label' && slug.length > 0,
  })

  const festivalQuery = useFestival({
    idOrSlug: slug,
    enabled: enabledType === 'festival' && slug.length > 0,
  })

  return useMemo(() => {
    if (!selected || !enabledType) return null

    const inGraphArtists = (edgeTypes: readonly string[]) =>
      collectionGraphNeighbors(
        selected.id,
        links,
        nodesById,
        edgeTypes,
        'artist',
      )

    switch (enabledType) {
      case 'venue': {
        const played = inGraphArtists(VENUE_ARTIST_EDGE_TYPES)
        const shows = venueShows.data?.shows ?? []
        const next = shows[0]
        const upcomingCount = venueShows.data?.total ?? shows.length
        const primary: EntityPanelPrimary | null = next
          ? {
              kind: 'labeled',
              label: 'Next show',
              text: [
                formatShowDate(next.event_date, next.state ?? selected.state),
                next.artists.length > 0
                  ? next.artists
                      .slice(0, 3)
                      .map(a => a.name)
                      .join(' + ')
                  : null,
              ]
                .filter(Boolean)
                .join(' · '),
            }
          : null
        const facts: string[] = []
        if (!venueShows.isLoading && upcomingCount > 0) {
          facts.push(
            `${upcomingCount} upcoming ${upcomingCount === 1 ? 'show' : 'shows'}`,
          )
        }
        if (played.length > 0) {
          facts.push(
            `${played.length} ${played.length === 1 ? 'artist' : 'artists'} in this graph ${played.length === 1 ? 'has' : 'have'} played here`,
          )
        }
        return {
          entityType: 'venue',
          name: selected.name,
          slug: selected.slug,
          meta: locationMeta(selected),
          primary,
          facts,
          isLoading: venueShows.isLoading,
          isError: venueShows.isError,
        }
      }

      case 'label': {
        const roster = inGraphArtists(LABEL_ARTIST_EDGE_TYPES)
        const releaseCount = labelQuery.data?.release_count
        const latest = labelCatalog.data?.releases?.[0]
        const primary: EntityPanelPrimary | null =
          roster.length > 0
            ? {
                kind: 'emphasis',
                text: `${roster.length} roster ${roster.length === 1 ? 'artist' : 'artists'} in this graph`,
              }
            : null
        const facts: string[] = []
        if (latest) {
          facts.push(
            `Latest: ${latest.title}${
              latest.release_year ? ` (${latest.release_year})` : ''
            }`,
          )
        }
        if (releaseCount != null && releaseCount > 0) {
          facts.push(
            `${releaseCount} ${releaseCount === 1 ? 'release' : 'releases'} in catalog`,
          )
        }
        return {
          entityType: 'label',
          name: selected.name,
          slug: selected.slug,
          meta: locationMeta(selected),
          primary,
          facts,
          isLoading: labelQuery.isLoading || labelCatalog.isLoading,
          isError: labelQuery.isError,
        }
      }

      case 'release': {
        const release = releaseQuery.data as ReleaseDetail | undefined
        const graphArtists = inGraphArtists(RELEASE_ARTIST_EDGE_TYPES)
        const artistNames =
          release?.artists?.map(a => a.name).join(' · ') ||
          formatArtistNameList(graphArtists) ||
          null
        const year = release?.release_year
        const meta = [artistNames, year != null ? String(year) : null]
          .filter(Boolean)
          .join(' · ')
        const links = release?.external_links ?? []
        const bandcamp = findBandcampEmbedUrl(links)
        const spotify = findSpotifyEmbedUrl(links)
        const primary: EntityPanelPrimary | null =
          bandcamp || spotify
            ? {
                kind: 'embed',
                bandcampAlbumUrl: bandcamp,
                spotifyUrl: spotify,
                title: selected.name,
              }
            : null
        const facts: string[] = []
        const labelName = release?.labels?.[0]?.name
        if (labelName) facts.push(labelName)
        return {
          entityType: 'release',
          name: selected.name,
          slug: selected.slug,
          meta: meta || null,
          primary,
          facts,
          isLoading: releaseQuery.isLoading,
          isError: releaseQuery.isError,
        }
      }

      case 'show': {
        const graphBill = inGraphArtists(SHOW_ARTIST_EDGE_TYPES)
        const showArtists = showQuery.data?.artists ?? []
        const billNames =
          showArtists.length > 0
            ? showArtists.map(a => a.name)
            : graphBill.map(a => a.name)
        const primary: EntityPanelPrimary | null =
          billNames.length > 0
            ? {
                kind: 'labeled',
                label: 'Bill',
                text: billNames.slice(0, 6).join(' · '),
              }
            : null
        const facts: string[] = []
        if (graphBill.length > 0) {
          facts.push(
            `${graphBill.length} bill ${graphBill.length === 1 ? 'artist' : 'artists'} in this graph`,
          )
        }
        return {
          entityType: 'show',
          name: selected.name,
          slug: selected.slug,
          meta: locationMeta(selected),
          primary,
          facts,
          isLoading: showQuery.isLoading,
          isError: showQuery.isError,
        }
      }

      case 'festival': {
        const lineup = inGraphArtists(FESTIVAL_ARTIST_EDGE_TYPES)
        const fest = festivalQuery.data
        const dateMeta = fest
          ? [
              fest.start_date && fest.end_date
                ? formatFestivalDateRange(fest.start_date, fest.end_date)
                : null,
              fest.location_name,
            ]
              .filter(Boolean)
              .join(' · ')
          : locationMeta(selected)
        const primary: EntityPanelPrimary | null =
          lineup.length > 0
            ? {
                kind: 'emphasis',
                text: `${lineup.length} lineup ${lineup.length === 1 ? 'artist' : 'artists'} in this graph`,
              }
            : null
        const facts: string[] = []
        if (fest && fest.artist_count > 0) {
          facts.push(
            `${fest.artist_count} ${fest.artist_count === 1 ? 'artist' : 'artists'} across the festival`,
          )
        }
        return {
          entityType: 'festival',
          name: selected.name,
          slug: selected.slug,
          meta: dateMeta || null,
          primary,
          facts,
          isLoading: festivalQuery.isLoading,
          isError: festivalQuery.isError,
        }
      }
    }
  }, [
    selected,
    enabledType,
    links,
    nodesById,
    venueShows.data,
    venueShows.isLoading,
    venueShows.isError,
    releaseQuery.data,
    releaseQuery.isLoading,
    releaseQuery.isError,
    showQuery.data,
    showQuery.isLoading,
    showQuery.isError,
    labelQuery.data,
    labelQuery.isLoading,
    labelQuery.isError,
    labelCatalog.data,
    labelCatalog.isLoading,
    festivalQuery.data,
    festivalQuery.isLoading,
    festivalQuery.isError,
  ])
}

function formatFestivalDateRange(start: string, end: string): string {
  const startDate = new Date(start)
  const endDate = new Date(end)
  if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
    return ''
  }
  const sameMonth =
    startDate.getUTCMonth() === endDate.getUTCMonth() &&
    startDate.getUTCFullYear() === endDate.getUTCFullYear()
  const startFmt = startDate.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
  const endFmt = endDate.toLocaleDateString('en-US', {
    month: sameMonth ? undefined : 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
  return `${startFmt}–${endFmt}`
}
