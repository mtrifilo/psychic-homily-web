'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Loader2, MapPin } from 'lucide-react'
import { useShow } from '../hooks/useShows'
import type { ApiError } from '@/lib/api'
import { useSetShowSoldOut, useSetShowCancelled } from '@/lib/hooks/admin/useAdminShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ArtistResponse } from '../types'
import { Button } from '@/components/ui/button'
import { SocialLinks, MusicEmbed, EntityDetailLayout } from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { EntityTagList } from '@/features/tags'
import { CommentThread, FieldNotesSection } from '@/features/comments'
import { ShowForm } from '@/components/forms'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ShowHeader } from './ShowHeader'
import { ShowActions } from './ShowActions'

interface ShowDetailProps {
  showId: string | number
}

function artistHasMusic(artist: ArtistResponse): boolean {
  return !!(
    artist.bandcamp_embed_url ||
    artist.socials?.spotify ||
    artist.socials?.bandcamp
  )
}

export function ShowDetail({ showId }: ShowDetailProps) {
  const { data: show, isLoading, error } = useShow(showId)
  const { isAuthenticated, user } = useAuthContext()
  const isAdmin = !!(isAuthenticated && user?.is_admin)

  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)

  // Admin mutations for status flags
  const setSoldOutMutation = useSetShowSoldOut()
  const setCancelledMutation = useSetShowCancelled()

  // Check if user is the show owner (submitter)
  const isOwner = !!(user?.id && show?.submitted_by && String(show.submitted_by) === user.id)

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || isOwner

  // Check if user can manage status flags: admin or show owner
  const canManageStatus = isAdmin || isOwner

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

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
  const showTitle = show.title || artists.map(a => a.name).join(', ')

  return (
    <>
      {/* Cancelled Alert Banner — rendered above the layout so it stays
          above the fold even when the header gets tall. */}
      {show.is_cancelled && (
        <div className="container max-w-6xl mx-auto px-4 pt-6">
          <Alert variant="destructive">
            <AlertDescription className="font-semibold">
              This show has been cancelled.
            </AlertDescription>
          </Alert>
        </div>
      )}

      <EntityDetailLayout
        fallback={{ href: '/shows', label: 'Shows' }}
        entityName={showTitle}
        header={
          <>
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
            <EntityTagList
              entityType="show"
              entityId={show.id}
              isAuthenticated={isAuthenticated}
            />
          </>
        }
      >
        {/* Edit Form */}
        {isEditing && (
          <div className="mb-8 p-4 rounded-lg border border-border bg-muted/30">
            <ShowForm
              mode="edit"
              initialData={show}
              onSuccess={handleEditSuccess}
              onCancel={handleEditCancel}
            />
          </div>
        )}

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
      </EntityDetailLayout>

      {/* Discussion — rendered as a sibling below the layout to match the
          wrapper shape used by the 4 layout-based detail pages. */}
      <div className="mt-0 px-4 md:px-0">
        <CommentThread entityType="show" entityId={show.id} />
      </div>

      {/* PSY-461 / PSY-489: ShowDetail intentionally omits AttributionLine,
          ContributionPrompt, and RevisionHistory — shows flow through an
          admin/owner-only edit pathway, not the community suggest-edit
          pipeline used by the other 5 detail pages. See
          docs/learnings/entity-detail-layout-migration.md for the design
          rationale. Do not "align for parity" with the other detail pages. */}

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </>
  )
}
