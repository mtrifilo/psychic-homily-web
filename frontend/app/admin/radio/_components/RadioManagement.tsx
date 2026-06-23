'use client'

import { useState, useCallback, useEffect, useMemo, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  Search,
  Inbox,
  Radio,
  X,
  ChevronLeft,
  Download,
  AlertCircle,
  Link2,
  UserPlus,
  SkipForward,
  BarChart3,
  Radar,
  Upload,
  PlayCircle,
  XCircle,
  CheckCircle2,
  Lock,
} from 'lucide-react'
import { AdminEmptyState } from '@/components/admin'
import { AdminTable, type AdminTableColumn } from '@/components/admin/AdminTable'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DateInput } from '@/components/ui/date-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import {
  PLAYLIST_SOURCES,
  PLAYLIST_SOURCE_NONE,
  toPlaylistSelectValue,
  fromPlaylistSelectValue,
} from './playlistSourceSelect'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  AdminFormLayout,
  AdminFormRow,
  AdminFormField,
} from '@/components/admin/AdminFormLayout'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  useAdminRadioStations,
  useRadioStationDetail,
  useRadioShows,
  useRadioStats,
  useCreateRadioStation,
  useUpdateRadioStation,
  useDeleteRadioStation,
  useCreateRadioShow,
  useUpdateRadioShow,
  useDeleteRadioShow,
  radioQueryKeys,
  useTriggerStationSync,
  useTriggerShowBackfill,
  useSyncRun,
  useCancelSyncRun,
  useStationSyncRuns,
  useRecentFailedRuns,
  useStationHealth,
  useListStationHealth,
  useBulkSetShowLifecycle,
  useUnmatchedPlays,
  useBulkLinkPlays,
  type RadioStationListItem,
  type RadioStationDetail,
  type RadioShowListItem,
  type RadioSyncRun,
  type RadioStationHealth,
  type CreateRadioStationInput,
  type UpdateRadioStationInput,
  type CreateRadioShowInput,
  type UpdateRadioShowInput,
  type UnmatchedPlayGroup,
  type SuggestedMatch,
} from '@/lib/hooks/admin/useAdminRadio'

// ============================================================================
// Constants
// ============================================================================

const BROADCAST_TYPES = [
  { value: 'terrestrial', label: 'Terrestrial' },
  { value: 'internet', label: 'Internet' },
  { value: 'both', label: 'Both' },
] as const

type DialogMode = 'create-station' | 'edit-station' | 'delete-station' | 'create-show' | 'edit-show' | 'delete-show' | null

// ============================================================================
// Create Station Form
// ============================================================================

export function CreateStationForm({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const createMutation = useCreateRadioStation()

  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [description, setDescription] = useState('')
  const [city, setCity] = useState('')
  const [state, setState] = useState('')
  const [country, setCountry] = useState('US')
  const [timezone, setTimezone] = useState('')
  const [broadcastType, setBroadcastType] = useState('both')
  const [frequencyMHz, setFrequencyMHz] = useState('')
  const [streamUrl, setStreamUrl] = useState('')
  const [website, setWebsite] = useState('')
  const [donationUrl, setDonationUrl] = useState('')
  const [logoUrl, setLogoUrl] = useState('')
  const [playlistSource, setPlaylistSource] = useState('')
  const [playlistConfigJson, setPlaylistConfigJson] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Reset the form when the Sheet (re)opens. AdminFormLayout keeps this component
  // mounted (so the Sheet's close animation can run), so — unlike the old
  // unmount-on-close Dialog — its state would otherwise persist a prior session's
  // input (or a just-created station's values) into the next "Add Station".
  // This is the React "adjust state during render" pattern: resetting here rather
  // than in an effect avoids a cascading re-render (react-hooks/set-state-in-effect)
  // and an extra paint. (PSY-911)
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setName('')
      setSlug('')
      setDescription('')
      setCity('')
      setState('')
      setCountry('US')
      setTimezone('')
      setBroadcastType('both')
      setFrequencyMHz('')
      setStreamUrl('')
      setWebsite('')
      setDonationUrl('')
      setLogoUrl('')
      setPlaylistSource('')
      setPlaylistConfigJson('')
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
      if (!broadcastType) {
        setError('Broadcast type is required')
        return
      }

      let parsedConfig: Record<string, unknown> | null = null
      if (playlistConfigJson.trim()) {
        try {
          parsedConfig = JSON.parse(playlistConfigJson)
        } catch {
          setError('Invalid JSON in playlist config')
          return
        }
      }

      const input: CreateRadioStationInput = {
        name: name.trim(),
        broadcast_type: broadcastType,
        ...(slug.trim() && { slug: slug.trim() }),
        ...(description.trim() && { description: description.trim() }),
        ...(city.trim() && { city: city.trim() }),
        ...(state.trim() && { state: state.trim() }),
        ...(country.trim() && { country: country.trim() }),
        ...(timezone.trim() && { timezone: timezone.trim() }),
        ...(streamUrl.trim() && { stream_url: streamUrl.trim() }),
        ...(website.trim() && { website: website.trim() }),
        ...(donationUrl.trim() && { donation_url: donationUrl.trim() }),
        ...(logoUrl.trim() && { logo_url: logoUrl.trim() }),
        ...(playlistSource && { playlist_source: playlistSource }),
        ...(parsedConfig && { playlist_config: parsedConfig }),
        ...(frequencyMHz && { frequency_mhz: parseFloat(frequencyMHz) }),
      }

      createMutation.mutate(input, {
        onSuccess: () => onSuccess(),
        onError: (err) => setError(err.message),
      })
    },
    [name, slug, description, city, state, country, timezone, broadcastType, frequencyMHz, streamUrl, website, donationUrl, logoUrl, playlistSource, playlistConfigJson, createMutation, onSuccess]
  )

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Add Radio Station"
      description="Create a new radio station."
      error={error || undefined}
      onSubmit={handleSubmit}
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" disabled={createMutation.isPending}>
            {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create Station
          </Button>
        </>
      }
    >
      <AdminFormRow cols={2}>
        <AdminFormField label="Name *" htmlFor="station-name">
          <Input id="station-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="KEXP" />
        </AdminFormField>
        <AdminFormField label="Slug (auto if empty)" htmlFor="station-slug">
          <Input id="station-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="kexp" />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormField label="Description" htmlFor="station-description">
        <Textarea id="station-description" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Station description..." rows={2} />
      </AdminFormField>

      <AdminFormRow cols={4}>
        <AdminFormField label="City" htmlFor="station-city">
          <Input id="station-city" value={city} onChange={(e) => setCity(e.target.value)} placeholder="Seattle" />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="station-state">
          <Input id="station-state" value={state} onChange={(e) => setState(e.target.value)} placeholder="WA" />
        </AdminFormField>
        <AdminFormField label="Country" htmlFor="station-country">
          <Input id="station-country" value={country} onChange={(e) => setCountry(e.target.value)} placeholder="US" />
        </AdminFormField>
        <AdminFormField label="Timezone" htmlFor="station-timezone">
          <Input id="station-timezone" value={timezone} onChange={(e) => setTimezone(e.target.value)} placeholder="America/Los_Angeles" />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Broadcast Type *" htmlFor="station-broadcast-type">
          <Select value={broadcastType} onValueChange={setBroadcastType}>
            <SelectTrigger id="station-broadcast-type" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BROADCAST_TYPES.map((bt) => (
                <SelectItem key={bt.value} value={bt.value}>{bt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Frequency (MHz)" htmlFor="station-frequency">
          <Input id="station-frequency" type="number" step="0.1" value={frequencyMHz} onChange={(e) => setFrequencyMHz(e.target.value)} placeholder="90.3" />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Stream URL" htmlFor="station-stream-url">
          <Input id="station-stream-url" value={streamUrl} onChange={(e) => setStreamUrl(e.target.value)} placeholder="https://..." />
        </AdminFormField>
        <AdminFormField label="Website" htmlFor="station-website">
          <Input id="station-website" value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="https://..." />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Donation URL" htmlFor="station-donation-url">
          <Input id="station-donation-url" value={donationUrl} onChange={(e) => setDonationUrl(e.target.value)} placeholder="https://..." />
        </AdminFormField>
        <AdminFormField label="Logo URL" htmlFor="station-logo-url">
          <Input id="station-logo-url" value={logoUrl} onChange={(e) => setLogoUrl(e.target.value)} placeholder="https://..." />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Playlist Source" htmlFor="station-playlist-source">
          <Select
            value={toPlaylistSelectValue(playlistSource)}
            onValueChange={(v) => setPlaylistSource(fromPlaylistSelectValue(v))}
          >
            <SelectTrigger id="station-playlist-source" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={PLAYLIST_SOURCE_NONE}>None</SelectItem>
              {PLAYLIST_SOURCES.map((ps) => (
                <SelectItem key={ps.value} value={ps.value}>{ps.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Playlist Config (JSON)" htmlFor="station-playlist-config">
          <Input id="station-playlist-config" value={playlistConfigJson} onChange={(e) => setPlaylistConfigJson(e.target.value)} placeholder='{"api_key": "..."}' />
        </AdminFormField>
      </AdminFormRow>
    </AdminFormLayout>
  )
}

// ============================================================================
// Edit Station Form
// ============================================================================
//
// Migrated to AdminFormLayout (Sheet) in PSY-930. The station detail loads
// async (useRadioStationDetail), so per the PSY-930 decision the Sheet opens
// IMMEDIATELY on click and shows a spinner in the body until the detail
// resolves, then the fields populate (not render-when-the-whole-thing-loads).
// EditStationFormFields owns the AdminFormLayout and the field state;
// EditStationFormWrapper gates loading vs loaded.

// Exported only for direct regression-test access (rerender-with-different-key
// resets fields; rerender-with-same-key preserves dirty edits). Production
// callers go through EditStationFormWrapper.
export function EditStationFormFields({
  station,
  open,
  onOpenChange,
  onSuccess,
}: {
  station: RadioStationDetail
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const updateMutation = useUpdateRadioStation()

  const [name, setName] = useState(station.name)
  const [description, setDescription] = useState(station.description ?? '')
  const [city, setCity] = useState(station.city ?? '')
  const [state, setState] = useState(station.state ?? '')
  const [country, setCountry] = useState(station.country ?? 'US')
  const [timezone, setTimezone] = useState(station.timezone ?? '')
  const [broadcastType, setBroadcastType] = useState(station.broadcast_type)
  const [frequencyMHz, setFrequencyMHz] = useState(station.frequency_mhz?.toString() ?? '')
  const [streamUrl, setStreamUrl] = useState(station.stream_url ?? '')
  const [website, setWebsite] = useState(station.website ?? '')
  const [donationUrl, setDonationUrl] = useState(station.donation_url ?? '')
  const [logoUrl, setLogoUrl] = useState(station.logo_url ?? '')
  const [playlistSource, setPlaylistSource] = useState(station.playlist_source ?? '')
  const [playlistConfigJson, setPlaylistConfigJson] = useState(
    station.playlist_config ? JSON.stringify(station.playlist_config) : ''
  )
  const [isActive, setIsActive] = useState(station.is_active)
  const [error, setError] = useState<string | null>(null)

  // Clear the transient submit error when the Sheet (re)opens. AdminFormLayout
  // keeps this component mounted across close (for the close animation), and a
  // re-open for the SAME station does NOT remount (the key={station.id} reset in
  // EditStationFormWrapper only fires on a station switch). Without this, a
  // failed edit's error banner would persist into the next open of the same
  // station until the next submit. Mirrors CreateStationForm's reset-on-open
  // (the React "adjust state during render" pattern — no effect, no extra paint).
  // Only `error` is reset here: the field values intentionally keep their dirty
  // edits across a close+reopen of the same station (only a station switch, which
  // remounts via key, re-initializes them from props). (PSY-1121)
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
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

      let parsedConfig: Record<string, unknown> | null = null
      if (playlistConfigJson.trim()) {
        try {
          parsedConfig = JSON.parse(playlistConfigJson)
        } catch {
          setError('Invalid JSON in playlist config')
          return
        }
      }

      const input: UpdateRadioStationInput & { id: number } = {
        id: station.id,
        name: name.trim(),
        description: description.trim() || null,
        city: city.trim() || null,
        state: state.trim() || null,
        country: country.trim() || null,
        timezone: timezone.trim() || null,
        broadcast_type: broadcastType,
        frequency_mhz: frequencyMHz ? parseFloat(frequencyMHz) : null,
        stream_url: streamUrl.trim() || null,
        website: website.trim() || null,
        donation_url: donationUrl.trim() || null,
        logo_url: logoUrl.trim() || null,
        playlist_source: playlistSource || null,
        playlist_config: parsedConfig,
        is_active: isActive,
      }

      updateMutation.mutate(input, {
        onSuccess: () => onSuccess(),
        onError: (err) => setError(err.message),
      })
    },
    [name, description, city, state, country, timezone, broadcastType, frequencyMHz, streamUrl, website, donationUrl, logoUrl, playlistSource, playlistConfigJson, isActive, station.id, updateMutation, onSuccess]
  )

  return (
    <AdminFormLayout
      variant="sheet"
      open={open}
      onOpenChange={onOpenChange}
      title="Edit Station"
      description="Update station details."
      error={error || undefined}
      onSubmit={handleSubmit}
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" disabled={updateMutation.isPending}>
            {updateMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save Changes
          </Button>
        </>
      }
    >
      <AdminFormField label="Name *" htmlFor="edit-station-name">
        <Input id="edit-station-name" value={name} onChange={(e) => setName(e.target.value)} />
      </AdminFormField>

      <AdminFormField label="Description" htmlFor="edit-station-description">
        <Textarea id="edit-station-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </AdminFormField>

      <AdminFormRow cols={4}>
        <AdminFormField label="City" htmlFor="edit-station-city">
          <Input id="edit-station-city" value={city} onChange={(e) => setCity(e.target.value)} />
        </AdminFormField>
        <AdminFormField label="State" htmlFor="edit-station-state">
          <Input id="edit-station-state" value={state} onChange={(e) => setState(e.target.value)} />
        </AdminFormField>
        <AdminFormField label="Country" htmlFor="edit-station-country">
          <Input id="edit-station-country" value={country} onChange={(e) => setCountry(e.target.value)} />
        </AdminFormField>
        <AdminFormField label="Timezone" htmlFor="edit-station-timezone">
          <Input id="edit-station-timezone" value={timezone} onChange={(e) => setTimezone(e.target.value)} />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Broadcast Type" htmlFor="edit-station-broadcast-type">
          <Select value={broadcastType} onValueChange={setBroadcastType}>
            <SelectTrigger id="edit-station-broadcast-type" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BROADCAST_TYPES.map((bt) => (
                <SelectItem key={bt.value} value={bt.value}>{bt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Frequency (MHz)" htmlFor="edit-station-frequency">
          <Input id="edit-station-frequency" type="number" step="0.1" value={frequencyMHz} onChange={(e) => setFrequencyMHz(e.target.value)} />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Stream URL" htmlFor="edit-station-stream-url">
          <Input id="edit-station-stream-url" value={streamUrl} onChange={(e) => setStreamUrl(e.target.value)} />
        </AdminFormField>
        <AdminFormField label="Website" htmlFor="edit-station-website">
          <Input id="edit-station-website" value={website} onChange={(e) => setWebsite(e.target.value)} />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Donation URL" htmlFor="edit-station-donation-url">
          <Input id="edit-station-donation-url" value={donationUrl} onChange={(e) => setDonationUrl(e.target.value)} />
        </AdminFormField>
        <AdminFormField label="Logo URL" htmlFor="edit-station-logo-url">
          <Input id="edit-station-logo-url" value={logoUrl} onChange={(e) => setLogoUrl(e.target.value)} />
        </AdminFormField>
      </AdminFormRow>

      <AdminFormRow cols={2}>
        <AdminFormField label="Playlist Source" htmlFor="edit-station-playlist-source">
          <Select
            value={toPlaylistSelectValue(playlistSource)}
            onValueChange={(v) => setPlaylistSource(fromPlaylistSelectValue(v))}
          >
            <SelectTrigger id="edit-station-playlist-source" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={PLAYLIST_SOURCE_NONE}>None</SelectItem>
              {PLAYLIST_SOURCES.map((ps) => (
                <SelectItem key={ps.value} value={ps.value}>{ps.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </AdminFormField>
        <AdminFormField label="Playlist Config (JSON)" htmlFor="edit-station-playlist-config">
          <Input id="edit-station-playlist-config" value={playlistConfigJson} onChange={(e) => setPlaylistConfigJson(e.target.value)} />
        </AdminFormField>
      </AdminFormRow>

      <div className="flex items-center gap-2">
        <Switch id="edit-station-active" checked={isActive} onCheckedChange={setIsActive} />
        <Label htmlFor="edit-station-active">Active</Label>
      </div>
    </AdminFormLayout>
  )
}

// ============================================================================
// Create Show Form
// ============================================================================

function CreateShowForm({
  stationId,
  onSuccess,
  onCancel,
}: {
  stationId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const createMutation = useCreateRadioShow()

  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [hostName, setHostName] = useState('')
  const [description, setDescription] = useState('')
  const [scheduleDisplay, setScheduleDisplay] = useState('')
  const [archiveUrl, setArchiveUrl] = useState('')
  const [imageUrl, setImageUrl] = useState('')
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!name.trim()) {
        setError('Name is required')
        return
      }

      const input: CreateRadioShowInput & { stationId: number } = {
        stationId,
        name: name.trim(),
        ...(slug.trim() && { slug: slug.trim() }),
        ...(hostName.trim() && { host_name: hostName.trim() }),
        ...(description.trim() && { description: description.trim() }),
        ...(scheduleDisplay.trim() && { schedule_display: scheduleDisplay.trim() }),
        ...(archiveUrl.trim() && { archive_url: archiveUrl.trim() }),
        ...(imageUrl.trim() && { image_url: imageUrl.trim() }),
      }

      createMutation.mutate(input, {
        onSuccess: () => onSuccess(),
        onError: (err) => setError(err.message),
      })
    },
    [name, slug, hostName, description, scheduleDisplay, archiveUrl, imageUrl, stationId, createMutation, onSuccess]
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</div>
      )}

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="show-name">Name *</Label>
          <Input id="show-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Morning Show" />
        </div>
        <div>
          <Label htmlFor="show-slug">Slug (auto if empty)</Label>
          <Input id="show-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="morning-show" />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="show-host">Host Name</Label>
          <Input id="show-host" value={hostName} onChange={(e) => setHostName(e.target.value)} placeholder="DJ Cool" />
        </div>
        <div>
          <Label htmlFor="show-schedule">Schedule</Label>
          <Input id="show-schedule" value={scheduleDisplay} onChange={(e) => setScheduleDisplay(e.target.value)} placeholder="Mon-Fri 6-10am" />
        </div>
      </div>

      <div>
        <Label htmlFor="show-description">Description</Label>
        <Textarea id="show-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="show-archive-url">Archive URL</Label>
          <Input id="show-archive-url" value={archiveUrl} onChange={(e) => setArchiveUrl(e.target.value)} placeholder="https://..." />
        </div>
        <div>
          <Label htmlFor="show-image-url">Image URL</Label>
          <Input id="show-image-url" value={imageUrl} onChange={(e) => setImageUrl(e.target.value)} placeholder="https://..." />
        </div>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
        <Button type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Create Show
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Edit Show Form
// ============================================================================

function EditShowForm({
  show,
  stationId,
  onSuccess,
  onCancel,
}: {
  show: RadioShowListItem
  stationId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const updateMutation = useUpdateRadioShow()

  const [name, setName] = useState(show.name)
  const [hostName, setHostName] = useState(show.host_name ?? '')
  const [description, setDescription] = useState('')
  const [scheduleDisplay, setScheduleDisplay] = useState('')
  const [archiveUrl, setArchiveUrl] = useState('')
  const [imageUrl, setImageUrl] = useState(show.image_url ?? '')
  const [isActive, setIsActive] = useState(show.is_active)
  const [scheduleLocked, setScheduleLocked] = useState(show.schedule_locked)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!name.trim()) {
        setError('Name is required')
        return
      }

      const input: UpdateRadioShowInput & { showId: number; stationId: number } = {
        showId: show.id,
        stationId,
        name: name.trim(),
        host_name: hostName.trim() || null,
        description: description.trim() || null,
        schedule_display: scheduleDisplay.trim() || null,
        archive_url: archiveUrl.trim() || null,
        image_url: imageUrl.trim() || null,
        is_active: isActive,
        schedule_locked: scheduleLocked,
      }

      updateMutation.mutate(input, {
        onSuccess: () => onSuccess(),
        onError: (err) => setError(err.message),
      })
    },
    [name, hostName, description, scheduleDisplay, archiveUrl, imageUrl, isActive, scheduleLocked, show.id, stationId, updateMutation, onSuccess]
  )

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</div>
      )}

      <div>
        <Label htmlFor="edit-show-name">Name *</Label>
        <Input id="edit-show-name" value={name} onChange={(e) => setName(e.target.value)} />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-show-host">Host Name</Label>
          <Input id="edit-show-host" value={hostName} onChange={(e) => setHostName(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-show-schedule">Schedule</Label>
          <Input id="edit-show-schedule" value={scheduleDisplay} onChange={(e) => setScheduleDisplay(e.target.value)} />
        </div>
      </div>

      <div>
        <Label htmlFor="edit-show-description">Description</Label>
        <Textarea id="edit-show-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-show-archive-url">Archive URL</Label>
          <Input id="edit-show-archive-url" value={archiveUrl} onChange={(e) => setArchiveUrl(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-show-image-url">Image URL</Label>
          <Input id="edit-show-image-url" value={imageUrl} onChange={(e) => setImageUrl(e.target.value)} />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch id="edit-show-active" checked={isActive} onCheckedChange={setIsActive} />
        <Label htmlFor="edit-show-active">Active</Label>
      </div>

      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Switch id="edit-show-schedule-locked" checked={scheduleLocked} onCheckedChange={setScheduleLocked} />
          <Label htmlFor="edit-show-schedule-locked">Lock schedule</Label>
        </div>
        <p className="text-xs text-muted-foreground">
          When on, this schedule is hand-curated and the weekly WFMU scrape leaves it alone. Turn off to resume auto-scrape.
        </p>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
        <Button type="submit" disabled={updateMutation.isPending}>
          {updateMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Save Changes
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Station Detail Panel (with shows management)
// ============================================================================

// ============================================================================
// Sync run tracking + progress row
// ============================================================================

// Track a triggered sync run: hold its id, poll it (useSyncRun), and when it
// reaches a terminal status invalidate the station's shows + the stations/stats
// lists exactly ONCE per run (so newly-discovered shows / updated episode counts
// appear). The settled guard keys on the run ID, not the status string: two
// same-session runs that both end on the same terminal status (e.g. a fast
// discover that's already `success` on its first poll) must each invalidate, so
// a boolean/once-on-`running` guard would miss the second run.
//
// `trackRun` takes the FULL run returned by the trigger (not just its id) and
// seeds it into the query cache, so `isRunning` flips true on the same render the
// trigger resolves — without it there's a gap between the mutation settling and
// the first poll returning `running` where the trigger button is briefly
// re-enabled and a second run could be fired. Shared by ShowImportSection
// (backfill) and StationDetailPanel (discover). Exported for direct hook testing.
export function useTrackedSyncRun(stationId: number) {
  const queryClient = useQueryClient()
  const [runId, setRunId] = useState<number | null>(null)
  const { data: run, isError } = useSyncRun(runId ?? 0, runId != null)
  const settledRunRef = useRef<number | null>(null)
  const status = run?.status

  useEffect(() => {
    if (runId == null || !status || status === 'running') return
    if (settledRunRef.current === runId) return
    settledRunRef.current = runId
    queryClient.invalidateQueries({ queryKey: radioQueryKeys.shows(stationId) })
    queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
    queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
  }, [runId, status, queryClient, stationId])

  const trackRun = useCallback(
    (opened: RadioSyncRun) => {
      queryClient.setQueryData(radioQueryKeys.syncRun(opened.id), opened)
      setRunId(opened.id)
    },
    [queryClient]
  )

  // isRunning is false once the poll has errored out, even if the cache still
  // holds a seeded/last-good `running` value — react-query RETAINS prior data on a
  // fetch error, so without the `!isError` guard a first-poll error after seeding
  // would leave isRunning stuck true (button frozen "…Running") with the poll
  // already stopped. On error the consumer shows an error note + re-enables the
  // trigger instead of a permanently-spinning row.
  return { run, isRunning: status === 'running' && !isError, isError, trackRun }
}

// Render the play-match summary for a job. A job that imported zero plays has
// nothing to match, so "0% matched" reads as a matching failure when in fact
// there was simply nothing to match — show "no plays found" instead, and only
// show a percentage once plays_imported > 0. (PSY-1120)
function PlaysMatchSummary({
  playsImported,
  playsMatched,
}: {
  playsImported: number
  playsMatched: number
}) {
  if (playsImported === 0) {
    return <>no plays found</>
  }
  const percent = Math.round((playsMatched / playsImported) * 100)
  return (
    <>
      {playsImported.toLocaleString()} plays &mdash; {percent}% matched
    </>
  )
}

// Status display config for a sync run (PSY-1136). `partial` = imported data but
// hit per-episode/match errors (the old "completed with errors"); `skipped` = the
// station circuit breaker was open.
const SYNC_RUN_STATUS_DISPLAY: Record<RadioSyncRun['status'], { label: string; color: string }> = {
  running: { label: 'running', color: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' },
  success: { label: 'success', color: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  partial: { label: 'completed with errors', color: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' },
  failed: { label: 'failed', color: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400' },
  skipped: { label: 'skipped (breaker)', color: 'bg-muted text-muted-foreground' },
  cancelled: { label: 'cancelled', color: 'bg-muted text-muted-foreground' },
}

function SyncRunStatusIcon({ status }: { status: RadioSyncRun['status'] }) {
  switch (status) {
    case 'running':
      return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
    case 'success':
      return <CheckCircle2 className="h-4 w-4 text-green-500" />
    case 'partial':
      return <AlertCircle className="h-4 w-4 text-amber-500" />
    case 'failed':
      return <AlertCircle className="h-4 w-4 text-destructive" />
    case 'skipped':
    case 'cancelled':
      return <XCircle className="h-4 w-4 text-muted-foreground" />
  }
}

// Render a sync run (radio_sync_runs) — the unified ingestion-run row replacing
// ImportJobRow (PSY-1136). A `partial` run carries the old "completed with errors"
// meaning (imported data, but some episodes/matches failed); the structured
// errors[] list replaces the parsed error_log header. window_start/end render
// only for backfill runs; the episode progress bar only for episode-importing
// runs (fetch/backfill), not discover. Exported for direct regression-test access;
// production callers reach it through ShowImportSection / StationDetailPanel.
export function SyncRunRow({ run }: { run: RadioSyncRun }) {
  const cancelMutation = useCancelSyncRun()

  const isActive = run.status === 'running'
  const isTerminalGood = run.status === 'success' || run.status === 'partial'
  const display = SYNC_RUN_STATUS_DISPLAY[run.status]
  const progress = run.episodes_found > 0
    ? Math.round((run.episodes_imported / run.episodes_found) * 100)
    : 0
  const errors = run.errors ?? []

  return (
    <div className="rounded-lg border p-4 space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <SyncRunStatusIcon status={run.status} />
          <Badge className={display.color}>{display.label}</Badge>
          <span className="text-xs uppercase tracking-wide text-muted-foreground">
            {run.run_type}
          </span>
          {run.window_start && run.window_end && (
            <span className="text-sm text-muted-foreground">
              {run.window_start} to {run.window_end}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {run.started_at && (
            <span className="text-xs text-muted-foreground">
              Started {new Date(run.started_at).toLocaleString()}
            </span>
          )}
          {isActive && (
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              disabled={cancelMutation.isPending}
              onClick={() => cancelMutation.mutate(run.id)}
            >
              {cancelMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin mr-1" />
              ) : (
                <XCircle className="h-3 w-3 mr-1" />
              )}
              Cancel
            </Button>
          )}
        </div>
      </div>

      {/* Episode progress (running / terminal good) — episode-importing runs only
          (fetch/backfill). Gated on run_type, not just episodes_found, so a
          discover run never shows an episode-import progress bar even if the
          provider reports a non-zero episode scan count. */}
      {run.run_type !== 'discover' &&
        (run.status === 'running' || isTerminalGood) &&
        run.episodes_found > 0 && (
        <div className="space-y-1">
          <div className="h-2 rounded-full bg-muted overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${
                run.status === 'running' ? 'bg-chart-6' : 'bg-chart-2'
              }`}
              style={{ width: `${progress}%` }}
            />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {run.episodes_imported.toLocaleString()} / {run.episodes_found.toLocaleString()} episodes
              {run.current_episode_date && run.status === 'running' && (
                <> &mdash; processing {run.current_episode_date}</>
              )}
            </span>
            <span>
              <PlaysMatchSummary
                playsImported={run.plays_imported}
                playsMatched={run.plays_matched}
              />
            </span>
          </div>
        </div>
      )}

      {/* Partial-run warning: the run finished but some episodes/matches failed.
          We summarise the ERROR count (not plays_unmatched — unmatched plays are a
          normal outcome for unknown artists, not a failure). The episode count is
          shown only for episode-importing runs; a `partial` discover run just shows
          the error count + the categorized list below. */}
      {run.status === 'partial' && (
        <div className="rounded-md bg-amber-100 p-2 text-xs text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
          <p className="font-medium">Completed with errors</p>
          <p>
            {run.run_type !== 'discover' && (
              <>
                {run.episodes_imported.toLocaleString()} episode
                {run.episodes_imported === 1 ? '' : 's'} imported &mdash;{' '}
              </>
            )}
            {errors.length.toLocaleString()} error{errors.length === 1 ? '' : 's'}.
          </p>
        </div>
      )}

      {/* Categorized error list (failed / partial runs) */}
      {errors.length > 0 && (run.status === 'failed' || run.status === 'partial') && (
        <div className="rounded-md bg-destructive/10 p-2 text-xs text-destructive max-h-24 overflow-y-auto space-y-1">
          {errors.slice(0, 20).map((e, i) => (
            <p key={i}>
              <span className="font-medium">{e.category}</span>
              {e.detail ? `: ${e.detail}` : ''}
            </p>
          ))}
          {errors.length > 20 && (
            <p className="text-muted-foreground">+{(errors.length - 20).toLocaleString()} more</p>
          )}
        </div>
      )}

      {/* Failed run with no categorized errors recorded — still give the operator
          a reason rather than a bare red badge. */}
      {run.status === 'failed' && errors.length === 0 && (
        <div className="rounded-md bg-destructive/10 p-2 text-xs text-destructive">
          The run failed before any detailed error was recorded.
        </div>
      )}

      {/* Completed summary (success / partial) */}
      {isTerminalGood && (
        <div className="text-xs text-muted-foreground">
          Completed {run.finished_at ? new Date(run.finished_at).toLocaleString() : ''} &mdash;{' '}
          {run.run_type === 'discover' ? (
            'discovery finished'
          ) : (
            <>
              {run.episodes_imported.toLocaleString()} episodes,{' '}
              {run.plays_imported > 0 ? (
                <>
                  {run.plays_imported.toLocaleString()} plays,{' '}
                  {run.plays_matched.toLocaleString()} matched
                </>
              ) : (
                'no plays found'
              )}
            </>
          )}
        </div>
      )}

      {/* Skipped (breaker open) */}
      {run.status === 'skipped' && (
        <div className="text-xs text-muted-foreground">
          Skipped &mdash; the station circuit breaker was open.
        </div>
      )}

      {/* Cancelled — body line for symmetry with the other terminal states (so a
          cancelled run isn't a bare badge). */}
      {run.status === 'cancelled' && (
        <div className="text-xs text-muted-foreground">
          Cancelled before completion.
        </div>
      )}
    </div>
  )
}

// ============================================================================
// Sync-run feeds (PSY-1130): per-station history + global recent-failures
// ============================================================================

// Per-station feed: recent sync runs (newest first), reusing SyncRunRow. Anomalies
// (empty_unexpected, PSY-1156) surface inline via SyncRunRow's categorized error list
// on partial/failed runs. Handles loading / empty / error.
function StationSyncRunFeed({ stationId }: { stationId: number }) {
  const { data, isLoading, isError } = useStationSyncRuns(stationId)
  const runs = data?.sync_runs ?? []

  return (
    <div>
      <h4 className="font-medium mb-3">Recent sync runs</h4>
      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : isError ? (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          Couldn&apos;t load sync runs. Reload to try again.
        </div>
      ) : runs.length === 0 ? (
        <AdminEmptyState icon={Inbox} message="No sync runs recorded for this station yet." />
      ) : (
        <div className="space-y-3">
          {runs.map((run) => (
            <SyncRunRow key={run.id} run={run} />
          ))}
        </div>
      )}
    </div>
  )
}

// One compact cross-station failure row: station + status + when + a one-line error
// summary, clickable to open the station's detail.
function GlobalFailureRow({
  run,
  onOpen,
}: {
  run: RadioSyncRun
  onOpen: (stationId: number) => void
}) {
  const display = SYNC_RUN_STATUS_DISPLAY[run.status]
  const firstError = run.errors?.[0]
  const extraErrors = (run.errors?.length ?? 0) - 1
  return (
    <button
      type="button"
      onClick={() => onOpen(run.station_id)}
      className="w-full rounded-lg border p-3 text-left hover:bg-muted/50 transition-colors"
    >
      <div className="flex items-center gap-2">
        <SyncRunStatusIcon status={run.status} />
        <Badge className={display.color}>{display.label}</Badge>
        <span className="font-medium">{run.station_name}</span>
        <span className="text-xs uppercase tracking-wide text-muted-foreground">{run.run_type}</span>
        <span className="ml-auto text-xs text-muted-foreground">
          {new Date(run.started_at).toLocaleString()}
        </span>
      </div>
      {firstError && (
        <p className="mt-1 truncate text-xs text-destructive">
          <span className="font-medium">{firstError.category}</span>
          {firstError.detail ? `: ${firstError.detail}` : ''}
          {extraErrors > 0 ? ` (+${extraErrors} more)` : ''}
        </p>
      )}
    </button>
  )
}

// Global recent-failures view: the most recent failed + partial runs across all
// stations, clickable to the station. 'partial' carries the volume-anomaly signal
// (empty_unexpected), so it belongs here alongside outright failures.
function RecentFailuresPanel({
  onOpenStation,
}: {
  onOpenStation: (stationId: number) => void
}) {
  const { runs, isLoading, isError } = useRecentFailedRuns()

  if (isError) {
    return (
      <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
        Couldn&apos;t load recent sync failures.
      </div>
    )
  }
  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Checking recent sync runs&hellip;
      </div>
    )
  }
  if (runs.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <CheckCircle2 className="h-4 w-4 text-green-500" /> No recent sync failures.
      </div>
    )
  }
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <AlertCircle className="h-4 w-4 text-destructive" />
        <h3 className="font-medium">Recent sync failures</h3>
      </div>
      <div className="space-y-2">
        {runs.map((run) => (
          <GlobalFailureRow key={run.id} run={run} onOpen={onOpenStation} />
        ))}
      </div>
    </div>
  )
}

// ============================================================================
// Station health (PSY-1200): traffic-light rollup + metric card
// ============================================================================

export type StationHealthLevel = 'healthy' | 'warning' | 'breach' | 'unknown'

// deriveStationHealthLevel maps a health rollup to a traffic-light level. Thresholds are
// tunable; the signals follow the ticket — consecutive_failures, a low success rate, and
// a chronically-empty (high zero-play-episode) signal: the KEXP day-one case where syncs
// "succeed" but return nothing. nil rates ("never computed") never trigger a level on
// their own. A station that has never run is 'unknown'.
export function deriveStationHealthLevel(h: RadioStationHealth): StationHealthLevel {
  if (!h.last_run_at) return 'unknown'
  if (
    h.breaker_state === 'open' ||
    h.consecutive_failures >= 3 ||
    (h.recent_success_rate != null && h.recent_success_rate < 0.5) ||
    (h.zero_play_episode_rate != null && h.zero_play_episode_rate >= 0.8)
  ) {
    return 'breach'
  }
  if (
    h.consecutive_failures >= 1 ||
    (h.play_match_rate != null && h.play_match_rate < 0.3) ||
    (h.zero_play_episode_rate != null && h.zero_play_episode_rate >= 0.5)
  ) {
    return 'warning'
  }
  return 'healthy'
}

const STATION_HEALTH_LEVEL: Record<
  StationHealthLevel,
  { label: string; badge: string; dot: string }
> = {
  healthy: {
    label: 'Healthy',
    badge: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
    dot: 'bg-green-500',
  },
  warning: {
    label: 'Warning',
    badge: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    dot: 'bg-amber-500',
  },
  breach: {
    label: 'Needs attention',
    badge: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
    dot: 'bg-red-500',
  },
  unknown: {
    label: 'Never run',
    badge: 'bg-muted text-muted-foreground',
    dot: 'bg-muted-foreground/40',
  },
}

// Render a rate (0..1) as a whole percent, or an em-dash when not yet computed (null or
// omitted/undefined on the wire). The loose `== null` catches both.
function formatHealthRate(rate: number | null | undefined): string {
  if (rate == null) return '—'
  return `${Math.round(rate * 100)}%`
}

// Compact traffic-light badge for the stations table Health column.
function StationHealthBadge({ health }: { health: RadioStationHealth | undefined }) {
  if (!health) {
    return <span className="text-xs text-muted-foreground">&mdash;</span>
  }
  const display = STATION_HEALTH_LEVEL[deriveStationHealthLevel(health)]
  return (
    <Badge className={display.badge}>
      <span className={`mr-1.5 inline-block h-2 w-2 rounded-full ${display.dot}`} />
      {display.label}
    </Badge>
  )
}

function HealthMetric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="font-medium">{value}</div>
    </div>
  )
}

// Full station-health card for the detail panel: the traffic-light level + the metric
// grid (PSY-1200). A breach is visually emphasised (destructive border/tint).
function StationHealthCard({ stationId }: { stationId: number }) {
  const { data: health, isLoading, isError } = useStationHealth(stationId)

  if (isLoading) {
    return (
      <div className="flex justify-center py-6">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }
  if (isError) {
    return (
      <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
        Couldn&apos;t load station health. Reload to try again.
      </div>
    )
  }
  if (!health) return null

  const level = deriveStationHealthLevel(health)
  const display = STATION_HEALTH_LEVEL[level]
  return (
    <div
      className={`rounded-lg border p-4 ${
        level === 'breach' ? 'border-destructive/50 bg-destructive/5' : ''
      }`}
    >
      <div className="mb-3 flex items-center justify-between">
        <h4 className="font-medium">Station health</h4>
        <Badge className={display.badge}>
          <span className={`mr-1.5 inline-block h-2 w-2 rounded-full ${display.dot}`} />
          {display.label}
        </Badge>
      </div>
      <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
        <HealthMetric label="Success rate" value={formatHealthRate(health.recent_success_rate)} />
        <HealthMetric label="Play-match rate" value={formatHealthRate(health.play_match_rate)} />
        <HealthMetric label="0-play episodes" value={formatHealthRate(health.zero_play_episode_rate)} />
        <HealthMetric label="Consecutive failures" value={String(health.consecutive_failures)} />
        <HealthMetric label="Breaker" value={health.breaker_state} />
        <HealthMetric
          label="Last success"
          value={health.last_success_at ? new Date(health.last_success_at).toLocaleString() : 'Never'}
        />
      </div>
    </div>
  )
}

// ============================================================================
// Show Import Section (per-show historic backfill + live run tracking)
// ============================================================================

// Per-show backfill: trigger an async RunStationSync(backfill) over [since, until]
// and poll the returned run to completion (PSY-1136). The per-show import HISTORY
// list is intentionally gone — PR2 (PSY-1135) retired the list-jobs endpoint; the
// unified cross-show sync-run feed is P5 (PSY-1130). A run is tracked only within
// this component's session; reloading mid-run loses the handle (no list endpoint
// to recover it from until P5).
function ShowImportSection({
  show,
  stationId,
}: {
  show: RadioShowListItem
  stationId: number
}) {
  const triggerMutation = useTriggerShowBackfill()
  const { run: liveRun, isRunning, isError, trackRun } = useTrackedSyncRun(stationId)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const [error, setError] = useState<string | null>(null)

  const handleCreate = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!since) { setError('Start date is required'); return }
      if (!until) { setError('End date is required'); return }
      if (since > until) { setError('Start date must be before end date'); return }

      triggerMutation.mutate(
        { showId: show.id, since, until },
        {
          onSuccess: (run) => {
            trackRun(run)
            setShowCreateForm(false)
            setSince('')
            setUntil('')
          },
          onError: (err) => setError(err.message),
        }
      )
    },
    [since, until, show.id, triggerMutation, trackRun]
  )

  return (
    <div className="mt-3 space-y-3">
      {/* Live (and last-settled) run for this show. On a poll error we hide the
          (possibly-stale, retained-by-react-query) row and show the error instead,
          so the operator isn't left staring at a frozen "running" row. */}
      {liveRun && !isError && <SyncRunRow run={liveRun} />}
      {isError && (
        <div className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">
          Couldn&apos;t load the backfill run status. Reload to try again.
        </div>
      )}

      {/* Create backfill form */}
      {showCreateForm ? (
        <form onSubmit={handleCreate} className="rounded-lg border border-dashed p-4 space-y-3">
          {error && (
            <div className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">{error}</div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor={`since-${show.id}`}>From</Label>
              <DateInput
                id={`since-${show.id}`}
                value={since}
                onChange={(e) => setSince(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor={`until-${show.id}`}>To</Label>
              <DateInput
                id={`until-${show.id}`}
                value={until}
                onChange={(e) => setUntil(e.target.value)}
              />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" size="sm" disabled={triggerMutation.isPending}>
              {triggerMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin mr-1" />
              ) : (
                <PlayCircle className="h-4 w-4 mr-1" />
              )}
              Start Backfill
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setShowCreateForm(false)}>
              Cancel
            </Button>
          </div>
        </form>
      ) : (
        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowCreateForm(true)}
          disabled={isRunning}
        >
          <Download className="h-4 w-4 mr-1" />
          {isRunning ? 'Backfill Running...' : 'Backfill Episodes'}
        </Button>
      )}
    </div>
  )
}



// ============================================================================
// Station show list (PSY-1122): search / filter / sort / paginate + bulk lifecycle
// ============================================================================

const SHOW_LIFECYCLE_DISPLAY: Record<string, { label: string; className: string }> = {
  active: { label: 'Active', className: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  dormant: { label: 'Dormant', className: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' },
  retired: { label: 'Retired', className: 'bg-muted text-muted-foreground' },
}

function ShowLifecycleBadge({ state }: { state: string }) {
  const d = SHOW_LIFECYCLE_DISPLAY[state] ?? { label: state, className: 'bg-muted text-muted-foreground' }
  return <Badge className={`text-xs ${d.className}`}>{d.label}</Badge>
}

type ShowStatusFilter = 'current' | 'all' | 'active' | 'dormant' | 'retired'
type ShowSort = 'name' | 'episodes' | 'latest'
const SHOW_PAGE_SIZE = 25

// The navigable per-station show list: client-side search/filter/sort/paginate over the
// already-loaded shows (counts are small per station; move to server-side if a station
// ever holds hundreds). Default hides retired (active-first); 0-episode shows are
// de-emphasised; bulk lifecycle actions use the PSY-1172 write path.
function StationShowList({
  shows,
  stationId,
  isLoading,
  expandedShows,
  onToggleExpand,
  onAddShow,
  onEditShow,
  onDeleteShow,
}: {
  shows: RadioShowListItem[]
  stationId: number
  isLoading: boolean
  expandedShows: Set<number>
  onToggleExpand: (showId: number) => void
  onAddShow: () => void
  onEditShow: (show: RadioShowListItem) => void
  onDeleteShow: (show: RadioShowListItem) => void
}) {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<ShowStatusFilter>('current')
  const [sort, setSort] = useState<ShowSort>('name')
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const bulkLifecycle = useBulkSetShowLifecycle()

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    const list = shows.filter((s) => {
      if (statusFilter === 'current') {
        if (s.lifecycle_state === 'retired') return false
      } else if (statusFilter !== 'all') {
        if (s.lifecycle_state !== statusFilter) return false
      }
      if (q) {
        return s.name.toLowerCase().includes(q) || (s.host_name ?? '').toLowerCase().includes(q)
      }
      return true
    })
    return [...list].sort((a, b) => {
      if (sort === 'episodes') return b.episode_count - a.episode_count
      if (sort === 'latest') return (b.latest_air_date ?? '').localeCompare(a.latest_air_date ?? '')
      return a.name.localeCompare(b.name)
    })
  }, [shows, search, statusFilter, sort])

  const pageCount = Math.max(1, Math.ceil(filtered.length / SHOW_PAGE_SIZE))
  const safePage = Math.min(page, pageCount - 1)
  const pageItems = filtered.slice(safePage * SHOW_PAGE_SIZE, (safePage + 1) * SHOW_PAGE_SIZE)

  // Reset paging + selection whenever the filtered set is re-scoped.
  const resetView = useCallback(() => {
    setPage(0)
    setSelected(new Set())
  }, [])

  const toggleSelected = useCallback((showId: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(showId)) next.delete(showId)
      else next.add(showId)
      return next
    })
  }, [])

  const allOnPageSelected = pageItems.length > 0 && pageItems.every((s) => selected.has(s.id))
  const toggleSelectPage = useCallback(() => {
    setSelected((prev) => {
      const next = new Set(prev)
      const allSelected = pageItems.length > 0 && pageItems.every((s) => next.has(s.id))
      for (const s of pageItems) {
        if (allSelected) next.delete(s.id)
        else next.add(s.id)
      }
      return next
    })
  }, [pageItems])

  const applyBulkLifecycle = useCallback(
    (lifecycleState: string) => {
      const showIds = Array.from(selected)
      if (showIds.length === 0) return
      bulkLifecycle.mutate(
        { showIds, stationId, lifecycleState },
        { onSuccess: () => setSelected(new Set()) }
      )
    },
    [selected, stationId, bulkLifecycle]
  )

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h4 className="font-medium">Shows</h4>
        <Button size="sm" onClick={onAddShow}>
          <Plus className="mr-1 h-4 w-4" /> Add Show
        </Button>
      </div>

      {/* Search / filter / sort */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="relative min-w-[200px] flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => { setSearch(e.target.value); resetView() }}
            placeholder="Search shows by name or host..."
            className="pl-10"
            aria-label="Search shows"
          />
        </div>
        <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v as ShowStatusFilter); resetView() }}>
          <SelectTrigger className="w-[170px]" aria-label="Filter by status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="current">Active &amp; dormant</SelectItem>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="dormant">Dormant</SelectItem>
            <SelectItem value="retired">Retired</SelectItem>
          </SelectContent>
        </Select>
        <Select value={sort} onValueChange={(v) => setSort(v as ShowSort)}>
          <SelectTrigger className="w-[150px]" aria-label="Sort shows">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="name">Name (A–Z)</SelectItem>
            <SelectItem value="episodes">Most episodes</SelectItem>
            <SelectItem value="latest">Recently aired</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Bulk lifecycle action bar */}
      {selected.size > 0 && (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-md border bg-muted/40 p-2 text-sm">
          <span className="font-medium">{selected.size} selected</span>
          <span className="text-muted-foreground">&mdash; set status:</span>
          {(['active', 'dormant', 'retired'] as const).map((state) => (
            <Button
              key={state}
              size="sm"
              variant="outline"
              disabled={bulkLifecycle.isPending}
              onClick={() => applyBulkLifecycle(state)}
            >
              {SHOW_LIFECYCLE_DISPLAY[state].label}
            </Button>
          ))}
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>Clear</Button>
          {bulkLifecycle.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
        </div>
      )}

      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : shows.length === 0 ? (
        <AdminEmptyState icon={Inbox} message="No shows for this station yet." />
      ) : filtered.length === 0 ? (
        <AdminEmptyState icon={Search} message="No shows match your search / filter." />
      ) : (
        <>
          <div className="rounded-lg border divide-y">
            <div className="flex items-center gap-2 px-4 py-2 text-xs text-muted-foreground">
              <Checkbox
                checked={allOnPageSelected}
                onCheckedChange={toggleSelectPage}
                aria-label="Select all shows on this page"
              />
              <span>Select page</span>
            </div>
            {pageItems.map((show) => (
              <div key={show.id} className="px-4 py-3 hover:bg-muted/50 transition-colors">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 flex-1 items-start gap-2">
                    <Checkbox
                      checked={selected.has(show.id)}
                      onCheckedChange={() => toggleSelected(show.id)}
                      aria-label={`Select ${show.name}`}
                      className="mt-1"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className={`font-medium ${show.episode_count === 0 ? 'text-muted-foreground' : ''}`}>
                          {show.name}
                        </span>
                        <ShowLifecycleBadge state={show.lifecycle_state} />
                        {show.schedule_locked && (
                          <Badge variant="outline" className="text-xs">
                            <Lock className="mr-1 h-3 w-3" />
                            Schedule locked
                          </Badge>
                        )}
                      </div>
                      <p className="text-sm text-muted-foreground">
                        {show.host_name ? `Hosted by ${show.host_name}` : 'No host'} &middot;{' '}
                        {show.episode_count === 0 ? (
                          <span className="italic">no episodes</span>
                        ) : (
                          <>{show.episode_count} episode(s)</>
                        )}
                        {show.latest_air_date && <> &middot; last aired {show.latest_air_date}</>}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    <Button variant="ghost" size="sm" aria-label={`Backfill episodes for ${show.name}`} onClick={() => onToggleExpand(show.id)}>
                      <Upload className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="sm" aria-label={`Edit ${show.name}`} onClick={() => onEditShow(show)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="sm" className="text-destructive" aria-label={`Delete ${show.name}`} onClick={() => onDeleteShow(show)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {expandedShows.has(show.id) && <ShowImportSection show={show} stationId={stationId} />}
              </div>
            ))}
          </div>

          {/* Pagination */}
          <div className="mt-3 flex items-center justify-between text-sm text-muted-foreground">
            <span>
              {safePage * SHOW_PAGE_SIZE + 1}&ndash;
              {Math.min((safePage + 1) * SHOW_PAGE_SIZE, filtered.length)} of {filtered.length}
            </span>
            {pageCount > 1 && (
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" disabled={safePage === 0} onClick={() => setPage(safePage - 1)}>Previous</Button>
                <span>Page {safePage + 1} of {pageCount}</span>
                <Button size="sm" variant="outline" disabled={safePage >= pageCount - 1} onClick={() => setPage(safePage + 1)}>Next</Button>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

function StationDetailPanel({
  station,
  onBack,
  onEdit,
  onDelete,
}: {
  station: RadioStationListItem
  onBack: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { data: stationDetail } = useRadioStationDetail(station.id)
  const { data: showsData, isLoading: showsLoading } = useRadioShows(station.id)
  const triggerMutation = useTriggerStationSync()
  const deleteShowMutation = useDeleteRadioShow()

  const [dialogMode, setDialogMode] = useState<'create-show' | 'edit-show' | 'delete-show' | null>(null)
  const [selectedShow, setSelectedShow] = useState<RadioShowListItem | null>(null)
  const [discoverError, setDiscoverError] = useState<string | null>(null)
  const [expandedShows, setExpandedShows] = useState<Set<number>>(new Set())

  const shows = showsData?.shows ?? []

  // Discover is now async (PSY-1135): the trigger returns a run handle; the shows
  // it discovers are created by the BACKGROUND run, so useTrackedSyncRun polls the
  // run and refreshes the shows list once it settles. (The old endpoint returned
  // the discovered names synchronously; a discover sync-run carries no name list,
  // so the new shows simply appear in the list below on completion.)
  const {
    run: discoverRun,
    isRunning: discoverRunning,
    isError: discoverPollError,
    trackRun: trackDiscoverRun,
  } = useTrackedSyncRun(station.id)

  const handleDiscoverShows = useCallback(() => {
    setDiscoverError(null)
    triggerMutation.mutate(
      { stationId: station.id, mode: 'discover' },
      {
        onSuccess: (run) => trackDiscoverRun(run),
        onError: (err) => setDiscoverError(err.message),
      }
    )
  }, [station.id, triggerMutation, trackDiscoverRun])

  const handleDeleteShow = useCallback(
    (show: RadioShowListItem) => {
      deleteShowMutation.mutate(
        { showId: show.id, stationId: station.id },
        { onSuccess: () => setDialogMode(null) }
      )
    },
    [deleteShowMutation, station.id]
  )

  const toggleShowExpanded = useCallback((showId: number) => {
    setExpandedShows((prev) => {
      const next = new Set(prev)
      if (next.has(showId)) {
        next.delete(showId)
      } else {
        next.add(showId)
      }
      return next
    })
  }, [])

  const lastFetch = stationDetail?.last_playlist_fetch_at
    ? new Date(stationDetail.last_playlist_fetch_at).toLocaleString()
    : 'Never'

  // Render the header from the freshly-fetched detail when it's available,
  // falling back to the list-row prop while it loads. The `station` prop is a
  // point-in-time snapshot from the stations list (captured at row-click); after
  // an edit, useUpdateRadioStation invalidates radioQueryKeys.stationDetail so
  // this query refetches and the header reflects server values (name, city,
  // show_count, is_active, …) without navigating away and back. (PSY-1121)
  const header = stationDetail ?? station

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ChevronLeft className="h-4 w-4 mr-1" /> Back
        </Button>
        <div className="flex-1">
          <h3 className="text-lg font-semibold">{header.name}</h3>
          <p className="text-sm text-muted-foreground">
            {header.city}{header.state ? `, ${header.state}` : ''} &middot; {header.broadcast_type}
            {header.frequency_mhz ? ` ${header.frequency_mhz} MHz` : ''} &middot; {header.show_count} show(s)
          </p>
        </div>
        <Badge variant={header.is_active ? 'default' : 'secondary'}>
          {header.is_active ? 'Active' : 'Inactive'}
        </Badge>
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil className="h-4 w-4 mr-1" /> Edit
        </Button>
        <Button variant="outline" size="sm" className="text-destructive" onClick={onDelete}>
          <Trash2 className="h-4 w-4 mr-1" /> Delete
        </Button>
      </div>

      {/* Station info */}
      {stationDetail && (
        <div className="grid grid-cols-2 gap-4 text-sm">
          <div>
            <span className="text-muted-foreground">Playlist Source:</span>{' '}
            {stationDetail.playlist_source ?? 'None'}
          </div>
          <div>
            <span className="text-muted-foreground">Last Fetch:</span> {lastFetch}
          </div>
          {stationDetail.website && (
            <div>
              <span className="text-muted-foreground">Website:</span>{' '}
              <a href={stationDetail.website} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                {stationDetail.website}
              </a>
            </div>
          )}
          {stationDetail.stream_url && (
            <div>
              <span className="text-muted-foreground">Stream:</span>{' '}
              <a href={stationDetail.stream_url} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                {stationDetail.stream_url}
              </a>
            </div>
          )}
        </div>
      )}

      {/* Station health card (PSY-1200) */}
      <StationHealthCard stationId={station.id} />

      {/* Discover Shows (async — poll the run, shows appear in the list below on
          completion) */}
      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <Button
            onClick={handleDiscoverShows}
            disabled={triggerMutation.isPending || discoverRunning}
            size="sm"
          >
            {triggerMutation.isPending || discoverRunning ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Radar className="mr-2 h-4 w-4" />
            )}
            Discover Shows
          </Button>
          {discoverError && (
            <span className="text-sm text-destructive">Discovery failed: {discoverError}</span>
          )}
          {discoverPollError && (
            <span className="text-sm text-destructive">
              Couldn&apos;t load the discover run status. Reload to try again.
            </span>
          )}
        </div>
        {discoverRun && !discoverPollError && <SyncRunRow run={discoverRun} />}
      </div>

      {/* Per-station sync-run feed (PSY-1130) */}
      <StationSyncRunFeed stationId={station.id} />

      {/* Shows — navigable list (PSY-1122) */}
      <StationShowList
        shows={shows}
        stationId={station.id}
        isLoading={showsLoading}
        expandedShows={expandedShows}
        onToggleExpand={toggleShowExpanded}
        onAddShow={() => setDialogMode('create-show')}
        onEditShow={(show) => { setSelectedShow(show); setDialogMode('edit-show') }}
        onDeleteShow={(show) => { setSelectedShow(show); setDialogMode('delete-show') }}
      />

      {/* Create Show Dialog */}
      <Dialog open={dialogMode === 'create-show'} onOpenChange={(open) => !open && setDialogMode(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Add Radio Show</DialogTitle>
            <DialogDescription>Create a new show for {station.name}.</DialogDescription>
          </DialogHeader>
          <CreateShowForm
            stationId={station.id}
            onSuccess={() => setDialogMode(null)}
            onCancel={() => setDialogMode(null)}
          />
        </DialogContent>
      </Dialog>

      {/* Edit Show Dialog */}
      <Dialog open={dialogMode === 'edit-show'} onOpenChange={(open) => !open && setDialogMode(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit Radio Show</DialogTitle>
            <DialogDescription>Update show details.</DialogDescription>
          </DialogHeader>
          {selectedShow && (
            <EditShowForm
              show={selectedShow}
              stationId={station.id}
              onSuccess={() => setDialogMode(null)}
              onCancel={() => setDialogMode(null)}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Delete Show Dialog */}
      <Dialog open={dialogMode === 'delete-show'} onOpenChange={(open) => !open && setDialogMode(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Radio Show</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{selectedShow?.name}&quot;? This will also delete all episodes and plays.
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogMode(null)}>Cancel</Button>
            <Button
              variant="destructive"
              disabled={deleteShowMutation.isPending}
              onClick={() => selectedShow && handleDeleteShow(selectedShow)}
            >
              {deleteShowMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete Show
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ============================================================================
// Matching Tab
// ============================================================================

function RadioMatchingTab() {
  const { data: stats, isLoading: statsLoading } = useRadioStats()
  const { data: stationsData } = useAdminRadioStations()
  const [stationFilter, setStationFilter] = useState(0)
  const [page, setPage] = useState(0)
  const PAGE_SIZE = 50

  const {
    data: unmatchedData,
    isLoading: unmatchedLoading,
    isFetching: unmatchedFetching,
  } = useUnmatchedPlays(stationFilter, PAGE_SIZE, page * PAGE_SIZE)

  const bulkLink = useBulkLinkPlays()

  // Track which group is being linked and to which artist
  const [linkingGroup, setLinkingGroup] = useState<string | null>(null)
  const [selectedArtists, setSelectedArtists] = useState<Record<string, number>>({})
  const [successMessages, setSuccessMessages] = useState<Record<string, string>>({})

  const stations = stationsData?.stations ?? []
  const groups = unmatchedData?.groups ?? []
  const total = unmatchedData?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  const totalPlays = stats?.total_plays ?? 0
  const matchedPlays = stats?.matched_plays ?? 0
  const unmatchedPlays = totalPlays - matchedPlays
  const matchRate = totalPlays > 0 ? ((matchedPlays / totalPlays) * 100).toFixed(1) : '0.0'

  const handleSelectArtist = useCallback((artistName: string, artistId: number) => {
    setSelectedArtists((prev) => ({ ...prev, [artistName]: artistId }))
  }, [])

  const handleBulkLink = useCallback(
    (artistName: string) => {
      const artistId = selectedArtists[artistName]
      if (!artistId) return

      setLinkingGroup(artistName)
      bulkLink.mutate(
        { artistName, artistId },
        {
          onSuccess: (data) => {
            setSuccessMessages((prev) => ({
              ...prev,
              [artistName]: `Linked ${data.updated} play${data.updated === 1 ? '' : 's'}`,
            }))
            setSelectedArtists((prev) => {
              const next = { ...prev }
              delete next[artistName]
              return next
            })
            setLinkingGroup(null)
            // Clear success message after 4 seconds
            setTimeout(() => {
              setSuccessMessages((prev) => {
                const next = { ...prev }
                delete next[artistName]
                return next
              })
            }, 4000)
          },
          onError: () => {
            setLinkingGroup(null)
          },
        }
      )
    },
    [selectedArtists, bulkLink]
  )

  // Reset page when station filter changes
  const handleStationFilterChange = useCallback((value: string) => {
    setStationFilter(Number(value))
    setPage(0)
  }, [])

  if (statsLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Stats Overview */}
      <div className="grid grid-cols-4 gap-4">
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Total Plays</div>
          <div className="text-2xl font-bold">{totalPlays.toLocaleString()}</div>
        </div>
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Matched</div>
          <div className="text-2xl font-bold text-green-600">{matchedPlays.toLocaleString()}</div>
        </div>
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Unmatched</div>
          <div className="text-2xl font-bold text-amber-600">{unmatchedPlays.toLocaleString()}</div>
        </div>
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Match Rate</div>
          <div className="text-2xl font-bold">{matchRate}%</div>
        </div>
      </div>

      {/* Unmatched Plays */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-semibold">Unmatched Plays</h3>
          <div className="flex items-center gap-2">
            <Label htmlFor="station-filter" className="text-sm text-muted-foreground">
              Station:
            </Label>
            <Select
              value={String(stationFilter)}
              onValueChange={handleStationFilterChange}
            >
              <SelectTrigger
                id="station-filter"
                className="w-44"
                aria-label="Filter by station"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {/* Station ids are positive integers, so "0" is a safe
                    non-empty Radix value for the "All Stations" option;
                    handleStationFilterChange maps it back to the numeric
                    0 the unmatched-plays query treats as "no filter". */}
                <SelectItem value="0">All Stations</SelectItem>
                {stations.map((s) => (
                  <SelectItem key={s.id} value={String(s.id)}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {unmatchedFetching && (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            )}
          </div>
        </div>

        {unmatchedLoading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : groups.length === 0 ? (
          <div className="rounded-lg border border-dashed p-12 text-center">
            <CheckCircle2 className="mx-auto mb-3 h-10 w-10 text-green-500 opacity-60" />
            <p className="text-muted-foreground">
              {stationFilter > 0
                ? 'No unmatched plays for this station.'
                : 'All plays are matched!'}
            </p>
          </div>
        ) : (
          <>
            <div className="rounded-lg border">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                      Artist Name
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                      Plays
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                      Stations
                    </th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                      Suggested Matches
                    </th>
                    <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">
                      Action
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {groups.map((group) => (
                    <UnmatchedPlayRow
                      key={group.artist_name}
                      group={group}
                      selectedArtistId={selectedArtists[group.artist_name]}
                      onSelectArtist={handleSelectArtist}
                      onBulkLink={handleBulkLink}
                      isLinking={linkingGroup === group.artist_name}
                      successMessage={successMessages[group.artist_name]}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between pt-2">
                <p className="text-sm text-muted-foreground">
                  Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of{' '}
                  {total} artist{total === 1 ? '' : 's'}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page === 0}
                    onClick={() => setPage((p) => p - 1)}
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {page + 1} of {totalPages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page >= totalPages - 1}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

// ============================================================================
// Unmatched Play Row
// ============================================================================

function UnmatchedPlayRow({
  group,
  selectedArtistId,
  onSelectArtist,
  onBulkLink,
  isLinking,
  successMessage,
}: {
  group: UnmatchedPlayGroup
  selectedArtistId: number | undefined
  onSelectArtist: (artistName: string, artistId: number) => void
  onBulkLink: (artistName: string) => void
  isLinking: boolean
  successMessage: string | undefined
}) {
  const suggestions = group.suggested_matches ?? []

  return (
    <tr className="hover:bg-muted/30 transition-colors">
      <td className="px-4 py-3">
        <span className="font-medium">{group.artist_name}</span>
      </td>
      <td className="px-4 py-3">
        <Badge variant="secondary">{group.play_count}</Badge>
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {group.station_names.map((name) => (
            <Badge key={name} variant="outline" className="text-xs">
              {name}
            </Badge>
          ))}
        </div>
      </td>
      <td className="px-4 py-3">
        {suggestions.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {suggestions.map((match) => (
              <Button
                key={match.artist_id}
                variant={selectedArtistId === match.artist_id ? 'default' : 'outline'}
                size="sm"
                className="text-xs h-7"
                onClick={() => onSelectArtist(group.artist_name, match.artist_id)}
                disabled={isLinking}
              >
                {match.artist_name}
              </Button>
            ))}
          </div>
        ) : (
          <span className="text-sm text-muted-foreground italic">No suggestions</span>
        )}
      </td>
      <td className="px-4 py-3 text-right">
        {successMessage ? (
          <span className="inline-flex items-center gap-1 text-sm text-green-600">
            <CheckCircle2 className="h-4 w-4" />
            {successMessage}
          </span>
        ) : (
          <Button
            size="sm"
            disabled={!selectedArtistId || isLinking}
            onClick={() => onBulkLink(group.artist_name)}
          >
            {isLinking ? (
              <Loader2 className="mr-1 h-3 w-3 animate-spin" />
            ) : (
              <Link2 className="mr-1 h-3 w-3" />
            )}
            Link
          </Button>
        )}
      </td>
    </tr>
  )
}

// ============================================================================
// Main Component
// ============================================================================

export function RadioManagement() {
  const { data: stationsData, isLoading } = useAdminRadioStations()
  const deleteMutation = useDeleteRadioStation()

  const [searchQuery, setSearchQuery] = useState('')
  const [dialogMode, setDialogMode] = useState<DialogMode>(null)
  const [selectedStation, setSelectedStation] = useState<RadioStationListItem | null>(null)
  const [detailStation, setDetailStation] = useState<RadioStationListItem | null>(null)

  const stations = useMemo(() => stationsData?.stations ?? [], [stationsData])

  // Bulk station health for the at-a-glance Health column (PSY-1200), keyed by id.
  const { data: healthData } = useListStationHealth()
  const healthByStation = useMemo(() => {
    const map = new Map<number, RadioStationHealth>()
    for (const h of healthData?.stations ?? []) map.set(h.station_id, h)
    return map
  }, [healthData])

  const filteredStations = stations.filter((s) =>
    s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (s.city ?? '').toLowerCase().includes(searchQuery.toLowerCase())
  )

  const handleStationClick = useCallback((station: RadioStationListItem) => {
    setDetailStation(station)
  }, [])

  // Open a station's detail from the global failures panel (which carries a run's
  // station_id, not the full row). No-op if the station isn't in the loaded list.
  const handleOpenStationById = useCallback(
    (stationId: number) => {
      const station = stations.find((s) => s.id === stationId)
      if (station) setDetailStation(station)
    },
    [stations]
  )

  const handleDeleteStation = useCallback(
    (station: RadioStationListItem) => {
      deleteMutation.mutate(station.id, {
        onSuccess: () => {
          setDialogMode(null)
          setDetailStation(null)
        },
      })
    },
    [deleteMutation]
  )

  const stationColumns: AdminTableColumn<RadioStationListItem>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (s) => (
        <div className="flex items-center gap-2">
          <Radio className="h-4 w-4 text-muted-foreground" />
          <span className="font-medium">{s.name}</span>
          {s.frequency_mhz && (
            <Badge variant="outline" className="text-xs">
              {s.frequency_mhz} MHz
            </Badge>
          )}
        </div>
      ),
    },
    {
      key: 'city',
      header: 'City',
      cellClassName: 'text-muted-foreground',
      render: (s) => `${s.city}${s.state ? `, ${s.state}` : ''}`,
    },
    {
      key: 'broadcast',
      header: 'Broadcast Type',
      cellClassName: 'text-muted-foreground capitalize',
      render: (s) => s.broadcast_type,
    },
    { key: 'shows', header: 'Shows', render: (s) => s.show_count },
    {
      key: 'active',
      header: 'Active',
      render: (s) => (
        <Badge variant={s.is_active ? 'default' : 'secondary'} className="text-xs">
          {s.is_active ? 'Active' : 'Inactive'}
        </Badge>
      ),
    },
    {
      key: 'health',
      header: 'Health',
      render: (s) => <StationHealthBadge health={healthByStation.get(s.id)} />,
    },
  ]

  // If viewing station detail, render that panel
  if (detailStation) {
    return (
      <Tabs defaultValue="stations" className="w-full">
        <TabsList className="mb-4">
          <TabsTrigger value="stations" className="gap-2">
            <Radio className="h-4 w-4" />
            Stations
          </TabsTrigger>
          <TabsTrigger value="matching" className="gap-2">
            <BarChart3 className="h-4 w-4" />
            Matching
          </TabsTrigger>
        </TabsList>

        <TabsContent value="stations">
          <StationDetailPanel
            station={detailStation}
            onBack={() => setDetailStation(null)}
            onEdit={() => {
              setSelectedStation(detailStation)
              setDialogMode('edit-station')
            }}
            onDelete={() => {
              setSelectedStation(detailStation)
              setDialogMode('delete-station')
            }}
          />

          {/* Edit Station — right-anchored Sheet (PSY-930 AdminFormLayout) */}
          {selectedStation && (
            <EditStationFormWrapper
              stationId={selectedStation.id}
              open={dialogMode === 'edit-station'}
              onOpenChange={(open) => !open && setDialogMode(null)}
              onSuccess={() => {
                setDialogMode(null)
                // No manual refresh needed: useUpdateRadioStation invalidates
                // radioQueryKeys.stationDetail + .stations, so the detail panel's
                // useRadioStationDetail refetches and the header re-renders from
                // the fresh server values. The old setDetailStation({ ...detailStation })
                // was a no-op shallow copy of the same stale list snapshot. (PSY-1121)
              }}
            />
          )}

          {/* Delete Station — centered Modal (PSY-930, first modal-variant consumer) */}
          <AdminFormLayout
            variant="modal"
            open={dialogMode === 'delete-station'}
            onOpenChange={(open) => !open && setDialogMode(null)}
            title="Delete Station"
            description="This action is permanent and cannot be undone."
            onSubmit={(e) => {
              e.preventDefault()
              if (selectedStation) handleDeleteStation(selectedStation)
            }}
            footer={
              <>
                <Button type="button" variant="outline" onClick={() => setDialogMode(null)}>
                  Cancel
                </Button>
                <Button type="submit" variant="destructive" disabled={deleteMutation.isPending}>
                  {deleteMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Delete Station
                </Button>
              </>
            }
          >
            <p className="text-sm text-muted-foreground">
              Are you sure you want to delete &quot;{selectedStation?.name}&quot;? This will
              also delete all shows, episodes, and plays. This action cannot be undone.
            </p>
          </AdminFormLayout>
        </TabsContent>

        <TabsContent value="matching">
          <RadioMatchingTab />
        </TabsContent>
      </Tabs>
    )
  }

  // Station list view
  return (
    <Tabs defaultValue="stations" className="w-full">
      <TabsList className="mb-4">
        <TabsTrigger value="stations" className="gap-2">
          <Radio className="h-4 w-4" />
          Stations
        </TabsTrigger>
        <TabsTrigger value="matching" className="gap-2">
          <BarChart3 className="h-4 w-4" />
          Matching
        </TabsTrigger>
      </TabsList>

      <TabsContent value="stations">
        <div className="space-y-4">
          {/* Header */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3 flex-1">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search stations..."
                  className="pl-10"
                />
                {searchQuery && (
                  <button
                    onClick={() => setSearchQuery('')}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-4 w-4" />
                  </button>
                )}
              </div>
            </div>
            <Button onClick={() => setDialogMode('create-station')}>
              <Plus className="mr-2 h-4 w-4" /> Add Station
            </Button>
          </div>

          {/* Global recent-failures feed (PSY-1130) */}
          <RecentFailuresPanel onOpenStation={handleOpenStationById} />

          {/* Station Table */}
          {isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : filteredStations.length === 0 ? (
            <AdminEmptyState
              icon={Inbox}
              message={
                searchQuery
                  ? 'No stations match your search.'
                  : 'No radio stations yet. Add one to get started.'
              }
            />
          ) : (
            <AdminTable
              columns={stationColumns}
              rows={filteredStations}
              rowKey={(s) => s.id}
              onRowClick={handleStationClick}
              rowLabel={(s) => `Station: ${s.name}`}
            />
          )}
        </div>

        {/* Create Station — right-anchored Sheet (PSY-911 AdminFormLayout) */}
        <CreateStationForm
          open={dialogMode === 'create-station'}
          onOpenChange={(open) => !open && setDialogMode(null)}
          onSuccess={() => setDialogMode(null)}
        />
      </TabsContent>

      <TabsContent value="matching">
        <RadioMatchingTab />
      </TabsContent>
    </Tabs>
  )
}

// ============================================================================
// Edit Station Form Wrapper (loads station detail)
// ============================================================================

// Per the PSY-930 decision the Edit Station Sheet opens immediately on click:
// while the station detail loads this wrapper renders an AdminFormLayout (open)
// with a spinner body — `open` stays true throughout, so the Sheet stays open
// — then swaps to EditStationFormFields (keyed on station.id) once the detail
// resolves, initializing the fields from it.
function EditStationFormWrapper({
  stationId,
  open,
  onOpenChange,
  onSuccess,
}: {
  stationId: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  const { data: station, isLoading } = useRadioStationDetail(stationId)

  if (isLoading || !station) {
    return (
      <AdminFormLayout
        variant="sheet"
        open={open}
        onOpenChange={onOpenChange}
        title="Edit Station"
        description="Update station details."
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
        <div className="flex justify-center py-8">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </AdminFormLayout>
    )
  }

  return (
    <EditStationFormFields
      key={station.id}
      station={station}
      open={open}
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  )
}
