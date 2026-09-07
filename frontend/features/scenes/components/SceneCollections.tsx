'use client'

import Link from 'next/link'
import { Library } from 'lucide-react'
// Deep-imported, not through a barrel — see the note in
// features/scenes/components/index.ts (PSY-1772).
import { CollectionCoverImage } from '@/features/collections/components/CollectionCoverImage'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import { useSceneCollections } from '../hooks'
import { entityHref, SceneSectionHeading } from './sceneChrome'
import type { SceneCollectionSummary, SceneDetail } from '../types'

/**
 * The collections that are about this city.
 *
 * The module HIDES COMPLETELY when nothing qualifies, which is most scenes. An
 * empty shelf under a heading would report a shortfall in the catalog as if it
 * were a fact about the city.
 *
 * The qualifying counts the payload carries are deliberately not drawn: they
 * audit the backend's ranking, and the reader is being offered a collection
 * rather than a ranking.
 */

/**
 * `Built by 4 · Updated 3 days ago`, printed in mono micro-caps.
 *
 * `formatTimeAgo` rather than its `formatRelativeTime` sibling, which the
 * collections browse card uses: only this one has the weeks and months
 * phrasing the mock's `UPDATED 1 WEEK AGO` needs, where the other jumps from
 * days straight to an absolute date.
 *
 * `updated_at` moves on collection-ROW edits only; adding an item does not
 * touch it. The clause therefore dates the collection's own record, which is
 * what it says, and is not a claim about curation activity.
 *
 * `collection_items.added_by_user_id` is NOT NULL, so a collection with any
 * member has at least one contributor and the wire should never carry a zero.
 * The clause drops at zero anyway: `Built by 0` is a claim about who assembled
 * the collection that would be false however it arrived.
 */
function collectionMeta(collection: SceneCollectionSummary): string {
  const clauses: string[] = []
  if (collection.contributor_count > 0) {
    clauses.push(`Built by ${collection.contributor_count}`)
  }
  clauses.push(`Updated ${formatTimeAgo(collection.updated_at)}`)
  return clauses.join(' · ')
}

/**
 * One row, linked to the collection when `entityHref` can build a link for it.
 *
 * An unlinkable row is still NAMED: it is one of the collections that earned
 * the slot, and dropping it would misstate the list.
 */
function CollectionRow({ collection }: { collection: SceneCollectionSummary }) {
  const href = entityHref('/collections', collection.slug)
  const content = (
    <div className="flex items-center gap-3 py-2">
      {/* Empty alt: the title sits immediately beside the tile, so a described
          cover would make every row announce its name twice. */}
      <CollectionCoverImage
        url={collection.cover_image_url}
        alt=""
        className="h-9 w-9 shrink-0 rounded-sm border border-border bg-muted/50"
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

  return (
    <li className="border-b border-border/40 last:border-b-0">
      {href ? (
        <Link href={href} className="block transition-colors hover:bg-muted/40">
          {content}
        </Link>
      ) : (
        content
      )}
    </li>
  )
}

export function SceneCollections({ scene }: { scene: SceneDetail }) {
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
