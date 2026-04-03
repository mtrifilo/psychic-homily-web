'use client'

import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { getRotationStatusLabel, getRotationStatusColor } from '../types'
import type { RadioPlay } from '../types'

interface RadioPlayRowProps {
  play: RadioPlay
  showPosition?: boolean
}

export function RadioPlayRow({ play, showPosition = true }: RadioPlayRowProps) {
  return (
    <div className="px-3 py-2.5 hover:bg-muted/30 transition-colors rounded-md">
      <div className="flex items-start gap-3">
        {/* Position number */}
        {showPosition && (
          <span className="text-xs text-muted-foreground tabular-nums w-6 text-right shrink-0 pt-0.5">
            {play.position}
          </span>
        )}

        {/* Main content */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {/* Artist */}
            {play.artist_slug ? (
              <Link
                href={`/artists/${play.artist_slug}`}
                className="text-sm font-medium hover:text-primary transition-colors"
              >
                {play.artist_name}
              </Link>
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
                  <Link
                    href={`/releases/${play.release_slug}`}
                    className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {play.album_title}
                  </Link>
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
                  <Link
                    href={`/labels/${play.label_slug}`}
                    className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {play.label_name}
                  </Link>
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
