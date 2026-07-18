'use client'

/**
 * EntityContextPanel (PSY-1473) — the non-artist node-select context card for
 * mixed-type graphs (Figma: Product Designs → Graph → PSY-1473, node 1053:2).
 *
 * Shared geometry with ArtistContextPanel (GraphPanelShell, w-72, same card
 * language) but a type-tag header instead of "Selected", and no "Center here"
 * (collection graphs don't re-root). Artist nodes keep using ArtistContextPanel
 * unchanged.
 *
 * Presentational only — the caller owns selection, data fetching / graph-derived
 * neighbors, and positioning. Content shape per type (approved mock):
 *   mono type-tag → name + meta → one primary payoff → fact rows → Open page →
 */

import type { Ref } from 'react'
import Link from 'next/link'

import { cn } from '@/lib/utils'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { GraphPanelShell } from './GraphPanelShell'

/**
 * Mixed-type select-gesture sentence for collection (and future multi-type)
 * canvases. Artist-only surfaces keep `graphSelectGestureHint` so their
 * aria-labels stay artist-phrased.
 */
export const graphEntitySelectGestureHint =
  'Click a node for that item’s details.'

export type EntityPanelEntityType =
  | 'venue'
  | 'label'
  | 'release'
  | 'show'
  | 'festival'

export function isEntityPanelType(
  value: string,
): value is EntityPanelEntityType {
  return (
    value === 'venue' ||
    value === 'label' ||
    value === 'release' ||
    value === 'show' ||
    value === 'festival'
  )
}

const ENTITY_TYPE_LABEL: Record<EntityPanelEntityType, string> = {
  venue: 'Venue',
  label: 'Label',
  release: 'Release',
  show: 'Show',
  festival: 'Festival',
}

const ENTITY_TYPE_PATH: Record<EntityPanelEntityType, string> = {
  venue: 'venues',
  label: 'labels',
  release: 'releases',
  show: 'shows',
  festival: 'festivals',
}

/** Headed text payoff (NEXT SHOW / BILL) — label renders as mono-caps. */
export interface EntityPanelLabeledPrimary {
  kind: 'labeled'
  label: string
  text: string
}

/** Whole-line emphasis payoff (roster/lineup-in-graph counts). */
export interface EntityPanelEmphasisPrimary {
  kind: 'emphasis'
  text: string
}

/** Release Bandcamp/Spotify embed via shipped MusicEmbed. */
export interface EntityPanelEmbedPrimary {
  kind: 'embed'
  bandcampAlbumUrl?: string | null
  spotifyUrl?: string | null
  title: string
}

export type EntityPanelPrimary =
  | EntityPanelLabeledPrimary
  | EntityPanelEmphasisPrimary
  | EntityPanelEmbedPrimary

export interface EntityContextPanelProps {
  entityType: EntityPanelEntityType
  /** Name rendered immediately, before enrichment loads. */
  name: string
  /** Slug keeps "Open page" working when enrichment fails. */
  slug: string
  /** Location / date / artist·year line under the name. */
  meta?: string | null
  /** One primary payoff; omit while loading or when unavailable. */
  primary?: EntityPanelPrimary | null
  /** 1–2 secondary fact rows. */
  facts?: string[]
  /** True while enrichment is in flight (skeleton under the header). */
  isLoading?: boolean
  /** True when enrichment failed — degrades to name + Open page. */
  isError?: boolean
  onClose: () => void
  className?: string
  panelRef?: Ref<HTMLElement>
}

function FieldLabel({
  children,
  className,
}: {
  children: string
  className?: string
}) {
  return (
    <div
      className={cn(
        'font-mono text-[10px] uppercase tracking-wider text-muted-foreground',
        className,
      )}
    >
      {children}
    </div>
  )
}

function SkeletonRow() {
  return (
    <div
      className="h-3.5 w-40 rounded bg-muted/60 animate-pulse"
      aria-hidden="true"
    />
  )
}

export function EntityContextPanel({
  entityType,
  name,
  slug,
  meta,
  primary,
  facts = [],
  isLoading = false,
  isError = false,
  onClose,
  className,
  panelRef,
}: EntityContextPanelProps) {
  const typeLabel = ENTITY_TYPE_LABEL[entityType].toUpperCase()
  const pageHref = `/${ENTITY_TYPE_PATH[entityType]}/${encodeURIComponent(slug)}`
  const hasContent = primary != null || facts.length > 0

  return (
    <GraphPanelShell
      ariaLabel={`About ${name}`}
      closeLabel={`Close details for ${name}`}
      onClose={onClose}
      panelRef={panelRef}
      className={cn('max-h-[85%] p-4 space-y-2.5', className)}
      header={
        <div className="space-y-0.5">
          <FieldLabel>{typeLabel}</FieldLabel>
          <h3 className="text-base font-semibold leading-tight text-foreground">
            {name}
          </h3>
          {meta ? <p className="text-muted-foreground">{meta}</p> : null}
        </div>
      }
    >
      {/* Graph-derived facts/primary can land before enrichment finishes —
          keep showing them; only skeleton when there's nothing yet. */}
      {isLoading && !hasContent && (
        <div className="space-y-2" aria-label="Loading details">
          <SkeletonRow />
          <SkeletonRow />
        </div>
      )}

      {isError && !hasContent && !isLoading && (
        <p className="text-muted-foreground">
          Details couldn’t load — the {ENTITY_TYPE_LABEL[entityType].toLowerCase()}{' '}
          page has the full picture.
        </p>
      )}

      {hasContent && (
        <div className="space-y-2.5">
          {primary?.kind === 'labeled' && primary.text && (
            <div className="space-y-0.5">
              <FieldLabel>{primary.label}</FieldLabel>
              <p className="text-foreground/90">{primary.text}</p>
            </div>
          )}

          {primary?.kind === 'emphasis' && primary.text && (
            <p className="font-medium text-foreground/90">{primary.text}</p>
          )}

          {primary?.kind === 'embed' &&
            (primary.bandcampAlbumUrl || primary.spotifyUrl) && (
              <MusicEmbed
                compact
                bandcampAlbumUrl={primary.bandcampAlbumUrl}
                spotifyUrl={primary.spotifyUrl}
                artistName={primary.title}
              />
            )}

          {facts.map((fact, i) => (
            <p key={`${i}-${fact}`} className="text-muted-foreground">
              {fact}
            </p>
          ))}
        </div>
      )}

      <Link
        href={pageHref}
        className="inline-block font-mono text-[11px] text-primary hover:underline underline-offset-4"
      >
        [ Open page → ]
      </Link>
    </GraphPanelShell>
  )
}
