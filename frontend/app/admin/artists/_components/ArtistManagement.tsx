'use client'

import { useState } from 'react'
import { Loader2, Plus, X, Trash2, GitMerge, Search } from 'lucide-react'
import { useArtistSearch } from '@/features/artists'
import {
  useArtistAliases,
  useCreateArtistAlias,
  useDeleteArtistAlias,
  useMergeArtists,
} from '@/lib/hooks/admin/useAdminArtists'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Artist, MergeArtistResult } from '@/features/artists'

// --- Alias Manager ---

function AliasManager({ artist }: { artist: Artist }) {
  const [newAlias, setNewAlias] = useState('')
  const [error, setError] = useState<string | null>(null)

  const { data, isLoading } = useArtistAliases(artist.id)
  const createAlias = useCreateArtistAlias()
  const deleteAlias = useDeleteArtistAlias()

  const handleAdd = () => {
    const trimmed = newAlias.trim()
    if (!trimmed) return
    setError(null)
    createAlias.mutate(
      { artistId: artist.id, alias: trimmed },
      {
        onSuccess: () => setNewAlias(''),
        onError: err => setError(err instanceof Error ? err.message : 'Failed to add alias'),
      }
    )
  }

  return (
    <div className="space-y-3">
      <h4 className="text-sm font-medium">Aliases for {artist.name}</h4>

      {isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading...
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {data?.aliases?.map(alias => (
            <Badge key={alias.id} variant="secondary" className="gap-1 pr-1">
              {alias.alias}
              <button
                onClick={() =>
                  deleteAlias.mutate({ artistId: artist.id, aliasId: alias.id })
                }
                disabled={deleteAlias.isPending}
                className="ml-1 rounded-full p-0.5 hover:bg-destructive/20"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
          {(!data?.aliases || data.aliases.length === 0) && (
            <span className="text-sm text-muted-foreground">No aliases</span>
          )}
        </div>
      )}

      <div className="flex gap-2">
        <Input
          placeholder="Add alias..."
          value={newAlias}
          onChange={e => setNewAlias(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && handleAdd()}
          className="flex-1"
        />
        <Button
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

      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}

// --- Artist Search/Select ---

function ArtistSelector({
  label,
  selected,
  onSelect,
  excludeId,
}: {
  label: string
  selected: Artist | null
  onSelect: (artist: Artist | null) => void
  excludeId?: number
}) {
  const [query, setQuery] = useState('')
  const { data: searchData } = useArtistSearch({
    query,
  })

  const filteredResults = searchData?.artists?.filter(a => a.id !== excludeId) ?? []

  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {selected ? (
        <div className="flex items-center gap-2 p-2 rounded-md border bg-muted/50">
          <span className="flex-1 text-sm font-medium">{selected.name}</span>
          <span className="text-xs text-muted-foreground">ID: {selected.id}</span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onSelect(null)
              setQuery('')
            }}
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      ) : (
        <div className="relative">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search artists..."
              value={query}
              onChange={e => setQuery(e.target.value)}
              className="pl-9"
            />
          </div>
          {query.length >= 2 && filteredResults.length > 0 && (
            <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover shadow-md max-h-48 overflow-y-auto">
              {filteredResults.map(artist => (
                <button
                  key={artist.id}
                  onClick={() => {
                    onSelect(artist)
                    setQuery('')
                  }}
                  className="w-full text-left px-3 py-2 text-sm hover:bg-muted transition-colors flex items-center justify-between"
                >
                  <span>{artist.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {[artist.city, artist.state].filter(Boolean).join(', ')}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// --- Merge Dialog ---

function MergeDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [canonical, setCanonical] = useState<Artist | null>(null)
  const [mergeFrom, setMergeFrom] = useState<Artist | null>(null)
  const [result, setResult] = useState<MergeArtistResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  const merge = useMergeArtists()

  const handleMerge = () => {
    if (!canonical || !mergeFrom) return
    setError(null)
    setResult(null)
    merge.mutate(
      {
        canonicalArtistId: canonical.id,
        mergeFromArtistId: mergeFrom.id,
      },
      {
        onSuccess: data => {
          setResult(data)
          setMergeFrom(null)
        },
        onError: err => setError(err instanceof Error ? err.message : 'Merge failed'),
      }
    )
  }

  const handleClose = () => {
    onOpenChange(false)
    setCanonical(null)
    setMergeFrom(null)
    setResult(null)
    setError(null)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitMerge className="h-5 w-5" />
            Merge Artists
          </DialogTitle>
          <DialogDescription>
            Merge a duplicate artist into the canonical one. All shows, releases,
            labels, and other relationships will be transferred. The merged
            artist will be deleted and its name added as an alias.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <ArtistSelector
            label="Keep (canonical)"
            selected={canonical}
            onSelect={setCanonical}
            excludeId={mergeFrom?.id}
          />
          <ArtistSelector
            label="Merge & delete"
            selected={mergeFrom}
            onSelect={setMergeFrom}
            excludeId={canonical?.id}
          />
        </div>

        {result && (
          <div className="rounded-md border border-green-500/20 bg-green-500/10 p-3 text-sm space-y-1">
            <p className="font-medium text-green-700 dark:text-green-400">
              Merged &ldquo;{result.merged_artist_name}&rdquo; successfully
            </p>
            <ul className="text-muted-foreground text-xs space-y-0.5">
              {result.shows_moved > 0 && <li>{result.shows_moved} shows transferred</li>}
              {result.releases_moved > 0 && <li>{result.releases_moved} releases transferred</li>}
              {result.labels_moved > 0 && <li>{result.labels_moved} labels transferred</li>}
              {result.festivals_moved > 0 && <li>{result.festivals_moved} festivals transferred</li>}
              {result.bookmarks_moved > 0 && <li>{result.bookmarks_moved} bookmarks transferred</li>}
              {result.alias_created && <li>Alias created from merged name</li>}
            </ul>
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            {result ? 'Done' : 'Cancel'}
          </Button>
          {!result && (
            <Button
              variant="destructive"
              onClick={handleMerge}
              disabled={!canonical || !mergeFrom || merge.isPending}
            >
              {merge.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Merging...
                </>
              ) : (
                <>
                  <GitMerge className="h-4 w-4 mr-2" />
                  Merge Artists
                </>
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Main Component ---

export function ArtistManagement() {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedArtist, setSelectedArtist] = useState<Artist | null>(null)
  const [showMergeDialog, setShowMergeDialog] = useState(false)

  const { data: searchData, isLoading: searchLoading } = useArtistSearch({
    query: searchQuery,
  })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Artist Management</h2>
          <p className="text-sm text-muted-foreground">
            Manage aliases and merge duplicate artists
          </p>
        </div>
        <Button onClick={() => setShowMergeDialog(true)} variant="outline" className="gap-2">
          <GitMerge className="h-4 w-4" />
          Merge Artists
        </Button>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search artists to manage aliases..."
          value={searchQuery}
          onChange={e => {
            setSearchQuery(e.target.value)
            if (e.target.value.length < 2) setSelectedArtist(null)
          }}
          className="pl-9"
        />
      </div>

      {/* Search Results */}
      {searchQuery.length >= 2 && !selectedArtist && (
        <div className="rounded-md border divide-y">
          {searchLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : searchData?.artists && searchData.artists.length > 0 ? (
            searchData.artists.map(artist => (
              <button
                key={artist.id}
                onClick={() => setSelectedArtist(artist)}
                className="w-full text-left px-4 py-3 hover:bg-muted/50 transition-colors flex items-center justify-between"
              >
                <div>
                  <span className="font-medium">{artist.name}</span>
                  {(artist.city || artist.state) && (
                    <span className="ml-2 text-sm text-muted-foreground">
                      {[artist.city, artist.state].filter(Boolean).join(', ')}
                    </span>
                  )}
                </div>
                <span className="text-xs text-muted-foreground">ID: {artist.id}</span>
              </button>
            ))
          ) : (
            <div className="px-4 py-8 text-center text-sm text-muted-foreground">
              No artists found
            </div>
          )}
        </div>
      )}

      {/* Selected Artist - Alias Manager */}
      {selectedArtist && (
        <div className="rounded-md border p-4 space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium">{selectedArtist.name}</h3>
              <p className="text-sm text-muted-foreground">
                {[selectedArtist.city, selectedArtist.state].filter(Boolean).join(', ') || 'No location'}
                {' \u00b7 '}ID: {selectedArtist.id}
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setSelectedArtist(null)}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          <AliasManager artist={selectedArtist} />
        </div>
      )}

      <MergeDialog open={showMergeDialog} onOpenChange={setShowMergeDialog} />
    </div>
  )
}
