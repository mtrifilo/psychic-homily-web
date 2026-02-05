import { useState } from 'react'
import { VenueSelector } from './components/VenueSelector'
import { EventPreview } from './components/EventPreview'
import { EventSelector } from './components/EventSelector'
import { ImportPanel } from './components/ImportPanel'
import { Settings } from './components/Settings'
import { DataExport } from './components/DataExport'
import type { VenueConfig, PreviewEvent, ScrapedEvent, AppSettings } from './lib/types'
import { VENUES } from './lib/config'
import { getSettings } from './lib/api'

type Step = 'venues' | 'preview' | 'select' | 'import' | 'settings' | 'data-export'

export default function App() {
  const [step, setStep] = useState<Step>('venues')
  const [selectedVenues, setSelectedVenues] = useState<VenueConfig[]>([])
  const [previewEvents, setPreviewEvents] = useState<Record<string, PreviewEvent[]>>({})
  const [selectedEventIds, setSelectedEventIds] = useState<Record<string, Set<string>>>({})
  const [scrapedEvents, setScrapedEvents] = useState<ScrapedEvent[]>([])
  const [settings, setSettings] = useState<AppSettings>(getSettings())

  // Check if token is configured
  const hasToken = settings.apiToken.length > 0

  const handleVenueSelect = (venues: VenueConfig[]) => {
    setSelectedVenues(venues)
    setPreviewEvents({})
    setSelectedEventIds({})
  }

  const handlePreviewComplete = (venueSlug: string, events: PreviewEvent[]) => {
    setPreviewEvents(prev => ({ ...prev, [venueSlug]: events }))
    // Pre-select all events by default
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set(events.map(e => e.id)),
    }))
  }

  const handleEventToggle = (venueSlug: string, eventId: string) => {
    setSelectedEventIds(prev => {
      const current = prev[venueSlug] || new Set()
      const updated = new Set(current)
      if (updated.has(eventId)) {
        updated.delete(eventId)
      } else {
        updated.add(eventId)
      }
      return { ...prev, [venueSlug]: updated }
    })
  }

  const handleSelectAll = (venueSlug: string) => {
    const events = previewEvents[venueSlug] || []
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set(events.map(e => e.id)),
    }))
  }

  const handleSelectNone = (venueSlug: string) => {
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set(),
    }))
  }

  const handleScrapeComplete = (events: ScrapedEvent[]) => {
    setScrapedEvents(prev => [...prev, ...events])
  }

  const handleSettingsSave = (newSettings: AppSettings) => {
    setSettings(newSettings)
  }

  const handleStartOver = () => {
    setStep('venues')
    setSelectedVenues([])
    setPreviewEvents({})
    setSelectedEventIds({})
    setScrapedEvents([])
  }

  const totalSelectedEvents = Object.values(selectedEventIds).reduce(
    (sum, set) => sum + set.size,
    0
  )

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="bg-white border-b border-gray-200 px-6 py-4">
        <div className="max-w-6xl mx-auto flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold text-gray-900">Venue Scraper</h1>
            <p className="text-sm text-gray-500">Psychic Homily Admin Tool</p>
          </div>
          <div className="flex items-center gap-4">
            {!hasToken && (
              <span className="text-sm text-amber-600 bg-amber-50 px-3 py-1 rounded-full">
                Token not configured
              </span>
            )}
            <button
              onClick={() => setStep('data-export')}
              className={`text-sm px-3 py-1 rounded ${
                step === 'data-export'
                  ? 'bg-blue-100 text-blue-700'
                  : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
              }`}
            >
              Data Export
            </button>
            <button
              onClick={() => setStep('settings')}
              className={`text-sm px-3 py-1 rounded ${
                step === 'settings'
                  ? 'bg-blue-100 text-blue-700'
                  : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
              }`}
            >
              Settings
            </button>
          </div>
        </div>
      </header>

      {/* Progress Steps */}
      {step !== 'settings' && step !== 'data-export' && (
        <div className="bg-white border-b border-gray-200 px-6 py-3">
          <div className="max-w-6xl mx-auto">
            <div className="flex items-center gap-2 text-sm">
              <StepIndicator
                label="1. Venues"
                active={step === 'venues'}
                complete={selectedVenues.length > 0 && step !== 'venues'}
                onClick={() => setStep('venues')}
              />
              <span className="text-gray-300">/</span>
              <StepIndicator
                label="2. Preview"
                active={step === 'preview'}
                complete={Object.keys(previewEvents).length > 0 && step !== 'preview'}
                onClick={() => selectedVenues.length > 0 && setStep('preview')}
                disabled={selectedVenues.length === 0}
              />
              <span className="text-gray-300">/</span>
              <StepIndicator
                label="3. Select"
                active={step === 'select'}
                complete={scrapedEvents.length > 0 && step !== 'select'}
                onClick={() => Object.keys(previewEvents).length > 0 && setStep('select')}
                disabled={Object.keys(previewEvents).length === 0}
              />
              <span className="text-gray-300">/</span>
              <StepIndicator
                label="4. Import"
                active={step === 'import'}
                complete={false}
                onClick={() => scrapedEvents.length > 0 && setStep('import')}
                disabled={scrapedEvents.length === 0}
              />
            </div>
          </div>
        </div>
      )}

      {/* Main Content */}
      <main className="max-w-6xl mx-auto px-6 py-8">
        {step === 'settings' && (
          <Settings
            settings={settings}
            onSave={handleSettingsSave}
            onBack={() => setStep('venues')}
          />
        )}

        {step === 'data-export' && (
          <DataExport
            settings={settings}
            onBack={() => setStep('venues')}
          />
        )}

        {step === 'venues' && (
          <VenueSelector
            venues={VENUES}
            selectedVenues={selectedVenues}
            onSelect={handleVenueSelect}
            onNext={() => setStep('preview')}
          />
        )}

        {step === 'preview' && (
          <EventPreview
            venues={selectedVenues}
            onPreviewComplete={handlePreviewComplete}
            onBack={() => setStep('venues')}
            onNext={() => setStep('select')}
            previewEvents={previewEvents}
          />
        )}

        {step === 'select' && (
          <EventSelector
            venues={selectedVenues}
            previewEvents={previewEvents}
            selectedEventIds={selectedEventIds}
            onToggle={handleEventToggle}
            onSelectAll={handleSelectAll}
            onSelectNone={handleSelectNone}
            onScrapeComplete={handleScrapeComplete}
            onBack={() => setStep('preview')}
            onNext={() => setStep('import')}
            scrapedEvents={scrapedEvents}
          />
        )}

        {step === 'import' && (
          <ImportPanel
            events={scrapedEvents}
            settings={settings}
            onBack={() => setStep('select')}
            onStartOver={handleStartOver}
          />
        )}
      </main>
    </div>
  )
}

function StepIndicator({
  label,
  active,
  complete,
  onClick,
  disabled = false,
}: {
  label: string
  active: boolean
  complete: boolean
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`px-2 py-1 rounded transition-colors ${
        active
          ? 'text-blue-600 font-medium bg-blue-50'
          : complete
          ? 'text-green-600 hover:bg-green-50'
          : disabled
          ? 'text-gray-300 cursor-not-allowed'
          : 'text-gray-500 hover:text-gray-700 hover:bg-gray-50'
      }`}
    >
      {label}
      {complete && ' ✓'}
    </button>
  )
}
