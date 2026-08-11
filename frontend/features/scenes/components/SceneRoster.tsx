'use client'

import { useState } from 'react'
import Link from 'next/link'
import { BracketLink, MusicEmbed } from '@/components/shared'
import { useSceneArtists } from '../hooks'
import type { SceneArtist, SceneDetail } from '../types'

/**
 * The bands based here, with their music playing.
 *
 * The page already fetched `bandcamp_embed_url` on every roster call and threw
 * it away, so a scene page could play music at zero backend cost and did not.
 * Players render OPEN — never behind a disclosure — which is the standing rule
 * for every embed in this product.
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
 * What we can honestly say about a band's shows, which is NOT what the mock
 * draws.
 *
 * The locked frame prints `6 upcoming · next Aug 8, Nile Theater` on every row.
 * `GET /scenes/{slug}/artists` carries neither half: `SceneArtistResponse` has
 * `show_count` (TOTAL approved shows, all time, anywhere) and `is_active`, and
 * no upcoming count and no next show. Printing `show_count` under an "upcoming"
 * label would put a band's career total against a calendar that says one — the
 * exact defect PSY-1623 removed from two other surfaces — and deriving a count
 * from the calendar's four-week window would undercount every band with
 * anything further out.
 *
 * So the row states the figure it actually has, labelled as what it is, and the
 * mock's line waits for the field. Same call Wave 1A made about the status
 * band's `THIS WEEK` and `TONIGHT` clauses, for the same reason.
 */
function showsLabel(artist: SceneArtist): string {
  const count = artist.show_count
  return `${count} show${count === 1 ? '' : 's'} listed`
}

function RosterRow({ artist }: { artist: SceneArtist }) {
  const slug = artist.slug?.trim()

  return (
    <li className="border-b border-border/40 py-3 last:border-b-0">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
        {/* Guarded: an artist slug is nullable and can generate as "", and
            `/artists/` resolves to the artists INDEX rather than 404ing. */}
        {slug ? (
          <Link href={`/artists/${slug}`} className="font-medium hover:underline">
            {artist.name}
          </Link>
        ) : (
          <span className="font-medium">{artist.name}</span>
        )}

        {/* The one-line descriptor slot sits HERE, and renders nothing.
            Descriptors are a global per-artist field authored through the
            trusted-tier inline edit (decision 8), and neither the field nor the
            authoring exists yet — that is Wave 3. The mock's `[+ add a line]`
            prompt is part of the authoring, not part of this slot: shown to
            trusted contributors and to nobody else. Until then the absent state
            is simply nothing, which is what the locked sparse frame draws for
            every other unauthored slot on this page. */}

        <span className="hidden flex-1 sm:block" aria-hidden="true" />

        {artist.is_active && (
          <span
            className="font-mono text-xs uppercase tracking-wide text-primary"
            title="Has an upcoming show or one in the last ~6 months"
          >
            Active
          </span>
        )}
        <span className="font-mono text-xs tabular-nums text-muted-foreground">
          {showsLabel(artist)}
        </span>
      </div>

      {/* OPEN, never collapsed. `MusicEmbed` renders nothing at all when the
          band has no embeddable URL, so a roster of bands without music stays a
          plain list rather than a column of empty player frames. */}
      {artist.bandcamp_embed_url && (
        <div className="mt-2 max-w-2xl">
          <MusicEmbed
            bandcampAlbumUrl={artist.bandcamp_embed_url}
            artistName={artist.name}
            compact
          />
        </div>
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
  const { data, isLoading } = useSceneArtists({
    slug: scene.slug,
    limit,
    // The reader stays looking at the list while the rest of it arrives.
    keepPreviousPage: true,
  })

  const artists = data?.artists ?? []

  // Loading and empty both render nothing. A heading that appears, empties and
  // disappears is worse than one that arrives once, and a scene with no
  // based-here bands has no section to draw.
  if (isLoading || artists.length === 0) return null

  const total = data?.total ?? artists.length
  const withheld = Math.max(total - artists.length, 0)
  // A roster past the endpoint's ceiling cannot be shown in full here, so the
  // control must not promise it. `/artists` is the page that can.
  const canExpand = withheld > 0 && artists.length < ROSTER_MAX

  return (
    <section id={anchorId} className="scroll-mt-20 border-t border-border pt-4">
      <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-2">
        <h2 className="font-mono text-[11px] uppercase tracking-widest">
          Bands / based in {scene.city}{' '}
          <span className="text-muted-foreground">· {total}</span>
        </h2>

        {canExpand && (
          <BracketLink
            label={`Show all ${total}`}
            onClick={() => setLimit(Math.min(total, ROSTER_MAX))}
          />
        )}
      </div>

      <ul className="mt-2">
        {artists.map(artist => (
          <RosterRow key={artist.id} artist={artist} />
        ))}
      </ul>

      {/* Only reachable once the ceiling is the thing withholding bands, so it
          states that rather than offering a control that cannot deliver. */}
      {withheld > 0 && !canExpand && (
        <p className="mt-2 font-mono text-[11px] text-muted-foreground">
          Showing {artists.length} of {total} bands based in {scene.city}
        </p>
      )}
    </section>
  )
}
