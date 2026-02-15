'use client'

import { useState } from 'react'
import {
  Calendar,
  MapPin,
  Music,
  Users,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Radar,
} from 'lucide-react'
import Link from 'next/link'
import type { ShowResponse } from '@/lib/types/show'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { formatAdminDate, formatAdminTime } from '@/lib/utils/formatters'
import { ApproveShowDialog } from './ApproveShowDialog'
import { RejectShowDialog } from './RejectShowDialog'

interface PendingShowCardProps {
  show: ShowResponse
}

export function PendingShowCard({ show }: PendingShowCardProps) {
  const [showApproveDialog, setShowApproveDialog] = useState(false)
  const [showRejectDialog, setShowRejectDialog] = useState(false)

  const venue = show.venues[0]
  const hasUnverifiedVenue = show.venues.some(v => !v.verified)
  const headliner = show.artists.find(a => a.is_headliner)
  const openers = show.artists.filter(a => !a.is_headliner)

  return (
    <>
      <Card className="border-amber-500/30 bg-card/50">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-lg truncate">
                {show.title ||
                  show.artists.map(a => a.name).join(', ') ||
                  'Untitled Show'}
              </h3>
              <div className="flex flex-wrap items-center gap-2 mt-1">
                <Badge
                  variant="outline"
                  className="text-amber-500 border-amber-500/50"
                >
                  Pending Review
                </Badge>
                {show.source === 'discovery' && (
                  <Badge
                    variant="outline"
                    className="text-blue-500 border-blue-500/50 gap-1"
                  >
                    <Radar className="h-3 w-3" />
                    Discovery Import
                  </Badge>
                )}
                {hasUnverifiedVenue && (
                  <Badge
                    variant="outline"
                    className="text-orange-500 border-orange-500/50 gap-1"
                  >
                    <AlertTriangle className="h-3 w-3" />
                    Unverified Venue
                  </Badge>
                )}
              </div>
              {show.duplicate_of_show_id && (
                <p className="text-xs text-blue-500 mt-1">
                  Potential duplicate of{' '}
                  <Link
                    href={`/shows/${show.duplicate_of_show_id}`}
                    target="_blank"
                    className="underline hover:text-blue-400"
                  >
                    show #{show.duplicate_of_show_id}
                  </Link>
                </p>
              )}
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          {/* Event Details */}
          <div className="grid gap-3 text-sm">
            {/* Date & Time */}
            <div className="flex items-center gap-2 text-muted-foreground">
              <Calendar className="h-4 w-4 shrink-0" />
              <span>
                {formatAdminDate(show.event_date)} at {formatAdminTime(show.event_date)}
              </span>
            </div>

            {/* Venue */}
            {venue && (
              <div className="flex items-start gap-2 text-muted-foreground">
                <MapPin className="h-4 w-4 shrink-0 mt-0.5" />
                <div>
                  <span className="font-medium text-foreground">
                    {venue.name}
                  </span>
                  {!venue.verified && (
                    <span className="text-orange-500 text-xs ml-2">
                      (New venue)
                    </span>
                  )}
                  <div className="text-xs">
                    {venue.city}, {venue.state}
                    {venue.address && ` • ${venue.address}`}
                  </div>
                </div>
              </div>
            )}

            {/* Artists */}
            <div className="flex items-start gap-2 text-muted-foreground">
              <Users className="h-4 w-4 shrink-0 mt-0.5" />
              <div>
                {headliner && (
                  <div className="flex items-center gap-1">
                    <Music className="h-3 w-3" />
                    <span className="font-medium text-foreground">
                      {headliner.name}
                    </span>
                    <span className="text-xs">(Headliner)</span>
                  </div>
                )}
                {openers.length > 0 && (
                  <div className="text-xs mt-1">
                    With: {openers.map(a => a.name).join(', ')}
                  </div>
                )}
              </div>
            </div>

            {/* Additional Details */}
            {(show.price || show.age_requirement) && (
              <div className="flex flex-wrap gap-2 text-xs">
                {show.price !== null && show.price !== undefined && (
                  <Badge variant="secondary">${show.price}</Badge>
                )}
                {show.age_requirement && (
                  <Badge variant="secondary">{show.age_requirement}</Badge>
                )}
              </div>
            )}

            {/* Description */}
            {show.description && (
              <p className="text-xs text-muted-foreground line-clamp-2">
                {show.description}
              </p>
            )}
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
      <ApproveShowDialog
        show={show}
        open={showApproveDialog}
        onOpenChange={setShowApproveDialog}
      />
      <RejectShowDialog
        show={show}
        open={showRejectDialog}
        onOpenChange={setShowRejectDialog}
      />
    </>
  )
}
