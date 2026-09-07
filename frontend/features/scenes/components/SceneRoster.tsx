'use client'

import { useState } from 'react'
// Deep-imported, not through `@/components/shared` — see the note in
// SceneRooms.tsx and features/scenes/components/index.ts (PSY-1772).
import { BracketLink } from '@/components/shared/BracketLink'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { hasRenderableMusic } from '@/lib/musicAvailability'
import { useSceneArtists } from '../hooks'
import { EntityNameLink, EntityNameList, SceneSectionHeading } from './sceneChrome'
import type { SceneDetail, SceneRepresentativeEmbed } from '../types'

/**
 * The bands based here, named in one line, with one of them playing.
 *
 * Names and nothing else. The payload's other per-band fields are all-time or
 * derived figures, and this module sits under a calendar, where any of them
 * would read as a count of what is coming up.
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
 * The link carries its own underline: the caption's type is mono micro-caps in
 * the muted tone, so an unstyled link inside it is indistinguishable from the
 * label beside it until a pointer hovers, which a touch reader never does.
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
          className="underline underline-offset-4 hover:text-primary"
          unlinkedClassName=""
        />
      </p>
    </div>
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
  const { data, isLoading } = useSceneArtists({ slug: scene.slug, limit })
  // The player is read from the FIRST page's answer, never the current one.
  // `representative_embed` is derived from the rows the response carries, so a
  // wider page can nominate a different band; taking it from the live response
  // would swap the iframe's src under a reader who had pressed play, purely
  // because they asked for more names. At the first page size this is the same
  // cache entry as the query above, so it costs no extra request.
  const { data: firstPage } = useSceneArtists({
    slug: scene.slug,
    limit: ROSTER_PAGE_SIZE,
  })

  const artists = data?.artists ?? []

  // Loading and empty both render nothing. A heading that appears, empties and
  // disappears is worse than one that arrives once, and a scene with no
  // based-here bands has no section to draw.
  if (isLoading || artists.length === 0) return null

  const total = data?.total ?? artists.length
  const withheld = Math.max(total - artists.length, 0)
  // What one more fetch could actually put on the page — capped, because the
  // endpoint is. The control is LABELLED from this rather than from `total`:
  // a 340-band roster offering "Show all 340" and then delivering 100 breaks
  // its promise on the click, which is worse than naming the ceiling up front.
  const expandTo = Math.min(total, ROSTER_MAX)
  const canExpand = withheld > 0 && artists.length < expandTo

  const embed = firstPage?.representative_embed

  return (
    <section id={anchorId} className="scroll-mt-20 border-t border-border pt-4">
      <SceneSectionHeading title="Bands based here" note={total} />

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

      {/* Only reachable once the ceiling is the thing withholding bands, so it
          states that rather than offering a control that cannot deliver. */}
      {withheld > 0 && !canExpand && (
        <p className="mt-2 font-mono text-[11px] text-muted-foreground">
          Showing {artists.length} of {total} bands based in {scene.city}
        </p>
      )}

      {/* One gate for the player and its caption, so the two cannot disagree
          about whether there is anything here.

          `hasRenderableMusic` is NECESSARY, not sufficient: it host-anchors the
          stored URL, which is every check that can be made before MusicEmbed
          mounts and asks the resolver. A value that clears it and then resolves
          to no player degrades to MusicEmbed's Bandcamp link when the URL names
          a release, and to nothing when it does not, in which case the caption
          is left over blank space. Closing that needs the caption to live inside
          MusicEmbed, which is where the same residual sits on the atlas
          preview's Listen heading. */}
      {embed && hasRenderableMusic({ bandcampAlbumUrl: embed.embed_url }) && (
        <RosterEmbed embed={embed} />
      )}
    </section>
  )
}
