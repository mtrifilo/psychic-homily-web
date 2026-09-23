'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { BracketLink } from '@/components/shared/BracketLink'
import { DenseTable } from '@/components/shared/DenseTable'
import {
  AirDateCellContent,
  airDateCellText,
  ArtistHops,
  isLiveNow,
  previewToHops,
} from '@/features/radio'
import type { RadioEpisodeListItem } from '@/features/radio'
import { unanchoredLinkHref } from '@/lib/socialLinks'
import { cn } from '@/lib/utils'

interface EpisodeArchiveTableProps {
  episodes: RadioEpisodeListItem[]
  stationSlug: string
  showSlug: string
}

/**
 * The show page's playlist archive (PSY-1051): one reverse-chron episode
 * table — date · episode title · played preview · tracks · [mp3]. The show
 * page IS its archive (WFMU model, locked PSY-1049 decision 4); there is no
 * separate episodes sub-page. Episode previews come from the PSY-1048
 * artist_preview extension on the episodes list — no per-row detail fetch.
 *
 * ONE element tree across the breakpoint, reflowed by CSS rather than swapped,
 * so no row's text is in the document twice. From `sm` up it is the
 * five-column table. Below `sm` the header row is visually hidden (it stays
 * for assistive tech) and each row is a three-line grid: date with tracks and
 * archive, then the title, then the played artists at full width, wrapping.
 * A line with nothing to say (no title, no played artists) is dropped there.
 */
export function EpisodeArchiveTable({
  episodes,
  stationSlug,
  showSlug,
}: EpisodeArchiveTableProps) {
  return (
    // Explicit roles: below `sm` the display of the table, its groups, rows and
    // cells is overridden, which strips the implicit table semantics with it.
    <DenseTable role="table" className="block sm:table">
      <thead role="rowgroup" className="max-sm:sr-only">
        <tr role="row">
          <th role="columnheader" scope="col" className="w-28">
            Date
          </th>
          <th role="columnheader" scope="col" className="w-48">
            Episode
          </th>
          <th role="columnheader" scope="col">
            Played
          </th>
          <th role="columnheader" scope="col" className="w-16 text-right">
            Tracks
          </th>
          <th role="columnheader" scope="col" className="w-20 text-right">
            <span className="sr-only">Archive</span>
          </th>
        </tr>
      </thead>
      <tbody role="rowgroup" className="block sm:table-row-group">
        {episodes.map(episode => {
          // Upcoming rows aren't linkable — there's no playlist yet, so a link
          // would lead to an empty, not-yet-aired page (PSY-1205).
          const episodeUrl = episode.is_upcoming
            ? undefined
            : `/radio/${stationSlug}/${showSlug}/${episode.air_date}`
          const isLive = isLiveNow(episode.starts_at, episode.ends_at)
          // Same gate as the episode page's Play-archive button, so one stored
          // value cannot link in the table and not on the page it links to.
          // BracketLink's own floor is a scheme prefix test on the raw string;
          // this decides the href BEFORE the primitive sees it, so a value the
          // gate refuses renders no bracket at all rather than a greyed one.
          const archiveHref = unanchoredLinkHref(episode.archive_url)
          // Same viewer-local date the cell shows — accessible names must not
          // announce a different day than the rendered text (PSY-1306).
          const cellDate = airDateCellText(episode.starts_at, episode.ends_at, episode.air_date, {
            withYear: true,
          }).dateLine
          const hops = previewToHops(episode.artist_preview)
          const archiveStatus = episode.is_upcoming ? (
            // Not yet aired (PSY-1205): label it rather than linking to an
            // empty, aired-looking [mp3] archive page.
            <span className="text-muted-foreground">upcoming</span>
          ) : isLive ? (
            <span className="text-primary">
              <span aria-hidden="true">●</span> live
            </span>
          ) : archiveHref ? (
            <BracketLink
              label="mp3"
              href={archiveHref}
              external
              // text-xs beats BracketLink's text-sm base; the enclosing
              // <td> already supplies font-mono. Adopts the primitive's
              // tight [mp3], replacing the old padded [ mp3 ].
              className="text-xs text-primary hover:text-primary/80"
              // Dates the row, because every row's bracket reads "mp3";
              // the new-tab half is BracketLink's to append.
              ariaLabel={`Listen to the ${cellDate} archive`}
            />
          ) : null

          return (
            <tr key={episode.id} role="row" className={mobileRowClass}>
              {/* PSY-1306: viewer-local date (+ air-time block) — the same
                  AirDateCellContent treatment as the playlists feeds, with the
                  year (archives span years). */}
              <td role="cell" className="col-start-1 row-start-1 whitespace-nowrap align-top">
                <MaybeLink
                  href={episodeUrl}
                  linkedClassName="font-mono text-xs uppercase text-primary hover:text-primary/80 transition-colors"
                  plainClassName="font-mono text-xs uppercase text-muted-foreground"
                >
                  <AirDateCellContent
                    startsAt={episode.starts_at}
                    endsAt={episode.ends_at}
                    airDate={episode.air_date}
                    withYear
                  />
                </MaybeLink>
              </td>
              <td
                role="cell"
                className={cn('col-span-full wrap-anywhere', !episode.title && collapsedLineClass)}
              >
                {episode.title ? (
                  <MaybeLink
                    href={episodeUrl}
                    linkedClassName="text-primary hover:text-primary/80 transition-colors"
                    plainClassName="text-muted-foreground"
                  >
                    {episode.title}
                  </MaybeLink>
                ) : (
                  <span className="text-muted-foreground/50" aria-hidden="true">
                    —
                  </span>
                )}
              </td>
              {/* Wraps at every width: the played names are the row's bridge
                  into the artist graph, so they are never clipped.
                  `wrap-anywhere` keeps one unbreakable name from widening the
                  table past its container. */}
              <td
                role="cell"
                className={cn('col-span-full wrap-anywhere', hops.length === 0 && collapsedLineClass)}
              >
                {hops.length > 0 ? (
                  <>
                    {/* The column header is not visible in the stacked row, so
                        the line labels itself; assistive tech has the header. */}
                    <span
                      aria-hidden="true"
                      className="mr-2 font-mono text-[10px] uppercase text-muted-foreground sm:hidden"
                    >
                      Played
                    </span>
                    <ArtistHops
                      hops={hops}
                      className="text-muted-foreground [&_a]:hover:text-primary"
                    />
                  </>
                ) : (
                  <span className="text-muted-foreground/50">&nbsp;</span>
                )}
              </td>
              <td
                role="cell"
                className="col-start-2 row-start-1 whitespace-nowrap text-right tabular-nums text-muted-foreground max-sm:font-mono max-sm:text-xs"
              >
                <span>{episode.play_count}</span>
                {/* The column header carries the unit at table widths; the
                    stacked row has no visible header, so the cell carries it. */}
                <span className="sm:hidden">
                  {episode.play_count === 1 ? ' track' : ' tracks'}
                </span>
              </td>
              <td
                role="cell"
                className="col-start-3 row-start-1 text-right whitespace-nowrap font-mono text-xs"
              >
                {/* The gap before the archive status lives on the status, not
                    the grid, so a row without one keeps its tracks flush right. */}
                {archiveStatus && <span className="max-sm:pl-3">{archiveStatus}</span>}
              </td>
            </tr>
          )
        })}
      </tbody>
    </DenseTable>
  )
}

/**
 * Below `sm`: a three-column grid (date, tracks, archive status) whose title
 * and played cells span the full width on their own lines, with the table's
 * cell padding dropped for the row's own. From `sm` up: a table row.
 */
const mobileRowClass =
  'grid grid-cols-[minmax(0,1fr)_auto_auto] items-baseline gap-y-1 py-2 max-sm:[&>td]:p-0 sm:table-row'

/**
 * Drops an empty cell's line from the stacked row while keeping the cell in
 * the table, so every row still has one cell per column header for assistive
 * tech. `display: none` would remove the cell and shift the rest under the
 * wrong headers.
 */
const collapsedLineClass = 'max-sm:sr-only'

/**
 * Renders children inside a Link when `href` is set, else as a plain span. Lets
 * the archive show an upcoming (non-linkable) row's date/title as text — one
 * boolean (`linkable`) instead of a duplicated link-vs-text branch per cell.
 */
function MaybeLink({
  href,
  linkedClassName,
  plainClassName,
  children,
}: {
  href: string | undefined
  linkedClassName: string
  plainClassName: string
  children: ReactNode
}) {
  return href ? (
    <Link href={href} className={linkedClassName}>
      {children}
    </Link>
  ) : (
    <span className={plainClassName}>{children}</span>
  )
}
