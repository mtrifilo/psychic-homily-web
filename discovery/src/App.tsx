import { VenueSelector } from './components/VenueSelector'
import { EventPreview } from './components/EventPreview'
import { ImportPanel } from './components/ImportPanel'
import { Settings } from './components/Settings'
import { DataExport } from './components/DataExport'
import { Header } from './components/layout/Header'
import { ProgressSteps } from './components/layout/ProgressSteps'
import { useWizard } from './context/WizardContext'
import { useSettings } from './context/SettingsContext'
import { VENUES } from './lib/config'

export default function App() {
  const {
    step,
    setStep,
    selectedVenues,
    previewEvents,
    selectedEventIds,
    scrapedEvents,
    importStatuses,
    selectVenues,
    handlePreviewComplete,
    toggleEvent,
    selectAllEvents,
    clearEventSelection,
    addScrapedEvents,
    setImportStatuses,
    startOver,
  } = useWizard()

  const { settings, updateSettings } = useSettings()

  return (
    <div className="min-h-screen bg-background">
      <Header />
      <ProgressSteps />

      <main className="max-w-6xl mx-auto px-6 py-8">
        {step === 'settings' && (
          <Settings
            settings={settings}
            onSave={updateSettings}
          />
        )}

        {step === 'data-export' && (
          <DataExport
            settings={settings}
          />
        )}

        {step === 'venues' && (
          <VenueSelector
            venues={VENUES}
            selectedVenues={selectedVenues}
            onSelect={selectVenues}
            onNext={() => setStep('preview')}
          />
        )}

        {step === 'preview' && (
          <EventPreview
            venues={selectedVenues}
            previewEvents={previewEvents}
            selectedEventIds={selectedEventIds}
            importStatuses={importStatuses}
            onPreviewComplete={handlePreviewComplete}
            onSetImportStatuses={setImportStatuses}
            onToggle={toggleEvent}
            onSelectAll={selectAllEvents}
            onSelectNone={clearEventSelection}
            onScrapeComplete={addScrapedEvents}
            onBack={() => setStep('venues')}
            onNext={() => setStep('import')}
          />
        )}

        {step === 'import' && (
          <ImportPanel
            events={scrapedEvents}
            settings={settings}
            onBack={() => setStep('preview')}
            onStartOver={startOver}
          />
        )}
      </main>
    </div>
  )
}
