'use client'

import Link from 'next/link'
import { Library } from 'lucide-react'
// Deep-imported, not through a barrel — see the note in
// features/scenes/components/index.ts (PSY-1772).
import { CollectionCoverImage } from '@/features/collections/components/CollectionCoverImage'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import { useSceneCollections } from '../hooks'
import { SceneSectionHeading } from './sceneChrome'
import type { SceneCollectionSummary, SceneDetail } from '../types'

/**
 * The collections that are about this city.
 *
 * Community knowledge, so the row's second line is about the people and the
 * record rather than about size: who built it and when it was last edited. The
 * qualifying counts that earned the slot ride in the payload and are not drawn,
 * because the rule they audit is the backend's and the reader is being offered
 * a collection, not a ranking.
 *
 * The module HIDES COMPLETELY when nothing qualifies. Most scenes have no
 * qualifying collection, and an empty shelf under a heading would report a
 * shortfall in the catalog as if it were a fact about the city.
 *
 * Nothing here filters for visibility. Private collections are creator-only on
 * every read surface and the endpoint's query emits public rows only, so a
 * second client-side gate would be a second definition of a rule that must have
 * exactly one.
 */

/** Row height for the cover tile, sized to the row's two lines of text. */
const COVER_TILE_CLASS = 'h-9 w-9 shrink-0 rounded-sm border border-border bg-muted/50'

/**
 * `Built by 4 · Updated 3 days ago`, printed in mono micro-caps.
 *
 * The builder clause DROPS at zero rather than printing `Built by 0`.
 * `contributor_count` counts distinct non-null `added_by_user_id` values, so a
 * collection assembled entirely by imports has none to name.
 *
 * `updated_at` moves on collection-ROW edits only; adding an item does not
 * touch it. The clause therefore dates the collection's own record, which is
 * what it says, and is not a claim about curation activity.
 */
function collectionMeta(collection: SceneCollectionSummary): string {
  const clauses: string[] = []
  if (collection.contributor_count > 0) {
    clauses.push(`Built by ${collection.contributor_count}`)
  }
  clauses.push(`Updated ${formatTimeAgo(collection.updated_at)}`)
  return clauses.join(' · ')
}

function CollectionRowContent({
  collection,
}: {
  collection: SceneCollectionSummary
}) {
  return (
    <div className="flex items-center gap-3 py-2">
      {/* Empty alt: the title sits immediately beside the tile, so a described
          cover would make every row announce its name twice. */}
      <CollectionCoverImage
        url={collection.cover_image_url}
        alt=""
        className={COVER_TILE_CLASS}
        fallback={<Library className="h-4 w-4 text-muted-foreground/40" />}
      />
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{collection.title}</p>
        <p className="font-mono text-[11px] uppercase tracking-widest text-muted-foreground">
          {collectionMeta(collection)}
        </p>
      </div>
    </div>
  )
}

/**
 * One row, linked to the collection when it can be.
 *
 * A slug that is empty resolves `/collections/` to the browse INDEX rather than
 * to a 404, which would send a reader who clicked a named collection to a
 * directory that never mentions it (PSY-1754). Such a row is still NAMED: it is
 * one of the collections that earned the slot, and dropping it would misstate
 * the list. Encoded, so a stored `/` cannot splice a second path segment on.
 */
function CollectionRow({ collection }: { collection: SceneCollectionSummary }) {
  const slug = collection.slug?.trim()
  if (!slug) {
    return (
      <li className="border-b border-border/40 last:border-b-0">
        <CollectionRowContent collection={collection} />
      </li>
    )
  }
  return (
    <li className="border-b border-border/40 last:border-b-0">
      <Link
        href={`/collections/${encodeURIComponent(slug)}`}
        className="block transition-colors hover:bg-muted/40"
      >
        <CollectionRowContent collection={collection} />
      </Link>
    </li>
  )
}

export function SceneCollections({ scene }: { scene: SceneDetail }) {
  // No `limit`: the endpoint's own default owns the cap, the rule this hook
  // documents and the sibling rails already follow.
  const { data } = useSceneCollections({ slug: scene.slug })

  const collections = data?.collections ?? []
  // One `return null` covers loading, error and a scene nothing qualifies for.
  // All three mean there is nothing to offer here, and the two temporary ones
  // must not flash a heading over empty space on the way past.
  if (collections.length === 0) return null

  return (
    <section className="border-t border-border pt-4">
      <SceneSectionHeading title="Collections" note={scene.city} />

      <ul className="mt-2">
        {collections.map(collection => (
          <CollectionRow key={collection.id} collection={collection} />
        ))}
      </ul>
    </section>
  )
}
