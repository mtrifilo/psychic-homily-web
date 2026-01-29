'use client'

import { Suspense, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useSavedShows } from '@/lib/hooks/useSavedShows'
import { useMySubmissions } from '@/lib/hooks/useMySubmissions'
import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import Link from 'next/link'
import {
  Heart,
  Loader2,
  Clock,
  CheckCircle2,
  EyeOff,
  Pencil,
  X,
  Trash2,
  Globe,
  Send,
  Library,
} from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { SavedShowResponse, ShowResponse } from '@/lib/types/show'
import { SaveButton } from '@/components/SaveButton'
import { DeleteShowDialog } from '@/components/DeleteShowDialog'
import { UnpublishShowDialog } from '@/components/UnpublishShowDialog'
import { MakePrivateDialog } from '@/components/MakePrivateDialog'
import { PublishShowDialog } from '@/components/PublishShowDialog'
import { VenueDeniedDialog } from '@/components/VenueDeniedDialog'
import { SubmissionSuccessDialog } from '@/components/SubmissionSuccessDialog'
import { ShowForm } from '@/components/forms'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

function formatDate(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatDateInTimezone(dateString, timezone)
}

function formatTime(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatTimeInTimezone(dateString, timezone)
}

function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface ShowCardProps {
  show: SavedShowResponse | ShowResponse
  currentUserId?: number
  isAdmin?: boolean
  showSaveButton?: boolean
}

function ShowCard({
  show,
  currentUserId,
  isAdmin,
  showSaveButton = true,
}: ShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isUnpublishDialogOpen, setIsUnpublishDialogOpen] = useState(false)
  const [isMakePrivateDialogOpen, setIsMakePrivateDialogOpen] = useState(false)
  const [isPublishDialogOpen, setIsPublishDialogOpen] = useState(false)
  const [isVenueDeniedDialogOpen, setIsVenueDeniedDialogOpen] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists

  // Check if user owns this show
  const isOwner = currentUserId && show.submitted_by === currentUserId

  // Check if user can unpublish this show (approved -> private)
  const canUnpublish = show.status === 'approved' && (isAdmin || isOwner)

  // Check if user can make show private (pending -> private)
  const canMakePrivate = show.status === 'pending' && (isAdmin || isOwner)

  // Check if user can publish show (private/rejected -> approved/pending)
  // Rejected shows will show a VenueDeniedDialog instead of actual publish
  const canPublish =
    (show.status === 'private' || show.status === 'rejected') &&
    (isAdmin || isOwner)

  // Check if user can edit: admin or show owner
  const canEdit = isAdmin || isOwner

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || isOwner

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  return (
    <article className="border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date, Location, and Status */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <h2 className="text-sm font-bold tracking-wide text-primary">
            {formatDate(show.event_date, show.state)}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>

          {/* Status Badge */}
          <div className="mt-2">
            {show.status === 'approved' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-3 w-3" />
                Published
              </span>
            ) : show.status === 'pending' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400">
                <Clock className="h-3 w-3" />
                Pending
              </span>
            ) : show.status === 'private' || show.status === 'rejected' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-slate-500/10 text-slate-600 dark:text-slate-400">
                <EyeOff className="h-3 w-3" />
                Private
              </span>
            ) : null}
          </div>
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          <div className="flex items-start justify-between gap-2">
            {/* Artists */}
            <h1 className="text-lg font-semibold leading-tight tracking-tight flex-1">
              {artists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && (
                    <span className="text-muted-foreground/60 font-normal">
                      &nbsp;•&nbsp;
                    </span>
                  )}
                  {artist.socials?.instagram ? (
                    <a
                      href={`https://instagram.com/${artist.socials.instagram}`}
                      className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
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
            </h1>

            {/* Action Buttons */}
            <div className="flex items-center gap-1 shrink-0">
              {/* Save Button */}
              {showSaveButton && (
                <SaveButton showId={show.id} variant="ghost" size="sm" />
              )}

              {/* Unpublish Button (approved -> private) */}
              {canUnpublish && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsUnpublishDialogOpen(true)}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                  title="Make private"
                >
                  <EyeOff className="h-3.5 w-3.5" />
                </Button>
              )}

              {/* Make Private Button (pending -> private) */}
              {canMakePrivate && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsMakePrivateDialogOpen(true)}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                  title="Make private"
                >
                  <EyeOff className="h-3.5 w-3.5" />
                </Button>
              )}

              {/* Publish Button (private -> approved/pending, rejected -> shows denied dialog) */}
              {canPublish && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    if (show.status === 'rejected') {
                      setIsVenueDeniedDialogOpen(true)
                    } else {
                      setIsPublishDialogOpen(true)
                    }
                  }}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-emerald-500"
                  title="Publish show"
                >
                  <Globe className="h-3.5 w-3.5" />
                </Button>
              )}

              {/* Edit Button (admin or owner) */}
              {canEdit && (
                <Button
                  variant={isEditing ? 'secondary' : 'ghost'}
                  size="sm"
                  onClick={() => setIsEditing(!isEditing)}
                  className="h-7 w-7 p-0"
                  title={isEditing ? 'Cancel editing' : 'Edit show'}
                >
                  {isEditing ? (
                    <X className="h-4 w-4" />
                  ) : (
                    <Pencil className="h-3.5 w-3.5" />
                  )}
                </Button>
              )}

              {/* Delete Button (admin or owner) */}
              {canDelete && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsDeleteDialogOpen(true)}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                  title="Delete show"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
          </div>

          {/* Venue and Details */}
          <div className="text-sm mt-1.5 text-muted-foreground">
            {venue && (
              <Link
                href={`/venues/${venue.id}`}
                className="text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {venue.name}
              </Link>
            )}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>&nbsp;•&nbsp;{formatTime(show.event_date, show.state)}</span>
          </div>
        </div>
      </div>

      {/* Inline Edit Form */}
      {isEditing && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={handleEditSuccess}
            onCancel={handleEditCancel}
          />
        </div>
      )}

      {/* Unpublish Confirmation Dialog */}
      <UnpublishShowDialog
        show={show}
        open={isUnpublishDialogOpen}
        onOpenChange={setIsUnpublishDialogOpen}
      />

      {/* Make Private Dialog */}
      <MakePrivateDialog
        show={show}
        open={isMakePrivateDialogOpen}
        onOpenChange={setIsMakePrivateDialogOpen}
      />

      {/* Publish Show Dialog */}
      <PublishShowDialog
        show={show}
        open={isPublishDialogOpen}
        onOpenChange={setIsPublishDialogOpen}
      />

      {/* Venue Denied Dialog (for rejected shows) */}
      <VenueDeniedDialog
        show={show}
        open={isVenueDeniedDialogOpen}
        onOpenChange={setIsVenueDeniedDialogOpen}
      />

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </article>
  )
}

function SavedShowsList({
  currentUserId,
  isAdmin,
}: {
  currentUserId?: number
  isAdmin?: boolean
}) {
  const { data, isLoading, error } = useSavedShows()

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load your saved shows. Please try again later.</p>
      </div>
    )
  }

  const shows = data?.shows || []

  if (shows.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Heart className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
        <p className="text-lg mb-2">No saved shows yet</p>
        <p className="text-sm">
          Save shows by clicking the heart icon on any show
        </p>
        <Link
          href="/shows"
          className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          Browse Shows
        </Link>
      </div>
    )
  }

  return (
    <section className="w-full">
      {shows.map(show => (
        <ShowCard
          key={show.id}
          show={show}
          currentUserId={currentUserId}
          isAdmin={isAdmin}
          showSaveButton={true}
        />
      ))}
    </section>
  )
}

function MySubmissionsList({
  currentUserId,
  isAdmin,
}: {
  currentUserId?: number
  isAdmin?: boolean
}) {
  const { data, isLoading, error } = useMySubmissions()

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load your submissions. Please try again later.</p>
      </div>
    )
  }

  const shows = data?.shows || []

  if (shows.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Send className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
        <p className="text-lg mb-2">No submissions yet</p>
        <p className="text-sm">Shows you submit will appear here</p>
        <Link
          href="/submissions"
          className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          Submit a Show
        </Link>
      </div>
    )
  }

  return (
    <section className="w-full">
      {shows.map(show => (
        <ShowCard
          key={show.id}
          show={show as SavedShowResponse}
          currentUserId={currentUserId}
          isAdmin={isAdmin}
          showSaveButton={true}
        />
      ))}
    </section>
  )
}

function CollectionPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()

  // Get current tab from URL or default to "saved"
  const currentTab = searchParams.get('tab') || 'saved'

  // Handle submission success dialog from query param
  const submittedStatus = searchParams.get('submitted') as
    | 'pending'
    | 'private'
    | null
  const [dialogDismissed, setDialogDismissed] = useState(false)

  // Derive dialog state from URL param and dismissed state
  const showSuccessDialog =
    !dialogDismissed &&
    (submittedStatus === 'pending' || submittedStatus === 'private')
  const dialogStatus = showSuccessDialog ? submittedStatus : null

  // Clean up URL when dialog is closed
  const handleDialogClose = (open: boolean) => {
    if (!open) {
      setDialogDismissed(true)
      // Remove query param from URL without triggering a navigation
      const newParams = new URLSearchParams(searchParams.toString())
      newParams.delete('submitted')
      const newPath = newParams.toString()
        ? `/collection?${newParams.toString()}`
        : '/collection'
      router.replace(newPath, { scroll: false })
    }
  }

  // Handle tab change
  const handleTabChange = (tab: string) => {
    const newParams = new URLSearchParams(searchParams.toString())
    if (tab === 'saved') {
      newParams.delete('tab')
    } else {
      newParams.set('tab', tab)
    }
    // Preserve submitted param if present
    const newPath = newParams.toString()
      ? `/collection?${newParams.toString()}`
      : '/collection'
    router.replace(newPath, { scroll: false })
  }

  // Redirect if not authenticated
  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  const currentUserId = user?.id ? Number(user.id) : undefined

  return (
    <div className="container max-w-4xl mx-auto px-4 py-12">
      {/* Submission Success Dialog */}
      <SubmissionSuccessDialog
        status={dialogStatus}
        open={showSuccessDialog}
        onOpenChange={handleDialogClose}
      />

      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <Library className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold tracking-tight">My Collection</h1>
        </div>
        <p className="text-muted-foreground">
          Your saved shows and submissions
        </p>
      </div>

      {/* Tabs */}
      <Tabs
        value={currentTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList className="mb-6">
          <TabsTrigger value="saved" className="gap-1.5">
            <Heart className="h-4 w-4" />
            Saved Shows
          </TabsTrigger>
          <TabsTrigger value="submissions" className="gap-1.5">
            <Send className="h-4 w-4" />
            My Submissions
          </TabsTrigger>
        </TabsList>

        <TabsContent value="saved">
          <SavedShowsList
            currentUserId={currentUserId}
            isAdmin={user?.is_admin}
          />
        </TabsContent>

        <TabsContent value="submissions">
          <MySubmissionsList
            currentUserId={currentUserId}
            isAdmin={user?.is_admin}
          />
        </TabsContent>

      </Tabs>
    </div>
  )
}

function CollectionPageLoading() {
  return (
    <div className="flex justify-center items-center min-h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}

export default function CollectionPage() {
  return (
    <Suspense fallback={<CollectionPageLoading />}>
      <CollectionPageContent />
    </Suspense>
  )
}
