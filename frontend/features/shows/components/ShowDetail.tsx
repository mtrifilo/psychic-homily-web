'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useShow } from '../hooks/useShows'
import type { ApiError } from '@/lib/api'
import { useSetShowSoldOut, useSetShowCancelled } from '@/lib/hooks/admin/useAdminShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { queryKeys } from '@/lib/queryClient'
import { Button } from '@/components/ui/button'
import { EntityDetailLayout, EntityDetailContainer, RevisionHistory } from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { EntityChartRankBadge, useChartEntityRank } from '@/features/charts'
import { EntityTagList } from '@/features/tags'
import { CommentThread, FieldNotesSection } from '@/features/comments'
import {
  EntityEditDrawer,
  EntitySaveSuccessBanner,
  useEntitySaveSuccessBanner,
} from '@/features/contributions'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ShowHeader } from './ShowHeader'
import { ShowListenModule } from './ShowListenModule'
import { ShowActions } from './ShowActions'
import { ShowProvenanceLine } from './ShowProvenanceLine'
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

  // ONE moderation predicate, owned here. Delete, status flags, direct edit,
  // and the whole ShowActions cluster all gate on admin-or-owner today; the
  // named aliases below exist so the day one of them narrows (say, delete
  // goes admin-only) the change is a one-line edit at the definition, not a
  // hunt through the render tree. The one deliberate exception: ShowActions'
  // own Edit BUTTON is admin-only (moderation chrome) — an owner's edit path
  // is the provenance line's [Edit], through the same drawer.
  const canModerateShow = isAdmin || isOwner

  // Check if user can delete: admin or show owner
  const canDelete = canModerateShow

  // Check if user can manage status flags: admin or show owner
  const canManageStatus = canModerateShow

  // PSY-563: shows route through the EntityEditDrawer + show direct-save
  // path. The suggest-edit pipeline is intentionally NOT extended to
  // shows (PSY-461 / PSY-489); the drawer dispatches show saves to
  // /shows/{id} PUT via useShowEdit. An alias of canModerateShow, not a
  // fresh derivation — the "one predicate" comment above only holds if
  // every gate actually routes through the definition.
  const canEditShow = canModerateShow

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
            lifecycle={lifecycle}
            actions={
              // Gated HERE and only here (ShowActions carries no internal
              // guard): the cluster is admin/owner-only, and handing
              // ShowHeader an element that renders null would still reserve
              // the slot's margin under the ticket row for every public
              // viewer.
              canModerateShow ? (
                <ShowActions
                  show={show}
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
              ) : undefined
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

        {/* SLOT: listen module. Self-hiding, so no guard here.

            Why the removals here are the mock rather than an oversight: the
            block this replaced repeated each artist's hometown, which the bill
            block above already states, and hung a SocialLinks row off every
            card. The mock's card carries the player and two verbs. The socials
            still live one click away on the artist page the card links to. */}
        <ShowListenModule artists={artists} />

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
            The mock puts them after the page's own modules, where a newspaper
            puts its byline: still reachable, no longer the headline. History
            and Discussion still render below this, outside the layout, exactly
            as they do on every other detail page.

            This makes the show page the ONLY one of the six detail pages with
            them down here; artist / venue / release / label / festival still
            carry both in the header. That asymmetry is the show mock, not an
            oversight, so a consistency sweep should move the other five rather
            than move this one back.

            A `div`, not a `footer`: there is no `article` or `section` between
            here and `body`, so a `footer` element would publish a second
            `contentinfo` landmark alongside the site footer. */}
        <div className="mt-8 border-t border-border/60 pt-4" data-testid="show-provenance-footer">
          <EntityTagList
            entityType="show"
            entityId={show.id}
            isAuthenticated={isAuthenticated}
          />
          {/* The mock's byline (PSY-1686): listing credit, timestamps, edit
              count, and the working [Edit] / [Report issue] verbs. Supersedes
              the generic AttributionLine on this page only — the other five
              detail pages keep theirs (same deliberate asymmetry as the
              footer position, documented above). */}
          <ShowProvenanceLine
            show={show}
            showTitle={showTitle}
            canEdit={canEditShow}
            onEdit={() => setIsEditing(true)}
          />
        </div>
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
