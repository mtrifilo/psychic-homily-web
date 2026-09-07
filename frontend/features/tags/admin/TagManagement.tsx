'use client'

import { useState, useCallback, useEffect } from 'react'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  Search,
  Inbox,
  Tags,
  X,
  GitMerge,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { InlineErrorBanner } from '@/components/shared'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  FILTER_SELECT_ALL,
  toFilterSelectValue,
  fromFilterSelectValue,
} from '@/lib/filterSelectValue'
import { useTags, useTag } from '../hooks'
import { AliasListing } from './AliasListing'
import { LowQualityTagQueue } from './LowQualityTagQueue'
import { MergeTagDialog } from './MergeTagDialog'
import { TagHierarchyEditor } from './TagHierarchyEditor'
import { TagOfficialIndicator } from '../components/TagOfficialIndicator'
import {
  useCreateTag,
  useUpdateTag,
  useDeleteTag,
  useTagAliases,
  useCreateAlias,
  useDeleteAlias,
  useLowQualityTagQueue,
} from './useAdminTags'
import {
  TAG_CATEGORIES,
  TAG_CATEGORY_CREW,
  canonicalTagCategory,
  getCategoryChipClasses,
  getCategoryLabel,
  type TagCategory,
  type TagDetailResponse,
} from '../types'

type DialogMode = 'create' | 'edit' | 'delete' | 'merge' | null

// ============================================================================
// Outbound links (crew tags)
// ============================================================================

/** The three link inputs as edited, before they are trimmed and submitted. */
interface TagLinkDraft {
  website: string
  instagram: string
  bandcamp: string
}

const EMPTY_TAG_LINKS: TagLinkDraft = { website: '', instagram: '', bandcamp: '' }

/**
 * Whether the form should offer the link inputs.
 *
 * Keyed on the category the form is ABOUT TO SAVE rather than the stored one,
 * so recategorizing a tag to crew and filling its links is one pass. The
 * canonical spelling is used because the stored column is unconstrained.
 */
function offersTagLinks(category: string): boolean {
  return canonicalTagCategory(category) === TAG_CATEGORY_CREW
}

/**
 * The link half of a tag write, or nothing.
 *
 * A form that is not offering the inputs sends no link keys at all, so a
 * category change cannot silently clear values the admin was not shown. A
 * trimmed-empty input is sent, and clears its column.
 */
function tagLinkPayload(
  category: string,
  links: TagLinkDraft
): Partial<TagLinkDraft> {
  if (!offersTagLinks(category)) return {}
  return {
    website: links.website.trim(),
    instagram: links.instagram.trim(),
    bandcamp: links.bandcamp.trim(),
  }
}

/**
 * The three outbound-link inputs.
 *
 * Free text, not validated here: the host rule is the API's, and duplicating
 * it in the browser would be a second rule to keep in step. A refused value
 * comes back as the same message every other entity's link fields produce and
 * is shown in the form's error banner.
 */
function TagLinkFields({
  idPrefix,
  links,
  onChange,
  disabled,
}: {
  idPrefix: string
  links: TagLinkDraft
  onChange: (next: TagLinkDraft) => void
  disabled?: boolean
}) {
  const fields: { key: keyof TagLinkDraft; label: string; placeholder: string }[] = [
    { key: 'website', label: 'Website', placeholder: 'https://example.com' },
    { key: 'instagram', label: 'Instagram', placeholder: 'https://instagram.com/handle' },
    { key: 'bandcamp', label: 'Bandcamp', placeholder: 'https://name.bandcamp.com' },
  ]

  return (
    <div className="space-y-3 rounded-md border border-dashed p-3">
      <p className="text-xs text-muted-foreground">
        Links shown on the tag page. Leave blank to remove.
      </p>
      {fields.map(({ key, label, placeholder }) => (
        <div key={key} className="space-y-2">
          <Label htmlFor={`${idPrefix}-${key}`}>{label}</Label>
          <Input
            id={`${idPrefix}-${key}`}
            value={links[key]}
            onChange={(e) => onChange({ ...links, [key]: e.target.value })}
            placeholder={placeholder}
            disabled={disabled}
          />
        </div>
      ))}
    </div>
  )
}

// ============================================================================
// Needs-Review Tab Badge
// ============================================================================

function LowQualityBadge() {
  const { data } = useLowQualityTagQueue({ limit: 1 })
  const total = data?.total ?? 0
  if (total === 0) return null
  return (
    <Badge
      variant="secondary"
      className="h-5 min-w-5 justify-center px-1.5 text-xs"
      aria-label={`${total} tags need review`}
    >
      {total > 99 ? '99+' : total}
    </Badge>
  )
}

// ============================================================================
// Alias Manager Sub-Component
// ============================================================================

function AliasManager({ tagId }: { tagId: number }) {
  const { data: aliasData, isLoading } = useTagAliases(tagId)
  const createAlias = useCreateAlias()
  const deleteAlias = useDeleteAlias()
  const [newAlias, setNewAlias] = useState('')
  const [aliasError, setAliasError] = useState<string | null>(null)

  const handleAdd = useCallback(() => {
    if (!newAlias.trim()) return
    setAliasError(null)
    createAlias.mutate(
      { tagId, alias: newAlias.trim() },
      {
        onSuccess: () => setNewAlias(''),
        onError: (err) => {
          setAliasError(
            err instanceof Error ? err.message : 'Failed to add alias'
          )
        },
      }
    )
  }, [tagId, newAlias, createAlias])

  const handleRemove = useCallback(
    (aliasId: number) => {
      setAliasError(null)
      deleteAlias.mutate(
        { tagId, aliasId },
        {
          onError: (err) => {
            setAliasError(
              err instanceof Error ? err.message : 'Failed to remove alias'
            )
          },
        }
      )
    },
    [tagId, deleteAlias]
  )

  return (
    <div className="space-y-3">
      <Label>Aliases</Label>
      {aliasError && <InlineErrorBanner>{aliasError}</InlineErrorBanner>}
      <div className="flex items-end gap-2">
        <div className="flex-1">
          <Input
            placeholder="Add alias (e.g., post punk)..."
            value={newAlias}
            onChange={(e) => setNewAlias(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                handleAdd()
              }
            }}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleAdd}
          disabled={createAlias.isPending || !newAlias.trim()}
        >
          {createAlias.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Plus className="h-4 w-4" />
          )}
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-2">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      ) : aliasData?.aliases && aliasData.aliases.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {aliasData.aliases.map((alias) => (
            <Badge
              key={alias.id}
              variant="secondary"
              className="gap-1 pl-2 pr-1"
            >
              {alias.alias}
              <button
                onClick={() => handleRemove(alias.id)}
                disabled={deleteAlias.isPending}
                className="ml-0.5 rounded-full p-0.5 hover:bg-muted-foreground/20"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">No aliases yet.</p>
      )}
    </div>
  )
}

// ============================================================================
// Create Tag Form
// ============================================================================

function CreateTagForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: () => void
  onCancel: () => void
}) {
  const createMutation = useCreateTag()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState<string>('genre')
  const [isOfficial, setIsOfficial] = useState(false)
  const [links, setLinks] = useState<TagLinkDraft>(EMPTY_TAG_LINKS)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!name.trim()) {
        setError('Name is required')
        return
      }

      createMutation.mutate(
        {
          name: name.trim(),
          description: description.trim() || undefined,
          category,
          is_official: isOfficial,
          ...tagLinkPayload(category, links),
        },
        {
          onSuccess: () => onSuccess(),
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to create tag'
            )
          },
        }
      )
    },
    [name, description, category, isOfficial, links, createMutation, onSuccess]
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <InlineErrorBanner>{error}</InlineErrorBanner>}

      <div className="space-y-2">
        <Label htmlFor="create-name">Name *</Label>
        <Input
          id="create-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g., post-punk"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="create-category">Category *</Label>
        <Select value={category} onValueChange={setCategory}>
          <SelectTrigger id="create-category" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TAG_CATEGORIES.map((cat) => (
              <SelectItem key={cat} value={cat}>
                {getCategoryLabel(cat)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {offersTagLinks(category) && (
        <TagLinkFields
          idPrefix="create"
          links={links}
          onChange={setLinks}
          disabled={createMutation.isPending}
        />
      )}

      <div className="space-y-2">
        <Label htmlFor="create-desc">Description</Label>
        <Textarea
          id="create-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={2}
        />
      </div>

      <div className="flex items-center gap-2">
        <input
          type="checkbox"
          id="create-official"
          checked={isOfficial}
          onChange={(e) => setIsOfficial(e.target.checked)}
          className="h-4 w-4 rounded border-muted-foreground"
        />
        <Label htmlFor="create-official" className="text-sm font-normal">
          Official tag (canonical, curated by admins)
        </Label>
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onCancel}
          disabled={createMutation.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Creating...
            </>
          ) : (
            'Create Tag'
          )}
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Edit Tag Form
// ============================================================================

// Outer fetches the tag; once loaded, mounts EditTagFormFields with
// `key={tag.id}` so a switch-tag-without-closing-dialog scenario
// remounts with fresh state. The inner component initializes local state
// from props inline (React's preferred "calculate during render" path —
// see https://react.dev/learn/you-might-not-need-an-effect). No useEffect,
// no `initialized` ratchet.
function EditTagForm({
  tagId,
  onSuccess,
  onCancel,
}: {
  tagId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const { data: tag, isLoading } = useTag(tagId, { enabled: tagId > 0 })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!tag) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        Tag not found.
      </div>
    )
  }

  return (
    <EditTagFormFields
      key={tag.id}
      tag={tag}
      onSuccess={onSuccess}
      onCancel={onCancel}
    />
  )
}

// Exported only for direct regression-test access (rerender-with-different-key
// resets fields; rerender-with-same-key preserves dirty edits). Not part of
// the surface's public API — production callers use EditTagForm.
export function EditTagFormFields({
  tag,
  onSuccess,
  onCancel,
}: {
  tag: TagDetailResponse
  onSuccess: () => void
  onCancel: () => void
}) {
  const updateMutation = useUpdateTag()

  const [name, setName] = useState(tag.name)
  const [description, setDescription] = useState(tag.description || '')
  const [category, setCategory] = useState<string>(
    canonicalTagCategory(tag.category)
  )
  const [isOfficial, setIsOfficial] = useState(tag.is_official)
  const [links, setLinks] = useState<TagLinkDraft>({
    website: tag.social?.website ?? '',
    instagram: tag.social?.instagram ?? '',
    bandcamp: tag.social?.bandcamp ?? '',
  })
  const [error, setError] = useState<string | null>(null)

  // A Select whose value has no matching option renders an EMPTY trigger while
  // the form still submits the value behind it, so an admin who "fixes" the
  // blank field re-categorizes the tag without meaning to. `tags.category` is
  // an unconstrained column, so the stored value is offered whatever it is.
  //
  // Two exceptions. A value that is only a spelling of a known category is
  // canonicalized above instead of appended, so the list never holds two
  // options that render alike. And the empty string is left alone: Radix
  // throws on a SelectItem with an empty value, so such a tag still gets a
  // blank trigger.
  //
  // Derived from the tag, not the edit state: from state, picking a listed
  // option would drop the stored one with no way back inside the dialog.
  const storedCategory = canonicalTagCategory(tag.category)
  const categoryOptions: string[] =
    storedCategory === '' ||
    (TAG_CATEGORIES as readonly string[]).includes(storedCategory)
      ? [...TAG_CATEGORIES]
      : [...TAG_CATEGORIES, storedCategory]

  const tagId = tag.id

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!name.trim()) {
        setError('Name is required')
        return
      }

      updateMutation.mutate(
        {
          tagId,
          data: {
            name: name.trim(),
            description: description.trim() || null,
            category,
            is_official: isOfficial,
            ...tagLinkPayload(category, links),
          },
        },
        {
          onSuccess: () => onSuccess(),
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to update tag'
            )
          },
        }
      )
    },
    [
      name,
      description,
      category,
      isOfficial,
      links,
      tagId,
      updateMutation,
      onSuccess,
    ]
  )

  return (
    <div className="space-y-4">
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && <InlineErrorBanner>{error}</InlineErrorBanner>}

        <div className="space-y-2">
          <Label htmlFor="edit-name">Name *</Label>
          <Input
            id="edit-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="edit-category">Category *</Label>
          <Select value={category} onValueChange={setCategory}>
            <SelectTrigger id="edit-category" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {categoryOptions.map((cat) => (
                <SelectItem key={cat} value={cat}>
                  {getCategoryLabel(cat)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {offersTagLinks(category) && (
          <TagLinkFields
            idPrefix="edit"
            links={links}
            onChange={setLinks}
            disabled={updateMutation.isPending}
          />
        )}

        <div className="space-y-2">
          <Label htmlFor="edit-desc">Description</Label>
          <Textarea
            id="edit-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description..."
            rows={2}
          />
        </div>

        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            id="edit-official"
            checked={isOfficial}
            onChange={(e) => setIsOfficial(e.target.checked)}
            className="h-4 w-4 rounded border-muted-foreground"
          />
          <Label htmlFor="edit-official" className="text-sm font-normal">
            Official tag
          </Label>
        </div>

        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>Slug: {tag.slug}</span>
          <span>|</span>
          <span>Usage: {tag.usage_count}</span>
          {tag.child_count > 0 && (
            <>
              <span>|</span>
              <span>{tag.child_count} children</span>
            </>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={updateMutation.isPending}
          >
            Cancel
          </Button>
          <Button type="submit" disabled={updateMutation.isPending}>
            {updateMutation.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              'Save Changes'
            )}
          </Button>
        </DialogFooter>
      </form>

      {/* Alias Management */}
      <div className="border-t pt-4">
        <AliasManager tagId={tagId} />
      </div>
    </div>
  )
}

// ============================================================================
// Delete Confirmation
// ============================================================================

function DeleteConfirmation({
  tagName,
  tagId,
  onSuccess,
  onCancel,
}: {
  tagName: string
  tagId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const deleteMutation = useDeleteTag()
  const [error, setError] = useState<string | null>(null)

  const handleDelete = useCallback(() => {
    setError(null)
    deleteMutation.mutate(tagId, {
      onSuccess: () => onSuccess(),
      onError: (err) => {
        setError(
          err instanceof Error ? err.message : 'Failed to delete tag'
        )
      },
    })
  }, [tagId, deleteMutation, onSuccess])

  return (
    <div className="space-y-4">
      {error && <InlineErrorBanner>{error}</InlineErrorBanner>}

      <p className="text-sm text-muted-foreground">
        Are you sure you want to delete{' '}
        <span className="font-semibold text-foreground">
          &quot;{tagName}&quot;
        </span>
        ? This will remove it from all entities and delete all associated aliases
        and votes.
      </p>

      <DialogFooter>
        <Button
          variant="outline"
          onClick={onCancel}
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
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Deleting...
            </>
          ) : (
            'Delete Tag'
          )}
        </Button>
      </DialogFooter>
    </div>
  )
}

// ============================================================================
// Main Component
// ============================================================================

export function TagManagement() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [categoryFilter, setCategoryFilter] = useState<string>('')
  const [dialogMode, setDialogMode] = useState<DialogMode>(null)
  const [selectedTagId, setSelectedTagId] = useState<number | null>(null)
  const [selectedTagName, setSelectedTagName] = useState('')

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  const {
    data: tagsData,
    isLoading,
    error,
  } = useTags({
    category: categoryFilter || undefined,
    search: debouncedSearch || undefined,
    sort: 'usage',
    limit: 100,
  })

  const tags = tagsData?.tags || []

  const openCreate = useCallback(() => {
    setDialogMode('create')
    setSelectedTagId(null)
    setSelectedTagName('')
  }, [])

  const openEdit = useCallback((tagId: number) => {
    setDialogMode('edit')
    setSelectedTagId(tagId)
  }, [])

  const openDelete = useCallback((tagId: number, name: string) => {
    setDialogMode('delete')
    setSelectedTagId(tagId)
    setSelectedTagName(name)
  }, [])

  const openMerge = useCallback((tagId: number, name: string) => {
    setDialogMode('merge')
    setSelectedTagId(tagId)
    setSelectedTagName(name)
  }, [])

  const closeDialog = useCallback(() => {
    setDialogMode(null)
    setSelectedTagId(null)
    setSelectedTagName('')
  }, [])

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold flex items-center gap-2">
            <Tags className="h-5 w-5" />
            Tags
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            Create, edit, and manage tags and aliases.
          </p>
        </div>
      </div>

      <Tabs defaultValue="tags" className="space-y-4">
        <TabsList>
          <TabsTrigger value="tags">Tags</TabsTrigger>
          <TabsTrigger value="aliases">Aliases</TabsTrigger>
          <TabsTrigger value="needs-review" className="gap-2">
            Needs Review
            <LowQualityBadge />
          </TabsTrigger>
          <TabsTrigger value="hierarchy">Hierarchy</TabsTrigger>
        </TabsList>

        <TabsContent value="tags" className="space-y-4">
          <div className="flex items-center justify-end">
            <Button onClick={openCreate}>
              <Plus className="mr-2 h-4 w-4" />
              New Tag
            </Button>
          </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search tags..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        <Select
          value={toFilterSelectValue(categoryFilter)}
          onValueChange={(value) => setCategoryFilter(fromFilterSelectValue(value))}
        >
          <SelectTrigger className="w-44" aria-label="Filter by category">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={FILTER_SELECT_ALL}>All Categories</SelectItem>
            {TAG_CATEGORIES.map((cat) => (
              <SelectItem key={cat} value={cat}>
                {getCategoryLabel(cat)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error */}
      {error && (
        <InlineErrorBanner variant="queryFallback">
          {error instanceof Error
            ? error.message
            : 'Failed to load tags.'}
        </InlineErrorBanner>
      )}

      {/* Empty state */}
      {!isLoading && !error && tags.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No Tags Found</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {debouncedSearch || categoryFilter
              ? 'No tags match your filters. Try a different search.'
              : 'No tags yet. Create your first tag to get started.'}
          </p>
        </div>
      )}

      {/* Tag list */}
      {!isLoading && !error && tags.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {tags.length} tag{tags.length !== 1 ? 's' : ''}
            {debouncedSearch && ` matching "${debouncedSearch}"`}
            {tags.length < (tagsData?.total ?? 0) && (
              <span> (of {tagsData?.total} total)</span>
            )}
          </div>

          <div className="space-y-2">
            {tags.map((tag) => (
              <div
                key={tag.id}
                className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
              >
                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm truncate">
                      {tag.name}
                    </span>
                    <Badge
                      variant="outline"
                      className={`text-xs flex-shrink-0 ${getCategoryChipClasses(tag.category)}`}
                    >
                      {getCategoryLabel(tag.category)}
                    </Badge>
                    {tag.is_official && (
                      <TagOfficialIndicator size="sm" tagName={tag.name} />
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                    <span>{tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}</span>
                    <span className="text-muted-foreground/50">
                      /{tag.slug}
                    </span>
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openEdit(tag.id)}
                    className="h-8 w-8 p-0"
                    aria-label={`Edit ${tag.name}`}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openMerge(tag.id, tag.name)}
                    className="h-8 w-8 p-0"
                    aria-label={`Merge ${tag.name}`}
                  >
                    <GitMerge className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openDelete(tag.id, tag.name)}
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    aria-label={`Delete ${tag.name}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
        </TabsContent>

        <TabsContent value="aliases">
          <AliasListing />
        </TabsContent>

        <TabsContent value="needs-review">
          <LowQualityTagQueue />
        </TabsContent>

        <TabsContent value="hierarchy">
          <TagHierarchyEditor />
        </TabsContent>
      </Tabs>

      {/* Create Dialog */}
      <Dialog
        open={dialogMode === 'create'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Create Tag</DialogTitle>
            <DialogDescription>
              Add a new tag to the taxonomy.
            </DialogDescription>
          </DialogHeader>
          <CreateTagForm onSuccess={closeDialog} onCancel={closeDialog} />
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog
        open={dialogMode === 'edit'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Tag</DialogTitle>
            <DialogDescription>
              Update tag details and manage aliases.
            </DialogDescription>
          </DialogHeader>
          {selectedTagId && (
            <EditTagForm
              tagId={selectedTagId}
              onSuccess={closeDialog}
              onCancel={closeDialog}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Merge Dialog */}
      <MergeTagDialog
        open={dialogMode === 'merge'}
        sourceTagId={dialogMode === 'merge' ? selectedTagId : null}
        sourceTagName={selectedTagName}
        onClose={closeDialog}
      />

      {/* Delete Dialog */}
      <Dialog
        open={dialogMode === 'delete'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Tag</DialogTitle>
            <DialogDescription>
              This action is permanent and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          {selectedTagId && (
            <DeleteConfirmation
              tagName={selectedTagName}
              tagId={selectedTagId}
              onSuccess={closeDialog}
              onCancel={closeDialog}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default TagManagement
