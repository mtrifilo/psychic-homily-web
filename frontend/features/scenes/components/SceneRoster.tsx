'use client'

import { useState } from 'react'
// Deep-imported, not through `@/components/shared` — see the note in
// SceneRooms.tsx and features/scenes/components/index.ts (PSY-1772).
import { BracketLink } from '@/components/shared/BracketLink'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { hasRenderableMusic } from '@/lib/musicAvailability'
import { useSceneArtists } from '../hooks'
import { EntityNameLink, SceneSectionHeading } from './sceneChrome'
import type { SceneArtist, SceneDetail, SceneRepresentativeEmbed } from '../types'

/**
 * The bands based here, named in one line, with one of them playing.
 *
 * The roster is a NAMES list, not a row list. `GET /scenes/{slug}/artists`
 * carries `show_count` (approved shows all time, anywhere) and `is_active`, and
 * neither is an upcoming figure; the locked front-page frame spends its height
 * on the calendar above and gives this module a preview line, so the names are
 * what it prints and the per-band figures live on the artist pages the names
 * link to.
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
 * `A · B · C`, each band linked when it has a slug.
 *
 * The separator sits in its own muted span rather than inside the name, so a
 * middot never lands inside a link's hit area or its accessible name.
 */
function RosterNames({ artists }: { artists: SceneArtist[] }) {
  return (
    <>
      {artists.map((artist, i) => (
        <span key={artist.id}>
          {i > 0 && <span className="text-muted-foreground"> · </span>}
          <EntityNameLink name={artist.name} slug={artist.slug} basePath="/artists" />
        </span>
      ))}
    </>
  )
}

/**
 * The scene's one open player.
 *
 * OPEN, never behind a disclosure, which is the standing rule for every embed
 * in this product. ONE of them, not one per band: ten players is ten iframes
 * and ten `/api/bandcamp/album-id` resolves on a page whose point is the
 * calendar above it.
 *
 * The band is the backend's `representative_embed` pick (PSY-1294), computed
 * over the FULL metro roster rather than the fetched page, so the scene's only
 * embed-having band cannot fall below the preview above. That band is therefore
 * often NOT one of the names in that line, which is why the caption names and
 * links it rather than leaving the player to speak for itself.
 *
 * The caption states the platform and the band, and nothing about the release:
 * `artists.bandcamp_embed_url` is fill-when-empty (a newer release never
 * refreshes an already-set value) and can equally be a manual or
 * profile-resolved value, so "latest release" is a claim the field does not
 * support.
 */
function RosterEmbed({ embed }: { embed: SceneRepresentativeEmbed }) {
  return (
    <div className="mt-3 max-w-2xl">
      <MusicEmbed
        bandcampAlbumUrl={embed.embed_url}
        artistName={embed.artist_name}
        compact
      />
      <p className="font-mono text-[11px] uppercase tracking-widest text-muted-foreground">
        Bandcamp{' · '}
        <EntityNameLink
          name={embed.artist_name}
          slug={embed.artist_slug}
          basePath="/artists"
          className="hover:underline"
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

  // Populated on the first page only (offset 0), which is the only page this
  // component asks for. Gated on what MusicEmbed will actually produce, not on
  // the field existing: a stored value that the renderer refuses would
  // otherwise leave a caption over nothing (PSY-1966).
  const embed = data?.representative_embed

  return (
    <section id={anchorId} className="scroll-mt-20 border-t border-border pt-4">
      <SceneSectionHeading title="Bands based here" note={total} />

      <p className="mt-2 text-sm leading-relaxed">
        <RosterNames artists={artists} />
        {/* The ellipsis is the truncation, and the control is what resolves it,
            so they travel together: without the control there is nothing for a
            reader to do about the elision and the page should not imply there
            is. */}
        {canExpand && (
          <>
            <span className="text-muted-foreground">{' … '}</span>
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

      {embed && hasRenderableMusic({ bandcampAlbumUrl: embed.embed_url }) && (
        <RosterEmbed embed={embed} />
      )}
    </section>
  )
}
