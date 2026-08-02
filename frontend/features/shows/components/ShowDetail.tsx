'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Loader2, MapPin } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useShow } from '../hooks/useShows'
import type { ApiError } from '@/lib/api'
import { useSetShowSoldOut, useSetShowCancelled } from '@/lib/hooks/admin/useAdminShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { queryKeys } from '@/lib/queryClient'
import type { ArtistResponse } from '../types'
import { Button } from '@/components/ui/button'
import { SocialLinks, MusicEmbed, EntityDetailLayout, EntityDetailContainer, RevisionHistory } from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { EntityChartRankBadge, useChartEntityRank } from '@/features/charts'
import { EntityTagList } from '@/features/tags'
import { CommentThread, FieldNotesSection } from '@/features/comments'
import {
  EntityEditDrawer,
  EntitySaveSuccessBanner,
  useEntitySaveSuccessBanner,
  AttributionLine,
} from '@/features/contributions'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ShowHeader } from './ShowHeader'
import { ShowActions } from './ShowActions'
import { ShowStatusStripe } from './ShowStatusStripe'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'

interface ShowDetailProps {
  showId: string | number
  /**
   * Where the show sits on the venue's calendar, computed ON THE SERVER (see
   * the show route) and passed through to {@link ShowStatusStripe}. A prop
   * rather than a hook because this component runs on the client; see
   * `getShowLifecycleState` for the boundary and why it is not the reader's.
   *
   * Cancellation is deliberately NOT folded in here. It is a data flag the
   * live query already tracks, so the stripe follows an admin's toggle
   * immediately while the clock half stays frozen at what the server saw.
   */
  lifecycle: ShowLifecycleState
}

function artistHasMusic(artist: ArtistResponse): boolean {
  return !!(
    artist.bandcamp_embed_url ||
    artist.socials?.spotify ||
    artist.socials?.bandcamp
  )
}

export function ShowDetail({ showId, lifecycle }: ShowDetailProps) {
  const queryClient = useQueryClient()
  const { data: show, isLoading, error } = useShow(showId)
  const { isAuthenticated, user } = useAuthContext()
  const isAdmin = !!(isAuthenticated && user?.is_admin)

  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const saveBanner = useEntitySaveSuccessBanner()
  // PSY-1420: only mount a sidebar column when the show actually ranks —
  // EntityDetailLayout treats any sidebar ReactNode as present, so we must
  // not pass the self-hiding badge alone (that would leave an empty aside).
  const chartRank = useChartEntityRank(
    'show',
    show?.id ?? 0,
    'quarter',
    { enabled: !!show?.id }
  )

  // Admin mutations for status flags
  const setSoldOutMutation = useSetShowSoldOut()
  const setCancelledMutation = useSetShowCancelled()

  // Check if user is the show owner (submitter)
  const isOwner = !!(user?.id && show?.submitted_by && String(show.submitted_by) === user.id)

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || isOwner

  // Check if user can manage status flags: admin or show owner
  const canManageStatus = isAdmin || isOwner

  // PSY-563: shows route through the EntityEditDrawer + show direct-save
  // path. The suggest-edit pipeline is intentionally NOT extended to
  // shows (PSY-461 / PSY-489); the drawer dispatches show saves to
  // /shows/{id} PUT via useShowEdit. canEditDirectly mirrors the legacy
  // inline-form gate (admin OR submitter) — non-owners see no Edit
  // button, so the drawer never opens for them.
  const canEditShow = isAdmin || isOwner

  const handleToggleSoldOut = () => {
    if (!show) return
    setSoldOutMutation.mutate({ showId: show.id, value: !show.is_sold_out })
  }

  const handleToggleCancelled = () => {
    if (!show) return
    setCancelledMutation.mutate({ showId: show.id, value: !show.is_cancelled })
  }

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load show'
    const is404 = (error as ApiError).status === 404

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Show Not Found' : 'Error Loading Show'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The show you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!show) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Show Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The show you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const artists = show.artists
  const artistsWithMusic = artists.filter(artistHasMusic)
  const showTitle = showDisplayTitle(show.title, artists.map(a => a.name))

  return (
    <>
      {/* The status stripe is the page's first line in every state, including
          cancellation. It replaces the destructive Alert that used to sit
          here: the same fact, said once, in the one place a reader has learned
          to look, instead of twice within a screen of itself. */}
      <ShowStatusStripe show={show} lifecycle={lifecycle} />

      <EntityDetailLayout
        fallback={{ href: '/shows', label: 'Shows' }}
        entityName={showTitle}
        sidebar={
          chartRank.data?.rank != null ? (
            <EntityChartRankBadge entityType="show" entityId={show.id} />
          ) : undefined
        }
        header={
          <ShowHeader
            show={show}
            actions={
              <ShowActions
                show={show}
                showTitle={showTitle}
                isAdmin={isAdmin}
                canDelete={canDelete}
                canManageStatus={canManageStatus}
                isEditing={isEditing}
                onToggleEdit={() => setIsEditing(!isEditing)}
                onOpenDelete={() => setIsDeleteDialogOpen(true)}
                onToggleSoldOut={handleToggleSoldOut}
                onToggleCancelled={handleToggleCancelled}
                isSoldOutPending={setSoldOutMutation.isPending}
                isCancelledPending={setCancelledMutation.isPending}
              />
            }
          />
        }
      >
        {/* Page-level "Changes saved" banner. Mirrors the artist / venue /
            release / label / festival detail pages — fed by the
            EntityEditDrawer's onSuccess callback (PSY-563). Show edits
            still run through an admin/owner-only direct-save path
            (PSY-461 / PSY-489); the suggest-edit pipeline is intentionally
            NOT extended to shows. */}
        <EntitySaveSuccessBanner visible={saveBanner.isVisible} />

        {/* Artist Music Section */}
        {artistsWithMusic.length > 0 && (
          <section className="mb-8">
            <h2 className="text-lg font-semibold mb-4">Listen to the Artists</h2>
            <div className="space-y-6">
              {artistsWithMusic.map(artist => (
                <div key={artist.id} className="space-y-2">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      {artist.slug ? (
                        <Link
                          href={`/artists/${artist.slug}`}
                          className="font-medium hover:text-primary transition-colors"
                        >
                          {artist.name}
                        </Link>
                      ) : (
                        <span className="font-medium">{artist.name}</span>
                      )}
                      {(artist.city || artist.state) && (
                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5">
                          <MapPin className="h-3 w-3" />
                          <span>
                            {[artist.city, artist.state].filter(Boolean).join(', ')}
                          </span>
                        </div>
                      )}
                    </div>
                    <SocialLinks social={artist.socials} className="shrink-0" />
                  </div>
                  <MusicEmbed
                    bandcampAlbumUrl={artist.bandcamp_embed_url}
                    bandcampProfileUrl={artist.socials?.bandcamp}
                    spotifyUrl={artist.socials?.spotify}
                    artistName={artist.name}
                  />
                </div>
              ))}
            </div>
          </section>
        )}

        {/* In Collections */}
        <section className="mb-8">
          <EntityCollections entityType="show" entityId={show.id} />
        </section>

        {/* Field Notes */}
        <section className="mb-8">
          <FieldNotesSection
            showId={show.id}
            showDate={show.event_date}
            artists={artists.map(a => ({ id: a.id, name: a.name }))}
          />
        </section>

        {/* SLOT: rails row. The "also tonight in this city" and "more at this
            venue" columns sit here, below the embeds and above the footer.
            Neither is built; a later wave fills the slot. */}

        {/* Tags and provenance footer. Both were in the header slot, above the
            fold, competing with the bill for the first thing a reader sees.
            The mock puts them last, where a newspaper puts its byline: still
            reachable, no longer the headline. */}
        <footer className="mt-8 border-t border-border/60 pt-4">
          <EntityTagList
            entityType="show"
            entityId={show.id}
            isAuthenticated={isAuthenticated}
          />
          {/* PSY-563: "Last edited by …" attribution row, mirroring
              artist/venue/release/label/festival detail pages. Renders nothing
              until at least one revision exists. */}
          <AttributionLine entityType="show" entityId={show.id} />
        </footer>
      </EntityDetailLayout>

      {/* History + Discussion — rendered as siblings below the layout. The
          shared EntityDetailContainer gives them the SAME gutter + max-width
          as EntityDetailLayout so they don't render flush against the nav /
          full-bleed on desktop (PSY-1026). The suggest-edit pipeline is still
          intentionally excluded for shows (PSY-461 / PSY-489) — the History
          accordion shows direct-save revisions only. */}
      <EntityDetailContainer>
        <RevisionHistory entityType="show" entityId={show.id} isAdmin={isAdmin} />
        <CommentThread entityType="show" entityId={show.id} />
      </EntityDetailContainer>

      {/* Edit Drawer (PSY-563). Admin/owner gated via canEditShow.
          Dispatches to /shows/{id} PUT through useShowEdit — NOT the
          suggest-edit endpoint, preserving the PSY-461 / PSY-489 design
          (shows are admin/owner-only direct-save). */}
      {canEditShow && (
        <EntityEditDrawer
          open={isEditing}
          onOpenChange={setIsEditing}
          entityType="show"
          entityId={show.id}
          entityName={showTitle}
          entity={show as unknown as Record<string, unknown>}
          canEditDirectly={true}
          onSuccess={(result) => {
            queryClient.invalidateQueries({
              queryKey: queryKeys.shows.detail(String(showId)),
            })
            queryClient.invalidateQueries({
              queryKey: queryKeys.revisions.entity('show', show.id),
            })
            saveBanner.handleSaveSuccess(result)
          }}
        />
      )}

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </>
  )
}
