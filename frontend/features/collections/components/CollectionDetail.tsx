'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Library,
  Users,
  Star,
  Bell,
  BellOff,
  Pencil,
  Check,
  X,
  Trash2,
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag,
  Tent,
} from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  useCollection,
  useUpdateCollection,
  useRemoveCollectionItem,
  useSubscribeCollection,
  useUnsubscribeCollection,
  useDeleteCollection,
} from '../hooks'
import { getEntityUrl, getEntityTypeLabel } from '../types'
import type { CollectionItem } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb } from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useNavigationBreadcrumbs } from '@/lib/context/NavigationBreadcrumbContext'
import { useRouter, usePathname } from 'next/navigation'
import type { ApiError } from '@/lib/api'

interface CollectionDetailProps {
  slug: string
}

const ENTITY_ICONS: Record<string, React.ElementType> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

export function CollectionDetail({ slug }: CollectionDetailProps) {
  const router = useRouter()
  const pathname = usePathname()
  const { user, isAuthenticated } = useAuthContext()
  const { pushBreadcrumb } = useNavigationBreadcrumbs()
  const { data: collection, isLoading, error } = useCollection(slug)
  const subscribeMutation = useSubscribeCollection()
  const unsubscribeMutation = useUnsubscribeCollection()
  const deleteMutation = useDeleteCollection()

  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)

  // Push breadcrumb when collection data is loaded
  useEffect(() => {
    if (collection) {
      pushBreadcrumb(collection.title, pathname)
    }
  }, [collection, pathname, pushBreadcrumb])

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load collection'
    const is404 = (error as ApiError).status === 404

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Collection Not Found' : 'Error Loading Collection'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The collection you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/collections">Back to Collections</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!collection) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Collection Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The collection you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/collections">Back to Collections</Link>
          </Button>
        </div>
      </div>
    )
  }

  const currentUserId = user?.id ? Number(user.id) : undefined
  const isCreator = currentUserId === collection.creator_id
  const canSubscribe = isAuthenticated && !isCreator

  const handleSubscribe = () => {
    if (collection.is_subscribed) {
      unsubscribeMutation.mutate({ slug })
    } else {
      subscribeMutation.mutate({ slug })
    }
  }

  const handleDelete = () => {
    deleteMutation.mutate(
      { slug },
      {
        onSuccess: () => {
          setIsDeleteDialogOpen(false)
          router.push('/collections')
        },
      }
    )
  }

  const items = collection.items ?? []

  return (
    <div className="container max-w-6xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/collections', label: 'Collections' }}
        currentPage={collection.title}
      />

      {/* Header */}
      <header className="mb-8">
        {isEditing ? (
          <InlineEditForm
            slug={slug}
            title={collection.title}
            description={collection.description}
            isPublic={collection.is_public}
            collaborative={collection.collaborative}
            onDone={() => setIsEditing(false)}
          />
        ) : (
          <div>
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-3 mb-1">
                  <h1 className="text-3xl font-bold tracking-tight">
                    {collection.title}
                  </h1>
                  {collection.is_featured && (
                    <Badge variant="default" className="text-xs">
                      <Star className="h-3 w-3 mr-0.5" />
                      Featured
                    </Badge>
                  )}
                  {collection.collaborative && (
                    <Badge variant="secondary" className="text-xs">
                      Collaborative
                    </Badge>
                  )}
                </div>

                <p className="text-sm text-muted-foreground">
                  by {collection.creator_name}
                </p>

                {collection.description && (
                  <p className="text-muted-foreground mt-3 whitespace-pre-line">
                    {collection.description}
                  </p>
                )}

                {/* Stats */}
                <div className="mt-3 flex items-center gap-4 text-sm text-muted-foreground">
                  <span className="flex items-center gap-1">
                    <Library className="h-4 w-4" />
                    {collection.item_count === 1
                      ? '1 item'
                      : `${collection.item_count} items`}
                  </span>
                  <span className="flex items-center gap-1">
                    <Users className="h-4 w-4" />
                    {collection.subscriber_count === 1
                      ? '1 subscriber'
                      : `${collection.subscriber_count} subscribers`}
                  </span>
                </div>
              </div>

              {/* Action buttons */}
              <div className="flex items-center gap-2 shrink-0">
                {canSubscribe && (
                  <Button
                    variant={collection.is_subscribed ? 'secondary' : 'default'}
                    size="sm"
                    onClick={handleSubscribe}
                    disabled={
                      subscribeMutation.isPending ||
                      unsubscribeMutation.isPending
                    }
                  >
                    {collection.is_subscribed ? (
                      <>
                        <BellOff className="h-4 w-4 mr-1.5" />
                        Unsubscribe
                      </>
                    ) : (
                      <>
                        <Bell className="h-4 w-4 mr-1.5" />
                        Subscribe
                      </>
                    )}
                  </Button>
                )}

                {isCreator && (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setIsEditing(true)}
                    >
                      <Pencil className="h-4 w-4 mr-1.5" />
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setIsDeleteDialogOpen(true)}
                      disabled={deleteMutation.isPending}
                      className="text-destructive hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </>
                )}
              </div>
            </div>
          </div>
        )}
      </header>

      {/* Items list */}
      <div>
        <h2 className="text-lg font-semibold mb-4">Items</h2>
        {items.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <Library className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
            <p>This collection is empty.</p>
          </div>
        ) : (
          <div className="space-y-2">
            {items.map(item => (
              <CollectionItemRow
                key={item.id}
                item={item}
                slug={slug}
                isCreator={isCreator}
              />
            ))}
          </div>
        )}
      </div>

      {/* Delete Confirmation Dialog */}
      <Dialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Trash2 className="h-5 w-5 text-destructive" />
              Delete Collection
            </DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{collection.title}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>

          {deleteMutation.isError && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {deleteMutation.error?.message ||
                'Failed to delete collection. Please try again.'}
            </div>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsDeleteDialogOpen(false)}
              disabled={deleteMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Deleting...
                </>
              ) : (
                <>
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete Collection
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ──────────────────────────────────────────────
// Item Row
// ──────────────────────────────────────────────

function CollectionItemRow({
  item,
  slug,
  isCreator,
}: {
  item: CollectionItem
  slug: string
  isCreator: boolean
}) {
  const removeMutation = useRemoveCollectionItem()
  const Icon = ENTITY_ICONS[item.entity_type] ?? Library

  const handleRemove = () => {
    removeMutation.mutate({ slug, itemId: item.id })
  }

  return (
    <div className="flex items-center gap-3 rounded-lg border border-border/50 bg-card p-3">
      {/* Entity type icon */}
      <div className="h-8 w-8 shrink-0 rounded-md bg-muted/50 flex items-center justify-center">
        <Icon className="h-4 w-4 text-muted-foreground/60" />
      </div>

      {/* Entity info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <Link
            href={getEntityUrl(item.entity_type, item.entity_slug)}
            className="font-medium text-foreground hover:text-primary transition-colors truncate"
          >
            {item.entity_name}
          </Link>
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
            {getEntityTypeLabel(item.entity_type)}
          </Badge>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
          <span>added by {item.added_by_name}</span>
          {item.notes && (
            <>
              <span className="text-muted-foreground/40">|</span>
              <span className="truncate">{item.notes}</span>
            </>
          )}
        </div>
      </div>

      {/* Remove button (creator only) */}
      {isCreator && (
        <Button
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive shrink-0"
          onClick={handleRemove}
          disabled={removeMutation.isPending}
          title="Remove from collection"
        >
          <X className="h-4 w-4" />
        </Button>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Inline Edit Form
// ──────────────────────────────────────────────

function InlineEditForm({
  slug,
  title: initialTitle,
  description: initialDescription,
  isPublic: initialPublic,
  collaborative: initialCollaborative,
  onDone,
}: {
  slug: string
  title: string
  description: string
  isPublic: boolean
  collaborative: boolean
  onDone: () => void
}) {
  const updateMutation = useUpdateCollection()
  const [title, setTitle] = useState(initialTitle)
  const [description, setDescription] = useState(initialDescription)
  const [isPublic, setIsPublic] = useState(initialPublic)
  const [collaborative, setCollaborative] = useState(initialCollaborative)

  const handleSave = () => {
    updateMutation.mutate(
      {
        slug,
        title: title.trim(),
        description: description.trim(),
        is_public: isPublic,
        collaborative,
      },
      { onSuccess: () => onDone() }
    )
  }

  return (
    <div className="space-y-4 rounded-lg border border-border/50 bg-card p-4">
      <div>
        <label htmlFor="edit-title" className="text-sm font-medium mb-1.5 block">
          Title
        </label>
        <Input
          id="edit-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="edit-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description
        </label>
        <Textarea
          id="edit-description"
          value={description}
          onChange={e => setDescription(e.target.value)}
          rows={3}
        />
      </div>

      <div className="flex items-center gap-6">
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={e => setIsPublic(e.target.checked)}
            className="rounded border-border"
          />
          Public
        </label>

        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={collaborative}
            onChange={e => setCollaborative(e.target.checked)}
            className="rounded border-border"
          />
          Collaborative
        </label>
      </div>

      {updateMutation.error && (
        <p className="text-sm text-destructive">
          {updateMutation.error instanceof Error
            ? updateMutation.error.message
            : 'Failed to update collection'}
        </p>
      )}

      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={handleSave}
          disabled={!title.trim() || updateMutation.isPending}
        >
          <Check className="h-4 w-4 mr-1" />
          {updateMutation.isPending ? 'Saving...' : 'Save'}
        </Button>
        <Button size="sm" variant="outline" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
