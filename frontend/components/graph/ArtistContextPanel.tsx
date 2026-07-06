'use client'

/**
 * ArtistContextPanel (PSY-1345) — the node-select context card for graph
 * surfaces (Figma: Product Designs → Home → Option D, panel node 907:43).
 *
 * Click an artist node and this floating card answers "who is this and how
 * do they fit": next show, labels, where freeform radio plays them, and how
 * connected they are — the "everything is connected" payoff in one glance,
 * with "Open page →" as the dig-in path. Consumed by the homepage scene
 * graph today; intended for the /graph Observatory (PSY-1079…1086,
 * unshipped) as its node-select card.
 *
 * Presentational only — the caller owns data fetching (useArtistGraphCard),
 * selection state, and positioning (floated over the canvas). Mirrors
 * ConnectionPanel's conventions: DOM (not canvas) so it works on touch and
 * carries links; capture-phase Escape so an outer fullscreen overlay's own
 * Esc listener doesn't double-fire; non-modal (no focus trap) — it's an
 * inspector, not a dialog. All strings render through React text nodes
 * (auto-escaped); slugs are encodeURIComponent-pinned to one path segment.
 */

import Link from 'next/link'

import { cn } from '@/lib/utils'
import { formatShowDate } from '@/lib/utils/formatters'
import { parseSpotifyEmbed } from '@/lib/spotify'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import type { ArtistGraphCard } from '@/features/artists/types'
import { GraphPanelShell } from './GraphPanelShell'

export interface ArtistContextPanelProps {
  /** Name of the selected node — rendered immediately, before the card loads. */
  artistName: string
  /** Slug from the graph node — keeps "Open page" working when the card fetch fails. */
  artistSlug: string
  /** Card payload; undefined while loading (skeleton rows render). */
  card: ArtistGraphCard | undefined
  /** True when the card fetch failed — the panel degrades to name + link. */
  isError?: boolean
  onClose: () => void
  className?: string
}

/** Mono-caps field label, matching the mock's NEXT SHOW / LABEL rows. */
function FieldLabel({ children, className }: { children: string; className?: string }) {
  return (
    <div className={cn('font-mono text-[10px] uppercase tracking-wider text-muted-foreground', className)}>
      {children}
    </div>
  )
}

function SkeletonRow() {
  return <div className="h-3.5 w-40 rounded bg-muted/60 animate-pulse" aria-hidden="true" />
}

export function ArtistContextPanel({
  artistName,
  artistSlug,
  card,
  isError = false,
  onClose,
  className,
}: ArtistContextPanelProps) {
  // Esc closes via GraphPanelShell's DismissableLayer, coordinated innermost-first
  // against every other Radix layer (sibling panel, ⌘K palette, enclosing dialog)
  // by Radix's shared layer stack — PSY-1355/1360.

  const loading = !card && !isError
  const location = card && (card.city || card.state)
    ? [card.city, card.state].filter(Boolean).join(', ')
    : null
  const connectionParts = card
    ? [
        card.connections.bills > 0 && `${card.connections.bills} bills`,
        card.connections.similar > 0 && `${card.connections.similar} similar`,
        card.connections.members > 0 && `${card.connections.members} members`,
        card.connections.radio > 0 && `${card.connections.radio} radio`,
        card.connections.shared_labels > 0 && `${card.connections.shared_labels} label ties`,
      ].filter((part): part is string => Boolean(part))
    : []
  // Whether the Listen row will actually render a player — mirrors MusicEmbed's
  // own resolution so the headed row never strands empty (PSY-1302): a Bandcamp
  // embed URL always yields content (an iframe, or a fallback link), a Spotify
  // link only when it parses to an embeddable id.
  const hasPlayableAudio = card
    ? Boolean(card.bandcamp_embed_url) || Boolean(card.spotify && parseSpotifyEmbed(card.spotify))
    : false

  return (
    <GraphPanelShell
      ariaLabel={`About ${artistName}`}
      closeLabel={`Close details for ${artistName}`}
      onClose={onClose}
      className={cn('max-h-[85%] p-4 space-y-2.5', className)}
      header={
        <div className="space-y-0.5">
          <FieldLabel className="text-primary">Selected</FieldLabel>
          <h3 className="text-base font-semibold leading-tight text-foreground">
            {artistName}
          </h3>
          {location && <p className="text-muted-foreground">{location}</p>}
        </div>
      }
    >
      {loading && (
        <div className="space-y-2" aria-label="Loading artist details">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      )}

      {/* Only when there's nothing better to show — a failed background
          refetch retains cached data (isError + card both set), and the
          card must win over an apology. */}
      {isError && !card && (
        <p className="text-muted-foreground">
          Details couldn’t load — the artist page has the full picture.
        </p>
      )}

      {card && (
        <div className="space-y-2.5">
          {card.next_show && (
            <div className="space-y-0.5">
              <FieldLabel>Next show</FieldLabel>
              <p className="text-foreground/90">
                {formatShowDate(
                  card.next_show.event_date,
                  card.next_show.venue_state,
                  false,
                  card.next_show.venue_timezone,
                )}
                {card.next_show.venue_name && (
                  <>
                    {' · '}
                    {card.next_show.venue_name}
                    {card.next_show.venue_city && `, ${card.next_show.venue_city}`}
                  </>
                )}
              </p>
            </div>
          )}

          {card.labels.length > 0 && (
            <div className="space-y-0.5">
              <FieldLabel>{card.labels.length === 1 ? 'Label' : 'Labels'}</FieldLabel>
              <p className="text-foreground/90">
                {card.labels.map((label, i) => (
                  <span key={label.slug || label.name}>
                    {i > 0 && ' · '}
                    {label.slug ? (
                      <Link
                        href={`/labels/${encodeURIComponent(label.slug)}`}
                        className="hover:underline"
                      >
                        {label.name}
                      </Link>
                    ) : (
                      label.name
                    )}
                  </span>
                ))}
              </p>
            </div>
          )}

          {card.radio && card.radio.stations.length > 0 && (
            <div className="space-y-0.5">
              <FieldLabel>As heard on</FieldLabel>
              <p className="text-foreground/90">
                {card.radio.stations.slice(0, 3).join(' · ')}
                {/* play_count spans ALL stations — flag truncation so the
                    number can't misread as the sum of the named three. */}
                {card.radio.stations.length > 3 && ` +${card.radio.stations.length - 3}`}
                {` · ${card.radio.play_count} ${card.radio.play_count === 1 ? 'play' : 'plays'}`}
              </p>
            </div>
          )}

          {connectionParts.length > 0 && (
            <div className="space-y-0.5">
              <FieldLabel>Connections</FieldLabel>
              <p className="text-foreground/90">{connectionParts.join(' · ')}</p>
            </div>
          )}

          {/* Playable audio (PSY-1302) — the graph's payoff: hear the artist
              without leaving the view. Gated on hasPlayableAudio so the row
              appears only when MusicEmbed will render a player (never a dead
              "Listen" header). Only one panel is open at a time, which is what
              keeps a single embed playing. */}
          {hasPlayableAudio && (
            <div className="space-y-0.5">
              <FieldLabel>Listen</FieldLabel>
              <MusicEmbed
                compact
                bandcampAlbumUrl={card.bandcamp_embed_url}
                spotifyUrl={card.spotify}
                artistName={card.name}
              />
            </div>
          )}
        </div>
      )}

      <Link
        // Always rendered — the node's slug keeps navigation available even
        // while the card loads or after a failed fetch (the panel replaced
        // click-to-navigate, so it must never strand the user pathless).
        href={`/artists/${encodeURIComponent(card?.slug ?? artistSlug)}`}
        className="inline-block font-mono text-[11px] text-primary hover:underline underline-offset-4"
      >
        [ Open page → ]
      </Link>
    </GraphPanelShell>
  )
}
