'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  MapPin,
  Loader2,
  BadgeCheck,
  ExternalLink,
  Calendar,
  Music,
} from 'lucide-react'
import { useUnverifiedVenues, useVerifyVenue } from '@/lib/hooks/useAdminVenues'
import type { UnverifiedVenue } from '@/lib/types/venue'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

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

interface VerifyVenueDialogProps {
  venue: UnverifiedVenue
  open: boolean
  onOpenChange: (open: boolean) => void
}

function VerifyVenueDialog({
  venue,
  open,
  onOpenChange,
}: VerifyVenueDialogProps) {
  const verifyMutation = useVerifyVenue()

  const handleVerify = () => {
    verifyMutation.mutate(venue.id, {
      onSuccess: () => {
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BadgeCheck className="h-5 w-5 text-primary" />
            Verify Venue
          </DialogTitle>
          <DialogDescription>
            Verify &quot;{venue.name}&quot; as a legitimate venue. This will
            allow the full address to be displayed publicly on all shows at this
            venue.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <div className="rounded-lg border border-border bg-muted/50 p-4 space-y-2 text-sm">
            <div>
              <span className="font-medium">Address:</span>{' '}
              <span className="text-muted-foreground">
                {venue.address || '(not provided)'}
              </span>
            </div>
            <div>
              <span className="font-medium">Location:</span>{' '}
              <span className="text-muted-foreground">
                {venue.city}, {venue.state}
                {venue.zipcode && ` ${venue.zipcode}`}
              </span>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={verifyMutation.isPending}
          >
            Cancel
          </Button>
          <Button onClick={handleVerify} disabled={verifyMutation.isPending}>
            {verifyMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Verifying...
              </>
            ) : (
              <>
                <BadgeCheck className="h-4 w-4 mr-2" />
                Verify Venue
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface UnverifiedVenueCardProps {
  venue: UnverifiedVenue
}

function UnverifiedVenueCard({ venue }: UnverifiedVenueCardProps) {
  const [showVerifyDialog, setShowVerifyDialog] = useState(false)

  return (
    <>
      <Card className="border-orange-500/30 bg-card/50">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-lg truncate">{venue.name}</h3>
              <div className="flex flex-wrap items-center gap-2 mt-1">
                <Badge
                  variant="outline"
                  className="text-orange-500 border-orange-500/50"
                >
                  Unverified
                </Badge>
                <span className="text-sm text-muted-foreground flex items-center gap-1">
                  <MapPin className="h-3 w-3" />
                  {venue.city}, {venue.state}
                </span>
                {venue.show_count > 0 && (
                  <span className="text-sm text-muted-foreground flex items-center gap-1">
                    <Music className="h-3 w-3" />
                    {venue.show_count} {venue.show_count === 1 ? 'show' : 'shows'}
                  </span>
                )}
              </div>
            </div>
            {venue.slug && (
              <Link
                href={`/venues/${venue.slug}`}
                className="text-muted-foreground hover:text-foreground transition-colors"
                target="_blank"
              >
                <ExternalLink className="h-4 w-4" />
              </Link>
            )}
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          {/* Venue Details */}
          <div className="rounded-lg border border-border bg-muted/50 p-4 space-y-2 text-sm">
            <div>
              <span className="font-medium">Address:</span>{' '}
              <span className="text-muted-foreground">
                {venue.address || '(not provided)'}
              </span>
            </div>
            {venue.zipcode && (
              <div>
                <span className="font-medium">Zipcode:</span>{' '}
                <span className="text-muted-foreground">{venue.zipcode}</span>
              </div>
            )}
          </div>

          {/* Submitted Info */}
          <div className="text-sm text-muted-foreground flex items-center gap-1">
            <Calendar className="h-3 w-3" />
            Added {formatDate(venue.created_at)}
          </div>

          {/* Actions */}
          <div className="flex gap-2 pt-2 border-t border-border/50">
            <Button
              variant="default"
              size="sm"
              className="flex-1 gap-2"
              onClick={() => setShowVerifyDialog(true)}
            >
              <BadgeCheck className="h-4 w-4" />
              Verify Venue
            </Button>
          </div>
        </CardContent>
      </Card>

      <VerifyVenueDialog
        venue={venue}
        open={showVerifyDialog}
        onOpenChange={setShowVerifyDialog}
      />
    </>
  )
}

export default function UnverifiedVenuesPage() {
  const { data, isLoading, error } = useUnverifiedVenues()

  return (
    <div className="space-y-4">
      <div className="mb-6">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <MapPin className="h-5 w-5" />
          Unverified Venues
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and verify new venues. Verified venues will display their full
          address publicly.
        </p>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
          Failed to load unverified venues. Please try again.
        </div>
      )}

      {!isLoading && !error && data?.venues.length === 0 && (
        <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
            <BadgeCheck className="h-6 w-6 text-muted-foreground" />
          </div>
          <h3 className="font-medium mb-1">All Venues Verified</h3>
          <p className="text-sm text-muted-foreground">
            All venues have been verified. Check back later for new venue
            submissions.
          </p>
        </div>
      )}

      {!isLoading && !error && data?.venues && data.venues.length > 0 && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {data.total} unverified{' '}
            {data.total === 1 ? 'venue' : 'venues'} awaiting review
          </p>
          {data.venues.map(venue => (
            <UnverifiedVenueCard key={venue.id} venue={venue} />
          ))}
        </div>
      )}
    </div>
  )
}
