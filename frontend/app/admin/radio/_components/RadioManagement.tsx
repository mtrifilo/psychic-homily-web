'use client'

import { useState, useCallback } from 'react'
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
  Clock,
  PlayCircle,
  XCircle,
  CheckCircle2,
  History,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
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
  useFetchPlaylists,
  useDiscoverShows,
  useImportShowEpisodes,
  useCreateImportJob,
  useImportJob,
  useCancelImportJob,
  useShowImportJobs,
  type RadioStationListItem,
  type RadioStationDetail,
  type RadioShowListItem,
  type RadioDiscoverResult,
  type RadioImportResult,
  type RadioImportJob,
  type CreateRadioStationInput,
  type UpdateRadioStationInput,
  type CreateRadioShowInput,
  type UpdateRadioShowInput,
} from '@/lib/hooks/admin/useAdminRadio'

// ============================================================================
// Constants
// ============================================================================

const BROADCAST_TYPES = [
  { value: 'terrestrial', label: 'Terrestrial' },
  { value: 'internet', label: 'Internet' },
  { value: 'both', label: 'Both' },
] as const

const PLAYLIST_SOURCES = [
  { value: 'kexp_api', label: 'KEXP API' },
  { value: 'wfmu_scrape', label: 'WFMU Scrape' },
  { value: 'nts_api', label: 'NTS API' },
  { value: 'manual', label: 'Manual' },
] as const

type DialogMode = 'create-station' | 'edit-station' | 'delete-station' | 'create-show' | 'edit-show' | 'delete-show' | null

// ============================================================================
// Create Station Form
// ============================================================================

function CreateStationForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: () => void
  onCancel: () => void
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
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</div>
      )}

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="station-name">Name *</Label>
          <Input id="station-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="KEXP" />
        </div>
        <div>
          <Label htmlFor="station-slug">Slug (auto if empty)</Label>
          <Input id="station-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="kexp" />
        </div>
      </div>

      <div>
        <Label htmlFor="station-description">Description</Label>
        <Textarea id="station-description" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Station description..." rows={2} />
      </div>

      <div className="grid grid-cols-4 gap-4">
        <div>
          <Label htmlFor="station-city">City</Label>
          <Input id="station-city" value={city} onChange={(e) => setCity(e.target.value)} placeholder="Seattle" />
        </div>
        <div>
          <Label htmlFor="station-state">State</Label>
          <Input id="station-state" value={state} onChange={(e) => setState(e.target.value)} placeholder="WA" />
        </div>
        <div>
          <Label htmlFor="station-country">Country</Label>
          <Input id="station-country" value={country} onChange={(e) => setCountry(e.target.value)} placeholder="US" />
        </div>
        <div>
          <Label htmlFor="station-timezone">Timezone</Label>
          <Input id="station-timezone" value={timezone} onChange={(e) => setTimezone(e.target.value)} placeholder="America/Los_Angeles" />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="station-broadcast-type">Broadcast Type *</Label>
          <select
            id="station-broadcast-type"
            value={broadcastType}
            onChange={(e) => setBroadcastType(e.target.value)}
            className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          >
            {BROADCAST_TYPES.map((bt) => (
              <option key={bt.value} value={bt.value}>{bt.label}</option>
            ))}
          </select>
        </div>
        <div>
          <Label htmlFor="station-frequency">Frequency (MHz)</Label>
          <Input id="station-frequency" type="number" step="0.1" value={frequencyMHz} onChange={(e) => setFrequencyMHz(e.target.value)} placeholder="90.3" />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="station-stream-url">Stream URL</Label>
          <Input id="station-stream-url" value={streamUrl} onChange={(e) => setStreamUrl(e.target.value)} placeholder="https://..." />
        </div>
        <div>
          <Label htmlFor="station-website">Website</Label>
          <Input id="station-website" value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="https://..." />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="station-donation-url">Donation URL</Label>
          <Input id="station-donation-url" value={donationUrl} onChange={(e) => setDonationUrl(e.target.value)} placeholder="https://..." />
        </div>
        <div>
          <Label htmlFor="station-logo-url">Logo URL</Label>
          <Input id="station-logo-url" value={logoUrl} onChange={(e) => setLogoUrl(e.target.value)} placeholder="https://..." />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="station-playlist-source">Playlist Source</Label>
          <select
            id="station-playlist-source"
            value={playlistSource}
            onChange={(e) => setPlaylistSource(e.target.value)}
            className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          >
            <option value="">None</option>
            {PLAYLIST_SOURCES.map((ps) => (
              <option key={ps.value} value={ps.value}>{ps.label}</option>
            ))}
          </select>
        </div>
        <div>
          <Label htmlFor="station-playlist-config">Playlist Config (JSON)</Label>
          <Input id="station-playlist-config" value={playlistConfigJson} onChange={(e) => setPlaylistConfigJson(e.target.value)} placeholder='{"api_key": "..."}' />
        </div>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
        <Button type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Create Station
        </Button>
      </DialogFooter>
    </form>
  )
}

// ============================================================================
// Edit Station Form
// ============================================================================

function EditStationForm({
  station,
  onSuccess,
  onCancel,
}: {
  station: RadioStationDetail
  onSuccess: () => void
  onCancel: () => void
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
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</div>
      )}

      <div>
        <Label htmlFor="edit-station-name">Name *</Label>
        <Input id="edit-station-name" value={name} onChange={(e) => setName(e.target.value)} />
      </div>

      <div>
        <Label htmlFor="edit-station-description">Description</Label>
        <Textarea id="edit-station-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </div>

      <div className="grid grid-cols-4 gap-4">
        <div>
          <Label htmlFor="edit-station-city">City</Label>
          <Input id="edit-station-city" value={city} onChange={(e) => setCity(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-station-state">State</Label>
          <Input id="edit-station-state" value={state} onChange={(e) => setState(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-station-country">Country</Label>
          <Input id="edit-station-country" value={country} onChange={(e) => setCountry(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-station-timezone">Timezone</Label>
          <Input id="edit-station-timezone" value={timezone} onChange={(e) => setTimezone(e.target.value)} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-station-broadcast-type">Broadcast Type</Label>
          <select
            id="edit-station-broadcast-type"
            value={broadcastType}
            onChange={(e) => setBroadcastType(e.target.value)}
            className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          >
            {BROADCAST_TYPES.map((bt) => (
              <option key={bt.value} value={bt.value}>{bt.label}</option>
            ))}
          </select>
        </div>
        <div>
          <Label htmlFor="edit-station-frequency">Frequency (MHz)</Label>
          <Input id="edit-station-frequency" type="number" step="0.1" value={frequencyMHz} onChange={(e) => setFrequencyMHz(e.target.value)} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-station-stream-url">Stream URL</Label>
          <Input id="edit-station-stream-url" value={streamUrl} onChange={(e) => setStreamUrl(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-station-website">Website</Label>
          <Input id="edit-station-website" value={website} onChange={(e) => setWebsite(e.target.value)} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-station-donation-url">Donation URL</Label>
          <Input id="edit-station-donation-url" value={donationUrl} onChange={(e) => setDonationUrl(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="edit-station-logo-url">Logo URL</Label>
          <Input id="edit-station-logo-url" value={logoUrl} onChange={(e) => setLogoUrl(e.target.value)} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label htmlFor="edit-station-playlist-source">Playlist Source</Label>
          <select
            id="edit-station-playlist-source"
            value={playlistSource}
            onChange={(e) => setPlaylistSource(e.target.value)}
            className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          >
            <option value="">None</option>
            {PLAYLIST_SOURCES.map((ps) => (
              <option key={ps.value} value={ps.value}>{ps.label}</option>
            ))}
          </select>
        </div>
        <div>
          <Label htmlFor="edit-station-playlist-config">Playlist Config (JSON)</Label>
          <Input id="edit-station-playlist-config" value={playlistConfigJson} onChange={(e) => setPlaylistConfigJson(e.target.value)} />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch id="edit-station-active" checked={isActive} onCheckedChange={setIsActive} />
        <Label htmlFor="edit-station-active">Active</Label>
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
      }

      updateMutation.mutate(input, {
        onSuccess: () => onSuccess(),
        onError: (err) => setError(err.message),
      })
    },
    [name, hostName, description, scheduleDisplay, archiveUrl, imageUrl, isActive, show.id, stationId, updateMutation, onSuccess]
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
// Per-Show Import Controls
// ============================================================================

function ShowImportControls({ show }: { show: RadioShowListItem }) {
  const importMutation = useImportShowEpisodes()
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const [importResult, setImportResult] = useState<RadioImportResult | null>(null)
  const [importError, setImportError] = useState<string | null>(null)

  const handleImport = useCallback(() => {
    if (!since || !until) return
    setImportResult(null)
    setImportError(null)
    importMutation.mutate(
      { showId: show.id, since, until },
      {
        onSuccess: (result) => {
          setImportResult(result)
        },
        onError: (err) => {
          setImportError(err.message)
        },
      }
    )
  }, [show.id, since, until, importMutation])

  return (
    <div className="mt-2 space-y-2">
      <div className="flex items-end gap-2">
        <div>
          <Label className="text-xs text-muted-foreground">Since</Label>
          <Input
            type="date"
            value={since}
            onChange={(e) => setSince(e.target.value)}
            className="h-8 text-xs w-36"
          />
        </div>
        <div>
          <Label className="text-xs text-muted-foreground">Until</Label>
          <Input
            type="date"
            value={until}
            onChange={(e) => setUntil(e.target.value)}
            className="h-8 text-xs w-36"
          />
        </div>
        <Button
          size="sm"
          variant="outline"
          onClick={handleImport}
          disabled={importMutation.isPending || !since || !until}
          className="h-8"
        >
          {importMutation.isPending ? (
            <Loader2 className="mr-1 h-3 w-3 animate-spin" />
          ) : (
            <Upload className="mr-1 h-3 w-3" />
          )}
          Import Episodes
        </Button>
      </div>
      {importResult && (
        <div className="text-xs rounded-md bg-muted p-2 space-y-0.5">
          <p>Episodes imported: <strong>{importResult.episodes_imported}</strong></p>
          <p>Plays imported: <strong>{importResult.plays_imported}</strong></p>
          <p>Plays matched: <strong>{importResult.plays_matched}</strong></p>
          {importResult.errors && importResult.errors.length > 0 && (
            <div className="mt-1 text-destructive">
              <p className="font-medium">Errors:</p>
              {importResult.errors.map((e, i) => (
                <p key={i}>{e}</p>
              ))}
            </div>
          )}
        </div>
      )}
      {importError && (
        <p className="text-xs text-destructive">{importError}</p>
      )}
    </div>
  )
}

// ============================================================================
// Import Job Progress Row
// ============================================================================

function ImportJobRow({ job }: { job: RadioImportJob }) {
  const cancelMutation = useCancelImportJob()

  const isActive = job.status === 'running' || job.status === 'pending'
  const progress = job.episodes_found > 0
    ? Math.round((job.episodes_imported / job.episodes_found) * 100)
    : 0

  const statusIcon = {
    pending: <Clock className="h-4 w-4 text-muted-foreground" />,
    running: <Loader2 className="h-4 w-4 animate-spin text-blue-500" />,
    completed: <CheckCircle2 className="h-4 w-4 text-green-500" />,
    failed: <AlertCircle className="h-4 w-4 text-destructive" />,
    cancelled: <XCircle className="h-4 w-4 text-muted-foreground" />,
  }[job.status]

  const statusColor = {
    pending: 'bg-muted text-muted-foreground',
    running: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
    completed: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
    failed: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
    cancelled: 'bg-muted text-muted-foreground',
  }[job.status]

  return (
    <div className="rounded-lg border p-4 space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {statusIcon}
          <Badge className={statusColor}>{job.status}</Badge>
          <span className="text-sm text-muted-foreground">
            {job.since} to {job.until}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {job.started_at && (
            <span className="text-xs text-muted-foreground">
              Started {new Date(job.started_at).toLocaleString()}
            </span>
          )}
          {isActive && (
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              disabled={cancelMutation.isPending}
              onClick={() => cancelMutation.mutate(job.id)}
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

      {/* Progress bar for running/completed jobs */}
      {(job.status === 'running' || job.status === 'completed') && job.episodes_found > 0 && (
        <div className="space-y-1">
          <div className="h-2 rounded-full bg-muted overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${
                job.status === 'completed' ? 'bg-green-500' : 'bg-blue-500'
              }`}
              style={{ width: `${progress}%` }}
            />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {job.episodes_imported.toLocaleString()} / {job.episodes_found.toLocaleString()} episodes
              {job.current_episode_date && job.status === 'running' && (
                <> &mdash; processing {job.current_episode_date}</>
              )}
            </span>
            <span>
              {job.plays_imported.toLocaleString()} plays &mdash;{' '}
              {job.plays_imported > 0
                ? Math.round((job.plays_matched / job.plays_imported) * 100)
                : 0}% matched
            </span>
          </div>
        </div>
      )}

      {/* Error log for failed jobs */}
      {job.status === 'failed' && job.error_log && (
        <div className="rounded-md bg-destructive/10 p-2 text-xs text-destructive whitespace-pre-wrap max-h-24 overflow-y-auto">
          {job.error_log}
        </div>
      )}

      {/* Completed summary */}
      {job.status === 'completed' && (
        <div className="text-xs text-muted-foreground">
          Completed {job.completed_at ? new Date(job.completed_at).toLocaleString() : ''} &mdash;{' '}
          {job.episodes_imported.toLocaleString()} episodes, {job.plays_imported.toLocaleString()} plays, {job.plays_matched.toLocaleString()} matched
        </div>
      )}
    </div>
  )
}

// ============================================================================
// Show Import Section (per-show import history + active job tracking)
// ============================================================================

function ShowImportSection({
  show,
  stationId,
}: {
  show: RadioShowListItem
  stationId: number
}) {
  const { data: jobsData, isLoading } = useShowImportJobs(show.id)
  const createMutation = useCreateImportJob()
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const [error, setError] = useState<string | null>(null)

  const jobs = jobsData?.jobs ?? []
  const hasActiveJob = jobs.some(j => j.status === 'running' || j.status === 'pending')

  // Track the most recent active job for live polling
  const activeJob = jobs.find(j => j.status === 'running' || j.status === 'pending')
  const { data: liveJob } = useImportJob(activeJob?.id ?? 0, !!activeJob)

  const handleCreate = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      setError(null)

      if (!since) { setError('Start date is required'); return }
      if (!until) { setError('End date is required'); return }
      if (since > until) { setError('Start date must be before end date'); return }

      createMutation.mutate(
        { showId: show.id, since, until },
        {
          onSuccess: () => {
            setShowCreateForm(false)
            setSince('')
            setUntil('')
          },
          onError: (err) => setError(err.message),
        }
      )
    },
    [since, until, show.id, createMutation]
  )

  return (
    <div className="mt-3 space-y-3">
      {/* Active job with live progress */}
      {liveJob && (liveJob.status === 'running' || liveJob.status === 'pending') && (
        <ImportJobRow job={liveJob} />
      )}

      {/* Create import form */}
      {showCreateForm ? (
        <form onSubmit={handleCreate} className="rounded-lg border border-dashed p-4 space-y-3">
          {error && (
            <div className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">{error}</div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor={`since-${show.id}`}>From</Label>
              <Input
                id={`since-${show.id}`}
                type="date"
                value={since}
                onChange={(e) => setSince(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor={`until-${show.id}`}>To</Label>
              <Input
                id={`until-${show.id}`}
                type="date"
                value={until}
                onChange={(e) => setUntil(e.target.value)}
              />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" size="sm" disabled={createMutation.isPending}>
              {createMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin mr-1" />
              ) : (
                <PlayCircle className="h-4 w-4 mr-1" />
              )}
              Start Import
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
          disabled={hasActiveJob}
        >
          <Download className="h-4 w-4 mr-1" />
          {hasActiveJob ? 'Import Running...' : 'Import Episodes'}
        </Button>
      )}

      {/* Job history (non-active) */}
      {!isLoading && jobs.filter(j => j.status !== 'running' && j.status !== 'pending').length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground hover:text-foreground flex items-center gap-1">
            <History className="h-3 w-3" />
            Import History ({jobs.filter(j => j.status !== 'running' && j.status !== 'pending').length})
          </summary>
          <div className="mt-2 space-y-2">
            {jobs
              .filter(j => j.status !== 'running' && j.status !== 'pending')
              .map(job => (
                <ImportJobRow key={job.id} job={job} />
              ))
            }
          </div>
        </details>
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
  const discoverMutation = useDiscoverShows()
  const deleteShowMutation = useDeleteRadioShow()

  const [dialogMode, setDialogMode] = useState<'create-show' | 'edit-show' | 'delete-show' | null>(null)
  const [selectedShow, setSelectedShow] = useState<RadioShowListItem | null>(null)
  const [discoverResult, setDiscoverResult] = useState<RadioDiscoverResult | null>(null)
  const [discoverError, setDiscoverError] = useState<string | null>(null)
  const [expandedShows, setExpandedShows] = useState<Set<number>>(new Set())

  const shows = showsData?.shows ?? []

  const handleDiscoverShows = useCallback(() => {
    setDiscoverResult(null)
    setDiscoverError(null)
    discoverMutation.mutate(station.id, {
      onSuccess: (result) => {
        setDiscoverResult(result)
      },
      onError: (err) => {
        setDiscoverError(err.message)
      },
    })
  }, [station.id, discoverMutation])

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

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ChevronLeft className="h-4 w-4 mr-1" /> Back
        </Button>
        <div className="flex-1">
          <h3 className="text-lg font-semibold">{station.name}</h3>
          <p className="text-sm text-muted-foreground">
            {station.city}{station.state ? `, ${station.state}` : ''} &middot; {station.broadcast_type}
            {station.frequency_mhz ? ` ${station.frequency_mhz} MHz` : ''} &middot; {station.show_count} show(s)
          </p>
        </div>
        <Badge variant={station.is_active ? 'default' : 'secondary'}>
          {station.is_active ? 'Active' : 'Inactive'}
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

      {/* Discover Shows */}
      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <Button onClick={handleDiscoverShows} disabled={discoverMutation.isPending} size="sm">
            {discoverMutation.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Radar className="mr-2 h-4 w-4" />
            )}
            Discover Shows
          </Button>
          {discoverResult && (
            <span className="text-sm text-muted-foreground">
              Discovered {discoverResult.shows_discovered} show(s)
              {discoverResult.show_names.length > 0 && `: ${discoverResult.show_names.join(', ')}`}
            </span>
          )}
          {discoverError && (
            <span className="text-sm text-destructive">Discovery failed: {discoverError}</span>
          )}
        </div>
        {discoverResult?.errors && discoverResult.errors.length > 0 && (
          <div className="text-xs text-destructive space-y-0.5">
            {discoverResult.errors.map((e, i) => (
              <p key={i}>{e}</p>
            ))}
          </div>
        )}
      </div>

      {/* Shows */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h4 className="font-medium">Shows</h4>
          <Button size="sm" onClick={() => setDialogMode('create-show')}>
            <Plus className="mr-1 h-4 w-4" /> Add Show
          </Button>
        </div>

        {showsLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : shows.length === 0 ? (
          <div className="rounded-lg border border-dashed p-6 text-center text-muted-foreground text-sm">
            <Inbox className="mx-auto mb-2 h-8 w-8 opacity-50" />
            No shows for this station yet.
          </div>
        ) : (
          <div className="rounded-lg border divide-y">
            {shows.map((show) => (
              <div key={show.id} className="px-4 py-3">
                <div className="flex items-center justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{show.name}</span>
                      <Badge variant={show.is_active ? 'default' : 'secondary'} className="text-xs">
                        {show.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {show.host_name ? `Hosted by ${show.host_name}` : 'No host'} &middot; {show.episode_count} episode(s)
                    </p>
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      title="Import episodes"
                      onClick={() => toggleShowExpanded(show.id)}
                    >
                      <Upload className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => { setSelectedShow(show); setDialogMode('edit-show') }}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive"
                      onClick={() => { setSelectedShow(show); setDialogMode('delete-show') }}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {expandedShows.has(show.id) && (
                  <>
                    <ShowImportControls show={show} />
                    <ShowImportSection show={show} stationId={station.id} />
                  </>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

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
  const { data: stats, isLoading } = useRadioStats()

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const totalPlays = stats?.total_plays ?? 0
  const matchedPlays = stats?.matched_plays ?? 0
  const unmatchedPlays = totalPlays - matchedPlays
  const matchRate = totalPlays > 0 ? ((matchedPlays / totalPlays) * 100).toFixed(1) : '0.0'

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

      {/* Additional stats */}
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Stations</div>
          <div className="text-xl font-bold">{stats?.total_stations ?? 0}</div>
        </div>
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Shows</div>
          <div className="text-xl font-bold">{stats?.total_shows ?? 0}</div>
        </div>
        <div className="rounded-lg border p-4">
          <div className="text-sm text-muted-foreground">Episodes</div>
          <div className="text-xl font-bold">{stats?.total_episodes ?? 0}</div>
        </div>
      </div>

      {/* Unmatched Plays Stub */}
      <div>
        <h3 className="text-lg font-semibold mb-3">Unmatched Plays</h3>
        <div className="rounded-lg border border-dashed p-6 text-center">
          <AlertCircle className="mx-auto mb-3 h-10 w-10 text-muted-foreground opacity-50" />
          <p className="text-sm text-muted-foreground mb-4">
            {/* TODO: A dedicated unmatched plays endpoint (grouped by artist_name with play counts)
                will be needed for full functionality. For now, showing aggregate stats from /radio/stats. */}
            Unmatched plays will be listed here grouped by artist name once a dedicated
            endpoint is available. Currently {unmatchedPlays.toLocaleString()} {unmatchedPlays === 1 ? 'play is' : 'plays are'} unmatched.
          </p>
        </div>
      </div>

      {/* Matching Actions Stub */}
      <div>
        <h3 className="text-lg font-semibold mb-3">Matching Actions</h3>
        <p className="text-sm text-muted-foreground mb-4">
          When unmatched plays are listed, you will be able to:
        </p>
        <div className="grid grid-cols-3 gap-4">
          <div className="rounded-lg border border-dashed p-4 text-center opacity-60">
            <Link2 className="mx-auto mb-2 h-6 w-6 text-muted-foreground" />
            <p className="text-sm font-medium">Link to Artist</p>
            <p className="text-xs text-muted-foreground mt-1">Search existing artists and link all plays by this artist name</p>
          </div>
          <div className="rounded-lg border border-dashed p-4 text-center opacity-60">
            <UserPlus className="mx-auto mb-2 h-6 w-6 text-muted-foreground" />
            <p className="text-sm font-medium">Create Artist</p>
            <p className="text-xs text-muted-foreground mt-1">Create a new artist and link all plays</p>
          </div>
          <div className="rounded-lg border border-dashed p-4 text-center opacity-60">
            <SkipForward className="mx-auto mb-2 h-6 w-6 text-muted-foreground" />
            <p className="text-sm font-medium">Skip</p>
            <p className="text-xs text-muted-foreground mt-1">Mark as intentionally unlinked (e.g., spoken word, station IDs)</p>
          </div>
        </div>
      </div>
    </div>
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

  const stations = stationsData?.stations ?? []
  const filteredStations = stations.filter((s) =>
    s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (s.city ?? '').toLowerCase().includes(searchQuery.toLowerCase())
  )

  const handleStationClick = useCallback((station: RadioStationListItem) => {
    setDetailStation(station)
  }, [])

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

          {/* Edit Station Dialog */}
          <Dialog open={dialogMode === 'edit-station'} onOpenChange={(open) => !open && setDialogMode(null)}>
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle>Edit Station</DialogTitle>
                <DialogDescription>Update station details.</DialogDescription>
              </DialogHeader>
              {selectedStation && (
                <EditStationFormWrapper
                  stationId={selectedStation.id}
                  onSuccess={() => {
                    setDialogMode(null)
                    // Refresh detail view
                    setDetailStation({ ...detailStation })
                  }}
                  onCancel={() => setDialogMode(null)}
                />
              )}
            </DialogContent>
          </Dialog>

          {/* Delete Station Dialog */}
          <Dialog open={dialogMode === 'delete-station'} onOpenChange={(open) => !open && setDialogMode(null)}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Delete Station</DialogTitle>
                <DialogDescription>
                  Are you sure you want to delete &quot;{selectedStation?.name}&quot;? This will also delete all shows, episodes, and plays.
                  This action cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDialogMode(null)}>Cancel</Button>
                <Button
                  variant="destructive"
                  disabled={deleteMutation.isPending}
                  onClick={() => selectedStation && handleDeleteStation(selectedStation)}
                >
                  {deleteMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Delete Station
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
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

          {/* Station Table */}
          {isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : filteredStations.length === 0 ? (
            <div className="rounded-lg border border-dashed p-12 text-center">
              <Inbox className="mx-auto mb-3 h-10 w-10 text-muted-foreground opacity-50" />
              <p className="text-muted-foreground">
                {searchQuery ? 'No stations match your search.' : 'No radio stations yet. Add one to get started.'}
              </p>
            </div>
          ) : (
            <div className="rounded-lg border">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Name</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">City</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Source</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Shows</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Active</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {filteredStations.map((station) => (
                    <tr
                      key={station.id}
                      className="cursor-pointer hover:bg-muted/50 transition-colors"
                      onClick={() => handleStationClick(station)}
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Radio className="h-4 w-4 text-muted-foreground" />
                          <span className="font-medium">{station.name}</span>
                          {station.frequency_mhz && (
                            <Badge variant="outline" className="text-xs">
                              {station.frequency_mhz} MHz
                            </Badge>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {station.city}{station.state ? `, ${station.state}` : ''}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground capitalize">
                        {station.broadcast_type}
                      </td>
                      <td className="px-4 py-3 text-sm">{station.show_count}</td>
                      <td className="px-4 py-3">
                        <Badge variant={station.is_active ? 'default' : 'secondary'} className="text-xs">
                          {station.is_active ? 'Active' : 'Inactive'}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Create Station Dialog */}
        <Dialog open={dialogMode === 'create-station'} onOpenChange={(open) => !open && setDialogMode(null)}>
          <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Add Radio Station</DialogTitle>
              <DialogDescription>Create a new radio station.</DialogDescription>
            </DialogHeader>
            <CreateStationForm
              onSuccess={() => setDialogMode(null)}
              onCancel={() => setDialogMode(null)}
            />
          </DialogContent>
        </Dialog>
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

function EditStationFormWrapper({
  stationId,
  onSuccess,
  onCancel,
}: {
  stationId: number
  onSuccess: () => void
  onCancel: () => void
}) {
  const { data: station, isLoading } = useRadioStationDetail(stationId)

  if (isLoading || !station) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return <EditStationForm station={station} onSuccess={onSuccess} onCancel={onCancel} />
}
