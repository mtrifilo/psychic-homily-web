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

import { useEffect } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'

import { cn } from '@/lib/utils'
import { formatShowDate } from '@/lib/utils/formatters'
import { parseSpotifyEmbed } from '@/lib/spotify'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import type { ArtistGraphCard } from '@/features/artists/types'

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
  // Esc closes — capture phase so bubble-phase overlay hooks (which skip
  // defaultPrevented events) defer to us. Two guards make one keypress
  // close exactly one layer (PSY-1313 lesson):
  //   - input/dialog-targeted keydowns are ignored (the CommandPalette's
  //     Esc must dismiss the palette alone);
  //   - defaultPrevented events are ignored AND we stopImmediatePropagation
  //     — stopPropagation alone does NOT stop sibling listeners on the
  //     same document/capture phase (e.g. ConnectionPanel when both panels
  //     mount on /graph), so without these, one Esc closes both panels.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      if (
        e.target instanceof Element &&
        e.target.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')
      ) {
        return
      }
      e.preventDefault()
      e.stopImmediatePropagation()
      onClose()
    }
    document.addEventListener('keydown', onKeyDown, { capture: true })
    return () => document.removeEventListener('keydown', onKeyDown, { capture: true })
  }, [onClose])

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
    <section
      aria-label={`About ${artistName}`}
      className={cn(
        'w-72 max-w-[calc(100%-1rem)] max-h-[85%] overflow-y-auto rounded-md border border-border/50',
        'bg-background/95 backdrop-blur-sm p-4 text-xs shadow-lg space-y-2.5',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="space-y-0.5">
          <FieldLabel className="text-primary">Selected</FieldLabel>
          <h3 className="text-base font-semibold leading-tight text-foreground">
            {artistName}
          </h3>
          {location && <p className="text-muted-foreground">{location}</p>}
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label={`Close details for ${artistName}`}
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground hover:text-foreground hover:bg-muted/50"
        >
          <X className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

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
    </section>
  )
}
