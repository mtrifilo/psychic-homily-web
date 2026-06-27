'use client'

import Link from 'next/link'
import { Fragment, useState } from 'react'
import {
  ArrowLeft,
  Loader2,
  MapPin,
  Sparkles,
  Pencil,
  X,
  Check,
  AlertCircle,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useArtist } from '../hooks/useArtists'
import { getArtistLocation, hasAnySocialLink } from '../types'
import type { Artist } from '../types'
import { useArtistReleases } from '@/features/releases/hooks/useReleases'
import { useArtistAliases } from '@/lib/hooks/admin/useAdminArtists'
import { useArtistLabels, useLabelRoster } from '@/features/labels/hooks/useLabels'
import { queryKeys } from '@/lib/queryClient'
import { useIsAuthenticated } from '@/features/auth'
import {
  useDiscoverMusic,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
  useArtistUpdate,
  type MusicLinkCandidate,
} from '@/lib/hooks/admin/useAdminArtists'
import {
  SocialLinks,
  MusicEmbed,
  EntityDetailLayout,
  EntityHeader,
  RevisionHistory,
  FollowButton,
  EntityDescription,
  AddToCollectionButton,
  BracketLink,
  SectionHeader,
  StatsList,
  DenseTable,
  DenseTableGroupHeader,
  ImageAttribution,
} from '@/components/shared'
import { ArtistTrajectoryChart } from '@/features/festivals/components/ArtistTrajectoryChart'
import { EntityTagList, AddTagDialog } from '@/features/tags'
import { EntityEditDrawer, EntitySaveSuccessBanner, useEntitySaveSuccessBanner, AttributionLine, ReportEntityDialog, useSuggestEdit } from '@/features/contributions'
import { AsHeardOn } from '@/features/radio'
import { EntityCollections } from '@/features/collections'
import { NotifyMeButton } from '@/features/notifications'
import { ArtistShowsList } from './ArtistShowsList'
import { ArtistSimilarSidebar, ArtistGraphDialog } from './RelatedArtists'
import { BillComposition } from './BillComposition'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { getReleaseTypeLabel } from '@/features/releases/types'
import type { ArtistReleaseListItem } from '@/features/releases/types'
import type { ArtistLabel } from '@/features/labels/types'

interface ArtistDetailProps {
  artistId: string | number
}

// --- Discography ---

// Gazelle-style ~10 buckets, mapped to PH's release_type + ArtistRelease.role
// enums. "Soundtracks" from the spec's option list is dropped (no PH
// release_type) and an "Other Releases" catch-all handles main-role rows
// that aren't lp/ep/single/compilation (e.g. live, demo). Empty buckets
// don't render; the whole section hides when 0 releases.
const MAIN_NAMED_TYPES = new Set(['lp', 'ep', 'single', 'compilation'])
const DISCOGRAPHY_BUCKETS: ReadonlyArray<{
  key: string
  label: string
  match: (r: ArtistReleaseListItem) => boolean
}> = [
  { key: 'albums', label: 'Albums', match: r => r.role === 'main' && r.release_type === 'lp' },
  { key: 'eps', label: 'EPs', match: r => r.role === 'main' && r.release_type === 'ep' },
  { key: 'singles', label: 'Singles', match: r => r.role === 'main' && r.release_type === 'single' },
  { key: 'compilations', label: 'Compilations', match: r => r.role === 'main' && r.release_type === 'compilation' },
  { key: 'other_main', label: 'Other Releases', match: r => r.role === 'main' && !MAIN_NAMED_TYPES.has(r.release_type) },
  { key: 'guest', label: 'Guest Appearances', match: r => r.role === 'featured' },
  { key: 'remixes', label: 'Remixes', match: r => r.role === 'remixer' },
  { key: 'dj', label: 'DJ Mixes', match: r => r.role === 'dj' },
  { key: 'production', label: 'Production', match: r => r.role === 'producer' },
  { key: 'composition', label: 'Composition', match: r => r.role === 'composer' },
]

function DiscographyTab({ artistIdOrSlug }: { artistIdOrSlug: string | number }) {
  const { data, isLoading, error } = useArtistReleases({ artistIdOrSlug })

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-8 text-center text-sm text-destructive">
        Failed to load discography
      </div>
    )
  }

  // Hide the whole section when empty — primary content sections in the
  // redesign hide entirely rather than render an apologetic empty state
  // (the only exception is upcoming shows, where the empty state has an
  // action affordance).
  if (!data?.releases || data.releases.length === 0) return null

  const groups = DISCOGRAPHY_BUCKETS.map(b => ({
    ...b,
    releases: data.releases.filter(b.match),
  })).filter(g => g.releases.length > 0)

  if (groups.length === 0) return null

  return (
    <section>
      <SectionHeader title="Discography" as="h2" size="md" />
      <DenseTable variant="alternating" aria-label="Discography">
        <thead>
          <tr>
            <th>Title</th>
            <th className="text-right">Year</th>
            <th>Type</th>
            <th>Label</th>
          </tr>
        </thead>
        <tbody>
          {groups.map(g => (
            <Fragment key={g.key}>
              <DenseTableGroupHeader title={g.label} colSpan={4} />
              {g.releases.map(r => (
                <tr key={`${r.id}-${r.role}`}>
                  <td>
                    <Link
                      href={`/releases/${r.slug}`}
                      className="hover:text-primary underline-offset-2 hover:underline"
                    >
                      {r.title}
                    </Link>
                  </td>
                  <td className="text-right text-muted-foreground">
                    {r.release_year ?? '—'}
                  </td>
                  <td className="text-muted-foreground">
                    {getReleaseTypeLabel(r.release_type)}
                  </td>
                  <td className="text-muted-foreground">
                    {r.label_name && r.label_slug ? (
                      <Link
                        href={`/labels/${r.label_slug}`}
                        className="hover:text-foreground hover:underline"
                      >
                        {r.label_name}
                      </Link>
                    ) : r.label_name ? (
                      r.label_name
                    ) : (
                      '—'
                    )}
                  </td>
                </tr>
              ))}
            </Fragment>
          ))}
        </tbody>
      </DenseTable>
    </section>
  )
}

// --- Also on this label sidebar section ---

function AlsoOnThisLabel({
  labels,
  currentArtistId,
}: {
  labels: ArtistLabel[]
  currentArtistId: number
}) {
  if (labels.length === 0) return null

  return (
    <div className="space-y-4">
      {labels.map(label => (
        <AlsoOnLabelSection
          key={label.id}
          label={label}
          currentArtistId={currentArtistId}
        />
      ))}
    </div>
  )
}

function AlsoOnLabelSection({
  label,
  currentArtistId,
}: {
  label: ArtistLabel
  currentArtistId: number
}) {
  const { data } = useLabelRoster({
    labelIdOrSlug: label.id,
    enabled: true,
  })

  // Filter out current artist and limit to 5
  const otherArtists = data?.artists
    ?.filter(a => a.id !== currentArtistId)
    .slice(0, 5)

  if (!otherArtists || otherArtists.length === 0) return null

  return (
    <div>
      <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        Also on{' '}
        <Link
          href={`/labels/${label.slug}`}
          className="text-foreground hover:underline"
        >
          {label.name}
        </Link>
      </h4>
      <div className="space-y-1">
        {otherArtists.map(artist => (
          <Link
            key={artist.id}
            href={`/artists/${artist.slug}`}
            className="block text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5"
          >
            {artist.name}
          </Link>
        ))}
      </div>
    </div>
  )
}

// --- Sidebar ---

function ArtistSidebar({
  artist,
  labels,
  labelsLoading,
  isAuthenticated,
  onOpenGraph,
}: {
  artist: Artist
  labels: ArtistLabel[]
  labelsLoading: boolean
  isAuthenticated: boolean
  onOpenGraph: () => void
}) {
  const { data: aliasesData } = useArtistAliases(artist.id)
  const aliases = aliasesData?.aliases ?? []

  const statsItems = artist.stats
    ? [
        { label: 'Releases', value: artist.stats.releases },
        { label: 'Labels', value: artist.stats.labels },
        { label: 'Shows tracked', value: artist.stats.shows_tracked },
        { label: 'Similar artists', value: artist.stats.similar_artists },
        {
          label: 'Festival appearances',
          value: artist.stats.festival_appearances,
        },
      ]
    : []

  const hasSocialLinks = !!artist.social && hasAnySocialLink(artist.social)
  const hasMusicLink = Boolean(
    artist.social?.spotify ||
      artist.bandcamp_embed_url ||
      artist.social?.bandcamp
  )

  return (
    <div className="space-y-6">
      {/* Top tracks — FIRST in the column (PSY-1065): listening is the
          fastest way to decide whether you like an unfamiliar band, so the
          embed leads. Rendered only when a music link exists. */}
      {hasMusicLink && (
        <section>
          <SectionHeader title="Top tracks" />
          <MusicEmbed
            bandcampAlbumUrl={artist.bandcamp_embed_url}
            bandcampProfileUrl={artist.social?.bandcamp}
            spotifyUrl={artist.social?.spotify}
            artistName={artist.name}
            compact
          />
        </section>
      )}

      {/* Photo */}
      {artist.image_url && (
        <div>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={artist.image_url}
            alt={artist.name}
            loading="lazy"
            className="w-full rounded-md border border-border/50"
          />
          <ImageAttribution
            source={artist.image_source}
            sourceUrl={artist.image_source_url}
            kind="photo"
            className="mt-1.5 px-0.5"
          />
        </div>
      )}

      {/* Statistics */}
      {statsItems.length > 0 && (
        <section>
          <SectionHeader title="Statistics" />
          <StatsList items={statsItems} />
        </section>
      )}

      {/* Similar artists — dense list + [Explore graph] affordance (opens
          the page-level Dialog). Self-hides when there are no relationships
          AND the viewer can't contribute. */}
      <ArtistSimilarSidebar
        artistId={artist.id}
        artistSlug={artist.slug}
        onOpenGraph={onOpenGraph}
      />

      {/* Tags — EntityTagList renders its own header. PSY-654 hides the
          whole component when no tags exist; the [Add tag] linkbox entry
          below owns the affordance for tagless artists. */}
      <EntityTagList
        entityType="artist"
        entityId={artist.id}
        isAuthenticated={isAuthenticated}
      />

      {/* Aliases */}
      {aliases.length > 0 && (
        <section>
          <SectionHeader title="Also known as" />
          <div className="space-y-1">
            {aliases.map(alias => (
              <p key={alias.id} className="text-sm text-muted-foreground">
                {alias.alias}
              </p>
            ))}
          </div>
        </section>
      )}

      {/* Labels (inline list) */}
      {!labelsLoading && labels.length > 0 && (
        <section>
          <SectionHeader title="Labels" />
          <div className="space-y-1">
            {labels.map(label => (
              <Link
                key={label.id}
                href={`/labels/${label.slug}`}
                className="block text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5"
              >
                {label.name}
              </Link>
            ))}
          </div>
        </section>
      )}

      {/* Links */}
      {hasSocialLinks && (
        <section>
          <SectionHeader title="Links" />
          <SocialLinks social={artist.social} />
        </section>
      )}

      {/* Also on this label */}
      {!labelsLoading && labels.length > 0 && (
        <AlsoOnThisLabel labels={labels} currentArtistId={artist.id} />
      )}

      {/* As heard on (radio) — self-hides when empty */}
      <AsHeardOn entityType="artist" entitySlug={artist.slug} />

      {/* In collections — self-hides when empty */}
      <EntityCollections entityType="artist" entityId={artist.id} />
    </div>
  )
}

// --- Admin Controls ---

function AdminMusicControls({
  artist,
}: {
  artist: {
    id: number
    name: string
    bandcamp_embed_url: string | null
    social: {
      spotify: string | null
      bandcamp: string | null
    }
  }
}) {
  const [showManualInput, setShowManualInput] = useState<
    'bandcamp' | 'spotify' | null
  >(null)
  const [manualUrl, setManualUrl] = useState('')
  const [feedback, setFeedback] = useState<{
    type: 'success' | 'error'
    message: string
  } | null>(null)
  // The flat candidate list from the discover-music endpoint (PSY-1191). The
  // backend returns candidates carrying their own `platform`; the UI groups
  // them for display. null = the picker is closed.
  const [candidates, setCandidates] = useState<MusicLinkCandidate[] | null>(
    null
  )
  // The in-flight URL (not a bool) so peer candidate cards stay enabled
  // without spinners while one is saving.
  const [savingCandidateUrl, setSavingCandidateUrl] = useState<string | null>(
    null
  )

  const discoverMusic = useDiscoverMusic()
  const updateBandcamp = useUpdateArtistBandcamp()
  const clearBandcamp = useClearArtistBandcamp()
  const updateSpotify = useUpdateArtistSpotify()
  const clearSpotify = useClearArtistSpotify()
  // Bandcamp candidate accept routes through the artist-update PATCH (sets
  // social.bandcamp = profile root), which triggers the backend's async
  // profile→album resolver (PSY-1190) to fill bandcamp_embed_url. The dedicated
  // /bandcamp endpoint only accepts album/track URLs, so it can't take a
  // MusicBrainz profile root.
  const updateArtist = useArtistUpdate()

  const isAnyLoading =
    discoverMusic.isPending ||
    updateBandcamp.isPending ||
    clearBandcamp.isPending ||
    updateSpotify.isPending ||
    clearSpotify.isPending ||
    updateArtist.isPending

  const handleDiscover = () => {
    setFeedback(null)
    setCandidates(null)
    discoverMusic.mutate(artist.id, {
      onSuccess: data => {
        // The contract guarantees `candidates: []`, but default to an empty
        // array so a malformed body opens the picker's "no candidates" state
        // rather than silently rendering nothing (a null candidates is falsy).
        setCandidates(data.candidates ?? [])
        setShowManualInput(null)
      },
      onError: err => {
        // The backend + proxy return user-friendly messages (502 upstream
        // failure, timeout backstop); pass them through.
        const message = err instanceof Error ? err.message : 'Discovery failed'
        setFeedback({ type: 'error', message })
      },
    })
  }

  const handlePickCandidate = (candidate: MusicLinkCandidate) => {
    const platform = candidate.platform
    setFeedback(null)
    setSavingCandidateUrl(candidate.url)
    const onError = (err: Error) => {
      setFeedback({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to save URL',
      })
      setSavingCandidateUrl(null)
    }
    // Drop ONLY the accepted candidate from the list; auto-close the picker
    // once none remain. Match on (platform, url) — the backend's dedup key — so
    // a URL that ever appeared under both platforms drops only the one accepted.
    // Cache invalidation is owned by the mutation hooks (PSY-1109).
    const dropAccepted = () => {
      setCandidates(prev => {
        if (!prev) return null
        const next = prev.filter(
          c => !(c.platform === candidate.platform && c.url === candidate.url)
        )
        return next.length === 0 ? null : next
      })
      setSavingCandidateUrl(null)
    }

    if (platform === 'bandcamp') {
      // Accepting a Bandcamp candidate sets the social.bandcamp PROFILE root.
      // The backend's PSY-1190 resolver then fills bandcamp_embed_url ASYNC (a
      // background fetch), so the embed appears on a subsequent load — NOT
      // instantly. The success copy says "resolving" rather than falsely
      // claiming the embed is live.
      updateArtist.mutate(
        { artistId: artist.id, data: { bandcamp: candidate.url } },
        {
          onSuccess: () => {
            setFeedback({
              type: 'success',
              message:
                'Bandcamp profile saved — fetching the embed in the background. Refresh in a moment to see the player.',
            })
            dropAccepted()
          },
          onError,
        }
      )
    } else {
      updateSpotify.mutate(
        { artistId: artist.id, spotifyUrl: candidate.url },
        {
          onSuccess: () => {
            setFeedback({ type: 'success', message: 'Spotify URL saved' })
            dropAccepted()
          },
          onError,
        }
      )
    }
  }

  const handleManualSaveBandcamp = () => {
    if (!manualUrl.trim()) return
    setFeedback(null)
    updateBandcamp.mutate(
      { artistId: artist.id, bandcampUrl: manualUrl.trim() },
      {
        onSuccess: () => {
          setFeedback({ type: 'success', message: 'Bandcamp URL saved' })
          setShowManualInput(null)
          setManualUrl('')
        },
        onError: err => {
          setFeedback({
            type: 'error',
            message: err instanceof Error ? err.message : 'Failed to save URL',
          })
        },
      }
    )
  }

  const handleManualSaveSpotify = () => {
    if (!manualUrl.trim()) return
    setFeedback(null)
    updateSpotify.mutate(
      { artistId: artist.id, spotifyUrl: manualUrl.trim() },
      {
        onSuccess: () => {
          setFeedback({ type: 'success', message: 'Spotify URL saved' })
          setShowManualInput(null)
          setManualUrl('')
        },
        onError: err => {
          setFeedback({
            type: 'error',
            message: err instanceof Error ? err.message : 'Failed to save URL',
          })
        },
      }
    )
  }

  const handleClearBandcamp = () => {
    setFeedback(null)
    clearBandcamp.mutate(artist.id, {
      onSuccess: () => {
        setFeedback({ type: 'success', message: 'Bandcamp URL cleared' })
        setShowManualInput(null)
      },
      onError: err => {
        setFeedback({
          type: 'error',
          message: err instanceof Error ? err.message : 'Failed to clear URL',
        })
      },
    })
  }

  const handleClearSpotify = () => {
    setFeedback(null)
    clearSpotify.mutate(artist.id, {
      onSuccess: () => {
        setFeedback({ type: 'success', message: 'Spotify URL cleared' })
        setShowManualInput(null)
      },
      onError: err => {
        setFeedback({
          type: 'error',
          message: err instanceof Error ? err.message : 'Failed to clear URL',
        })
      },
    })
  }

  const handleCancelEdit = () => {
    setShowManualInput(null)
    setManualUrl('')
    setFeedback(null)
  }

  const hasBandcamp = !!artist.bandcamp_embed_url
  const hasSpotify = !!artist.social?.spotify
  const hasAnyEmbed = hasBandcamp || hasSpotify

  return (
    <div className="mb-6">
      {feedback && (
        <Alert
          variant={feedback.type === 'error' ? 'destructive' : 'default'}
          className="mb-4"
        >
          {feedback.type === 'error' && <AlertCircle className="h-4 w-4" />}
          {feedback.type === 'success' && <Check className="h-4 w-4" />}
          <AlertDescription>{feedback.message}</AlertDescription>
        </Alert>
      )}

      {!hasAnyEmbed && !showManualInput && !candidates && (
        <div className="p-4 rounded-lg border border-dashed border-muted-foreground/25 bg-muted/30">
          <p className="text-sm text-muted-foreground mb-3">
            No music embed configured
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              onClick={handleDiscover}
              disabled={isAnyLoading}
              size="sm"
            >
              {discoverMusic.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Discovering...
                </>
              ) : (
                <>
                  <Sparkles className="h-4 w-4 mr-2" />
                  Discover Music
                </>
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('bandcamp')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Enter Bandcamp URL
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('spotify')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Enter Spotify URL
            </Button>
          </div>
        </div>
      )}

      {hasAnyEmbed && !showManualInput && !candidates && (
        <div className="flex flex-wrap gap-2">
          {hasBandcamp && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setManualUrl(artist.bandcamp_embed_url || '')
                setShowManualInput('bandcamp')
                setFeedback(null)
              }}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Edit Bandcamp URL
            </Button>
          )}
          {hasSpotify && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setManualUrl(artist.social?.spotify || '')
                setShowManualInput('spotify')
                setFeedback(null)
              }}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Edit Spotify URL
            </Button>
          )}
          {hasBandcamp && !hasSpotify && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('spotify')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Add Spotify URL
            </Button>
          )}
          {hasSpotify && !hasBandcamp && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('bandcamp')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Add Bandcamp URL
            </Button>
          )}
        </div>
      )}

      {showManualInput === 'bandcamp' && !candidates && (
        <div className="p-4 rounded-lg border border-muted-foreground/25 bg-muted/30">
          <label
            htmlFor="bandcamp-url"
            className="block text-sm font-medium mb-2"
          >
            Bandcamp Album URL
          </label>
          <div className="flex gap-2">
            <Input
              id="bandcamp-url"
              type="url"
              placeholder="https://artist.bandcamp.com/album/album-name"
              value={manualUrl}
              onChange={e => setManualUrl(e.target.value)}
              disabled={isAnyLoading}
              className="flex-1"
            />
            <Button
              onClick={handleManualSaveBandcamp}
              disabled={isAnyLoading || !manualUrl.trim()}
              size="sm"
            >
              {updateBandcamp.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Check className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancelEdit}
              disabled={isAnyLoading}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          {hasBandcamp && (
            <div className="mt-3 pt-3 border-t border-muted-foreground/25">
              <Button
                variant="ghost"
                size="sm"
                onClick={handleClearBandcamp}
                disabled={isAnyLoading}
                className="text-destructive hover:text-destructive"
              >
                {clearBandcamp.isPending ? (
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                ) : (
                  <X className="h-4 w-4 mr-2" />
                )}
                Clear Bandcamp URL
              </Button>
            </div>
          )}
        </div>
      )}

      {showManualInput === 'spotify' && !candidates && (
        <div className="p-4 rounded-lg border border-muted-foreground/25 bg-muted/30">
          <label
            htmlFor="spotify-url"
            className="block text-sm font-medium mb-2"
          >
            Spotify Artist URL
          </label>
          <div className="flex gap-2">
            <Input
              id="spotify-url"
              type="url"
              placeholder="https://open.spotify.com/artist/..."
              value={manualUrl}
              onChange={e => setManualUrl(e.target.value)}
              disabled={isAnyLoading}
              className="flex-1"
            />
            <Button
              onClick={handleManualSaveSpotify}
              disabled={isAnyLoading || !manualUrl.trim()}
              size="sm"
            >
              {updateSpotify.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Check className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancelEdit}
              disabled={isAnyLoading}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          {hasSpotify && (
            <div className="mt-3 pt-3 border-t border-muted-foreground/25">
              <Button
                variant="ghost"
                size="sm"
                onClick={handleClearSpotify}
                disabled={isAnyLoading}
                className="text-destructive hover:text-destructive"
              >
                {clearSpotify.isPending ? (
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                ) : (
                  <X className="h-4 w-4 mr-2" />
                )}
                Clear Spotify URL
              </Button>
            </div>
          )}
        </div>
      )}
      {candidates && (
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">
              Pick streaming links for this artist
            </h3>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setCandidates(null)
                setSavingCandidateUrl(null)
              }}
              disabled={savingCandidateUrl !== null}
            >
              Done
            </Button>
          </div>

          {candidates.length === 0 ? (
            <Alert>
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                No streaming-link candidates found for this artist. Click Done
                and use manual entry if you have a URL.
              </AlertDescription>
            </Alert>
          ) : (
            <>
              {(['bandcamp', 'spotify'] as const).map(platform => {
                const platformCandidates = candidates.filter(
                  c => c.platform === platform
                )
                if (platformCandidates.length === 0) return null
                return (
                  <section key={platform}>
                    <h4 className="text-xs font-semibold text-muted-foreground mb-2 uppercase tracking-wide">
                      {platform === 'bandcamp' ? 'Bandcamp' : 'Spotify'}{' '}
                      candidates
                    </h4>
                    <div className="space-y-2">
                      {platformCandidates.map(c => (
                        <DiscoveryCandidateCard
                          key={c.url}
                          candidate={c}
                          saving={savingCandidateUrl === c.url}
                          disabled={savingCandidateUrl !== null}
                          onPick={() => handlePickCandidate(c)}
                        />
                      ))}
                    </div>
                  </section>
                )
              })}
            </>
          )}
        </div>
      )}
    </div>
  )
}

function DiscoveryCandidateCard({
  candidate,
  saving,
  disabled,
  onPick,
}: {
  candidate: MusicLinkCandidate
  saving: boolean
  disabled: boolean
  onPick: () => void
}) {
  // Confidence is the region TIER (PSY-1191): `high` = MB geography aligned with
  // a PH show region (clear); `review` = region mismatch / non-US / no region —
  // a possible touring act or namesake the admin should verify before linking.
  // The `review` tier is NEVER auto-accepted or hidden — the admin still picks
  // it explicitly; the badge + caveat just flag the lower certainty.
  const isHigh = candidate.confidence === 'high'

  return (
    <div className="p-3 border rounded-md bg-card">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium">
              {candidate.mb_artist_name || 'Unknown name'}
            </span>
            {isHigh ? (
              <Badge variant="accent">High confidence</Badge>
            ) : (
              <Badge
                variant="outline"
                className="border-pending-foreground/40 text-pending-foreground"
              >
                Verify
              </Badge>
            )}
          </div>
          <a
            href={candidate.url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-muted-foreground hover:underline break-all block"
          >
            {candidate.url}
          </a>
          {/* Liveness + region-match indicators — small inline status row. */}
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span
              className={`inline-flex items-center gap-1 ${candidate.live ? 'text-success-foreground' : 'text-muted-foreground'}`}
            >
              {candidate.live ? (
                <Check className="h-3 w-3" />
              ) : (
                <AlertCircle className="h-3 w-3" />
              )}
              {candidate.live ? 'Reachable' : 'No response'}
            </span>
            <span className="inline-flex items-center gap-1">
              <MapPin className="h-3 w-3" />
              {candidate.region_match ? 'Region match' : 'Region mismatch'}
            </span>
          </div>
          {!isHigh && (
            <p className="text-xs italic text-pending-foreground">
              Verify — possible touring act or namesake.
            </p>
          )}
          {candidate.notes && (
            <p className="text-xs italic text-muted-foreground">
              {candidate.notes}
            </p>
          )}
        </div>
        <Button size="sm" onClick={onPick} disabled={disabled}>
          {saving ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            'Use this'
          )}
        </Button>
      </div>
    </div>
  )
}

// --- Main Component ---

export function ArtistDetail({ artistId }: ArtistDetailProps) {
  const queryClient = useQueryClient()
  const { data: artist, isLoading, error } = useArtist({ artistId })
  const { user, isAuthenticated } = useIsAuthenticated()
  const isAdmin = isAuthenticated && user?.is_admin
  const canEditDirectly = isAdmin || user?.user_tier === 'trusted_contributor' || user?.user_tier === 'local_ambassador'
  const updateArtist = useArtistUpdate()
  const suggestArtistEdit = useSuggestEdit()

  const [isEditing, setIsEditing] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)
  const [addTagDialogOpen, setAddTagDialogOpen] = useState(false)
  // Graph Dialog open state — reactive to the `#graph` URL hash (PSY-361
  // shareable graph URLs still work) AND user toggles (header [Graph] link,
  // sidebar [Explore graph], close button). User intent sticks once set.
  const hash = useUrlHash()
  const [graphDialogUserToggle, setGraphDialogUserToggle] = useState<
    boolean | null
  >(null)
  const graphDialogOpen = graphDialogUserToggle ?? hash === GRAPH_HASH

  // Open the Dialog AND push `#graph` to the URL so the page is shareable
  // (matches pre-PSY-645 behavior of the inline graph). `replaceState` to
  // avoid creating an extra history entry per open.
  const openGraphDialog = () => {
    if (typeof window !== 'undefined' && window.location.hash !== GRAPH_HASH) {
      window.history.replaceState(null, '', GRAPH_HASH)
    }
    setGraphDialogUserToggle(true)
  }

  // PSY-664: mirror openGraphDialog on the close side. The Dialog's
  // onOpenChange fires for every close path (X button, Escape, backdrop
  // click), so strip `#graph` here to keep the shareable-URL contract
  // symmetric — without this, closing left `#graph` in the URL and a
  // refresh or shared link re-opened the dialog. `replaceState` only when
  // the hash is actually `#graph` so we never clobber an unrelated hash.
  const handleGraphDialogOpenChange = (open: boolean) => {
    setGraphDialogUserToggle(open)
    if (
      !open &&
      typeof window !== 'undefined' &&
      window.location.hash === GRAPH_HASH
    ) {
      window.history.replaceState(
        null,
        '',
        window.location.pathname + window.location.search
      )
    }
  }
  const saveBanner = useEntitySaveSuccessBanner()

  // Fetch labels for sidebar
  const { data: labelsData, isLoading: labelsLoading } = useArtistLabels({
    artistIdOrSlug: artistId,
    enabled: !!artist,
  })

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load artist'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Artist Not Found' : 'Error Loading Artist'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The artist you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/artists">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Artists
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!artist) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Artist Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The artist you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/artists">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Artists
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const labels = labelsData?.labels ?? []

  const headerSubtitle = (artist.city || artist.state || artist.country) ? (
    <>
      <MapPin className="h-4 w-4" />
      <span>{getArtistLocation(artist)}</span>
    </>
  ) : undefined

  // Gazelle-style bracketed action linkbox — no icon buttons. The stateful
  // trio (Follow / Notify / Add to collection) render their own bracket
  // variant; they each handle the unauthenticated → /auth redirect internally.
  const headerActions = (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <FollowButton entityType="artists" entityId={artist.id} variant="bracket" />
      <NotifyMeButton
        entityType="artist"
        entityId={artist.id}
        entityName={artist.name}
        variant="bracket"
      />
      <AddToCollectionButton
        entityType="artist"
        entityId={artist.id}
        entityName={artist.name}
        variant="bracket"
      />
      {isAuthenticated && (
        <BracketLink
          label={canEditDirectly ? 'Edit' : 'Suggest edit'}
          onClick={() => setIsEditing(true)}
        />
      )}
      {isAuthenticated && (
        <BracketLink
          label="Add tag"
          onClick={() => setAddTagDialogOpen(true)}
        />
      )}
      <BracketLink label="Graph" onClick={openGraphDialog} />
      {isAuthenticated && (
        <BracketLink
          label="Report"
          title="Report an issue"
          onClick={() => setIsReportOpen(true)}
        />
      )}
    </div>
  )

  return (
    <>
      <EntityDetailLayout
        fallback={{ href: '/artists', label: 'Artists' }}
        entityName={artist.name}
        header={
          <>
            <EntityHeader
              title={artist.name}
              subtitle={headerSubtitle}
              actions={headerActions}
            />
            <EntitySaveSuccessBanner visible={saveBanner.isVisible} />
            <AttributionLine entityType="artist" entityId={artist.id} />
          </>
        }
        sidebar={
          <ArtistSidebar
            artist={artist}
            labels={labels}
            labelsLoading={labelsLoading}
            isAuthenticated={!!isAuthenticated}
            onOpenGraph={openGraphDialog}
          />
        }
      >
        <div className="space-y-8">
          <ArtistShowsList artistId={artist.id} artistName={artist.name} />

          <DiscographyTab artistIdOrSlug={artistId} />

          <BillComposition artistId={artist.id} defaultCollapsed />

          <ArtistTrajectoryChart artistIdOrSlug={artist.id} defaultCollapsed />

          <EntityDescription
            description={artist.description}
            canEdit={!!canEditDirectly}
            onSave={async (description) => {
              await new Promise<void>((resolve, reject) => {
                if (isAdmin) {
                  updateArtist.mutate(
                    { artistId: artist.id, data: { description } },
                    {
                      onSuccess: () => {
                        queryClient.invalidateQueries({
                          queryKey: queryKeys.artists.detail(artistId),
                        })
                        resolve()
                      },
                      onError: reject,
                    }
                  )
                  return
                }
                // PSY-642: trusted_contributor + local_ambassador route through
                // suggest-edit, which auto-applies for them via canEditDirectly
                // (backend pending_edit.go). useSuggestEdit's own onSuccess
                // invalidates ['artists'], which prefix-matches the detail key.
                suggestArtistEdit.mutate(
                  {
                    entityType: 'artist',
                    entityId: artist.id,
                    changes: [
                      {
                        field: 'description',
                        old_value: artist.description ?? '',
                        new_value: description,
                      },
                    ],
                    summary: 'Updated description via inline editor',
                  },
                  {
                    onSuccess: () => resolve(),
                    onError: reject,
                  }
                )
              })
            }}
          />

          {isAdmin && (
            <AdminMusicControls artist={artist} />
          )}

          <RevisionHistory
            entityType="artist"
            entityId={artist.id}
            isAdmin={!!isAdmin}
          />
        </div>
      </EntityDetailLayout>

      {/* Graph Dialog — opened by the header [Graph] link, the sidebar
          [Explore graph] link, or the #graph URL hash (PSY-361 shareable
          graph URLs preserved). Hosts the full re-centering graph. */}
      <ArtistGraphDialog
        artistId={artist.id}
        artistSlug={artist.slug}
        artistName={artist.name}
        open={graphDialogOpen}
        onOpenChange={handleGraphDialogOpenChange}
      />

      {/* Edit Drawer (all authenticated users) */}
      {isAuthenticated && (
        <EntityEditDrawer
          open={isEditing}
          onOpenChange={(open) => setIsEditing(open)}
          entityType="artist"
          entityId={artist.id}
          entityName={artist.name}
          entity={artist as unknown as Record<string, unknown>}
          canEditDirectly={!!canEditDirectly}
          onSuccess={(result) => {
            queryClient.invalidateQueries({
              queryKey: queryKeys.artists.detail(artistId),
            })
            saveBanner.handleSaveSuccess(result)
          }}
        />
      )}

      {/* Report Dialog (authenticated users) */}
      {isAuthenticated && (
        <ReportEntityDialog
          open={isReportOpen}
          onOpenChange={setIsReportOpen}
          entityType="artist"
          entityId={artist.id}
          entityName={artist.name}
        />
      )}

      {isAuthenticated && (
        <AddTagDialog
          entityType="artist"
          entityId={artist.id}
          open={addTagDialogOpen}
          onOpenChange={setAddTagDialogOpen}
        />
      )}
    </>
  )
}
