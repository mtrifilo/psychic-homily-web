import { useState } from 'react'
import type {
  AppSettings,
  ExportedShow,
  ExportedArtist,
  ExportedVenue,
  DataImportResult,
} from '../lib/types'
import { exportShows, exportArtists, exportVenues, importData } from '../lib/api'

type Tab = 'shows' | 'artists' | 'venues'

interface Props {
  settings: AppSettings
  onBack: () => void
}

export function DataExport({ settings, onBack }: Props) {
  const [activeTab, setActiveTab] = useState<Tab>('shows')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')

  // Data state
  const [shows, setShows] = useState<ExportedShow[]>([])
  const [artists, setArtists] = useState<ExportedArtist[]>([])
  const [venues, setVenues] = useState<ExportedVenue[]>([])

  // Selection state
  const [selectedShows, setSelectedShows] = useState<Set<number>>(new Set())
  const [selectedArtists, setSelectedArtists] = useState<Set<number>>(new Set())
  const [selectedVenues, setSelectedVenues] = useState<Set<number>>(new Set())

  // Totals for pagination info
  const [showsTotal, setShowsTotal] = useState(0)
  const [artistsTotal, setArtistsTotal] = useState(0)
  const [venuesTotal, setVenuesTotal] = useState(0)

  // Import state
  const [importing, setImporting] = useState(false)
  const [isDryRun, setIsDryRun] = useState(true)
  const [importResult, setImportResult] = useState<DataImportResult | null>(null)

  // Filters
  const [showStatus, setShowStatus] = useState('approved')
  const [artistSearch, setArtistSearch] = useState('')
  const [venueSearch, setVenueSearch] = useState('')

  const loadShows = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await exportShows({ limit: 100, status: showStatus })
      setShows(result.shows)
      setShowsTotal(result.total)
      setSelectedShows(new Set())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load shows')
    } finally {
      setLoading(false)
    }
  }

  const loadArtists = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await exportArtists({ limit: 100, search: artistSearch })
      setArtists(result.artists)
      setArtistsTotal(result.total)
      setSelectedArtists(new Set())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load artists')
    } finally {
      setLoading(false)
    }
  }

  const loadVenues = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await exportVenues({ limit: 100, search: venueSearch })
      setVenues(result.venues)
      setVenuesTotal(result.total)
      setSelectedVenues(new Set())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load venues')
    } finally {
      setLoading(false)
    }
  }

  const handleImport = async () => {
    setImporting(true)
    setError('')
    setImportResult(null)

    try {
      const selectedShowData = shows.filter((_, i) => selectedShows.has(i))
      const selectedArtistData = artists.filter((_, i) => selectedArtists.has(i))
      const selectedVenueData = venues.filter((_, i) => selectedVenues.has(i))

      const result = await importData(
        {
          shows: selectedShowData.length > 0 ? selectedShowData : undefined,
          artists: selectedArtistData.length > 0 ? selectedArtistData : undefined,
          venues: selectedVenueData.length > 0 ? selectedVenueData : undefined,
        },
        isDryRun
      )

      setImportResult(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to import data')
    } finally {
      setImporting(false)
    }
  }

  const totalSelected = selectedShows.size + selectedArtists.size + selectedVenues.size
  const targetEnv = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Data Export</h2>
          <p className="text-sm text-gray-500 mt-1">
            Browse local data and upload to {targetEnv}
          </p>
        </div>
        <button
          onClick={onBack}
          className="text-sm text-gray-600 hover:text-gray-900"
        >
          Back to Scraper
        </button>
      </div>

      {!settings.apiToken && (
        <div className="bg-amber-50 border border-amber-200 rounded-lg px-4 py-3 text-amber-800">
          <p className="font-medium">API Token Required</p>
          <p className="text-sm mt-1">
            Go to Settings to configure your API token before using Data Export.
          </p>
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg px-4 py-3 text-red-700">
          {error}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="flex gap-4">
          {(['shows', 'artists', 'venues'] as Tab[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                activeTab === tab
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
              {tab === 'shows' && selectedShows.size > 0 && (
                <span className="ml-2 bg-blue-100 text-blue-700 px-2 py-0.5 rounded-full text-xs">
                  {selectedShows.size}
                </span>
              )}
              {tab === 'artists' && selectedArtists.size > 0 && (
                <span className="ml-2 bg-blue-100 text-blue-700 px-2 py-0.5 rounded-full text-xs">
                  {selectedArtists.size}
                </span>
              )}
              {tab === 'venues' && selectedVenues.size > 0 && (
                <span className="ml-2 bg-blue-100 text-blue-700 px-2 py-0.5 rounded-full text-xs">
                  {selectedVenues.size}
                </span>
              )}
            </button>
          ))}
        </nav>
      </div>

      {/* Shows Tab */}
      {activeTab === 'shows' && (
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <select
              value={showStatus}
              onChange={(e) => setShowStatus(e.target.value)}
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm"
            >
              <option value="approved">Approved</option>
              <option value="pending">Pending</option>
              <option value="all">All</option>
            </select>
            <button
              onClick={loadShows}
              disabled={loading || !settings.apiToken}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
            >
              {loading ? 'Loading...' : 'Load Shows'}
            </button>
            {shows.length > 0 && (
              <span className="text-sm text-gray-500">
                Showing {shows.length} of {showsTotal}
              </span>
            )}
          </div>

          {shows.length > 0 && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="px-4 py-2 bg-gray-50 border-b flex items-center justify-between">
                <span className="text-sm font-medium text-gray-700">
                  {selectedShows.size} selected
                </span>
                <div className="flex gap-2">
                  <button
                    onClick={() => setSelectedShows(new Set(shows.map((_, i) => i)))}
                    className="text-sm text-blue-600 hover:text-blue-700"
                  >
                    Select All
                  </button>
                  <span className="text-gray-300">|</span>
                  <button
                    onClick={() => setSelectedShows(new Set())}
                    className="text-sm text-gray-500 hover:text-gray-700"
                  >
                    Clear
                  </button>
                </div>
              </div>
              <div className="max-h-96 overflow-y-auto">
                {shows.map((show, idx) => (
                  <label
                    key={idx}
                    className="flex items-start gap-3 px-4 py-3 hover:bg-gray-50 cursor-pointer border-b border-gray-100 last:border-b-0"
                  >
                    <input
                      type="checkbox"
                      checked={selectedShows.has(idx)}
                      onChange={() => {
                        const next = new Set(selectedShows)
                        if (next.has(idx)) next.delete(idx)
                        else next.add(idx)
                        setSelectedShows(next)
                      }}
                      className="mt-1 w-4 h-4 text-blue-600 rounded border-gray-300"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-gray-900 truncate">{show.title}</div>
                      <div className="text-sm text-gray-500">
                        {formatDate(show.eventDate)} •{' '}
                        {show.venues.map((v) => v.name).join(', ') || 'No venue'} •{' '}
                        {show.artists.length} artist{show.artists.length !== 1 ? 's' : ''}
                      </div>
                    </div>
                    <span
                      className={`text-xs px-2 py-1 rounded ${
                        show.status === 'approved'
                          ? 'bg-green-100 text-green-700'
                          : show.status === 'pending'
                          ? 'bg-amber-100 text-amber-700'
                          : 'bg-gray-100 text-gray-600'
                      }`}
                    >
                      {show.status}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Artists Tab */}
      {activeTab === 'artists' && (
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <input
              type="text"
              value={artistSearch}
              onChange={(e) => setArtistSearch(e.target.value)}
              placeholder="Search artists..."
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-64"
            />
            <button
              onClick={loadArtists}
              disabled={loading || !settings.apiToken}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
            >
              {loading ? 'Loading...' : 'Load Artists'}
            </button>
            {artists.length > 0 && (
              <span className="text-sm text-gray-500">
                Showing {artists.length} of {artistsTotal}
              </span>
            )}
          </div>

          {artists.length > 0 && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="px-4 py-2 bg-gray-50 border-b flex items-center justify-between">
                <span className="text-sm font-medium text-gray-700">
                  {selectedArtists.size} selected
                </span>
                <div className="flex gap-2">
                  <button
                    onClick={() => setSelectedArtists(new Set(artists.map((_, i) => i)))}
                    className="text-sm text-blue-600 hover:text-blue-700"
                  >
                    Select All
                  </button>
                  <span className="text-gray-300">|</span>
                  <button
                    onClick={() => setSelectedArtists(new Set())}
                    className="text-sm text-gray-500 hover:text-gray-700"
                  >
                    Clear
                  </button>
                </div>
              </div>
              <div className="max-h-96 overflow-y-auto">
                {artists.map((artist, idx) => (
                  <label
                    key={idx}
                    className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 cursor-pointer border-b border-gray-100 last:border-b-0"
                  >
                    <input
                      type="checkbox"
                      checked={selectedArtists.has(idx)}
                      onChange={() => {
                        const next = new Set(selectedArtists)
                        if (next.has(idx)) next.delete(idx)
                        else next.add(idx)
                        setSelectedArtists(next)
                      }}
                      className="w-4 h-4 text-blue-600 rounded border-gray-300"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-gray-900">{artist.name}</div>
                      {(artist.city || artist.state) && (
                        <div className="text-sm text-gray-500">
                          {[artist.city, artist.state].filter(Boolean).join(', ')}
                        </div>
                      )}
                    </div>
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Venues Tab */}
      {activeTab === 'venues' && (
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <input
              type="text"
              value={venueSearch}
              onChange={(e) => setVenueSearch(e.target.value)}
              placeholder="Search venues..."
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-64"
            />
            <button
              onClick={loadVenues}
              disabled={loading || !settings.apiToken}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
            >
              {loading ? 'Loading...' : 'Load Venues'}
            </button>
            {venues.length > 0 && (
              <span className="text-sm text-gray-500">
                Showing {venues.length} of {venuesTotal}
              </span>
            )}
          </div>

          {venues.length > 0 && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="px-4 py-2 bg-gray-50 border-b flex items-center justify-between">
                <span className="text-sm font-medium text-gray-700">
                  {selectedVenues.size} selected
                </span>
                <div className="flex gap-2">
                  <button
                    onClick={() => setSelectedVenues(new Set(venues.map((_, i) => i)))}
                    className="text-sm text-blue-600 hover:text-blue-700"
                  >
                    Select All
                  </button>
                  <span className="text-gray-300">|</span>
                  <button
                    onClick={() => setSelectedVenues(new Set())}
                    className="text-sm text-gray-500 hover:text-gray-700"
                  >
                    Clear
                  </button>
                </div>
              </div>
              <div className="max-h-96 overflow-y-auto">
                {venues.map((venue, idx) => (
                  <label
                    key={idx}
                    className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 cursor-pointer border-b border-gray-100 last:border-b-0"
                  >
                    <input
                      type="checkbox"
                      checked={selectedVenues.has(idx)}
                      onChange={() => {
                        const next = new Set(selectedVenues)
                        if (next.has(idx)) next.delete(idx)
                        else next.add(idx)
                        setSelectedVenues(next)
                      }}
                      className="w-4 h-4 text-blue-600 rounded border-gray-300"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-gray-900">{venue.name}</div>
                      <div className="text-sm text-gray-500">
                        {venue.city}, {venue.state}
                        {venue.address && ` • ${venue.address}`}
                      </div>
                    </div>
                    {venue.verified && (
                      <span className="text-xs bg-green-100 text-green-700 px-2 py-1 rounded">
                        Verified
                      </span>
                    )}
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Import Section */}
      {totalSelected > 0 && (
        <div className="bg-white rounded-lg border border-gray-200 p-4 space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium text-gray-900">Upload to {targetEnv}</h3>
              <p className="text-sm text-gray-500 mt-1">
                {selectedShows.size} shows, {selectedArtists.size} artists, {selectedVenues.size}{' '}
                venues selected
              </p>
            </div>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={!isDryRun}
                onChange={(e) => setIsDryRun(!e.target.checked)}
                className="w-4 h-4 text-blue-600 rounded border-gray-300"
              />
              <span className="text-sm text-gray-700">Live Import</span>
            </label>
          </div>

          <div className="flex items-center gap-4">
            <button
              onClick={handleImport}
              disabled={importing || !settings.apiToken}
              className={`px-4 py-2 rounded-lg font-medium flex items-center gap-2 ${
                importing || !settings.apiToken
                  ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                  : isDryRun
                  ? 'bg-blue-600 text-white hover:bg-blue-700'
                  : 'bg-green-600 text-white hover:bg-green-700'
              }`}
            >
              {importing && (
                <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
              )}
              {isDryRun ? 'Preview Import' : 'Import Now'}
            </button>
            <span
              className={`text-sm font-medium ${
                settings.targetEnvironment === 'production' ? 'text-red-600' : 'text-blue-600'
              }`}
            >
              Target: {targetEnv}
            </span>
          </div>
        </div>
      )}

      {/* Import Results */}
      {importResult && (
        <div className="bg-white rounded-lg border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 bg-gray-50 border-b">
            <h3 className="font-medium text-gray-900">
              {isDryRun ? 'Preview Results' : 'Import Results'}
            </h3>
          </div>
          <div className="p-4 space-y-4">
            {/* Shows */}
            {importResult.shows.total > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-700 mb-2">Shows</h4>
                <div className="flex gap-4 text-sm">
                  <span className="text-green-600">
                    {isDryRun ? 'Would import' : 'Imported'}: {importResult.shows.imported}
                  </span>
                  <span className="text-blue-600">Duplicates: {importResult.shows.duplicates}</span>
                  <span className="text-red-600">Errors: {importResult.shows.errors}</span>
                </div>
              </div>
            )}

            {/* Artists */}
            {importResult.artists.total > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-700 mb-2">Artists</h4>
                <div className="flex gap-4 text-sm">
                  <span className="text-green-600">
                    {isDryRun ? 'Would import' : 'Imported'}: {importResult.artists.imported}
                  </span>
                  <span className="text-blue-600">
                    Duplicates: {importResult.artists.duplicates}
                  </span>
                  <span className="text-red-600">Errors: {importResult.artists.errors}</span>
                </div>
              </div>
            )}

            {/* Venues */}
            {importResult.venues.total > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-700 mb-2">Venues</h4>
                <div className="flex gap-4 text-sm">
                  <span className="text-green-600">
                    {isDryRun ? 'Would import' : 'Imported'}: {importResult.venues.imported}
                  </span>
                  <span className="text-blue-600">
                    Duplicates: {importResult.venues.duplicates}
                  </span>
                  <span className="text-red-600">Errors: {importResult.venues.errors}</span>
                </div>
              </div>
            )}

            {/* Messages */}
            {(importResult.shows.messages.length > 0 ||
              importResult.artists.messages.length > 0 ||
              importResult.venues.messages.length > 0) && (
              <div className="mt-4">
                <h4 className="text-sm font-medium text-gray-700 mb-2">Details</h4>
                <div className="bg-gray-50 rounded-lg p-3 max-h-48 overflow-y-auto">
                  <ul className="text-xs font-mono space-y-1">
                    {[
                      ...importResult.shows.messages,
                      ...importResult.artists.messages,
                      ...importResult.venues.messages,
                    ].map((msg, i) => (
                      <li
                        key={i}
                        className={
                          msg.startsWith('IMPORTED') || msg.startsWith('WOULD IMPORT')
                            ? 'text-green-700'
                            : msg.startsWith('DUPLICATE')
                            ? 'text-blue-600'
                            : msg.startsWith('ERROR') || msg.startsWith('SKIP')
                            ? 'text-red-600'
                            : 'text-gray-600'
                        }
                      >
                        {msg}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Instructions */}
      <div className="bg-blue-50 rounded-lg p-4">
        <h3 className="text-sm font-medium text-blue-900">How to use Data Export</h3>
        <ol className="mt-2 text-sm text-blue-800 space-y-1 list-decimal list-inside">
          <li>Make sure your local Go backend is running (localhost:8080)</li>
          <li>Load data from your local database using the Load buttons</li>
          <li>Select items you want to upload</li>
          <li>Preview first (dry run), then Import to {targetEnv}</li>
        </ol>
        <p className="mt-2 text-sm text-blue-700">
          <strong>Note:</strong> The same API token works for both local and{' '}
          {targetEnv.toLowerCase()} environments if you have the same user account.
        </p>
      </div>
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}
