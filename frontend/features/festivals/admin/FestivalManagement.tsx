'use client'

import { useState, useCallback, useEffect } from 'react'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  Search,
  Inbox,
  Tent,
  X,
  Music,
  MapPin,
  ChevronLeft,
  Star,
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
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  AdminFormLayout,
  AdminFormRow,
  AdminFormField,
} from '@/components/admin/AdminFormLayout'
import { InlineErrorBanner } from '@/components/shared'
import { useFestivals, useFestival, useFestivalLineup, useFestivalVenues } from '../hooks/useFestivals'
import { useArtistSearch } from '@/features/artists'
import { useVenueSearch } from '@/features/venues'
import {
  useCreateFestival,
  useUpdateFestival,
  useDeleteFestival,
  useAddFestivalArtist,
  useUpdateFestivalArtist,
  useRemoveFestivalArtist,
  useAddFestivalVenue,
  useRemoveFestivalVenue,
} from '../hooks/useAdminFestivals'
import {
  FESTIVAL_STATUSES,
  FESTIVAL_STATUS_LABELS,
  getFestivalStatusLabel,
  getFestivalStatusVariant,
  formatFestivalLocation,
  formatFestivalDates,
  BILLING_TIERS,
  BILLING_TIER_LABELS,
  getBillingTierLabel,
  type FestivalDetail,
  type FestivalStatus,
  type FestivalArtist,
  type FestivalVenue,
} from '../types'

// ============================================================================
// Constants
// ============================================================================

type DialogMode = 'create' | 'edit' | 'delete' | null
type ManageMode = 'lineup' | 'venues' | null

// ============================================================================
// Create Festival Form
// ============================================================================

// Exported only for direct regression-test access (reset-on-open). Production
// callers render it from FestivalManagement.
export function CreateFestivalForm({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const createMutation = useCreateFestival()

  const [name, setName] = useState('')
  const [seriesSlug, setSeriesSlug] = useState('')
  const [editionYear, setEditionYear] = useState('')
  const [description, setDescription] = useState('')
  const [locationName, setLocationName] = useState('')
  const [city, setCity] = useState('')
  const [state, setState] = useState('')
  const [country, setCountry] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [website, setWebsite] = useState('')
  const [ticketUrl, setTicketUrl] = useState('')
  const [flyerUrl, setFlyerUrl] = useState('')
  const [status, setStatus] = useState<string>('announced')
  const [error, setError] = useState<string | null>(null)

  // Reset on (re)open — AdminFormLayout keeps the form mounted across the Sheet
  // close animation. Adjust state during render (not in an effect) per the
  // canonical CreateStationForm pattern (PSY-911/930).
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setName('')
      setSeriesSlug('')
      setEditionYear('')
      setDescription('')
      setLocationName('')
      setCity('')
      setState('')
      setCountry('')
      setStartDate('')
      setEndDate('')
      setWebsite('')
      setTicketUrl('')
      setFlyerUrl('')
      setStatus('announced')
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
      if (!seriesSlug.trim()) {
        setError('Series slug is required')
        return
      }
      if (!editionYear) {
        setError('Edition year is required')
        return
      }
      if (!startDate) {
        setError('Start date is required')
        return
      }
      if (!endDate) {
        setError('End date is required')
        return
      }

      createMutation.mutate(
        {
          name: name.trim(),
          series_slug: seriesSlug.trim(),
          edition_year: parseInt(editionYear, 10),
          description: description || undefined,
          location_name: locationName || undefined,
          city: city || undefined,
          state: state || undefined,
          country: country || undefined,
          start_date: startDate,
          end_date: endDate,
          website: website || undefined,
          ticket_url: ticketUrl || undefined,
          flyer_url: flyerUrl || undefined,
          status: status || undefined,
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to create festival'
            )
          },
        }
      )
    },
    [
      name, seriesSlug, editionYear, description, locationName,
      city, state, country, startDate, endDate, website,
      ticketUrl, flyerUrl, status, createMutation, onSuccess,
    ]
  )

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Create Festival"
      description="Add a new music festival with location and dates."
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
              'Create Festival'
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
          placeholder="M3F Festival"
        />
      </AdminFormField>

      <AdminFormRow cols={2}>
        <AdminFormField label="Series Slug *" htmlFor="create-series-slug">
          <Input
            id="create-series-slug"
            value={seriesSlug}
            onChange={(e) => setSeriesSlug(e.target.value)}
            placeholder="m3f"
          />
        </AdminFormField>
        <AdminFormField label="Edition Year *" htmlFor="create-edition-year">
          <Input
            id="create-edition-year"
            type="number"
            value={editionYear}
            onChange={(e) => setEditionYear(e.target.value)}
            placeholder="2026"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Start Date *" htmlFor="create-start-date">
          <Input
            id="create-start-date"
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
          />
        </AdminFormField>
        <AdminFormField label="End Date *" htmlFor="create-end-date">
          <Input
            id="create-end-date"
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Status" htmlFor="create-status">
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger id="create-status" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FESTIVAL_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {FESTIVAL_STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </AdminFormField>

      <AdminFormField label="Location Name" htmlFor="create-location-name">
        <Input
          id="create-location-name"
          value={locationName}
          onChange={(e) => setLocationName(e.target.value)}
          placeholder="Margaret T. Hance Park"
        />
      </AdminFormField>

      <AdminFormRow cols={3}>
        <AdminFormField label="City" htmlFor="create-city">
          <Input
            id="create-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Phoenix"
          />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="create-state">
          <Input
            id="create-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="AZ"
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

      <AdminFormField label="Description" htmlFor="create-desc">
        <Textarea
          id="create-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

      {/* Links */}
      <div className="space-y-3">
        <Label>Links</Label>
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
            label={<span className="text-xs text-muted-foreground">Ticket URL</span>}
            htmlFor="create-ticket-url"
          >
            <Input
              id="create-ticket-url"
              value={ticketUrl}
              onChange={(e) => setTicketUrl(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Flyer URL</span>}
            htmlFor="create-flyer-url"
          >
            <Input
              id="create-flyer-url"
              value={flyerUrl}
              onChange={(e) => setFlyerUrl(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
        </AdminFormRow>
      </div>
    </AdminFormLayout>
  )
}

// ============================================================================
// Edit Festival Form
// ============================================================================

// Per the PSY-930 decision the Edit Sheet opens immediately on click: while the
// festival detail loads this wrapper renders an AdminFormLayout (open) with a
// spinner body — `open` stays true throughout, so the Sheet stays open — then
// swaps to EditFestivalFormFields (keyed on festival.id, so a
// switch-festival-without-closing-dialog scenario remounts with fresh state)
// once the detail resolves. The inner component initializes local state from
// props inline (React's preferred "calculate during render" path — see
// https://react.dev/learn/you-might-not-need-an-effect). No useEffect, no
// `initialized` ratchet.
function EditFestivalForm({
  festivalId,
  open,
  onOpenChange,
  onSuccess,
}: {
  festivalId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const { data: festival, isLoading } = useFestival({
    idOrSlug: festivalId,
    enabled: festivalId > 0,
  })

  if (isLoading || !festival) {
    return (
      <AdminFormLayout
        variant="sheet"
        open={open}
        onOpenChange={onOpenChange}
        title="Edit Festival"
        description="Update festival details, location, and dates."
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
            Festival not found.
          </div>
        )}
      </AdminFormLayout>
    )
  }

  return (
    <EditFestivalFormFields
      key={festival.id}
      festival={festival}
      open={open}
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  )
}

// Exported only for direct regression-test access (rerender-with-different-key
// resets fields; rerender-with-same-key preserves dirty edits). Not part of
// the surface's public API — production callers use EditFestivalForm.
export function EditFestivalFormFields({
  festival,
  open,
  onOpenChange,
  onSuccess,
}: {
  festival: FestivalDetail
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const updateMutation = useUpdateFestival()

  const [name, setName] = useState(festival.name)
  const [seriesSlug, setSeriesSlug] = useState(festival.series_slug)
  const [editionYear, setEditionYear] = useState(
    festival.edition_year.toString()
  )
  const [description, setDescription] = useState(festival.description || '')
  const [locationName, setLocationName] = useState(festival.location_name || '')
  const [city, setCity] = useState(festival.city || '')
  const [state, setState] = useState(festival.state || '')
  const [country, setCountry] = useState(festival.country || '')
  const [startDate, setStartDate] = useState(festival.start_date || '')
  const [endDate, setEndDate] = useState(festival.end_date || '')
  const [website, setWebsite] = useState(festival.website || '')
  const [ticketUrl, setTicketUrl] = useState(festival.ticket_url || '')
  const [flyerUrl, setFlyerUrl] = useState(festival.flyer_url || '')
  const [status, setStatus] = useState<string>(festival.status || 'announced')
  const [error, setError] = useState<string | null>(null)

  const festivalId = festival.id

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
          festivalId,
          data: {
            name: name.trim(),
            series_slug: seriesSlug.trim() || undefined,
            edition_year: editionYear ? parseInt(editionYear, 10) : undefined,
            description: description || null,
            location_name: locationName || null,
            city: city || null,
            state: state || null,
            country: country || null,
            start_date: startDate || undefined,
            end_date: endDate || undefined,
            website: website || null,
            ticket_url: ticketUrl || null,
            flyer_url: flyerUrl || null,
            status: status || undefined,
          },
        },
        {
          onSuccess: () => {
            onSuccess()
          },
          onError: (err) => {
            setError(
              err instanceof Error ? err.message : 'Failed to update festival'
            )
          },
        }
      )
    },
    [
      name, seriesSlug, editionYear, description, locationName,
      city, state, country, startDate, endDate, website,
      ticketUrl, flyerUrl, status, festivalId, updateMutation, onSuccess,
    ]
  )

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Edit Festival"
      description="Update festival details, location, and dates."
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
          placeholder="Festival name"
        />
      </AdminFormField>

      <AdminFormRow cols={2}>
        <AdminFormField label="Series Slug" htmlFor="edit-series-slug">
          <Input
            id="edit-series-slug"
            value={seriesSlug}
            onChange={(e) => setSeriesSlug(e.target.value)}
            placeholder="m3f"
          />
        </AdminFormField>
        <AdminFormField label="Edition Year" htmlFor="edit-edition-year">
          <Input
            id="edit-edition-year"
            type="number"
            value={editionYear}
            onChange={(e) => setEditionYear(e.target.value)}
            placeholder="2026"
            min="1900"
            max="2100"
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Start Date" htmlFor="edit-start-date">
          <Input
            id="edit-start-date"
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
          />
        </AdminFormField>
        <AdminFormField label="End Date" htmlFor="edit-end-date">
          <Input
            id="edit-end-date"
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
          />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Status" htmlFor="edit-status">
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger id="edit-status" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FESTIVAL_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {FESTIVAL_STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </AdminFormField>

      <AdminFormField label="Location Name" htmlFor="edit-location-name">
        <Input
          id="edit-location-name"
          value={locationName}
          onChange={(e) => setLocationName(e.target.value)}
          placeholder="Margaret T. Hance Park"
        />
      </AdminFormField>

      <AdminFormRow cols={3}>
        <AdminFormField label="City" htmlFor="edit-city">
          <Input
            id="edit-city"
            value={city}
            onChange={(e) => setCity(e.target.value)}
            placeholder="Phoenix"
          />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="edit-state">
          <Input
            id="edit-state"
            value={state}
            onChange={(e) => setState(e.target.value)}
            placeholder="AZ"
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

      <AdminFormField label="Description" htmlFor="edit-desc">
        <Textarea
          id="edit-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description..."
          rows={3}
        />
      </AdminFormField>

      {/* Links */}
      <div className="space-y-3">
        <Label>Links</Label>
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
            label={<span className="text-xs text-muted-foreground">Ticket URL</span>}
            htmlFor="edit-ticket-url"
          >
            <Input
              id="edit-ticket-url"
              value={ticketUrl}
              onChange={(e) => setTicketUrl(e.target.value)}
              placeholder="https://..."
            />
          </AdminFormField>
          <AdminFormField
            className="space-y-1"
            label={<span className="text-xs text-muted-foreground">Flyer URL</span>}
            htmlFor="edit-flyer-url"
          >
            <Input
              id="edit-flyer-url"
              value={flyerUrl}
              onChange={(e) => setFlyerUrl(e.target.value)}
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
// Hybrid decision.
function DeleteConfirmation({
  festivalName,
  festivalId,
  open,
  onOpenChange,
  onSuccess,
}: {
  festivalName: string
  festivalId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const deleteMutation = useDeleteFestival()
  const [error, setError] = useState<string | null>(null)

  const handleDelete = useCallback(() => {
    setError(null)
    deleteMutation.mutate(festivalId, {
      onSuccess: () => {
        onSuccess()
      },
      onError: (err) => {
        setError(
          err instanceof Error ? err.message : 'Failed to delete festival'
        )
      },
    })
  }, [festivalId, deleteMutation, onSuccess])

  return (
    <AdminFormLayout
      variant="modal"
      open={open}
      onOpenChange={onOpenChange}
      title="Delete Festival"
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
              'Delete Festival'
            )}
          </Button>
        </>
      }
    >
      <p className="text-sm text-muted-foreground">
        Are you sure you want to delete{' '}
        <span className="font-semibold text-foreground">
          &quot;{festivalName}&quot;
        </span>
        ? This action cannot be undone. All lineup and venue associations will
        also be removed.
      </p>
    </AdminFormLayout>
  )
}

// ============================================================================
// Lineup Management Panel
// ============================================================================

function LineupManagement({ festivalId }: { festivalId: number }) {
  const { data: lineupData, isLoading } = useFestivalLineup({
    festivalId,
    enabled: festivalId > 0,
  })
  const addArtistMutation = useAddFestivalArtist()
  const updateArtistMutation = useUpdateFestivalArtist()
  const removeArtistMutation = useRemoveFestivalArtist()

  const [searchQuery, setSearchQuery] = useState('')
  const [showSearchResults, setShowSearchResults] = useState(false)
  const { data: searchData, isLoading: isSearching } = useArtistSearch({
    query: searchQuery,
    debounceMs: 200,
  })

  // Add artist form state
  const [addBillingTier, setAddBillingTier] = useState('mid_card')
  const [addPosition, setAddPosition] = useState('0')
  const [addDayDate, setAddDayDate] = useState('')
  const [addStage, setAddStage] = useState('')
  const [addSetTime, setAddSetTime] = useState('')

  // Edit artist state
  const [editingArtist, setEditingArtist] = useState<FestivalArtist | null>(null)
  const [editBillingTier, setEditBillingTier] = useState('')
  const [editPosition, setEditPosition] = useState('')
  const [editDayDate, setEditDayDate] = useState('')
  const [editStage, setEditStage] = useState('')
  const [editSetTime, setEditSetTime] = useState('')

  const [error, setError] = useState<string | null>(null)

  const handleAddArtist = useCallback(
    (artistId: number) => {
      setError(null)
      addArtistMutation.mutate(
        {
          festivalId,
          data: {
            artist_id: artistId,
            billing_tier: addBillingTier || undefined,
            position: addPosition ? parseInt(addPosition, 10) : undefined,
            day_date: addDayDate || undefined,
            stage: addStage || undefined,
            set_time: addSetTime ? `${addSetTime}:00` : undefined,
          },
        },
        {
          onSuccess: () => {
            setSearchQuery('')
            setShowSearchResults(false)
            setAddDayDate('')
            setAddStage('')
            setAddSetTime('')
          },
          onError: (err) => {
            setError(err instanceof Error ? err.message : 'Failed to add artist')
          },
        }
      )
    },
    [festivalId, addBillingTier, addPosition, addDayDate, addStage, addSetTime, addArtistMutation]
  )

  const openEditArtist = useCallback((artist: FestivalArtist) => {
    setEditingArtist(artist)
    setEditBillingTier(artist.billing_tier)
    setEditPosition(artist.position.toString())
    setEditDayDate(artist.day_date || '')
    setEditStage(artist.stage || '')
    setEditSetTime(artist.set_time ? artist.set_time.slice(0, 5) : '')
  }, [])

  const handleUpdateArtist = useCallback(() => {
    if (!editingArtist) return
    setError(null)
    updateArtistMutation.mutate(
      {
        festivalId,
        artistId: editingArtist.artist_id,
        data: {
          billing_tier: editBillingTier || undefined,
          position: editPosition ? parseInt(editPosition, 10) : undefined,
          day_date: editDayDate || null,
          stage: editStage || null,
          set_time: editSetTime ? `${editSetTime}:00` : null,
        },
      },
      {
        onSuccess: () => {
          setEditingArtist(null)
        },
        onError: (err) => {
          setError(err instanceof Error ? err.message : 'Failed to update artist')
        },
      }
    )
  }, [festivalId, editingArtist, editBillingTier, editPosition, editDayDate, editStage, editSetTime, updateArtistMutation])

  const handleRemoveArtist = useCallback(
    (artistId: number) => {
      setError(null)
      removeArtistMutation.mutate(
        { festivalId, artistId },
        {
          onError: (err) => {
            setError(err instanceof Error ? err.message : 'Failed to remove artist')
          },
        }
      )
    },
    [festivalId, removeArtistMutation]
  )

  const existingArtistIds = lineupData?.artists?.map((a) => a.artist_id) || []

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {error && (
        <InlineErrorBanner>{error}</InlineErrorBanner>
      )}

      {/* Add artist section */}
      <div className="space-y-3 rounded-lg border p-4">
        <Label className="text-sm font-medium">Add Artist to Lineup</Label>

        {/* Default billing options for new artists */}
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Billing Tier</Label>
            {/* Deferred to PSY-924; outside PSY-907's entity create/edit form-field scope. */}
            <select
              value={addBillingTier}
              onChange={(e) => setAddBillingTier(e.target.value)}
              className="h-8 w-full rounded-md border bg-background px-2 text-xs"
            >
              {BILLING_TIERS.map((t) => (
                <option key={t} value={t}>
                  {BILLING_TIER_LABELS[t]}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Position</Label>
            <Input
              type="number"
              value={addPosition}
              onChange={(e) => setAddPosition(e.target.value)}
              className="h-8 text-xs"
              min="0"
            />
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3">
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Day Date</Label>
            <Input
              type="date"
              value={addDayDate}
              onChange={(e) => setAddDayDate(e.target.value)}
              className="h-8 text-xs"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Stage</Label>
            <Input
              value={addStage}
              onChange={(e) => setAddStage(e.target.value)}
              className="h-8 text-xs"
              placeholder="Main Stage"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Set Time</Label>
            <Input
              type="time"
              value={addSetTime}
              onChange={(e) => setAddSetTime(e.target.value)}
              className="h-8 text-xs"
            />
          </div>
        </div>

        {/* Artist search */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search artists to add..."
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value)
              setShowSearchResults(true)
            }}
            onFocus={() => setShowSearchResults(true)}
            className="pl-9"
          />

          {showSearchResults && searchQuery.length > 0 && (
            <div className="absolute top-full left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border bg-popover shadow-md">
              {isSearching ? (
                <div className="flex items-center justify-center p-3">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
              ) : searchData?.artists && searchData.artists.length > 0 ? (
                searchData.artists.map((artist) => {
                  const alreadyAdded = existingArtistIds.includes(artist.id)
                  return (
                    <button
                      key={artist.id}
                      type="button"
                      disabled={alreadyAdded || addArtistMutation.isPending}
                      onClick={() => handleAddArtist(artist.id)}
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
      </div>

      {/* Current lineup */}
      <div className="text-sm text-muted-foreground">
        {lineupData?.count || 0} artist{(lineupData?.count || 0) !== 1 ? 's' : ''} in lineup
      </div>

      {lineupData?.artists && lineupData.artists.length > 0 ? (
        <div className="space-y-2">
          {lineupData.artists.map((artist) => (
            <div
              key={artist.id}
              className="flex items-center gap-2 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm truncate">
                    {artist.artist_name}
                  </span>
                  <Badge variant="outline" className="text-xs flex-shrink-0">
                    {getBillingTierLabel(artist.billing_tier)}
                  </Badge>
                  <span className="text-xs text-muted-foreground">
                    #{artist.position}
                  </span>
                </div>
                <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                  {artist.day_date && <span>{artist.day_date}</span>}
                  {artist.stage && <span>{artist.stage}</span>}
                  {artist.set_time && <span>{artist.set_time.slice(0, 5)}</span>}
                </div>
              </div>

              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => openEditArtist(artist)}
                  className="h-8 w-8 p-0"
                  aria-label={`Edit lineup entry for ${artist.artist_name}`}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRemoveArtist(artist.artist_id)}
                  disabled={removeArtistMutation.isPending}
                  className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                  aria-label={`Remove ${artist.artist_name} from lineup`}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <Music className="h-8 w-8 text-muted-foreground mb-2" />
          <p className="text-sm text-muted-foreground">
            No artists in lineup yet. Search above to add artists.
          </p>
        </div>
      )}

      {/* Edit Artist Dialog */}
      <Dialog
        open={editingArtist !== null}
        onOpenChange={(open) => !open && setEditingArtist(null)}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Edit Lineup Entry</DialogTitle>
            <DialogDescription>
              Update {editingArtist?.artist_name}&apos;s lineup details.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Billing Tier</Label>
                {/* Deferred to PSY-924; outside PSY-907's entity create/edit form-field scope. */}
                <select
                  value={editBillingTier}
                  onChange={(e) => setEditBillingTier(e.target.value)}
                  className="h-9 w-full rounded-md border bg-background px-2 text-sm"
                >
                  {BILLING_TIERS.map((t) => (
                    <option key={t} value={t}>
                      {BILLING_TIER_LABELS[t]}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-2">
                <Label>Position</Label>
                <Input
                  type="number"
                  value={editPosition}
                  onChange={(e) => setEditPosition(e.target.value)}
                  min="0"
                />
              </div>
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label>Day Date</Label>
                <Input
                  type="date"
                  value={editDayDate}
                  onChange={(e) => setEditDayDate(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label>Stage</Label>
                <Input
                  value={editStage}
                  onChange={(e) => setEditStage(e.target.value)}
                  placeholder="Main Stage"
                />
              </div>
              <div className="space-y-2">
                <Label>Set Time</Label>
                <Input
                  type="time"
                  value={editSetTime}
                  onChange={(e) => setEditSetTime(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setEditingArtist(null)}
                disabled={updateArtistMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                onClick={handleUpdateArtist}
                disabled={updateArtistMutation.isPending}
              >
                {updateArtistMutation.isPending ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Saving...
                  </>
                ) : (
                  'Save Changes'
                )}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ============================================================================
// Venue Management Panel
// ============================================================================

function VenueManagement({ festivalId }: { festivalId: number }) {
  const { data: venuesData, isLoading } = useFestivalVenues({
    festivalIdOrSlug: festivalId,
    enabled: festivalId > 0,
  })
  const addVenueMutation = useAddFestivalVenue()
  const removeVenueMutation = useRemoveFestivalVenue()

  const [searchQuery, setSearchQuery] = useState('')
  const [showSearchResults, setShowSearchResults] = useState(false)
  const [isPrimary, setIsPrimary] = useState(false)
  const { data: searchData, isLoading: isSearching } = useVenueSearch({
    query: searchQuery,
    debounceMs: 200,
  })

  const [error, setError] = useState<string | null>(null)

  const handleAddVenue = useCallback(
    (venueId: number) => {
      setError(null)
      addVenueMutation.mutate(
        {
          festivalId,
          data: {
            venue_id: venueId,
            is_primary: isPrimary,
          },
        },
        {
          onSuccess: () => {
            setSearchQuery('')
            setShowSearchResults(false)
            setIsPrimary(false)
          },
          onError: (err) => {
            setError(err instanceof Error ? err.message : 'Failed to add venue')
          },
        }
      )
    },
    [festivalId, isPrimary, addVenueMutation]
  )

  const handleRemoveVenue = useCallback(
    (venueId: number) => {
      setError(null)
      removeVenueMutation.mutate(
        { festivalId, venueId },
        {
          onError: (err) => {
            setError(err instanceof Error ? err.message : 'Failed to remove venue')
          },
        }
      )
    },
    [festivalId, removeVenueMutation]
  )

  const existingVenueIds = venuesData?.venues?.map((v) => v.venue_id) || []

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {error && (
        <InlineErrorBanner>{error}</InlineErrorBanner>
      )}

      {/* Add venue section */}
      <div className="space-y-3 rounded-lg border p-4">
        <Label className="text-sm font-medium">Add Venue to Festival</Label>

        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            id="is-primary"
            checked={isPrimary}
            onChange={(e) => setIsPrimary(e.target.checked)}
            className="h-4 w-4 rounded border"
          />
          <Label htmlFor="is-primary" className="text-sm text-muted-foreground cursor-pointer">
            Primary venue
          </Label>
        </div>

        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search venues to add..."
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value)
              setShowSearchResults(true)
            }}
            onFocus={() => setShowSearchResults(true)}
            className="pl-9"
          />

          {showSearchResults && searchQuery.length > 0 && (
            <div className="absolute top-full left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border bg-popover shadow-md">
              {isSearching ? (
                <div className="flex items-center justify-center p-3">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
              ) : searchData?.venues && searchData.venues.length > 0 ? (
                searchData.venues.map((venue) => {
                  const alreadyAdded = existingVenueIds.includes(venue.id)
                  return (
                    <button
                      key={venue.id}
                      type="button"
                      disabled={alreadyAdded || addVenueMutation.isPending}
                      onClick={() => handleAddVenue(venue.id)}
                      className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      <MapPin className="h-3.5 w-3.5 text-muted-foreground" />
                      <span>{venue.name}</span>
                      {venue.city && (
                        <span className="text-xs text-muted-foreground">
                          ({venue.city}, {venue.state})
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
                  No venues found
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Current venues */}
      <div className="text-sm text-muted-foreground">
        {venuesData?.count || 0} venue{(venuesData?.count || 0) !== 1 ? 's' : ''}
      </div>

      {venuesData?.venues && venuesData.venues.length > 0 ? (
        <div className="space-y-2">
          {venuesData.venues.map((venue) => (
            <div
              key={venue.id}
              className="flex items-center gap-2 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
            >
              <MapPin className="h-4 w-4 text-muted-foreground flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm truncate">
                    {venue.venue_name}
                  </span>
                  {venue.is_primary && (
                    <Badge variant="default" className="text-xs flex-shrink-0">
                      <Star className="mr-1 h-3 w-3" />
                      Primary
                    </Badge>
                  )}
                </div>
                <div className="text-xs text-muted-foreground mt-0.5">
                  {venue.city}, {venue.state}
                </div>
              </div>

              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleRemoveVenue(venue.venue_id)}
                disabled={removeVenueMutation.isPending}
                className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                aria-label={`Remove ${venue.venue_name} from venues`}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <MapPin className="h-8 w-8 text-muted-foreground mb-2" />
          <p className="text-sm text-muted-foreground">
            No venues added yet. Search above to add venues.
          </p>
        </div>
      )}
    </div>
  )
}

// ============================================================================
// Main Component
// ============================================================================

export function FestivalManagement() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [dialogMode, setDialogMode] = useState<DialogMode>(null)
  const [selectedFestivalId, setSelectedFestivalId] = useState<number | null>(null)
  const [selectedFestivalName, setSelectedFestivalName] = useState('')
  const [manageMode, setManageMode] = useState<ManageMode>(null)
  const [managedFestivalId, setManagedFestivalId] = useState<number | null>(null)
  const [managedFestivalName, setManagedFestivalName] = useState('')

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  const {
    data: festivalsData,
    isLoading,
    error,
  } = useFestivals({
    status: statusFilter || undefined,
  })

  // Client-side search filtering
  const filteredFestivals =
    festivalsData?.festivals?.filter((festival) => {
      if (!debouncedSearch) return true
      return festival.name
        .toLowerCase()
        .includes(debouncedSearch.toLowerCase())
    }) || []

  const openCreate = useCallback(() => {
    setDialogMode('create')
    setSelectedFestivalId(null)
    setSelectedFestivalName('')
  }, [])

  const openEdit = useCallback((festivalId: number) => {
    setDialogMode('edit')
    setSelectedFestivalId(festivalId)
  }, [])

  const openDelete = useCallback((festivalId: number, name: string) => {
    setDialogMode('delete')
    setSelectedFestivalId(festivalId)
    setSelectedFestivalName(name)
  }, [])

  // Close by clearing dialogMode only. The selected id/name persist so the
  // mounted Edit/Delete AdminFormLayout can animate closed (its `open` is
  // driven by dialogMode); openEdit/openDelete overwrite them on the next open.
  // Nulling the id here would unmount the form mid-animation and flash its
  // empty/not-found state. (PSY-930)
  const closeDialog = useCallback(() => {
    setDialogMode(null)
  }, [])

  const openManage = useCallback(
    (festivalId: number, festivalName: string, mode: ManageMode) => {
      setManagedFestivalId(festivalId)
      setManagedFestivalName(festivalName)
      setManageMode(mode)
    },
    []
  )

  const closeManage = useCallback(() => {
    setManageMode(null)
    setManagedFestivalId(null)
    setManagedFestivalName('')
  }, [])

  // If we're in a manage mode, show that panel instead
  if (manageMode && managedFestivalId) {
    return (
      <div className="space-y-4">
        {/* Back button + header */}
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="sm"
            onClick={closeManage}
            className="h-8 px-2"
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Back
          </Button>
          <div>
            <h2 className="text-xl font-semibold flex items-center gap-2">
              {manageMode === 'lineup' ? (
                <Music className="h-5 w-5" />
              ) : (
                <MapPin className="h-5 w-5" />
              )}
              {managedFestivalName} -{' '}
              {manageMode === 'lineup' ? 'Lineup' : 'Venues'}
            </h2>
          </div>
        </div>

        {manageMode === 'lineup' ? (
          <LineupManagement festivalId={managedFestivalId} />
        ) : (
          <VenueManagement festivalId={managedFestivalId} />
        )}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold flex items-center gap-2">
            <Tent className="h-5 w-5" />
            Festivals
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            Create, edit, and manage music festivals.
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="mr-2 h-4 w-4" />
          New Festival
        </Button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search festivals..."
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
          {FESTIVAL_STATUSES.map((s) => (
            <option key={s} value={s}>
              {FESTIVAL_STATUS_LABELS[s]}
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
            : 'Failed to load festivals.'}
        </InlineErrorBanner>
      )}

      {/* Empty state */}
      {!isLoading && !error && filteredFestivals.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No Festivals Found</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {debouncedSearch || statusFilter
              ? 'No festivals match your filters. Try a different search.'
              : 'No festivals yet. Create your first festival to get started.'}
          </p>
        </div>
      )}

      {/* Festival list */}
      {!isLoading && !error && filteredFestivals.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {filteredFestivals.length} festival
            {filteredFestivals.length !== 1 ? 's' : ''}
            {debouncedSearch && ` matching "${debouncedSearch}"`}
          </div>

          <div className="space-y-2">
            {filteredFestivals.map((festival) => {
              const location = formatFestivalLocation(festival)
              return (
                <div
                  key={festival.id}
                  className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                >
                  {/* Icon */}
                  <div className="flex h-10 w-10 items-center justify-center rounded bg-muted flex-shrink-0">
                    <Tent className="h-5 w-5 text-muted-foreground" />
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm truncate">
                        {festival.name}
                      </span>
                      <Badge
                        variant={getFestivalStatusVariant(festival.status)}
                        className="text-xs flex-shrink-0"
                      >
                        {getFestivalStatusLabel(festival.status)}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                      <span>{festival.edition_year}</span>
                      {location && <span>{location}</span>}
                      <span>
                        {formatFestivalDates(
                          festival.start_date,
                          festival.end_date
                        )}
                      </span>
                      {festival.artist_count > 0 && (
                        <span>
                          {festival.artist_count} artist
                          {festival.artist_count !== 1 ? 's' : ''}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        openManage(festival.id, festival.name, 'lineup')
                      }
                      className="h-8 px-2 text-xs"
                      title="Manage lineup"
                    >
                      <Music className="h-3.5 w-3.5 mr-1" />
                      Lineup
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        openManage(festival.id, festival.name, 'venues')
                      }
                      className="h-8 px-2 text-xs"
                      title="Manage venues"
                    >
                      <MapPin className="h-3.5 w-3.5 mr-1" />
                      Venues
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openEdit(festival.id)}
                      className="h-8 w-8 p-0"
                      aria-label={`Edit ${festival.name} ${festival.edition_year}`}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openDelete(festival.id, festival.name)}
                      className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                      aria-label={`Delete ${festival.name} ${festival.edition_year}`}
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
      <CreateFestivalForm
        open={dialogMode === 'create'}
        onOpenChange={(open) => !open && closeDialog()}
        onSuccess={closeDialog}
      />

      {/* Edit — right-anchored Sheet (PSY-930 AdminFormLayout) */}
      {selectedFestivalId && (
        <EditFestivalForm
          festivalId={selectedFestivalId}
          open={dialogMode === 'edit'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}

      {/* Delete — centered Modal (PSY-930 AdminFormLayout) */}
      {selectedFestivalId && (
        <DeleteConfirmation
          festivalName={selectedFestivalName}
          festivalId={selectedFestivalId}
          open={dialogMode === 'delete'}
          onOpenChange={(open) => !open && closeDialog()}
          onSuccess={closeDialog}
        />
      )}
    </div>
  )
}

export default FestivalManagement
