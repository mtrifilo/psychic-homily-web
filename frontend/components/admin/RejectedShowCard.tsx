'use client'

import { Calendar, MapPin, XCircle } from 'lucide-react'
import type { ShowResponse } from '@/lib/types/show'
import { Badge } from '@/components/ui/badge'

interface RejectedShowCardProps {
  show: ShowResponse
}

function formatShortDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function formatTime(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

export function RejectedShowCard({ show }: RejectedShowCardProps) {
  const venue = show.venues[0]
  const artistNames = show.artists.map(a => a.name).join(', ')

  return (
    <div className="flex items-start gap-4 p-3 rounded-lg border border-destructive/20 bg-card/50 hover:bg-card/80 transition-colors">
      {/* Date column */}
      <div className="flex-shrink-0 text-center min-w-[60px]">
        <div className="text-sm font-medium">{formatShortDate(show.event_date).split(',')[0].split(' ')[0]}</div>
        <div className="text-lg font-bold">{new Date(show.event_date).getDate()}</div>
        <div className="text-xs text-muted-foreground">{formatTime(show.event_date)}</div>
      </div>

      {/* Main content */}
      <div className="flex-1 min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <h3 className="font-medium truncate">
            {show.title || artistNames || 'Untitled Show'}
          </h3>
          <Badge
            variant="outline"
            className="text-destructive border-destructive/50 gap-1 shrink-0 text-xs py-0"
          >
            <XCircle className="h-3 w-3" />
            Rejected
          </Badge>
        </div>

        {/* Venue & Artists on same line */}
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          {venue && (
            <span className="flex items-center gap-1 truncate">
              <MapPin className="h-3 w-3 shrink-0" />
              {venue.name}, {venue.city}
            </span>
          )}
          {artistNames && !show.title && (
            <span className="truncate">• {artistNames}</span>
          )}
          {artistNames && show.title && (
            <span className="truncate">• {artistNames}</span>
          )}
        </div>

        {/* Rejection reason inline */}
        {show.rejection_reason && (
          <p className="text-xs text-destructive/80 truncate">
            <span className="font-medium">Reason:</span> {show.rejection_reason}
          </p>
        )}
      </div>

      {/* Rejection date */}
      <div className="flex-shrink-0 text-xs text-muted-foreground text-right">
        <Calendar className="h-3 w-3 inline mr-1" />
        {formatShortDate(show.updated_at)}
      </div>
    </div>
  )
}
