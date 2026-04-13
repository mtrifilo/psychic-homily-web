'use client'

import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getRotationStatusLabel, getRotationStatusColor } from '../types'
import type { RadioPlay } from '../types'

interface RadioPlayRowProps {
  play: RadioPlay
  showPosition?: boolean
}

/**
 * Format an ISO timestamp string as a short time like "6:32 AM".
 * Returns null if the input is missing or unparseable.
 */
function formatAirTimestamp(isoString: string | null): string | null {
  if (!isoString) return null
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return null
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/** Small dot indicating an entity exists in the knowledge graph */
function CatalogDot() {
  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className="inline-block w-1.5 h-1.5 rounded-full bg-primary/70 shrink-0"
            aria-label="In our catalog"
          />
        </TooltipTrigger>
        <TooltipContent side="top" className="text-xs">
          In our catalog
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function RadioPlayRow({ play, showPosition = true }: RadioPlayRowProps) {
  const airTime = formatAirTimestamp(play.air_timestamp)

  return (
    <div className="px-3 py-2.5 hover:bg-muted/30 transition-colors rounded-md">
      <div className="flex items-start gap-3">
        {/* Position number and air timestamp */}
        {showPosition && (
          <div className="shrink-0 pt-0.5 text-right">
            <span className="text-xs text-muted-foreground tabular-nums w-6 inline-block">
              {play.position}
            </span>
            {airTime && (
              <div className="text-[10px] text-muted-foreground/60 tabular-nums whitespace-nowrap">
                {airTime}
              </div>
            )}
          </div>
        )}

        {/* Main content */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {/* Artist */}
            {play.artist_slug ? (
              <span className="inline-flex items-center gap-1">
                <CatalogDot />
                <Link
                  href={`/artists/${play.artist_slug}`}
                  className="text-sm font-medium text-primary/90 hover:text-primary transition-colors"
                >
                  {play.artist_name}
                </Link>
              </span>
            ) : (
              <span className="text-sm font-medium">{play.artist_name}</span>
            )}

            {/* Separator */}
            {play.track_title && (
              <span className="text-muted-foreground">-</span>
            )}

            {/* Track title */}
            {play.track_title && (
              <span className="text-sm text-muted-foreground truncate">
                {play.track_title}
              </span>
            )}
          </div>

          {/* Album / Label row */}
          <div className="flex items-center gap-2 flex-wrap mt-0.5">
            {play.album_title && (
              <>
                {play.release_slug ? (
                  <span className="inline-flex items-center gap-1">
                    <CatalogDot />
                    <Link
                      href={`/releases/${play.release_slug}`}
                      className="text-xs text-foreground/70 hover:text-foreground transition-colors"
                    >
                      {play.album_title}
                    </Link>
                  </span>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {play.album_title}
                  </span>
                )}
              </>
            )}

            {play.album_title && play.label_name && (
              <span className="text-xs text-muted-foreground/50">/</span>
            )}

            {play.label_name && (
              <>
                {play.label_slug ? (
                  <span className="inline-flex items-center gap-1">
                    <CatalogDot />
                    <Link
                      href={`/labels/${play.label_slug}`}
                      className="text-xs text-foreground/70 hover:text-foreground transition-colors"
                    >
                      {play.label_name}
                    </Link>
                  </span>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {play.label_name}
                  </span>
                )}
              </>
            )}

            {play.release_year && (
              <span className="text-xs text-muted-foreground/50 tabular-nums">
                ({play.release_year})
              </span>
            )}
          </div>

          {/* DJ comment */}
          {play.dj_comment && (
            <p className="text-xs italic text-muted-foreground/70 mt-1">
              &ldquo;{play.dj_comment}&rdquo;
            </p>
          )}
        </div>

        {/* Badges */}
        <div className="flex items-center gap-1.5 shrink-0">
          {play.is_new && (
            <Badge className="bg-green-500/15 text-green-400 border-green-500/20 text-[10px] px-1.5 py-0">
              NEW
            </Badge>
          )}
          {play.rotation_status && play.rotation_status !== 'library' && (
            <Badge
              className={`${getRotationStatusColor(play.rotation_status)} text-[10px] px-1.5 py-0`}
            >
              {getRotationStatusLabel(play.rotation_status)}
            </Badge>
          )}
          {play.is_live_performance && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              LIVE
            </Badge>
          )}
          {play.is_request && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              REQ
            </Badge>
          )}
        </div>
      </div>
    </div>
  )
}
