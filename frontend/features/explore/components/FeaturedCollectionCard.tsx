'use client'

/**
 * FeaturedCollectionCard (PSY-837)
 *
 * Mirror of FeaturedBillCard for the collection slot. Renders a 160px
 * cover image when present, the collection title, the curator's note
 * (sanitized HTML), and a "View collection →" link.
 *
 * Parent collapses the section when `collection` is null.
 */

import Link from 'next/link'
import Image from 'next/image'
import type { ExploreFeaturedCollection } from '../types'

interface FeaturedCollectionCardProps {
  collection: ExploreFeaturedCollection
}

export function FeaturedCollectionCard({ collection }: FeaturedCollectionCardProps) {
  const detailsHref = `/collections/${collection.slug || collection.id}`

  return (
    <article className="bg-card/50 border border-border/50 rounded-xl p-6 hover:border-border transition-colors">
      <div className="flex flex-col sm:flex-row gap-5">
        {collection.cover_image_url && (
          <Link
            href={detailsHref}
            className="shrink-0 block overflow-hidden rounded-lg"
            aria-label={collection.title}
          >
            <Image
              src={collection.cover_image_url}
              alt={collection.title}
              width={160}
              height={160}
              className="h-40 w-40 object-cover"
            />
          </Link>
        )}

        <div className="flex-1 min-w-0">
          <div className="text-xs uppercase tracking-wider text-muted-foreground">
            Collection
          </div>

          <h3 className="mt-1.5 text-xl font-semibold leading-tight tracking-tight">
            <Link
              href={detailsHref}
              className="hover:text-primary transition-colors"
            >
              {collection.title}
            </Link>
          </h3>

          {collection.curator_note_html && (
            <div
              className="mt-3 text-sm leading-relaxed text-foreground/85 prose prose-sm max-w-none dark:prose-invert"
              dangerouslySetInnerHTML={{ __html: collection.curator_note_html }}
            />
          )}

          <Link
            href={detailsHref}
            className="inline-block mt-4 px-4 py-2 text-sm bg-muted/50 border border-border/50 rounded-lg hover:bg-muted hover:border-border transition-colors"
          >
            View collection →
          </Link>
        </div>
      </div>
    </article>
  )
}
