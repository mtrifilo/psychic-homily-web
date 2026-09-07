'use client'

import { useState } from 'react'
import Link from 'next/link'
// Deep-imported, not through `@/components/shared` — see the note in
// SceneRooms.tsx and features/scenes/components/index.ts (PSY-1772).
import { BracketLink } from '@/components/shared/BracketLink'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { hasRenderableMusic } from '@/lib/musicAvailability'
import { useSceneArtists } from '../hooks'
import { rosterUpcomingLine } from '../sceneRosterLine'
import {
  EntityNameLink,
  EntityNameList,
  SCENE_PROSE_LINK_CLASS,
  SceneSectionHeading,
} from './sceneChrome'
import type { SceneArtist, SceneDetail, SceneRepresentativeEmbed } from '../types'

/**
 * The bands based here: named in one line, and once the reader asks for the
 * whole roster, dated.
 *
 * The two shapes carry different amounts because they answer different
 * questions. The preview is an identity line — who is from here — and stays
 * names and nothing else, so it sits under a calendar without reading as a
 * count of what is coming up. The expanded list is the roster itself, which a
 * reader opens to browse, and there each band states what it has booked.
 *
 * Nothing all-time is printed in either. `show_count` counts every approved
 * show a band has ever played anywhere and `is_active` stays true for months
 * after a last gig; only `upcoming_show_count` and `next_show` answer "can I go
 * see them", which is the question this page is built around.
 *
 * The roster lists every band BASED in the metro, which is a different set from
 * "bands playing here soon": London has 197 upcoming shows and zero based-here
 * artists. That is why this section hides at zero rather than apologising — an
 * empty roster is a fact about where bands live, and a titled section over a
 * 130px collapsed stub was the shape the sparse matrix retired.
 */

/** The first page. Past this, the reader asks for the rest. */
const ROSTER_PAGE_SIZE = 10

/** The endpoint's own ceiling (`maximum: 100` on `limit`). */
const ROSTER_MAX = 100

/**
 * The scene's one open player.
 *
 * OPEN, never behind a disclosure, which is the standing rule for every embed
 * in this product. ONE of them, not one per band: ten players is ten iframes
 * and ten `/api/bandcamp/album-id` resolves on a page whose point is the
 * calendar above it.
 *
 * The caption names the band so the player is attributed without waiting on a
 * third-party iframe to paint, and links it because the pick can come from
 * outside the names line above, which is a page of the roster while the pick is
 * scoped to all of it. It says nothing about the release:
 * `artists.bandcamp_embed_url` is fill-when-empty, and the manual and
 * profile-resolved writers use the same column, so the field establishes no
 * release recency.
 *
 * The link carries its own underline. The caption's type is mono micro-caps in
 * the muted tone, where `EntityNameLink`'s default weight bump is nearly
 * invisible and its underline waits for a hover a touch reader never performs.
 */
function RosterEmbed({ embed }: { embed: SceneRepresentativeEmbed }) {
  return (
    <div className="mt-3 max-w-2xl">
      <MusicEmbed
        bandcampAlbumUrl={embed.embed_url}
        artistName={embed.artist_name}
        compact
      />
      <p className="mt-1 font-mono text-[11px] uppercase tracking-widest text-muted-foreground">
        Bandcamp{' · '}
        <EntityNameLink
          name={embed.artist_name}
          slug={embed.artist_slug}
          basePath="/artists"
          className={SCENE_PROSE_LINK_CLASS}
          unlinkedClassName=""
        />
      </p>
    </div>
  )
}

/**
 * One band on the expanded roster: its name, and under it what it has booked.
 *
 * A band with nothing booked is a NAME AND NOTHING ELSE. No `0 upcoming`, no
 * "no upcoming shows" — a quiet band is the ordinary state of most of a roster,
 * and forty rows each declaring their own emptiness is the shape this page
 * removes rather than adds.
 *
 * The date links to the show and the room to its own page, so the line is the
 * reader's way into both. Each degrades to plain text on its own: show and
 * venue slugs are both nullable, and neither half may build an href that
 * resolves to an index page instead of the thing named (PSY-1754).
 */
function RosterRow({ artist }: { artist: SceneArtist }) {
  const line = rosterUpcomingLine(artist)
  const day =
    line?.day && line.dayHref ? (
      <Link href={line.dayHref} className={SCENE_PROSE_LINK_CLASS}>
        {line.day}
      </Link>
    ) : (
      line?.day
    )
  const venue = line?.venue ? (
    <EntityNameLink
      name={line.venue.name}
      slug={line.venue.slug}
      basePath="/venues"
      className={SCENE_PROSE_LINK_CLASS}
      unlinkedClassName=""
    />
  ) : null

  return (
    <li className="border-b border-border/40 py-2 last:border-b-0">
      <EntityNameLink name={artist.name} slug={artist.slug} basePath="/artists" />
      {line && (
        <p className="mt-0.5 font-mono text-xs text-muted-foreground">
          {line.countText}
          {/* The clause drops whole when the payload can support neither half of
              it, and re-words to `next at {room}` when it can name the room but
              not date it: a room with a comma in front of nothing would read as
              a date the row does not have. */}
          {(day || venue) && (
            <>
              {' · next '}
              {day}
              {day && venue && ', '}
              {!day && venue && 'at '}
              {venue}
            </>
          )}
        </p>
      )}
    </li>
  )
}

export function SceneRoster({
  scene,
  anchorId,
}: {
  scene: SceneDetail
  /** The mobile graph teaser's link-out target (PSY-1472). */
  anchorId?: string
}) {
  const [limit, setLimit] = useState(ROSTER_PAGE_SIZE)
  // The hook retains the previous page across a limit change on its own, so the
  // reader keeps looking at the list while the rest of it arrives.
  const { data, isLoading, isError } = useSceneArtists({ slug: scene.slug, limit })
  // The first page is read separately, and is what the player and the failed
  // widening both fall back on.
  //
  // `representative_embed` is derived from the rows the response carries. A
  // wider page usually nominates the same band, because it extends the same
  // ordering, but not when the narrow page held no usable embed and the pick
  // came from the roster-wide fallback: a wider page can then answer for
  // itself and name someone else. Reading the player off the live response
  // would swap the iframe's src under a reader who had pressed play, purely
  // because they asked for more names. Until the reader widens, this is the
  // same cache entry as the query above and costs no request; after, it is a
  // second entry with a live observer, so the page holds two.
  const { data: firstPage } = useSceneArtists({
    slug: scene.slug,
    limit: ROSTER_PAGE_SIZE,
  })

  // A wider fetch that FAILS falls back to the page already on screen rather
  // than emptying the section. `placeholderData` cannot cover this: it applies
  // only while a query is pending, so an errored widening leaves `data`
  // undefined, and the guard below would unmount a section the reader is
  // reading, along with an embed that may be playing.
  const page = data ?? (isError ? firstPage : undefined)
  const artists = page?.artists ?? []

  // Loading and empty both render nothing. A heading that appears, empties and
  // disappears is worse than one that arrives once, and a scene with no
  // based-here bands has no section to draw.
  if (isLoading || artists.length === 0) return null

  const total = page?.total ?? artists.length
  const withheld = Math.max(total - artists.length, 0)
  // What one more fetch could actually put on the page — capped, because the
  // endpoint is. The control is LABELLED from this rather than from `total`:
  // a 340-band roster offering "Show all 340" and then delivering 100 breaks
  // its promise on the click, which is worse than naming the ceiling up front.
  const expandTo = Math.min(total, ROSTER_MAX)
  // Withdrawn while the widening is failing: `limit` already holds the value
  // the control would set, so pressing it again would change no state and fetch
  // nothing. The disclosure line below takes over and states the shortfall.
  const canExpand = !isError && withheld > 0 && artists.length < expandTo

  const embed = firstPage?.representative_embed

  // The shape follows the reader's REQUEST, not the response, and there is only
  // ever one rule: a reader who has asked for the whole roster is reading the
  // expanded list. A widening that then fails draws the rows it does have in
  // that same shape, with the shortfall stated below them, rather than snapping
  // back to a preview the reader has already left.
  const expanded = limit > ROSTER_PAGE_SIZE

  return (
    <section id={anchorId} className="scroll-mt-20 border-t border-border pt-4">
      <SceneSectionHeading title="Bands based here" note={total} />

      {expanded ? (
        <ul className="mt-2">
          {artists.map(artist => (
            <RosterRow key={artist.id} artist={artist} />
          ))}
        </ul>
      ) : (
        <p className="mt-2 text-sm leading-relaxed">
          <EntityNameList items={artists} basePath="/artists" />
          {/* The ellipsis rides with the CONTROL, not with the elision: it marks
              names one click away. Names withheld by the endpoint's ceiling are
              past any control, and the line below names the shown count and the
              total instead. Decorative either way, so it is hidden from assistive
              tech, which reads the control's own label. */}
          {canExpand && (
            <>
              <span aria-hidden="true" className="text-muted-foreground">
                {' … '}
              </span>
              <BracketLink
                label={
                  expandTo === total
                    ? `Show all ${total} →`
                    : `Show ${expandTo} of ${total} →`
                }
                onClick={() => setLimit(expandTo)}
              />
            </>
          )}
        </p>
      )}

      {/* Only reachable once the ceiling is the thing withholding bands, so it
          states that rather than offering a control that cannot deliver. */}
      {withheld > 0 && !canExpand && (
        <p className="mt-2 font-mono text-[11px] text-muted-foreground">
          Showing {artists.length} of {total} bands based in {scene.city}
        </p>
      )}

      {/* One gate, so the caption and the player are never rendered apart.

          `hasRenderableMusic` is NECESSARY, not sufficient: it host-anchors the
          stored URL, which is every check available before MusicEmbed mounts
          and asks the resolver. A value that clears it and then resolves to no
          player degrades to MusicEmbed's Bandcamp link when the URL names a
          release, and to nothing when it does not, and in that last case the
          caption stands over blank space. Closing that needs the caption inside
          MusicEmbed, where it could see the resolve. */}
      {embed && hasRenderableMusic({ bandcampAlbumUrl: embed.embed_url }) && (
        <RosterEmbed embed={embed} />
      )}
    </section>
  )
}
