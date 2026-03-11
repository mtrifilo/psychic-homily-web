'use client'

import { useState, useCallback, useEffect } from 'react'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  Search,
  Inbox,
  Tag,
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
import { useLabels, useLabel } from '../hooks/useLabels'
import {
  useCreateLabel,
  useUpdateLabel,
  useDeleteLabel,
} from '../hooks/useAdminLabels'
import {
  LABEL_STATUSES,
  LABEL_STATUS_LABELS,
  getLabelStatusLabel,
  getLabelStatusVariant,
  formatLabelLocation,
  type LabelStatus,
} from '../types'

// ============================================================================
// Constants
// ============================================================================

type DialogMode = 'create' | 'edit' | 'delete' | null

// ============================================================================
// Create Label Form
// ============================================================================

function CreateLabelForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: () => void
  onCancel: () => void
}) {
  const createMutation = useCreateLabel()

  const [name, setName] = useState('')
  const [city, setCity] = useState('')
  const [state, setState] = useState('')
  const [country, setCountry] = useState('')
  const [foundedYear, setFoundedYear] = useState('')
  const [status, setStatus] = useState<string>('active')
  const [description, setDescription] = useState('')
  const [instagram, setInstagram] = useState('')
  const [facebook, setFacebook] = useState('')
  const [twitter, setTwitter] = useState('')
  const [youtube, setYoutube] = useState('')
  const [spotify, setSpotify] = useState('')
  const [soundcloud, setSoundcloud] = useState('')
  const [bandcamp, setBandcamp] = useState('')
  const [website, setWebsite] = useState('')
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
          city: city || undefined,
          state: state || undefined,
          country: country || undefined,
          founded_year: foundedYear ? parseInt(foundedYear, 10) : undefined,
          status: status || undefined,
          description: description || undefined,
          instagram: instagram || undefined,
          facebook: facebook || undefined,
          twitter: twitter || undefined,
          youtube: youtube || undefined,
          spotify: spotify || undefined,
          soundcloud: soundcloud || undefined,
          bandcamp: bandcamp || undefined,
          website: website || undefined,
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to create label'
            )
          },
        }
      )
    },
    [
      name,
      city,
      state,
      country,
      foundedYear,
      status,
      description,
      instagram,
      facebook,
      twitter,
      youtube,
      spotify,
      soundcloud,
      bandcamp,
      website,
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
        <Label htmlFor="create-name">Name *</Label>
        <Input
          id="create-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Label name"
        />
      </div>

      <div className="grid grid-cols-3 gap-4">
        <div className="space-y-2">
          <Label htmlFor="create-city">City</Label>
          <Input
            id="create-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Seattle"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="create-state">State</Label>
          <Input
            id="create-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="WA"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="create-country">Country</Label>
          <Input
            id="create-country"
            value={country}
            onChange={(e) => setCountry(e.target.value)}
            placeholder="US"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="create-status">Status</Label>
          <select
            id="create-status"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          >
            {LABEL_STATUSES.map((s) => (
              <option key={s} value={s}>
                {LABEL_STATUS_LABELS[s]}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="create-founded-year">Founded Year</Label>
          <Input
            id="create-founded-year"
            type="number"
            value={foundedYear}
            onChange={(e) => setFoundedYear(e.target.value)}
            placeholder="1988"
            min="1900"
            max="2100"
          />
        </div>
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

      {/* Social Links */}
      <div className="space-y-3">
        <Label>Social Links</Label>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <Label htmlFor="create-website" className="text-xs text-muted-foreground">
              Website
            </Label>
            <Input
              id="create-website"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-bandcamp" className="text-xs text-muted-foreground">
              Bandcamp
            </Label>
            <Input
              id="create-bandcamp"
              value={bandcamp}
              onChange={(e) => setBandcamp(e.target.value)}
              placeholder="https://label.bandcamp.com"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-instagram" className="text-xs text-muted-foreground">
              Instagram
            </Label>
            <Input
              id="create-instagram"
              value={instagram}
              onChange={(e) => setInstagram(e.target.value)}
              placeholder="@handle"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-twitter" className="text-xs text-muted-foreground">
              Twitter
            </Label>
            <Input
              id="create-twitter"
              value={twitter}
              onChange={(e) => setTwitter(e.target.value)}
              placeholder="@handle"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-spotify" className="text-xs text-muted-foreground">
              Spotify
            </Label>
            <Input
              id="create-spotify"
              value={spotify}
              onChange={(e) => setSpotify(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-facebook" className="text-xs text-muted-foreground">
              Facebook
            </Label>
            <Input
              id="create-facebook"
              value={facebook}
              onChange={(e) => setFacebook(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-youtube" className="text-xs text-muted-foreground">
              YouTube
            </Label>
            <Input
              id="create-youtube"
              value={youtube}
              onChange={(e) => setYoutube(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="create-soundcloud" className="text-xs text-muted-foreground">
              SoundCloud
            </Label>
            <Input
              id="create-soundcloud"
              value={soundcloud}
              onChange={(e) => setSoundcloud(e.target.value)}
              placeholder="https://..."
            />
          </div>
        </div>
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
            'Create Label'
          )}
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Edit Label Form
// ============================================================================

function EditLabelForm({
  labelId,
  onSuccess,
  onCancel,
}: {
  labelId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const { data: label, isLoading } = useLabel({
    idOrSlug: labelId,
    enabled: labelId > 0,
  })
  const updateMutation = useUpdateLabel()

  const [name, setName] = useState('')
  const [city, setCity] = useState('')
  const [state, setState] = useState('')
  const [country, setCountry] = useState('')
  const [foundedYear, setFoundedYear] = useState('')
  const [status, setStatus] = useState<string>('active')
  const [description, setDescription] = useState('')
  const [instagram, setInstagram] = useState('')
  const [facebook, setFacebook] = useState('')
  const [twitter, setTwitter] = useState('')
  const [youtube, setYoutube] = useState('')
  const [spotify, setSpotify] = useState('')
  const [soundcloud, setSoundcloud] = useState('')
  const [bandcamp, setBandcamp] = useState('')
  const [website, setWebsite] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [initialized, setInitialized] = useState(false)

  // Populate form when label data loads
  useEffect(() => {
    if (label && !initialized) {
      setName(label.name)
      setCity(label.city || '')
      setState(label.state || '')
      setCountry(label.country || '')
      setFoundedYear(label.founded_year?.toString() || '')
      setStatus(label.status || 'active')
      setDescription(label.description || '')
      setInstagram(label.social?.instagram || '')
      setFacebook(label.social?.facebook || '')
      setTwitter(label.social?.twitter || '')
      setYoutube(label.social?.youtube || '')
      setSpotify(label.social?.spotify || '')
      setSoundcloud(label.social?.soundcloud || '')
      setBandcamp(label.social?.bandcamp || '')
      setWebsite(label.social?.website || '')
      setInitialized(true)
    }
  }, [label, initialized])

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
          labelId,
          data: {
            name: name.trim(),
            city: city || null,
            state: state || null,
            country: country || null,
            founded_year: foundedYear ? parseInt(foundedYear, 10) : null,
            status: status || null,
            description: description || null,
            instagram: instagram || null,
            facebook: facebook || null,
            twitter: twitter || null,
            youtube: youtube || null,
            spotify: spotify || null,
            soundcloud: soundcloud || null,
            bandcamp: bandcamp || null,
            website: website || null,
          },
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to update label'
            )
          },
        }
      )
    },
    [
      name,
      city,
      state,
      country,
      foundedYear,
      status,
      description,
      instagram,
      facebook,
      twitter,
      youtube,
      spotify,
      soundcloud,
      bandcamp,
      website,
      labelId,
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

  if (!label) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        Label not found.
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="space-y-2">
        <Label htmlFor="edit-name">Name *</Label>
        <Input
          id="edit-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Label name"
        />
      </div>

      <div className="grid grid-cols-3 gap-4">
        <div className="space-y-2">
          <Label htmlFor="edit-city">City</Label>
          <Input
            id="edit-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Seattle"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="edit-state">State</Label>
          <Input
            id="edit-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="WA"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="edit-country">Country</Label>
          <Input
            id="edit-country"
            value={country}
            onChange={(e) => setCountry(e.target.value)}
            placeholder="US"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="edit-status">Status</Label>
          <select
            id="edit-status"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          >
            {LABEL_STATUSES.map((s) => (
              <option key={s} value={s}>
                {LABEL_STATUS_LABELS[s]}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="edit-founded-year">Founded Year</Label>
          <Input
            id="edit-founded-year"
            type="number"
            value={foundedYear}
            onChange={(e) => setFoundedYear(e.target.value)}
            placeholder="1988"
            min="1900"
            max="2100"
          />
        </div>
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

      {/* Social Links */}
      <div className="space-y-3">
        <Label>Social Links</Label>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <Label htmlFor="edit-website" className="text-xs text-muted-foreground">
              Website
            </Label>
            <Input
              id="edit-website"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-bandcamp" className="text-xs text-muted-foreground">
              Bandcamp
            </Label>
            <Input
              id="edit-bandcamp"
              value={bandcamp}
              onChange={(e) => setBandcamp(e.target.value)}
              placeholder="https://label.bandcamp.com"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-instagram" className="text-xs text-muted-foreground">
              Instagram
            </Label>
            <Input
              id="edit-instagram"
              value={instagram}
              onChange={(e) => setInstagram(e.target.value)}
              placeholder="@handle"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-twitter" className="text-xs text-muted-foreground">
              Twitter
            </Label>
            <Input
              id="edit-twitter"
              value={twitter}
              onChange={(e) => setTwitter(e.target.value)}
              placeholder="@handle"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-spotify" className="text-xs text-muted-foreground">
              Spotify
            </Label>
            <Input
              id="edit-spotify"
              value={spotify}
              onChange={(e) => setSpotify(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-facebook" className="text-xs text-muted-foreground">
              Facebook
            </Label>
            <Input
              id="edit-facebook"
              value={facebook}
              onChange={(e) => setFacebook(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-youtube" className="text-xs text-muted-foreground">
              YouTube
            </Label>
            <Input
              id="edit-youtube"
              value={youtube}
              onChange={(e) => setYoutube(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-soundcloud" className="text-xs text-muted-foreground">
              SoundCloud
            </Label>
            <Input
              id="edit-soundcloud"
              value={soundcloud}
              onChange={(e) => setSoundcloud(e.target.value)}
              placeholder="https://..."
            />
          </div>
        </div>
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
  )
}

// ============================================================================
// Delete Confirmation
// ============================================================================

function DeleteConfirmation({
  labelName,
  labelId,
  onSuccess,
  onCancel,
}: {
  labelName: string
  labelId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const deleteMutation = useDeleteLabel()
  const [error, setError] = useState<string | null>(null)

  const handleDelete = useCallback(() => {
    setError(null)
    deleteMutation.mutate(labelId, {
      onSuccess: () => {
        onSuccess()
      },
      onError: (err) => {
        setError(
          err instanceof Error ? err.message : 'Failed to delete label'
        )
      },
    })
  }, [labelId, deleteMutation, onSuccess])

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
          &quot;{labelName}&quot;
        </span>
        ? This action cannot be undone. All artist and release associations will
        also be removed.
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
            'Delete Label'
          )}
        </Button>
      </DialogFooter>
    </div>
  )
}

// ============================================================================
// Main Component
// ============================================================================

export function LabelManagement() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [dialogMode, setDialogMode] = useState<DialogMode>(null)
  const [selectedLabelId, setSelectedLabelId] = useState<number | null>(null)
  const [selectedLabelName, setSelectedLabelName] = useState('')

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  const {
    data: labelsData,
    isLoading,
    error,
  } = useLabels({
    status: statusFilter || undefined,
  })

  // Client-side search filtering
  const filteredLabels =
    labelsData?.labels?.filter((label) => {
      if (!debouncedSearch) return true
      return label.name
        .toLowerCase()
        .includes(debouncedSearch.toLowerCase())
    }) || []

  const openCreate = useCallback(() => {
    setDialogMode('create')
    setSelectedLabelId(null)
    setSelectedLabelName('')
  }, [])

  const openEdit = useCallback((labelId: number) => {
    setDialogMode('edit')
    setSelectedLabelId(labelId)
  }, [])

  const openDelete = useCallback((labelId: number, name: string) => {
    setDialogMode('delete')
    setSelectedLabelId(labelId)
    setSelectedLabelName(name)
  }, [])

  const closeDialog = useCallback(() => {
    setDialogMode(null)
    setSelectedLabelId(null)
    setSelectedLabelName('')
  }, [])

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold flex items-center gap-2">
            <Tag className="h-5 w-5" />
            Labels
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            Create, edit, and manage record labels.
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="mr-2 h-4 w-4" />
          New Label
        </Button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search labels..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-sm"
        >
          <option value="">All Statuses</option>
          {LABEL_STATUSES.map((s) => (
            <option key={s} value={s}>
              {LABEL_STATUS_LABELS[s]}
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
              : 'Failed to load labels.'}
          </p>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && filteredLabels.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No Labels Found</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {debouncedSearch || statusFilter
              ? 'No labels match your filters. Try a different search.'
              : 'No labels yet. Create your first label to get started.'}
          </p>
        </div>
      )}

      {/* Label list */}
      {!isLoading && !error && filteredLabels.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {filteredLabels.length} label
            {filteredLabels.length !== 1 ? 's' : ''}
            {debouncedSearch && ` matching "${debouncedSearch}"`}
          </div>

          <div className="space-y-2">
            {filteredLabels.map((label) => {
              const location = formatLabelLocation(label)
              return (
                <div
                  key={label.id}
                  className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                >
                  {/* Icon */}
                  <div className="flex h-10 w-10 items-center justify-center rounded bg-muted flex-shrink-0">
                    <Tag className="h-5 w-5 text-muted-foreground" />
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm truncate">
                        {label.name}
                      </span>
                      <Badge
                        variant={getLabelStatusVariant(label.status)}
                        className="text-xs flex-shrink-0"
                      >
                        {getLabelStatusLabel(label.status)}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                      {location && <span>{location}</span>}
                      {label.artist_count > 0 && (
                        <span>
                          {label.artist_count} artist
                          {label.artist_count !== 1 ? 's' : ''}
                        </span>
                      )}
                      {label.release_count > 0 && (
                        <span>
                          {label.release_count} release
                          {label.release_count !== 1 ? 's' : ''}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openEdit(label.id)}
                      className="h-8 w-8 p-0"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openDelete(label.id, label.name)}
                      className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              )
            })}
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
            <DialogTitle>Create Label</DialogTitle>
            <DialogDescription>
              Add a new record label with location and social links.
            </DialogDescription>
          </DialogHeader>
          <CreateLabelForm onSuccess={closeDialog} onCancel={closeDialog} />
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog
        open={dialogMode === 'edit'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Label</DialogTitle>
            <DialogDescription>
              Update label details and social links.
            </DialogDescription>
          </DialogHeader>
          {selectedLabelId && (
            <EditLabelForm
              labelId={selectedLabelId}
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
            <DialogTitle>Delete Label</DialogTitle>
            <DialogDescription>
              This action is permanent and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          {selectedLabelId && (
            <DeleteConfirmation
              labelName={selectedLabelName}
              labelId={selectedLabelId}
              onSuccess={closeDialog}
              onCancel={closeDialog}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default LabelManagement
