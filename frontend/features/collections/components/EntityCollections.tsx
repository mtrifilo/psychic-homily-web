'use client'

import Link from 'next/link'
import { Library } from 'lucide-react'
import { useEntityCollections } from '../hooks'
import type { Collection } from '../types'
import { MarkdownContent } from './MarkdownEditor'

interface EntityCollectionsProps {
  entityType: string
  entityId: number
  enabled?: boolean
}

function CollectionsList({ collections }: { collections: Collection[] }) {
  if (collections.length === 0) return null

  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        In Collections
      </h3>
      <div className="space-y-2">
        {collections.map(collection => (
          <Link
            key={collection.id}
            href={`/collections/${collection.slug}`}
            className="flex items-start gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5 group"
          >
            <Library className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60 group-hover:text-foreground mt-0.5" />
            <div className="flex-1 min-w-0">
              <span className="truncate block">{collection.title}</span>
              <span className="text-xs text-muted-foreground/60">
                by {collection.creator_name}
                {collection.subscriber_count > 0 && (
                  <>
                    {' '}&middot; {collection.subscriber_count}{' '}
                    {collection.subscriber_count === 1 ? 'subscriber' : 'subscribers'}
                  </>
                )}
              </span>
              {/* PSY-349: server-rendered markdown description on backlink
                  cards. Single line so the list stays scannable; clicking
                  through to the detail page shows the full rendered desc. */}
              {collection.description_html && (
                <MarkdownContent
                  html={collection.description_html}
                  className="text-xs text-muted-foreground/60 mt-0.5 line-clamp-1"
                />
              )}
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}

export function EntityCollections({
  entityType,
  entityId,
  enabled = true,
}: EntityCollectionsProps) {
  const { data, isLoading } = useEntityCollections(entityType, entityId, {
    enabled,
  })

  const collections = data?.collections ?? []

  if (isLoading || collections.length === 0) return null

  return <CollectionsList collections={collections} />
}
