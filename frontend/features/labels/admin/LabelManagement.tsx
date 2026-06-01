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
  type LabelDetail,
  type LabelStatus,
} from '../types'

// ============================================================================
// Constants
// ============================================================================

type DialogMode = 'create' | 'edit' | 'delete' | null

// ============================================================================
// Create Label Form
// ============================================================================

// Exported only for direct regression-test access (reset-on-open). Production
// callers render it from LabelManagement.
export function CreateLabelForm({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
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

  // Reset on (re)open. AdminFormLayout keeps the form mounted across the Sheet's
  // close animation, so without this the next "New Label" would show stale input.
  // Adjusting state during render (not in an effect) avoids a cascading re-render
  // and an extra paint — the canonical CreateStationForm pattern (PSY-911/930).
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setName('')
      setCity('')
      setState('')
      setCountry('')
      setFoundedYear('')
      setStatus('active')
      setDescription('')
      setInstagram('')
      setFacebook('')
      setTwitter('')
      setYoutube('')
      setSpotify('')
      setSoundcloud('')
      setBandcamp('')
      setWebsite('')
      setError(null)
    }
  }

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
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Create Label"
      description="Add a new record label with location and social links."
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
              'Create Label'
            )}
          </Button>
        </>
      }
    >
      <AdminFormField label="Name *" htmlFor="create-name">
        <Input
          id="create-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Label name"
        />
      </AdminFormField>

      <AdminFormRow cols={3}>
        <AdminFormField label="City" htmlFor="create-city">
          <Input
            id="create-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Seattle"
          />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="create-state">
          <Input
            id="create-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="WA"
          />
        </AdminFormField>
        <AdminFormField label="Country" htmlFor="create-country">
          <Input
            id="create-country"
            value={country}
            onChange={(e) => setCountry(e.target.value)}
            placeholder="US"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Status" htmlFor="create-status">
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger id="create-status" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LABEL_STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {LABEL_STATUS_LABELS[s]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Founded Year" htmlFor="create-founded-year">
          <Input
            id="create-founded-year"
            type="number"
            value={foundedYear}
            onChange={(e) => setFoundedYear(e.target.value)}
            placeholder="1988"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Description" htmlFor="create-desc">
        <Textarea
          id="create-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

      {/* Social Links */}
      <div className="space-y-3">
        <Label>Social Links</Label>
        <AdminFormRow cols={2} className="gap-3">
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Website</span>}
            htmlFor="create-website"
          >
            <Input
              id="create-website"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Bandcamp</span>}
            htmlFor="create-bandcamp"
          >
            <Input
              id="create-bandcamp"
              value={bandcamp}
              onChange={(e) => setBandcamp(e.target.value)}
              placeholder="https://label.bandcamp.com"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Instagram</span>}
            htmlFor="create-instagram"
          >
            <Input
              id="create-instagram"
              value={instagram}
              onChange={(e) => setInstagram(e.target.value)}
              placeholder="@handle"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Twitter</span>}
            htmlFor="create-twitter"
          >
            <Input
              id="create-twitter"
              value={twitter}
              onChange={(e) => setTwitter(e.target.value)}
              placeholder="@handle"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Spotify</span>}
            htmlFor="create-spotify"
          >
            <Input
              id="create-spotify"
              value={spotify}
              onChange={(e) => setSpotify(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Facebook</span>}
            htmlFor="create-facebook"
          >
            <Input
              id="create-facebook"
              value={facebook}
              onChange={(e) => setFacebook(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">YouTube</span>}
            htmlFor="create-youtube"
          >
            <Input
              id="create-youtube"
              value={youtube}
              onChange={(e) => setYoutube(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">SoundCloud</span>}
            htmlFor="create-soundcloud"
          >
            <Input
              id="create-soundcloud"
              value={soundcloud}
              onChange={(e) => setSoundcloud(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
        </AdminFormRow>
      </div>
    </AdminFormLayout>
  )
}

// ============================================================================
// Edit Label Form
// ============================================================================

// Per the PSY-930 decision the Edit Sheet opens immediately on click: while the
// label detail loads this wrapper renders an AdminFormLayout (open) with a
// spinner body — `open` stays true throughout, so the Sheet stays open — then
// swaps to EditLabelFormFields (keyed on label.id, so a
// switch-label-without-closing-dialog scenario remounts with fresh state) once
// the detail resolves. The inner component initializes local state from props
// inline (React's preferred "calculate during render" path — see
// https://react.dev/learn/you-might-not-need-an-effect). No useEffect, no
// `initialized` ratchet.
function EditLabelForm({
  labelId,
  open,
  onOpenChange,
  onSuccess,
}: {
  labelId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const { data: label, isLoading } = useLabel({
    idOrSlug: labelId,
    enabled: labelId > 0,
  })

  if (isLoading || !label) {
    return (
      <AdminFormLayout
        variant="sheet"
        open={open}
        onOpenChange={onOpenChange}
        title="Edit Label"
        description="Update label details and social links."
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
            Label not found.
          </div>
        )}
      </AdminFormLayout>
    )
  }

  return (
    <EditLabelFormFields
      key={label.id}
      label={label}
      open={open}
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  )
}

// Exported only for direct regression-test access (rerender-with-different-key
// resets fields; rerender-with-same-key preserves dirty edits). Not part of
// the surface's public API — production callers use EditLabelForm.
export function EditLabelFormFields({
  label,
  open,
  onOpenChange,
  onSuccess,
}: {
  label: LabelDetail
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const updateMutation = useUpdateLabel()

  const [name, setName] = useState(label.name)
  const [city, setCity] = useState(label.city || '')
  const [state, setState] = useState(label.state || '')
  const [country, setCountry] = useState(label.country || '')
  const [foundedYear, setFoundedYear] = useState(
    label.founded_year?.toString() || ''
  )
  const [status, setStatus] = useState<string>(label.status || 'active')
  const [description, setDescription] = useState(label.description || '')
  const [instagram, setInstagram] = useState(label.social?.instagram || '')
  const [facebook, setFacebook] = useState(label.social?.facebook || '')
  const [twitter, setTwitter] = useState(label.social?.twitter || '')
  const [youtube, setYoutube] = useState(label.social?.youtube || '')
  const [spotify, setSpotify] = useState(label.social?.spotify || '')
  const [soundcloud, setSoundcloud] = useState(label.social?.soundcloud || '')
  const [bandcamp, setBandcamp] = useState(label.social?.bandcamp || '')
  const [website, setWebsite] = useState(label.social?.website || '')
  const [error, setError] = useState<string | null>(null)

  const labelId = label.id

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

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Edit Label"
      description="Update label details and social links."
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
      <AdminFormField label="Name *" htmlFor="edit-name">
        <Input
          id="edit-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Label name"
        />
      </AdminFormField>

      <AdminFormRow cols={3}>
        <AdminFormField label="City" htmlFor="edit-city">
          <Input
            id="edit-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Seattle"
          />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="edit-state">
          <Input
            id="edit-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="WA"
          />
        </AdminFormField>
        <AdminFormField label="Country" htmlFor="edit-country">
          <Input
            id="edit-country"
            value={country}
            onChange={(e) => setCountry(e.target.value)}
            placeholder="US"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Status" htmlFor="edit-status">
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger id="edit-status" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LABEL_STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {LABEL_STATUS_LABELS[s]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Founded Year" htmlFor="edit-founded-year">
          <Input
            id="edit-founded-year"
            type="number"
            value={foundedYear}
            onChange={(e) => setFoundedYear(e.target.value)}
            placeholder="1988"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Description" htmlFor="edit-desc">
        <Textarea
          id="edit-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

      {/* Social Links */}
      <div className="space-y-3">
        <Label>Social Links</Label>
        <AdminFormRow cols={2} className="gap-3">
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Website</span>}
            htmlFor="edit-website"
          >
            <Input
              id="edit-website"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Bandcamp</span>}
            htmlFor="edit-bandcamp"
          >
            <Input
              id="edit-bandcamp"
              value={bandcamp}
              onChange={(e) => setBandcamp(e.target.value)}
              placeholder="https://label.bandcamp.com"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Instagram</span>}
            htmlFor="edit-instagram"
          >
            <Input
              id="edit-instagram"
              value={instagram}
              onChange={(e) => setInstagram(e.target.value)}
              placeholder="@handle"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Twitter</span>}
            htmlFor="edit-twitter"
          >
            <Input
              id="edit-twitter"
              value={twitter}
              onChange={(e) => setTwitter(e.target.value)}
              placeholder="@handle"
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Spotify</span>}
            htmlFor="edit-spotify"
          >
            <Input
              id="edit-spotify"
              value={spotify}
              onChange={(e) => setSpotify(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Facebook</span>}
            htmlFor="edit-facebook"
          >
            <Input
              id="edit-facebook"
              value={facebook}
              onChange={(e) => setFacebook(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">YouTube</span>}
            htmlFor="edit-youtube"
          >
            <Input
              id="edit-youtube"
              value={youtube}
              onChange={(e) => setYoutube(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">SoundCloud</span>}
            htmlFor="edit-soundcloud"
          >
            <Input
              id="edit-soundcloud"
              value={soundcloud}
              onChange={(e) => setSoundcloud(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
        </AdminFormRow>
      </div>
    </AdminFormLayout>
  )
}

// ============================================================================
// Delete Confirmation
// ============================================================================

// Short confirm form -> centered Modal (variant="modal"), per the PSY-912
// Hybrid decision: long entity forms use the Sheet, short confirm/delete forms
// stay a centered Dialog.
function DeleteConfirmation({
  labelName,
  labelId,
  open,
  onOpenChange,
  onSuccess,
}: {
  labelName: string
  labelId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
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
    <AdminFormLayout
      variant="modal"
      open={open}
      onOpenChange={onOpenChange}
      title="Delete Label"
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
              'Delete Label'
            )}
          </Button>
        </>
      }
    >
      <p className="text-sm text-muted-foreground">
        Are you sure you want to delete{' '}
        <span className="font-semibold text-foreground">
          &quot;{labelName}&quot;
        </span>
        ? This action cannot be undone. All artist and release associations will
        also be removed.
      </p>
    </AdminFormLayout>
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

  // Close by clearing dialogMode only. The selected id/name persist so the
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
        {/* Deferred to PSY-924; outside PSY-907's entity create/edit form-field scope. */}
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
        <InlineErrorBanner variant="queryFallback">
          {error instanceof Error
            ? error.message
            : 'Failed to load labels.'}
        </InlineErrorBanner>
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
                      aria-label={`Edit ${label.name}`}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openDelete(label.id, label.name)}
                      className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                      aria-label={`Delete ${label.name}`}
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

      {/* Create — right-anchored Sheet (PSY-930 AdminFormLayout) */}
      <CreateLabelForm
        open={dialogMode === 'create'}
        onOpenChange={(open) => !open && closeDialog()}
        onSuccess={closeDialog}
      />

      {/* Edit — right-anchored Sheet (PSY-930 AdminFormLayout). Gated on the
          selected id; closeDialog keeps the id set so the form stays mounted
          and the Sheet animates closed (open driven by dialogMode), matching
          CreateLabelForm. The id is overwritten on the next open. */}
      {selectedLabelId && (
        <EditLabelForm
          labelId={selectedLabelId}
          open={dialogMode === 'edit'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}

      {/* Delete — centered Modal (PSY-930 AdminFormLayout) */}
      {selectedLabelId && (
        <DeleteConfirmation
          labelName={selectedLabelName}
          labelId={selectedLabelId}
          open={dialogMode === 'delete'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}
    </div>
  )
}

export default LabelManagement
