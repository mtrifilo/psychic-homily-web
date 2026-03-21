'use client'

import { useState, useEffect, useCallback } from 'react'
import { Loader2, X, Search, Plus } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { useCreateFilter, useUpdateFilter } from '../hooks'
import type { NotificationFilter, CreateFilterInput } from '../types'
import { useArtistSearch } from '@/features/artists/hooks/useArtistSearch'
import { useVenueSearch } from '@/features/venues/hooks/useVenueSearch'
import { useSearchTags } from '@/features/tags/hooks'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { useQuery } from '@tanstack/react-query'

// ──────────────────────────────────────────────
// Multi-select search combobox
// ──────────────────────────────────────────────

interface SearchableItem {
  id: number
  name: string
}

interface MultiSelectSearchProps {
  label: string
  placeholder: string
  selectedIds: number[]
  onSelectionChange: (ids: number[]) => void
  searchHook: (query: string) => { data: SearchableItem[] | undefined; isLoading: boolean }
  /** Pre-resolved items for edit mode (id + name pairs for existing selections) */
  initialItems?: SearchableItem[]
}

function MultiSelectSearch({
  label,
  placeholder,
  selectedIds,
  onSelectionChange,
  searchHook,
  initialItems,
}: MultiSelectSearchProps) {
  const [query, setQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)
  const [selectedItems, setSelectedItems] = useState<SearchableItem[]>([])
  const { data: results, isLoading } = searchHook(query)

  // Sync initialItems into selectedItems when they become available (edit mode hydration).
  // When initialItems is undefined/empty (create mode), clear selectedItems to match selectedIds.
  useEffect(() => {
    if (initialItems && initialItems.length > 0) {
      setSelectedItems(initialItems)
    } else if (selectedIds.length === 0) {
      setSelectedItems([])
    }
  }, [initialItems, selectedIds.length])

  const handleSelect = useCallback(
    (item: SearchableItem) => {
      if (!selectedIds.includes(item.id)) {
        const newIds = [...selectedIds, item.id]
        onSelectionChange(newIds)
        setSelectedItems(prev => [...prev, item])
      }
      setQuery('')
      setIsOpen(false)
    },
    [selectedIds, onSelectionChange]
  )

  const handleRemove = useCallback(
    (id: number) => {
      onSelectionChange(selectedIds.filter(sid => sid !== id))
      setSelectedItems(prev => prev.filter(item => item.id !== id))
    },
    [selectedIds, onSelectionChange]
  )

  // Filter out already-selected items from results
  const filteredResults = results?.filter(r => !selectedIds.includes(r.id)) ?? []

  return (
    <div className="space-y-2">
      <Label className="text-sm font-medium">{label}</Label>

      {/* Selected items */}
      {selectedItems.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {selectedItems.map(item => (
            <Badge
              key={item.id}
              variant="secondary"
              className="gap-1 pr-1"
            >
              {item.name}
              <button
                type="button"
                onClick={() => handleRemove(item.id)}
                className="ml-0.5 rounded-sm hover:bg-muted-foreground/20 p-0.5"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Search input */}
      <div className="relative">
        <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={placeholder}
          value={query}
          onChange={e => {
            setQuery(e.target.value)
            setIsOpen(e.target.value.length > 0)
          }}
          onFocus={() => {
            if (query.length > 0) setIsOpen(true)
          }}
          onBlur={() => {
            // Delay to allow click on results
            setTimeout(() => setIsOpen(false), 200)
          }}
          className="pl-9"
        />

        {/* Dropdown results */}
        {isOpen && query.length > 0 && (
          <div className="absolute z-50 top-full left-0 right-0 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover shadow-md">
            {isLoading ? (
              <div className="flex items-center justify-center py-4">
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              </div>
            ) : filteredResults.length === 0 ? (
              <div className="py-3 text-center text-xs text-muted-foreground">
                No results found
              </div>
            ) : (
              filteredResults.map(item => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => handleSelect(item)}
                  className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-muted/50 transition-colors text-left"
                >
                  <Plus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                  {item.name}
                </button>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────
// Search hook adapters
// ──────────────────────────────────────────────

function useArtistSearchAdapter(query: string) {
  const { data, isLoading } = useArtistSearch({ query })
  return {
    data: data?.artists?.map(a => ({ id: a.id, name: a.name })),
    isLoading,
  }
}

function useVenueSearchAdapter(query: string) {
  const { data, isLoading } = useVenueSearch({ query })
  return {
    data: data?.venues?.map(v => ({ id: v.id, name: v.name })),
    isLoading,
  }
}

function useTagSearchAdapter(query: string) {
  const { data, isLoading } = useSearchTags(query)
  return {
    data: data?.tags?.map(t => ({ id: t.id, name: t.name })),
    isLoading,
  }
}

function useLabelSearchAdapter(query: string) {
  const { data, isLoading } = useQuery({
    queryKey: ['labels', 'search', query.toLowerCase()],
    queryFn: () =>
      apiRequest<{ labels: Array<{ id: number; name: string; slug: string }> }>(
        `${API_ENDPOINTS.LABELS.LIST}?search=${encodeURIComponent(query)}&limit=10`
      ),
    enabled: query.length > 0,
    staleTime: 5 * 60 * 1000,
  })
  return {
    data: data?.labels?.map(l => ({ id: l.id, name: l.name })),
    isLoading,
  }
}

// ──────────────────────────────────────────────
// Entity name resolution for edit mode
// ──────────────────────────────────────────────

/**
 * Resolves an array of entity IDs to SearchableItem[] (id + name).
 * Fetches each entity individually via the GET endpoint.
 * Returns empty array while loading or if ids is empty.
 */
function useResolveEntityNames(
  ids: number[] | null | undefined,
  entityType: 'artist' | 'venue' | 'label' | 'tag',
  enabled: boolean
) {
  const stableKey = ids?.slice().sort().join(',') ?? ''

  return useQuery({
    queryKey: ['entity-names', entityType, stableKey],
    queryFn: async (): Promise<SearchableItem[]> => {
      if (!ids || ids.length === 0) return []

      const getEndpoint = (id: number) => {
        switch (entityType) {
          case 'artist':
            return API_ENDPOINTS.ARTISTS.GET(id)
          case 'venue':
            return API_ENDPOINTS.VENUES.GET(id)
          case 'label':
            return API_ENDPOINTS.LABELS.GET(id)
          case 'tag':
            return API_ENDPOINTS.TAGS.GET(id)
        }
      }

      const results = await Promise.all(
        ids.map(async (id) => {
          try {
            const data = await apiRequest<{ id: number; name: string }>(getEndpoint(id))
            return { id: data.id, name: data.name }
          } catch {
            // If an entity was deleted, show the ID as a fallback
            return { id, name: `#${id}` }
          }
        })
      )
      return results
    },
    enabled: enabled && !!ids && ids.length > 0,
    staleTime: 10 * 60 * 1000, // 10 minutes — entity names rarely change
  })
}

// ──────────────────────────────────────────────
// FilterForm component
// ──────────────────────────────────────────────

interface FilterFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** If editing, pass the existing filter; for create, pass undefined */
  filter?: NotificationFilter
}

export function FilterForm({ open, onOpenChange, filter }: FilterFormProps) {
  const isEditing = !!filter

  // Form state
  const [name, setName] = useState('')
  const [artistIds, setArtistIds] = useState<number[]>([])
  const [venueIds, setVenueIds] = useState<number[]>([])
  const [labelIds, setLabelIds] = useState<number[]>([])
  const [tagIds, setTagIds] = useState<number[]>([])
  const [excludeTagIds, setExcludeTagIds] = useState<number[]>([])
  const [priceMaxCents, setPriceMaxCents] = useState<string>('')
  const [notifyEmail, setNotifyEmail] = useState(true)
  const [notifyInApp, setNotifyInApp] = useState(true)

  const createFilter = useCreateFilter()
  const updateFilter = useUpdateFilter()

  const isMutating = createFilter.isPending || updateFilter.isPending

  // Resolve entity IDs to names for edit mode hydration
  const { data: resolvedArtists } = useResolveEntityNames(filter?.artist_ids, 'artist', open && isEditing)
  const { data: resolvedVenues } = useResolveEntityNames(filter?.venue_ids, 'venue', open && isEditing)
  const { data: resolvedLabels } = useResolveEntityNames(filter?.label_ids, 'label', open && isEditing)
  const { data: resolvedTags } = useResolveEntityNames(filter?.tag_ids, 'tag', open && isEditing)
  const { data: resolvedExcludeTags } = useResolveEntityNames(filter?.exclude_tag_ids, 'tag', open && isEditing)

  // Populate form when editing
  useEffect(() => {
    if (filter) {
      setName(filter.name)
      setArtistIds(filter.artist_ids ?? [])
      setVenueIds(filter.venue_ids ?? [])
      setLabelIds(filter.label_ids ?? [])
      setTagIds(filter.tag_ids ?? [])
      setExcludeTagIds(filter.exclude_tag_ids ?? [])
      setPriceMaxCents(
        filter.price_max_cents != null
          ? String(filter.price_max_cents / 100)
          : ''
      )
      setNotifyEmail(filter.notify_email)
      setNotifyInApp(filter.notify_in_app)
    } else {
      // Reset for create
      setName('')
      setArtistIds([])
      setVenueIds([])
      setLabelIds([])
      setTagIds([])
      setExcludeTagIds([])
      setPriceMaxCents('')
      setNotifyEmail(true)
      setNotifyInApp(true)
    }
  }, [filter, open])

  const hasCriteria =
    artistIds.length > 0 ||
    venueIds.length > 0 ||
    labelIds.length > 0 ||
    tagIds.length > 0 ||
    excludeTagIds.length > 0 ||
    priceMaxCents !== ''

  const canSubmit = name.trim().length > 0 && hasCriteria && !isMutating

  const handleSubmit = () => {
    if (!canSubmit) return

    const priceValue = priceMaxCents.trim()
      ? Math.round(parseFloat(priceMaxCents) * 100)
      : undefined

    if (isEditing && filter) {
      updateFilter.mutate(
        {
          id: filter.id,
          name: name.trim(),
          artist_ids: artistIds.length > 0 ? artistIds : undefined,
          venue_ids: venueIds.length > 0 ? venueIds : undefined,
          label_ids: labelIds.length > 0 ? labelIds : undefined,
          tag_ids: tagIds.length > 0 ? tagIds : undefined,
          exclude_tag_ids: excludeTagIds.length > 0 ? excludeTagIds : undefined,
          price_max_cents: priceValue ?? null,
          notify_email: notifyEmail,
          notify_in_app: notifyInApp,
        },
        {
          onSuccess: () => onOpenChange(false),
        }
      )
    } else {
      const input: CreateFilterInput = {
        name: name.trim(),
        notify_email: notifyEmail,
        notify_in_app: notifyInApp,
      }
      if (artistIds.length > 0) input.artist_ids = artistIds
      if (venueIds.length > 0) input.venue_ids = venueIds
      if (labelIds.length > 0) input.label_ids = labelIds
      if (tagIds.length > 0) input.tag_ids = tagIds
      if (excludeTagIds.length > 0) input.exclude_tag_ids = excludeTagIds
      if (priceValue != null) input.price_max_cents = priceValue

      createFilter.mutate(input, {
        onSuccess: () => onOpenChange(false),
      })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? 'Edit Notification Filter' : 'New Notification Filter'}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update the criteria for this notification filter.'
              : 'Create a filter to get notified when matching shows are approved. At least one criteria is required.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5 py-4">
          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="filter-name" className="text-sm font-medium">
              Filter Name
            </Label>
            <Input
              id="filter-name"
              placeholder="e.g., PHX punk shows"
              value={name}
              onChange={e => setName(e.target.value)}
              maxLength={128}
            />
          </div>

          {/* Artists */}
          <MultiSelectSearch
            label="Artists"
            placeholder="Search artists..."
            selectedIds={artistIds}
            onSelectionChange={setArtistIds}
            searchHook={useArtistSearchAdapter}
            initialItems={resolvedArtists}
          />

          {/* Venues */}
          <MultiSelectSearch
            label="Venues"
            placeholder="Search venues..."
            selectedIds={venueIds}
            onSelectionChange={setVenueIds}
            searchHook={useVenueSearchAdapter}
            initialItems={resolvedVenues}
          />

          {/* Labels */}
          <MultiSelectSearch
            label="Labels"
            placeholder="Search labels..."
            selectedIds={labelIds}
            onSelectionChange={setLabelIds}
            searchHook={useLabelSearchAdapter}
            initialItems={resolvedLabels}
          />

          {/* Tags (include) */}
          <MultiSelectSearch
            label="Tags (match any)"
            placeholder="Search tags..."
            selectedIds={tagIds}
            onSelectionChange={setTagIds}
            searchHook={useTagSearchAdapter}
            initialItems={resolvedTags}
          />

          {/* Tags (exclude) */}
          <MultiSelectSearch
            label="Exclude Tags"
            placeholder="Search tags to exclude..."
            selectedIds={excludeTagIds}
            onSelectionChange={setExcludeTagIds}
            searchHook={useTagSearchAdapter}
            initialItems={resolvedExcludeTags}
          />

          {/* Max price */}
          <div className="space-y-2">
            <Label htmlFor="filter-price" className="text-sm font-medium">
              Max Price
            </Label>
            <div className="relative">
              <span className="absolute left-3 top-2.5 text-sm text-muted-foreground">$</span>
              <Input
                id="filter-price"
                type="number"
                min="0"
                step="1"
                placeholder="Leave blank for any price"
                value={priceMaxCents}
                onChange={e => setPriceMaxCents(e.target.value)}
                className="pl-7"
              />
            </div>
            <p className="text-xs text-muted-foreground">
              Enter 0 for free shows only, or leave blank for any price.
            </p>
          </div>

          {/* Notification channels */}
          <div className="space-y-3">
            <Label className="text-sm font-medium">Notify via</Label>
            <div className="flex items-center justify-between">
              <span className="text-sm">Email</span>
              <Switch
                checked={notifyEmail}
                onCheckedChange={setNotifyEmail}
              />
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">
                In-app <span className="text-xs">(coming soon)</span>
              </span>
              <Switch
                checked={notifyInApp}
                onCheckedChange={setNotifyInApp}
              />
            </div>
          </div>

          {!hasCriteria && name.trim().length > 0 && (
            <p className="text-xs text-amber-600 dark:text-amber-400">
              Add at least one criteria (artist, venue, label, tag, or price) to create this filter.
            </p>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 pt-2">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isMutating}
          >
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!canSubmit}
          >
            {isMutating ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                {isEditing ? 'Saving...' : 'Creating...'}
              </>
            ) : isEditing ? (
              'Save Changes'
            ) : (
              'Create Filter'
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
