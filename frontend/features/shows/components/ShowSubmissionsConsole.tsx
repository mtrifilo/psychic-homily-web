'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import {
  Ban,
  CheckCircle2,
  Clock,
  EyeOff,
  Globe,
  Loader2,
  MoreVertical,
  Pencil,
  TicketX,
  Trash2,
  X,
} from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  formatPrice,
  formatShowDate,
  formatShowTime,
} from '@/lib/utils/formatters'
import {
  useSetShowCancelled,
  useSetShowSoldOut,
} from '@/lib/hooks/admin/useAdminShows'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  BracketLink,
  SaveButton,
  SubmissionSuccessDialog,
} from '@/components/shared'
import { VenueDeniedDialog } from '@/features/venues/components/VenueDeniedDialog'
import type { ShowResponse } from '../types'
import { useMySubmissions } from '../hooks'
import { DeleteShowDialog } from './DeleteShowDialog'
import { MakePrivateDialog } from './MakePrivateDialog'
import { PublishShowDialog } from './PublishShowDialog'
import { ShowForm } from './ShowForm'
import { UnpublishShowDialog } from './UnpublishShowDialog'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'

const SHOW_SUBMISSIONS_PATH = '/contribute/submissions'
const SUBMISSIONS_PAGE_SIZE = 50

interface SubmissionShowCardProps {
  show: ShowResponse
  currentUserId?: number
  isAdmin?: boolean
}

function SubmissionShowCard({
  show,
  currentUserId,
  isAdmin,
}: SubmissionShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isUnpublishDialogOpen, setIsUnpublishDialogOpen] = useState(false)
  const [isMakePrivateDialogOpen, setIsMakePrivateDialogOpen] = useState(false)
  const [isPublishDialogOpen, setIsPublishDialogOpen] = useState(false)
  const [isVenueDeniedDialogOpen, setIsVenueDeniedDialogOpen] = useState(false)
  const setSoldOutMutation = useSetShowSoldOut()
  const setCancelledMutation = useSetShowCancelled()
  const venue = show.venues[0]
  const artists = show.artists
  const isOwner =
    currentUserId !== undefined && show.submitted_by === currentUserId
  const canManage = Boolean(isAdmin || isOwner)
  const canUnpublish = show.status === 'approved' && canManage
  const canMakePrivate = show.status === 'pending' && canManage
  const canPublish =
    (show.status === 'private' || show.status === 'rejected') && canManage
  const headingId = `submission-show-${show.id}`

  return (
    <article
      aria-labelledby={headingId}
      className="-mx-3 rounded-lg border-b border-border/50 px-3 py-5 transition-colors duration-200 hover:bg-muted/30"
    >
      <div className="flex flex-col md:flex-row">
        <div className="mb-2 w-full md:mb-0 md:w-1/5 md:pr-4">
          <p className="text-sm font-bold tracking-wide text-primary">
            {formatShowDate(
              show.event_date,
              show.state,
              false,
              show.venues?.[0]?.timezone
            )}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {show.city}, {show.state}
          </p>

          <div className="mt-2 flex flex-col gap-1">
            {canManage &&
              (show.status === 'approved' ? (
                <span className="inline-flex w-fit items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                  <CheckCircle2 className="h-3 w-3" />
                  Published
                </span>
              ) : show.status === 'pending' ? (
                <span className="inline-flex w-fit items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-600 dark:text-amber-400">
                  <Clock className="h-3 w-3" />
                  Pending
                </span>
              ) : show.status === 'private' || show.status === 'rejected' ? (
                <span className="inline-flex w-fit items-center gap-1 rounded-full bg-slate-500/10 px-2 py-0.5 text-xs font-medium text-slate-600 dark:text-slate-400">
                  <EyeOff className="h-3 w-3" />
                  Private
                </span>
              ) : null)}

            {show.is_sold_out && (
              <span className="inline-flex w-fit items-center gap-1 rounded-full bg-rose-500/10 px-2 py-0.5 text-xs font-medium text-rose-600 dark:text-rose-400">
                <TicketX className="h-3 w-3" />
                Sold Out
              </span>
            )}

            {show.is_cancelled && (
              <span className="inline-flex w-fit items-center gap-1 rounded-full bg-slate-500/10 px-2 py-0.5 text-xs font-medium text-slate-600 dark:text-slate-400">
                <Ban className="h-3 w-3" />
                Cancelled
              </span>
            )}
          </div>
        </div>

        <div className="w-full md:w-4/5 md:pl-4">
          <div className="flex items-start justify-between gap-2">
            <h2
              id={headingId}
              className="flex-1 text-lg font-semibold leading-tight tracking-tight"
            >
              {artists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && (
                    <span className="font-normal text-muted-foreground/60">
                      &nbsp;•&nbsp;
                    </span>
                  )}
                  {artist.slug ? (
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="underline decoration-border underline-offset-4 transition-colors hover:text-primary hover:decoration-primary/50"
                    >
                      {artist.name}
                    </Link>
                  ) : artist.socials?.instagram ? (
                    <a
                      href={`https://instagram.com/${artist.socials.instagram}`}
                      className="underline decoration-border underline-offset-4 transition-colors hover:text-primary hover:decoration-primary/50"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {artist.name}
                    </a>
                  ) : (
                    <span>{artist.name}</span>
                  )}
                </span>
              ))}
            </h2>

            <div className="flex shrink-0 items-center gap-1">
              {SHOW_LIST_FEATURE_POLICY.ownership.showSaveButton && (
                <SaveButton showId={show.id} variant="ghost" size="sm" />
              )}

              {isEditing && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setIsEditing(false)}
                  className="h-7 w-7 p-0"
                  aria-label="Cancel editing"
                >
                  <X className="h-4 w-4" />
                </Button>
              )}

              {SHOW_LIST_FEATURE_POLICY.ownership.showOwnerActions &&
                canManage &&
                !isEditing && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                      >
                        <MoreVertical className="h-4 w-4" />
                        <span className="sr-only">Show actions</span>
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setIsEditing(true)}>
                        <Pencil className="mr-2 h-4 w-4" />
                        Edit show
                      </DropdownMenuItem>

                      {canUnpublish && (
                        <DropdownMenuItem
                          onClick={() => setIsUnpublishDialogOpen(true)}
                        >
                          <EyeOff className="mr-2 h-4 w-4" />
                          Make private
                        </DropdownMenuItem>
                      )}
                      {canMakePrivate && (
                        <DropdownMenuItem
                          onClick={() => setIsMakePrivateDialogOpen(true)}
                        >
                          <EyeOff className="mr-2 h-4 w-4" />
                          Make private
                        </DropdownMenuItem>
                      )}
                      {canPublish && (
                        <DropdownMenuItem
                          onClick={() => {
                            if (show.status === 'rejected') {
                              setIsVenueDeniedDialogOpen(true)
                            } else {
                              setIsPublishDialogOpen(true)
                            }
                          }}
                        >
                          <Globe className="mr-2 h-4 w-4" />
                          Publish show
                        </DropdownMenuItem>
                      )}

                      <DropdownMenuSeparator />

                      <DropdownMenuItem
                        onClick={() =>
                          setSoldOutMutation.mutate({
                            showId: show.id,
                            value: !show.is_sold_out,
                          })
                        }
                        disabled={setSoldOutMutation.isPending}
                      >
                        <TicketX className="mr-2 h-4 w-4" />
                        {show.is_sold_out ? 'Undo sold out' : 'Mark sold out'}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() =>
                          setCancelledMutation.mutate({
                            showId: show.id,
                            value: !show.is_cancelled,
                          })
                        }
                        disabled={setCancelledMutation.isPending}
                      >
                        <Ban className="mr-2 h-4 w-4" />
                        {show.is_cancelled
                          ? 'Undo cancelled'
                          : 'Mark cancelled'}
                      </DropdownMenuItem>

                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setIsDeleteDialogOpen(true)}
                      >
                        <Trash2 className="mr-2 h-4 w-4" />
                        Delete show
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
            </div>
          </div>

          <div className="mt-1.5 text-sm text-muted-foreground">
            {venue &&
              (venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className="font-medium text-primary/80 transition-colors hover:text-primary"
                >
                  {venue.name}
                </Link>
              ) : (
                <span className="font-medium text-primary/80">
                  {venue.name}
                </span>
              ))}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>
              &nbsp;•&nbsp;
              {formatShowTime(
                show.event_date,
                show.state,
                show.venues?.[0]?.timezone
              )}
            </span>
            {SHOW_LIST_FEATURE_POLICY.ownership.showDetailsLink && (
              <>
                <span>&nbsp;•&nbsp;</span>
                <Link
                  href={`/shows/${show.slug || show.id}`}
                  className="text-primary/80 underline underline-offset-2 transition-colors hover:text-primary"
                >
                  Details
                </Link>
              </>
            )}
          </div>
        </div>
      </div>

      {isEditing && (
        <div className="mt-4 border-t border-border/50 pt-4">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={() => setIsEditing(false)}
            onCancel={() => setIsEditing(false)}
          />
        </div>
      )}

      <UnpublishShowDialog
        show={show}
        open={isUnpublishDialogOpen}
        onOpenChange={setIsUnpublishDialogOpen}
      />
      <MakePrivateDialog
        show={show}
        open={isMakePrivateDialogOpen}
        onOpenChange={setIsMakePrivateDialogOpen}
      />
      <PublishShowDialog
        show={show}
        open={isPublishDialogOpen}
        onOpenChange={setIsPublishDialogOpen}
      />
      <VenueDeniedDialog
        show={show}
        open={isVenueDeniedDialogOpen}
        onOpenChange={setIsVenueDeniedDialogOpen}
      />
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </article>
  )
}

export function ShowSubmissionsConsole() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()
  const [submissionsOffset, setSubmissionsOffset] = useState(0)
  const { data, isLoading, error } = useMySubmissions({
    enabled: isAuthenticated,
    limit: SUBMISSIONS_PAGE_SIZE,
    offset: submissionsOffset,
  })
  const [dialogDismissed, setDialogDismissed] = useState(false)
  const isPrivateSubmission = searchParams.get('submitted') === 'private'
  const showSuccessDialog = !dialogDismissed && isPrivateSubmission
  const currentUserId = user?.id ? Number(user.id) : undefined
  const queryString = searchParams.toString()

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      const returnTo = queryString
        ? `${SHOW_SUBMISSIONS_PATH}?${queryString}`
        : SHOW_SUBMISSIONS_PATH
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
    }
  }, [authLoading, isAuthenticated, queryString, router])

  const handleDialogClose = (open: boolean) => {
    if (open) return

    setDialogDismissed(true)
    const nextParams = new URLSearchParams(searchParams.toString())
    nextParams.delete('submitted')
    const queryString = nextParams.toString()
    router.replace(
      queryString
        ? `${SHOW_SUBMISSIONS_PATH}?${queryString}`
        : SHOW_SUBMISSIONS_PATH,
      { scroll: false }
    )
  }

  if (authLoading) {
    return <ShowSubmissionsLoading />
  }

  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="container mx-auto max-w-6xl px-4 py-5 md:py-10">
      <SubmissionSuccessDialog
        open={showSuccessDialog}
        onOpenChange={handleDialogClose}
      />

      <header className="mb-4 md:mb-7">
        <div className="font-mono text-[11px] font-bold uppercase tracking-[1.2px] text-muted-foreground">
          Contribute
        </div>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight md:text-[28px]">
          Show submissions
        </h1>
        <p className="mt-1.5 text-sm text-muted-foreground">
          Track the shows you submitted and manage their visibility, details,
          and status.
        </p>
      </header>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      ) : error ? (
        <div className="py-12 text-center text-destructive">
          <p>Failed to load your submissions. Please try again later.</p>
        </div>
      ) : data?.shows.length ? (
        <>
          <section className="w-full" aria-label="Submitted shows">
            {data.shows.map(show => (
              <SubmissionShowCard
                key={show.id}
                show={show}
                currentUserId={currentUserId}
                isAdmin={user?.is_admin}
              />
            ))}
          </section>
          {data.total > SUBMISSIONS_PAGE_SIZE && (
            <nav
              aria-label="Show submissions pages"
              className="mt-6 flex items-center justify-between gap-4"
            >
              <Button
                variant="outline"
                size="sm"
                disabled={submissionsOffset === 0}
                onClick={() =>
                  setSubmissionsOffset(offset =>
                    Math.max(0, offset - SUBMISSIONS_PAGE_SIZE)
                  )
                }
              >
                Previous
              </Button>
              <span className="text-xs text-muted-foreground">
                {submissionsOffset + 1}–
                {Math.min(submissionsOffset + data.shows.length, data.total)} of{' '}
                {data.total}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={submissionsOffset + data.shows.length >= data.total}
                onClick={() =>
                  setSubmissionsOffset(offset => offset + SUBMISSIONS_PAGE_SIZE)
                }
              >
                Next
              </Button>
            </nav>
          )}
        </>
      ) : (
        <div className="pb-6 pt-12">
          <p className="font-medium text-foreground">
            No show submissions yet.
          </p>
          <p className="mt-2 text-sm text-muted-foreground">
            Shows you submit will appear here.
          </p>
          <div className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2">
            <Button asChild variant="outline" size="sm">
              <Link href="/shows/submit">Submit a show</Link>
            </Button>
            <BracketLink
              label="contribution opportunities"
              href="/contribute"
              className="font-mono text-[11px]"
            />
          </div>
        </div>
      )}
    </div>
  )
}

export function ShowSubmissionsLoading() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}
