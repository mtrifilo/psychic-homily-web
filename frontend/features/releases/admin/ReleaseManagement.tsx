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
import {
  AdminFormLayout,
  AdminFormRow,
  AdminFormField,
} from '@/components/admin/AdminFormLayout'
import { InlineErrorBanner } from '@/components/shared'
import {
  FILTER_SELECT_ALL,
  toFilterSelectValue,
  fromFilterSelectValue,
} from '@/lib/filterSelectValue'
import { useReleases, useRelease } from '../hooks/useReleases'
import { useArtistSearch } from '@/features/artists'
import {
  useCreateRelease,
  useUpdateRelease,
  useDeleteRelease,
  useAddReleaseLink,
  useRemoveReleaseLink,
  type CreateReleaseArtistInput,
  type CreateReleaseLinkInput,
} from '../hooks/useAdminReleases'
import {
  RELEASE_TYPES,
  RELEASE_TYPE_LABELS,
  getReleaseTypeLabel,
  type ReleaseType,
  type ReleaseDetail,
  type ReleaseExternalLink,
} from '../types'

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
              <Select
                value={artist.role}
                onValueChange={(value) => onRoleChange(index, value)}
              >
                <SelectTrigger
                  size="sm"
                  className="w-32 text-xs"
                  aria-label={`Role for ${artist.artist_name}`}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ARTIST_ROLES.map((r) => (
                    <SelectItem key={r.value} value={r.value}>
                      {r.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => onRemove(index)}
                className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                aria-label={`Remove ${artist.artist_name}`}
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
          <Select value={platform} onValueChange={setPlatform}>
            <SelectTrigger className="w-full" aria-label="External link platform">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EXTERNAL_LINK_PLATFORMS.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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
          aria-label="Add external link"
        >
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
                aria-label={`Remove link ${link.url}`}
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
          <Select value={platform} onValueChange={setPlatform}>
            <SelectTrigger className="w-full" aria-label="External link platform">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EXTERNAL_LINK_PLATFORMS.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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
          aria-label="Add external link"
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
                aria-label={`Remove link ${link.url}`}
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

// Exported only for direct regression-test access (reset-on-open). Production
// callers render it from ReleaseManagement.
export function CreateReleaseForm({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
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

  // Reset on (re)open — AdminFormLayout keeps the form mounted across the Sheet
  // close animation. Adjust state during render (not in an effect) per the
  // canonical CreateStationForm pattern (PSY-911/930). Note the embedded
  // ArtistPicker / LinkEditor hold their own draft state and remount with the
  // form, so the artists/links arrays here are the load-bearing reset.
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setTitle('')
      setReleaseType('lp')
      setReleaseYear('')
      setReleaseDate('')
      setCoverArtUrl('')
      setDescription('')
      setArtists([])
      setLinks([])
      setError(null)
    }
  }

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
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Create Release"
      description="Add a new music release with artist associations and external links."
      error={error || undefined}
      onSubmit={handleSubmit}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
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
        </>
      }
    >
      <AdminFormField label="Title *" htmlFor="create-title">
        <Input
          id="create-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Album title"
        />
      </AdminFormField>

      <AdminFormRow cols={2}>
        <AdminFormField label="Release Type" htmlFor="create-type">
          <Select value={releaseType} onValueChange={setReleaseType}>
            <SelectTrigger id="create-type" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RELEASE_TYPES.map((type) => (
                <SelectItem key={type} value={type}>
                  {RELEASE_TYPE_LABELS[type]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Year" htmlFor="create-year">
          <Input
            id="create-year"
            type="number"
            value={releaseYear}
            onChange={(e) => setReleaseYear(e.target.value)}
            placeholder="2024"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Release Date" htmlFor="create-date">
        <Input
          id="create-date"
          type="date"
          value={releaseDate}
          onChange={(e) => setReleaseDate(e.target.value)}
        />
      </AdminFormField>

      <AdminFormField label="Cover Art URL" htmlFor="create-cover">
        <Input
          id="create-cover"
          value={coverArtUrl}
          onChange={(e) => setCoverArtUrl(e.target.value)}
          placeholder="https://..."
        />
      </AdminFormField>

      <AdminFormField label="Description" htmlFor="create-desc">
        <Textarea
          id="create-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

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
    </AdminFormLayout>
  )
}

// ============================================================================
// Edit Release Form
// ============================================================================

// Per the PSY-930 decision the Edit Sheet opens immediately on click: while the
// release detail loads this wrapper renders an AdminFormLayout (open) with a
// spinner body — `open` stays true throughout, so the Sheet stays open — then
// swaps to EditReleaseFormFields (keyed on release.id, so a
// switch-release-without-closing-dialog scenario remounts with fresh state)
// once the detail resolves. The inner component initializes local state from
// props inline (React's preferred "calculate during render" path — see
// https://react.dev/learn/you-might-not-need-an-effect). No useEffect, no
// `initialized` ratchet.
function EditReleaseForm({
  releaseId,
  open,
  onOpenChange,
  onSuccess,
}: {
  releaseId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const { data: release, isLoading } = useRelease({
    idOrSlug: releaseId,
    enabled: releaseId > 0,
  })

  if (isLoading || !release) {
    return (
      <AdminFormLayout
        variant="sheet"
        open={open}
        onOpenChange={onOpenChange}
        title="Edit Release"
        description="Update release details and manage external links."
        onSubmit={(e) => e.preventDefault()}
        footer={
          <>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled>
              Save Changes
            </Button>
          </>
        }
      >
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <div className="text-center py-8 text-muted-foreground">
            Release not found.
          </div>
        )}
      </AdminFormLayout>
    )
  }

  return (
    <EditReleaseFormFields
      key={release.id}
      release={release}
      open={open}
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  )
}

// Exported only for direct regression-test access (rerender-with-different-key
// resets fields; rerender-with-same-key preserves dirty edits). Not part of
// the surface's public API — production callers use EditReleaseForm.
export function EditReleaseFormFields({
  release,
  open,
  onOpenChange,
  onSuccess,
}: {
  release: ReleaseDetail
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const updateMutation = useUpdateRelease()

  const [title, setTitle] = useState(release.title)
  const [releaseType, setReleaseType] = useState<string>(
    release.release_type || 'lp'
  )
  const [releaseYear, setReleaseYear] = useState(
    release.release_year?.toString() || ''
  )
  const [releaseDate, setReleaseDate] = useState(release.release_date || '')
  const [coverArtUrl, setCoverArtUrl] = useState(release.cover_art_url || '')
  const [description, setDescription] = useState(release.description || '')
  const [error, setError] = useState<string | null>(null)

  const releaseId = release.id

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

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Edit Release"
      description="Update release details and manage external links."
      error={error || undefined}
      onSubmit={handleSubmit}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
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
        </>
      }
    >
      <AdminFormField label="Title *" htmlFor="edit-title">
        <Input
          id="edit-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Album title"
        />
      </AdminFormField>

      <AdminFormRow cols={2}>
        <AdminFormField label="Release Type" htmlFor="edit-type">
          <Select value={releaseType} onValueChange={setReleaseType}>
            <SelectTrigger id="edit-type" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RELEASE_TYPES.map((type) => (
                <SelectItem key={type} value={type}>
                  {RELEASE_TYPE_LABELS[type]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Year" htmlFor="edit-year">
          <Input
            id="edit-year"
            type="number"
            value={releaseYear}
            onChange={(e) => setReleaseYear(e.target.value)}
            placeholder="2024"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Release Date" htmlFor="edit-date">
        <Input
          id="edit-date"
          type="date"
          value={releaseDate}
          onChange={(e) => setReleaseDate(e.target.value)}
        />
      </AdminFormField>

      <AdminFormField label="Cover Art URL" htmlFor="edit-cover">
        <Input
          id="edit-cover"
          value={coverArtUrl}
          onChange={(e) => setCoverArtUrl(e.target.value)}
          placeholder="https://..."
        />
      </AdminFormField>

      <AdminFormField label="Description" htmlFor="edit-desc">
        <Textarea
          id="edit-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

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
    </AdminFormLayout>
  )
}

// ============================================================================
// Delete Confirmation
// ============================================================================

// Short confirm form -> centered Modal (variant="modal"), per the PSY-912
// Hybrid decision.
function DeleteConfirmation({
  releaseTitle,
  releaseId,
  open,
  onOpenChange,
  onSuccess,
}: {
  releaseTitle: string
  releaseId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
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
    <AdminFormLayout
      variant="modal"
      open={open}
      onOpenChange={onOpenChange}
      title="Delete Release"
      description="This action is permanent and cannot be undone."
      error={error || undefined}
      onSubmit={(e) => {
        e.preventDefault()
        handleDelete()
      }}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={deleteMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            variant="destructive"
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
        </>
      }
    >
      <p className="text-sm text-muted-foreground">
        Are you sure you want to delete{' '}
        <span className="font-semibold text-foreground">
          &quot;{releaseTitle}&quot;
        </span>
        ? This action cannot be undone. All artist associations and external
        links will also be removed.
      </p>
    </AdminFormLayout>
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

  // Close by clearing dialogMode only. The selected id/title persist so the
  // mounted Edit/Delete AdminFormLayout can animate closed (its `open` is
  // driven by dialogMode); openEdit/openDelete overwrite them on the next open.
  // Nulling the id here would unmount the form mid-animation and flash its
  // empty/not-found state. (PSY-930)
  const closeDialog = useCallback(() => {
    setDialogMode(null)
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
        <Select
          value={toFilterSelectValue(typeFilter)}
          onValueChange={(value) => setTypeFilter(fromFilterSelectValue(value))}
        >
          <SelectTrigger className="w-44" aria-label="Filter by type">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={FILTER_SELECT_ALL}>All Types</SelectItem>
            {RELEASE_TYPES.map((type) => (
              <SelectItem key={type} value={type}>
                {RELEASE_TYPE_LABELS[type]}
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
            : 'Failed to load releases.'}
        </InlineErrorBanner>
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
                    aria-label={`Edit ${release.title}`}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openDelete(release.id, release.title)}
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    aria-label={`Delete ${release.title}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {/* Create — right-anchored Sheet (PSY-930 AdminFormLayout) */}
      <CreateReleaseForm
        open={dialogMode === 'create'}
        onOpenChange={(open) => !open && closeDialog()}
        onSuccess={closeDialog}
      />

      {/* Edit — right-anchored Sheet (PSY-930 AdminFormLayout) */}
      {selectedReleaseId && (
        <EditReleaseForm
          releaseId={selectedReleaseId}
          open={dialogMode === 'edit'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}

      {/* Delete — centered Modal (PSY-930 AdminFormLayout) */}
      {selectedReleaseId && (
        <DeleteConfirmation
          releaseTitle={selectedReleaseTitle}
          releaseId={selectedReleaseId}
          open={dialogMode === 'delete'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}
    </div>
  )
}

export default ReleaseManagement
