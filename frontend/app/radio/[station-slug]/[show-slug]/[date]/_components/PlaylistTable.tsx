'use client'

import { Fragment } from 'react'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { DenseTable } from '@/components/shared/DenseTable'
import { formatPlayTime, getRotationStatusColor } from '@/features/radio'
import type { RadioPlay } from '@/features/radio'

interface PlaylistTableProps {
  plays: RadioPlay[]
}

/**
 * Short mono-tag labels for rotation_status in the NOTES column (the long
 * "Heavy Rotation" labels don't fit the dense register). Unknown statuses
 * fall back to the raw value, uppercased.
 */
const ROTATION_TAG_LABELS: Record<string, string> = {
  heavy: 'HEAVY',
  medium: 'MEDIUM',
  light: 'LIGHT',
  recommended_new: 'REC NEW',
}

function rotationTagLabel(status: string): string {
  return ROTATION_TAG_LABELS[status] ?? status.replace(/_/g, ' ').toUpperCase()
}

const BADGE_CLASSES = 'font-mono text-[10px] px-1.5 py-0'

/**
 * The playlist page's full-width record-collector track table (PSY-1051,
 * locked PSY-1049 decision 5): TIME · ARTIST · TRACK · ALBUM · LABEL · YEAR ·
 * NOTES. Matched artists (artist_id) render as an orange link with a ● dot;
 * unmatched as plain text with ○ — explained by the legend row underneath.
 * TIME comes from air_timestamp where the feed carries one and is otherwise
 * blank (never fabricated); position keeps the row order. dj_comment renders
 * as an indented full-width sub-row under its track.
 *
 * Explicitly NOT here in v1 (locked): airbreak divider rows (no play_type
 * data), [suggest a match] on unmatched rows (PSY-1052 spike), TIME
 * deep-links into a seekable player.
 */
export function PlaylistTable({ plays }: PlaylistTableProps) {
  return (
    <div>
      <DenseTable>
        <thead>
          <tr>
            <th className="w-20">Time</th>
            <th className="w-[22%]">Artist</th>
            <th>Track</th>
            <th className="w-[18%]">Album</th>
            <th className="w-[15%]">Label</th>
            <th className="w-14">Year</th>
            <th className="w-28">Notes</th>
          </tr>
        </thead>
        <tbody>
          {plays.map(play => {
            const time = formatPlayTime(play.air_timestamp)
            const matched = play.artist_id != null

            return (
              <Fragment key={play.id}>
                <tr className={play.dj_comment ? 'border-b-0!' : undefined}>
                  <td className="whitespace-nowrap font-mono text-xs text-primary/90 tabular-nums">
                    {time ?? ''}
                  </td>
                  <td>
                    <span className="inline-flex items-baseline gap-1.5">
                      <span
                        className={matched ? 'text-primary' : 'text-muted-foreground/60'}
                        aria-hidden="true"
                      >
                        {matched ? '●' : '○'}
                      </span>
                      {play.artist_slug ? (
                        <Link
                          href={`/artists/${play.artist_slug}`}
                          className="font-medium text-primary hover:text-primary/80 transition-colors"
                        >
                          {play.artist_name}
                        </Link>
                      ) : (
                        <span className="font-medium text-foreground">
                          {play.artist_name}
                        </span>
                      )}
                    </span>
                  </td>
                  <td className="text-foreground">{play.track_title}</td>
                  <td>
                    {play.album_title &&
                      (play.release_slug ? (
                        <Link
                          href={`/releases/${play.release_slug}`}
                          className="text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {play.album_title}
                        </Link>
                      ) : (
                        <span className="text-muted-foreground">{play.album_title}</span>
                      ))}
                  </td>
                  <td>
                    {play.label_name &&
                      (play.label_slug ? (
                        <Link
                          href={`/labels/${play.label_slug}`}
                          className="text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {play.label_name}
                        </Link>
                      ) : (
                        <span className="text-muted-foreground">{play.label_name}</span>
                      ))}
                  </td>
                  <td className="tabular-nums text-muted-foreground">
                    {play.release_year}
                  </td>
                  <td>
                    <span className="inline-flex flex-wrap items-center gap-1">
                      {play.is_live_performance && (
                        <Badge
                          variant="outline"
                          className={`${BADGE_CLASSES} border-primary/40 text-primary`}
                        >
                          LIVE
                        </Badge>
                      )}
                      {play.is_new && (
                        <Badge variant="accent" className={BADGE_CLASSES}>
                          NEW
                        </Badge>
                      )}
                      {play.rotation_status && play.rotation_status !== 'library' && (
                        <Badge
                          className={`${BADGE_CLASSES} ${getRotationStatusColor(play.rotation_status)}`}
                        >
                          {rotationTagLabel(play.rotation_status)}
                        </Badge>
                      )}
                      {play.is_request && (
                        <Badge variant="outline" className={BADGE_CLASSES}>
                          REQ
                        </Badge>
                      )}
                    </span>
                  </td>
                </tr>
                {play.dj_comment && (
                  <tr>
                    <td aria-hidden="true" />
                    <td colSpan={6} className="pt-0!">
                      <span className="font-mono text-xs text-muted-foreground">
                        <span aria-hidden="true">└ </span>
                        {play.dj_comment}
                      </span>
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </DenseTable>

      {/* Legend (the [suggest a match] action is the PSY-1052 spike — v1 is legend-only) */}
      <p className="mt-3 font-mono text-xs text-muted-foreground">
        <span className="text-primary" aria-hidden="true">
          ●
        </span>{' '}
        <span>linked to artist page</span>
        <span className="mx-2" aria-hidden="true">
          ·
        </span>
        <span aria-hidden="true">○</span> <span>not matched yet</span>
      </p>
    </div>
  )
}
