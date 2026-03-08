'use client'

import { useState, useCallback, useEffect } from 'react'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  Search,
  Inbox,
  Disc3,
  X,
  ExternalLink,
  Music,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { useReleases, useRelease } from '@/lib/hooks/useReleases'
import { useArtistSearch } from '@/lib/hooks/useArtistSearch'
import {
  useCreateRelease,
  useUpdateRelease,
  useDeleteRelease,
  useAddReleaseLink,
  useRemoveReleaseLink,
  type CreateReleaseArtistInput,
  type CreateReleaseLinkInput,
} from '@/lib/hooks/useAdminReleases'
import {
  RELEASE_TYPES,
  RELEASE_TYPE_LABELS,
  getReleaseTypeLabel,
  type ReleaseType,
  type ReleaseDetail,
  type ReleaseExternalLink,
} from '@/lib/types/release'

// ============================================================================
// Constants
// ============================================================================

const ARTIST_ROLES = [
  { value: 'main', label: 'Main' },
  { value: 'featured', label: 'Featured' },
  { value: 'producer', label: 'Producer' },
  { value: 'remixer', label: 'Remixer' },
  { value: 'composer', label: 'Composer' },
  { value: 'dj', label: 'DJ' },
] as const

const EXTERNAL_LINK_PLATFORMS = [
  { value: 'bandcamp', label: 'Bandcamp' },
  { value: 'spotify', label: 'Spotify' },
  { value: 'apple_music', label: 'Apple Music' },
  { value: 'youtube', label: 'YouTube' },
  { value: 'discogs', label: 'Discogs' },
  { value: 'tidal', label: 'Tidal' },
  { value: 'soundcloud', label: 'SoundCloud' },
] as const

type DialogMode = 'create' | 'edit' | 'delete' | null

// ============================================================================
// Artist Picker Sub-Component
// ============================================================================

interface ArtistEntry {
  artist_id: number
  artist_name: string
  role: string
}

function ArtistPicker({
  artists,
  onAdd,
  onRemove,
  onRoleChange,
}: {
  artists: ArtistEntry[]
  onAdd: (artist: ArtistEntry) => void
  onRemove: (index: number) => void
  onRoleChange: (index: number, role: string) => void
}) {
  const [searchQuery, setSearchQuery] = useState('')
  const [showResults, setShowResults] = useState(false)
  const { data: searchData, isLoading: isSearching } = useArtistSearch({
    query: searchQuery,
    debounceMs: 200,
  })

  const handleSelect = useCallback(
    (artistId: number, artistName: string) => {
      // Prevent duplicates
      if (artists.some((a) => a.artist_id === artistId)) return
      onAdd({ artist_id: artistId, artist_name: artistName, role: 'main' })
      setSearchQuery('')
      setShowResults(false)
    },
    [artists, onAdd]
  )

  return (
    <div className="space-y-3">
      <Label>Artists</Label>

      {/* Search input */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search artists..."
          value={searchQuery}
          onChange={(e) => {
            setSearchQuery(e.target.value)
            setShowResults(true)
          }}
          onFocus={() => setShowResults(true)}
          className="pl-9"
        />

        {/* Results dropdown */}
        {showResults && searchQuery.length > 0 && (
          <div className="absolute top-full left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border bg-popover shadow-md">
            {isSearching ? (
              <div className="flex items-center justify-center p-3">
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              </div>
            ) : searchData?.artists && searchData.artists.length > 0 ? (
              searchData.artists.map((artist) => {
                const alreadyAdded = artists.some(
                  (a) => a.artist_id === artist.id
                )
                return (
                  <button
                    key={artist.id}
                    type="button"
                    disabled={alreadyAdded}
                    onClick={() => handleSelect(artist.id, artist.name)}
                    className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Music className="h-3.5 w-3.5 text-muted-foreground" />
                    <span>{artist.name}</span>
                    {artist.city && (
                      <span className="text-xs text-muted-foreground">
                        ({artist.city})
                      </span>
                    )}
                    {alreadyAdded && (
                      <span className="ml-auto text-xs text-muted-foreground">
                        Added
                      </span>
                    )}
                  </button>
                )
              })
            ) : (
              <div className="p-3 text-sm text-muted-foreground text-center">
                No artists found
              </div>
            )}
          </div>
        )}
      </div>

      {/* Selected artists */}
      {artists.length > 0 && (
        <div className="space-y-2">
          {artists.map((artist, index) => (
            <div
              key={artist.artist_id}
              className="flex items-center gap-2 rounded-md border px-3 py-2"
            >
              <span className="text-sm font-medium flex-1">
                {artist.artist_name}
              </span>
              <select
                value={artist.role}
                onChange={(e) => onRoleChange(index, e.target.value)}
                className="h-8 rounded-md border bg-background px-2 text-xs"
              >
                {ARTIST_ROLES.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => onRemove(index)}
                className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ============================================================================
// Link Editor Sub-Component
// ============================================================================

interface LinkEntry {
  platform: string
  url: string
}

function LinkEditor({
  links,
  onAdd,
  onRemove,
}: {
  links: LinkEntry[]
  onAdd: (link: LinkEntry) => void
  onRemove: (index: number) => void
}) {
  const [platform, setPlatform] = useState<string>(EXTERNAL_LINK_PLATFORMS[0].value)
  const [url, setUrl] = useState('')

  const handleAdd = useCallback(() => {
    if (!url.trim()) return
    onAdd({ platform, url: url.trim() })
    setUrl('')
  }, [platform, url, onAdd])

  return (
    <div className="space-y-3">
      <Label>External Links</Label>

      {/* Add new link */}
      <div className="flex items-end gap-2">
        <div className="w-36">
          <select
            value={platform}
            onChange={(e) => setPlatform(e.target.value)}
            className="h-9 w-full rounded-md border bg-background px-2 text-sm"
          >
            {EXTERNAL_LINK_PLATFORMS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </div>
        <div className="flex-1">
          <Input
            placeholder="https://..."
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                handleAdd()
              }
            }}
          />
        </div>
        <Button type="button" variant="outline" size="sm" onClick={handleAdd}>
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      {/* Existing links */}
      {links.length > 0 && (
        <div className="space-y-2">
          {links.map((link, index) => (
            <div
              key={index}
              className="flex items-center gap-2 rounded-md border px-3 py-2"
            >
              <Badge variant="secondary" className="text-xs">
                {EXTERNAL_LINK_PLATFORMS.find((p) => p.value === link.platform)
                  ?.label || link.platform}
              </Badge>
              <span className="text-sm text-muted-foreground truncate flex-1">
                {link.url}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => onRemove(index)}
                className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ============================================================================
// Existing Link Manager (for edit mode — uses add/remove API calls)
// ============================================================================

function ExistingLinkManager({
  releaseId,
  links,
}: {
  releaseId: number
  links: ReleaseExternalLink[]
}) {
  const addLinkMutation = useAddReleaseLink()
  const removeLinkMutation = useRemoveReleaseLink()
  const [platform, setPlatform] = useState<string>(EXTERNAL_LINK_PLATFORMS[0].value)
  const [url, setUrl] = useState('')

  const handleAdd = useCallback(() => {
    if (!url.trim()) return
    addLinkMutation.mutate(
      { releaseId, platform, url: url.trim() },
      {
        onSuccess: () => setUrl(''),
      }
    )
  }, [releaseId, platform, url, addLinkMutation])

  const handleRemove = useCallback(
    (linkId: number) => {
      removeLinkMutation.mutate({ releaseId, linkId })
    },
    [releaseId, removeLinkMutation]
  )

  return (
    <div className="space-y-3">
      <Label>External Links</Label>

      {/* Add new link */}
      <div className="flex items-end gap-2">
        <div className="w-36">
          <select
            value={platform}
            onChange={(e) => setPlatform(e.target.value)}
            className="h-9 w-full rounded-md border bg-background px-2 text-sm"
          >
            {EXTERNAL_LINK_PLATFORMS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </div>
        <div className="flex-1">
          <Input
            placeholder="https://..."
            value={url}
            onChange={(e) => setUrl(e.target.value)}
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
          disabled={addLinkMutation.isPending}
        >
          {addLinkMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Plus className="h-4 w-4" />
          )}
        </Button>
      </div>

      {/* Existing links */}
      {links.length > 0 && (
        <div className="space-y-2">
          {links.map((link) => (
            <div
              key={link.id}
              className="flex items-center gap-2 rounded-md border px-3 py-2"
            >
              <Badge variant="secondary" className="text-xs">
                {EXTERNAL_LINK_PLATFORMS.find(
                  (p) => p.value === link.platform
                )?.label || link.platform}
              </Badge>
              <a
                href={link.url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm text-muted-foreground truncate flex-1 hover:text-foreground"
              >
                {link.url}
              </a>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => handleRemove(link.id)}
                disabled={removeLinkMutation.isPending}
                className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}

      {links.length === 0 && (
        <p className="text-sm text-muted-foreground">No external links yet.</p>
      )}
    </div>
  )
}

// ============================================================================
// Release Form (Create)
// ============================================================================

function CreateReleaseForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: () => void
  onCancel: () => void
}) {
  const createMutation = useCreateRelease()

  const [title, setTitle] = useState('')
  const [releaseType, setReleaseType] = useState<string>('lp')
  const [releaseYear, setReleaseYear] = useState('')
  const [releaseDate, setReleaseDate] = useState('')
  const [coverArtUrl, setCoverArtUrl] = useState('')
  const [description, setDescription] = useState('')
  const [artists, setArtists] = useState<ArtistEntry[]>([])
  const [links, setLinks] = useState<LinkEntry[]>([])
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!title.trim()) {
        setError('Title is required')
        return
      }

      const artistInputs: CreateReleaseArtistInput[] = artists.map((a) => ({
        artist_id: a.artist_id,
        role: a.role,
      }))

      const linkInputs: CreateReleaseLinkInput[] = links.map((l) => ({
        platform: l.platform,
        url: l.url,
      }))

      createMutation.mutate(
        {
          title: title.trim(),
          release_type: releaseType || undefined,
          release_year: releaseYear ? parseInt(releaseYear, 10) : undefined,
          release_date: releaseDate || undefined,
          cover_art_url: coverArtUrl || undefined,
          description: description || undefined,
          artists: artistInputs.length > 0 ? artistInputs : undefined,
          external_links: linkInputs.length > 0 ? linkInputs : undefined,
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to create release'
            )
          },
        }
      )
    },
    [
      title,
      releaseType,
      releaseYear,
      releaseDate,
      coverArtUrl,
      description,
      artists,
      links,
      createMutation,
      onSuccess,
    ]
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="space-y-2">
        <Label htmlFor="create-title">Title *</Label>
        <Input
          id="create-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Album title"
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="create-type">Release Type</Label>
          <select
            id="create-type"
            value={releaseType}
            onChange={(e) => setReleaseType(e.target.value)}
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          >
            {RELEASE_TYPES.map((type) => (
              <option key={type} value={type}>
                {RELEASE_TYPE_LABELS[type]}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="create-year">Year</Label>
          <Input
            id="create-year"
            type="number"
            value={releaseYear}
            onChange={(e) => setReleaseYear(e.target.value)}
            placeholder="2024"
            min="1900"
            max="2100"
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="create-date">Release Date</Label>
        <Input
          id="create-date"
          type="date"
          value={releaseDate}
          onChange={(e) => setReleaseDate(e.target.value)}
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="create-cover">Cover Art URL</Label>
        <Input
          id="create-cover"
          value={coverArtUrl}
          onChange={(e) => setCoverArtUrl(e.target.value)}
          placeholder="https://..."
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="create-desc">Description</Label>
        <Textarea
          id="create-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </div>

      <ArtistPicker
        artists={artists}
        onAdd={(artist) => setArtists((prev) => [...prev, artist])}
        onRemove={(index) =>
          setArtists((prev) => prev.filter((_, i) => i !== index))
        }
        onRoleChange={(index, role) =>
          setArtists((prev) =>
            prev.map((a, i) => (i === index ? { ...a, role } : a))
          )
        }
      />

      <LinkEditor
        links={links}
        onAdd={(link) => setLinks((prev) => [...prev, link])}
        onRemove={(index) =>
          setLinks((prev) => prev.filter((_, i) => i !== index))
        }
      />

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
            'Create Release'
          )}
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Edit Release Form
// ============================================================================

function EditReleaseForm({
  releaseId,
  onSuccess,
  onCancel,
}: {
  releaseId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const { data: release, isLoading } = useRelease({
    idOrSlug: releaseId,
    enabled: releaseId > 0,
  })
  const updateMutation = useUpdateRelease()

  const [title, setTitle] = useState('')
  const [releaseType, setReleaseType] = useState<string>('lp')
  const [releaseYear, setReleaseYear] = useState('')
  const [releaseDate, setReleaseDate] = useState('')
  const [coverArtUrl, setCoverArtUrl] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [initialized, setInitialized] = useState(false)

  // Populate form when release data loads
  useEffect(() => {
    if (release && !initialized) {
      setTitle(release.title)
      setReleaseType(release.release_type || 'lp')
      setReleaseYear(release.release_year?.toString() || '')
      setReleaseDate(release.release_date || '')
      setCoverArtUrl(release.cover_art_url || '')
      setDescription(release.description || '')
      setInitialized(true)
    }
  }, [release, initialized])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!title.trim()) {
        setError('Title is required')
        return
      }

      updateMutation.mutate(
        {
          releaseId,
          data: {
            title: title.trim(),
            release_type: releaseType || undefined,
            release_year: releaseYear ? parseInt(releaseYear, 10) : null,
            release_date: releaseDate || null,
            cover_art_url: coverArtUrl || null,
            description: description || null,
          },
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to update release'
            )
          },
        }
      )
    },
    [
      title,
      releaseType,
      releaseYear,
      releaseDate,
      coverArtUrl,
      description,
      releaseId,
      updateMutation,
      onSuccess,
    ]
  )

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!release) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        Release not found.
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="edit-title">Title *</Label>
          <Input
            id="edit-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Album title"
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="edit-type">Release Type</Label>
            <select
              id="edit-type"
              value={releaseType}
              onChange={(e) => setReleaseType(e.target.value)}
              className="h-9 w-full rounded-md border bg-background px-3 text-sm"
            >
              {RELEASE_TYPES.map((type) => (
                <option key={type} value={type}>
                  {RELEASE_TYPE_LABELS[type]}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit-year">Year</Label>
            <Input
              id="edit-year"
              type="number"
              value={releaseYear}
              onChange={(e) => setReleaseYear(e.target.value)}
              placeholder="2024"
              min="1900"
              max="2100"
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="edit-date">Release Date</Label>
          <Input
            id="edit-date"
            type="date"
            value={releaseDate}
            onChange={(e) => setReleaseDate(e.target.value)}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="edit-cover">Cover Art URL</Label>
          <Input
            id="edit-cover"
            value={coverArtUrl}
            onChange={(e) => setCoverArtUrl(e.target.value)}
            placeholder="https://..."
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="edit-desc">Description</Label>
          <Textarea
            id="edit-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description..."
            rows={3}
          />
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

      {/* Artists (read-only in edit mode — artists are set at creation) */}
      {release.artists.length > 0 && (
        <div className="space-y-2 border-t pt-4">
          <Label>Artists</Label>
          <div className="space-y-1">
            {release.artists.map((artist) => (
              <div
                key={artist.id}
                className="flex items-center gap-2 text-sm"
              >
                <span className="font-medium">{artist.name}</span>
                <Badge variant="outline" className="text-xs">
                  {artist.role}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* External Links (managed via API) */}
      <div className="border-t pt-4">
        <ExistingLinkManager
          releaseId={releaseId}
          links={release.external_links || []}
        />
      </div>
    </div>
  )
}

// ============================================================================
// Delete Confirmation
// ============================================================================

function DeleteConfirmation({
  releaseTitle,
  releaseId,
  onSuccess,
  onCancel,
}: {
  releaseTitle: string
  releaseId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const deleteMutation = useDeleteRelease()
  const [error, setError] = useState<string | null>(null)

  const handleDelete = useCallback(() => {
    setError(null)
    deleteMutation.mutate(releaseId, {
      onSuccess: () => {
        onSuccess()
      },
      onError: (err) => {
        setError(
          err instanceof Error ? err.message : 'Failed to delete release'
        )
      },
    })
  }, [releaseId, deleteMutation, onSuccess])

  return (
    <div className="space-y-4">
      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <p className="text-sm text-muted-foreground">
        Are you sure you want to delete{' '}
        <span className="font-semibold text-foreground">
          &quot;{releaseTitle}&quot;
        </span>
        ? This action cannot be undone. All artist associations and external
        links will also be removed.
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
            'Delete Release'
          )}
        </Button>
      </DialogFooter>
    </div>
  )
}

// ============================================================================
// Main Component
// ============================================================================

export function ReleaseManagement() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [dialogMode, setDialogMode] = useState<DialogMode>(null)
  const [selectedReleaseId, setSelectedReleaseId] = useState<number | null>(
    null
  )
  const [selectedReleaseTitle, setSelectedReleaseTitle] = useState('')

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  const {
    data: releasesData,
    isLoading,
    error,
  } = useReleases({
    releaseType: typeFilter || undefined,
  })

  // Client-side search filtering
  const filteredReleases =
    releasesData?.releases?.filter((release) => {
      if (!debouncedSearch) return true
      return release.title
        .toLowerCase()
        .includes(debouncedSearch.toLowerCase())
    }) || []

  const openCreate = useCallback(() => {
    setDialogMode('create')
    setSelectedReleaseId(null)
    setSelectedReleaseTitle('')
  }, [])

  const openEdit = useCallback((releaseId: number) => {
    setDialogMode('edit')
    setSelectedReleaseId(releaseId)
  }, [])

  const openDelete = useCallback((releaseId: number, title: string) => {
    setDialogMode('delete')
    setSelectedReleaseId(releaseId)
    setSelectedReleaseTitle(title)
  }, [])

  const closeDialog = useCallback(() => {
    setDialogMode(null)
    setSelectedReleaseId(null)
    setSelectedReleaseTitle('')
  }, [])

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold flex items-center gap-2">
            <Disc3 className="h-5 w-5" />
            Releases
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            Create, edit, and manage music releases.
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="mr-2 h-4 w-4" />
          New Release
        </Button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search releases..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-sm"
        >
          <option value="">All Types</option>
          {RELEASE_TYPES.map((type) => (
            <option key={type} value={type}>
              {RELEASE_TYPE_LABELS[type]}
            </option>
          ))}
        </select>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error */}
      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
          <p className="text-destructive">
            {error instanceof Error
              ? error.message
              : 'Failed to load releases.'}
          </p>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && filteredReleases.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No Releases Found</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {debouncedSearch || typeFilter
              ? 'No releases match your filters. Try a different search.'
              : 'No releases yet. Create your first release to get started.'}
          </p>
        </div>
      )}

      {/* Release list */}
      {!isLoading && !error && filteredReleases.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {filteredReleases.length} release
            {filteredReleases.length !== 1 ? 's' : ''}
            {debouncedSearch && ` matching "${debouncedSearch}"`}
          </div>

          <div className="space-y-2">
            {filteredReleases.map((release) => (
              <div
                key={release.id}
                className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
              >
                {/* Cover art or placeholder */}
                <div className="flex h-10 w-10 items-center justify-center rounded bg-muted flex-shrink-0 overflow-hidden">
                  {release.cover_art_url ? (
                    <img
                      src={release.cover_art_url}
                      alt=""
                      className="h-10 w-10 object-cover rounded"
                    />
                  ) : (
                    <Disc3 className="h-5 w-5 text-muted-foreground" />
                  )}
                </div>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm truncate">
                      {release.title}
                    </span>
                    <Badge variant="outline" className="text-xs flex-shrink-0">
                      {getReleaseTypeLabel(release.release_type)}
                    </Badge>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                    {release.release_year && (
                      <span>{release.release_year}</span>
                    )}
                    {release.artist_count > 0 && (
                      <span>
                        {release.artist_count} artist
                        {release.artist_count !== 1 ? 's' : ''}
                      </span>
                    )}
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openEdit(release.id)}
                    className="h-8 w-8 p-0"
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openDelete(release.id, release.title)}
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {/* Create Dialog */}
      <Dialog
        open={dialogMode === 'create'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Create Release</DialogTitle>
            <DialogDescription>
              Add a new music release with artist associations and external
              links.
            </DialogDescription>
          </DialogHeader>
          <CreateReleaseForm onSuccess={closeDialog} onCancel={closeDialog} />
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog
        open={dialogMode === 'edit'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Release</DialogTitle>
            <DialogDescription>
              Update release details and manage external links.
            </DialogDescription>
          </DialogHeader>
          {selectedReleaseId && (
            <EditReleaseForm
              releaseId={selectedReleaseId}
              onSuccess={closeDialog}
              onCancel={closeDialog}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Delete Dialog */}
      <Dialog
        open={dialogMode === 'delete'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Release</DialogTitle>
            <DialogDescription>
              This action is permanent and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          {selectedReleaseId && (
            <DeleteConfirmation
              releaseTitle={selectedReleaseTitle}
              releaseId={selectedReleaseId}
              onSuccess={closeDialog}
              onCancel={closeDialog}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default ReleaseManagement
