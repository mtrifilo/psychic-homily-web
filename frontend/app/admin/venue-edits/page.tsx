'use client'

import { useState } from 'react'
import {
  MapPin,
  Loader2,
  CheckCircle,
  XCircle,
  ArrowRight,
} from 'lucide-react'
import { usePendingVenueEdits } from '@/lib/hooks/useAdminVenueEdits'
import type { PendingVenueEdit } from '@/lib/types/venue'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ApproveVenueEditDialog } from '@/components/admin/ApproveVenueEditDialog'
import { RejectVenueEditDialog } from '@/components/admin/RejectVenueEditDialog'

/**
 * Format date for display
 */
function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Show difference between current and proposed value
 */
function ChangeDiff({
  label,
  current,
  proposed,
}: {
  label: string
  current: string | null | undefined
  proposed: string | null | undefined
}) {
  if (!proposed) return null

  const currentValue = current || '(empty)'
  const hasChange = current !== proposed

  return (
    <div className="text-sm">
      <span className="font-medium">{label}:</span>{' '}
      {hasChange ? (
        <>
          <span className="text-muted-foreground line-through">
            {currentValue}
          </span>
          <ArrowRight className="inline h-3 w-3 mx-1" />
          <span className="text-primary">{proposed}</span>
        </>
      ) : (
        <span className="text-muted-foreground">(no change)</span>
      )}
    </div>
  )
}

interface PendingVenueEditCardProps {
  edit: PendingVenueEdit
}

function PendingVenueEditCard({ edit }: PendingVenueEditCardProps) {
  const [showApproveDialog, setShowApproveDialog] = useState(false)
  const [showRejectDialog, setShowRejectDialog] = useState(false)

  const venue = edit.venue

  return (
    <>
      <Card className="border-amber-500/30 bg-card/50">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-lg truncate">
                {venue?.name || `Venue #${edit.venue_id}`}
              </h3>
              <div className="flex flex-wrap items-center gap-2 mt-1">
                <Badge
                  variant="outline"
                  className="text-amber-500 border-amber-500/50"
                >
                  Pending Edit
                </Badge>
                {venue && (
                  <span className="text-sm text-muted-foreground flex items-center gap-1">
                    <MapPin className="h-3 w-3" />
                    {venue.city}, {venue.state}
                  </span>
                )}
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          {/* Submitter Info */}
          <div className="text-sm text-muted-foreground">
            Submitted by{' '}
            <span className="font-medium text-foreground">
              {edit.submitter_name || `User #${edit.submitted_by}`}
            </span>{' '}
            on {formatDate(edit.created_at)}
          </div>

          {/* Proposed Changes */}
          <div className="rounded-lg border border-border bg-muted/50 p-4 space-y-2">
            <p className="text-sm font-medium mb-3">Proposed Changes:</p>
            <ChangeDiff
              label="Name"
              current={venue?.name}
              proposed={edit.name}
            />
            <ChangeDiff
              label="Address"
              current={venue?.address}
              proposed={edit.address}
            />
            <ChangeDiff
              label="City"
              current={venue?.city}
              proposed={edit.city}
            />
            <ChangeDiff
              label="State"
              current={venue?.state}
              proposed={edit.state}
            />
            <ChangeDiff
              label="Zipcode"
              current={venue?.zipcode}
              proposed={edit.zipcode}
            />
            <ChangeDiff
              label="Website"
              current={venue?.social?.website}
              proposed={edit.website}
            />
            <ChangeDiff
              label="Instagram"
              current={venue?.social?.instagram}
              proposed={edit.instagram}
            />
            <ChangeDiff
              label="Facebook"
              current={venue?.social?.facebook}
              proposed={edit.facebook}
            />
            <ChangeDiff
              label="Twitter"
              current={venue?.social?.twitter}
              proposed={edit.twitter}
            />
          </div>

          {/* Actions */}
          <div className="flex gap-2 pt-2 border-t border-border/50">
            <Button
              variant="default"
              size="sm"
              className="flex-1 gap-2"
              onClick={() => setShowApproveDialog(true)}
            >
              <CheckCircle className="h-4 w-4" />
              Approve
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="flex-1 gap-2 text-destructive hover:text-destructive"
              onClick={() => setShowRejectDialog(true)}
            >
              <XCircle className="h-4 w-4" />
              Reject
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Dialogs */}
      <ApproveVenueEditDialog
        edit={edit}
        open={showApproveDialog}
        onOpenChange={setShowApproveDialog}
      />
      <RejectVenueEditDialog
        edit={edit}
        open={showRejectDialog}
        onOpenChange={setShowRejectDialog}
      />
    </>
  )
}

export default function VenueEditsPage() {
  const { data, isLoading, error } = usePendingVenueEdits()

  return (
    <div className="space-y-4">
      <div className="mb-6">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <MapPin className="h-5 w-5" />
          Pending Venue Edits
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and approve or reject user-submitted venue edits.
        </p>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
          Failed to load pending venue edits. Please try again.
        </div>
      )}

      {!isLoading && !error && data?.edits.length === 0 && (
        <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
            <MapPin className="h-6 w-6 text-muted-foreground" />
          </div>
          <h3 className="font-medium mb-1">No Pending Venue Edits</h3>
          <p className="text-sm text-muted-foreground">
            All venue edit requests have been reviewed. Check back later for new
            submissions.
          </p>
        </div>
      )}

      {!isLoading && !error && data?.edits && data.edits.length > 0 && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {data.total} pending {data.total === 1 ? 'edit' : 'edits'} awaiting
            review
          </p>
          {data.edits.map(edit => (
            <PendingVenueEditCard key={edit.id} edit={edit} />
          ))}
        </div>
      )}
    </div>
  )
}
