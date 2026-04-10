'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Library, Check, Plus, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useMyCollections, useAddCollectionItem } from '@/features/collections/hooks'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { CollectionEntityType } from '@/features/collections/types'

interface AddToCollectionButtonProps {
  entityType: CollectionEntityType
  entityId: number
  entityName: string
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'sm' | 'default' | 'icon'
}

export function AddToCollectionButton({
  entityType,
  entityId,
  entityName,
  variant = 'ghost',
  size = 'sm',
}: AddToCollectionButtonProps) {
  const { isAuthenticated } = useAuthContext()
  const [open, setOpen] = useState(false)
  const [addedMessage, setAddedMessage] = useState<string | null>(null)
  const { data: myCollectionsData, isLoading: collectionsLoading } = useMyCollections()
  const addMutation = useAddCollectionItem()

  if (!isAuthenticated) return null

  const collections = myCollectionsData?.collections ?? []

  // Check if entity is already in a collection by looking at its items
  // (We don't have items in the list response, so we track locally what we just added)
  const [recentlyAdded, setRecentlyAdded] = useState<Set<string>>(new Set())

  const handleAdd = (collectionSlug: string, collectionTitle: string) => {
    addMutation.mutate(
      {
        slug: collectionSlug,
        entityType,
        entityId,
      },
      {
        onSuccess: () => {
          setRecentlyAdded(prev => new Set(prev).add(collectionSlug))
          setAddedMessage(`Added to "${collectionTitle}"`)
          setTimeout(() => setAddedMessage(null), 2000)
        },
      }
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant={variant}
          size={size}
          className={size === 'icon' ? 'h-8 w-8 p-0' : ''}
          title={`Add "${entityName}" to a collection`}
          aria-label="Add to Collection"
        >
          <Library className="h-4 w-4" />
          {size !== 'icon' && <span className="ml-1.5">Collect</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="end">
        <div className="p-3 border-b border-border">
          <h4 className="text-sm font-semibold">Add to Collection</h4>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">
            {entityName}
          </p>
        </div>

        <div className="max-h-48 overflow-y-auto p-1">
          {collectionsLoading ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : collections.length === 0 ? (
            <div className="py-3 px-2 text-center">
              <p className="text-sm text-muted-foreground">No collections yet</p>
            </div>
          ) : (
            collections.map(collection => {
              const wasJustAdded = recentlyAdded.has(collection.slug)

              return (
                <button
                  key={collection.id}
                  className="w-full flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted/50 transition-colors disabled:opacity-50"
                  onClick={() => handleAdd(collection.slug, collection.title)}
                  disabled={addMutation.isPending || wasJustAdded}
                >
                  <Library className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                  <span className="flex-1 truncate">{collection.title}</span>
                  {wasJustAdded && (
                    <Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400 shrink-0" />
                  )}
                </button>
              )
            })
          )}
        </div>

        {/* Success feedback */}
        {addedMessage && (
          <div className="px-3 py-1.5 border-t border-border text-xs text-green-600 dark:text-green-400 flex items-center gap-1">
            <Check className="h-3 w-3" />
            {addedMessage}
          </div>
        )}

        {/* Error feedback */}
        {addMutation.isError && (
          <div className="px-3 py-1.5 border-t border-border text-xs text-destructive">
            {addMutation.error instanceof Error
              ? addMutation.error.message
              : 'Failed to add item'}
          </div>
        )}

        {/* Create new link */}
        <div className="p-2 border-t border-border">
          <Link
            href="/collections"
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors"
            onClick={() => setOpen(false)}
          >
            <Plus className="h-3.5 w-3.5" />
            Create new collection
          </Link>
        </div>
      </PopoverContent>
    </Popover>
  )
}
