import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import type { VenueConfig, PreviewEvent, ScrapedEvent, ImportStatusMap } from '../lib/types'
import { getLocalDateString } from '../lib/dates'

export type WizardStep = 'venues' | 'preview' | 'import' | 'settings' | 'data-export'

interface WizardState {
  step: WizardStep
  selectedVenues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  selectedEventIds: Record<string, Set<string>>
  scrapedEvents: ScrapedEvent[]
  importStatuses: ImportStatusMap
}

interface WizardActions {
  setStep: (step: WizardStep) => void
  selectVenues: (venues: VenueConfig[]) => void
  handlePreviewComplete: (venueSlug: string, events: PreviewEvent[]) => void
  toggleEvent: (venueSlug: string, eventId: string) => void
  selectAllEvents: (venueSlug: string) => void
  clearEventSelection: (venueSlug: string) => void
  addScrapedEvents: (events: ScrapedEvent[]) => void
  setImportStatuses: (statuses: ImportStatusMap) => void
  startOver: () => void
}

interface WizardContextValue extends WizardState, WizardActions {
  totalSelectedEvents: number
  totalAvailableEvents: number
}

const WizardContext = createContext<WizardContextValue | null>(null)

export function WizardProvider({ children }: { children: ReactNode }) {
  const [step, setStep] = useState<WizardStep>('venues')
  const [selectedVenues, setSelectedVenues] = useState<VenueConfig[]>([])
  const [previewEvents, setPreviewEvents] = useState<Record<string, PreviewEvent[]>>({})
  const [selectedEventIds, setSelectedEventIds] = useState<Record<string, Set<string>>>({})
  const [scrapedEvents, setScrapedEvents] = useState<ScrapedEvent[]>([])
  const [importStatuses, setImportStatuses] = useState<ImportStatusMap>({})

  const selectVenues = useCallback((venues: VenueConfig[]) => {
    setSelectedVenues(venues)
    setPreviewEvents({})
    setSelectedEventIds({})
    setImportStatuses({})
  }, [])

  const handlePreviewComplete = useCallback((venueSlug: string, events: PreviewEvent[]) => {
    setPreviewEvents(prev => ({ ...prev, [venueSlug]: events }))
    // Default to none selected — user picks what to scrape
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set<string>(),
    }))
  }, [])

  const toggleEvent = useCallback((venueSlug: string, eventId: string) => {
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
  }, [])

  const selectAllEvents = useCallback((venueSlug: string) => {
    const events = previewEvents[venueSlug] || []
    const today = getLocalDateString()
    const futureEvents = events.filter(e => e.date >= today)
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set(futureEvents.map(e => e.id)),
    }))
  }, [previewEvents])

  const clearEventSelection = useCallback((venueSlug: string) => {
    setSelectedEventIds(prev => ({
      ...prev,
      [venueSlug]: new Set(),
    }))
  }, [])

  const addScrapedEvents = useCallback((events: ScrapedEvent[]) => {
    setScrapedEvents(prev => {
      const existingIds = new Set(prev.map(e => e.id))
      const newEvents = events.filter(e => !existingIds.has(e.id))
      return [...prev, ...newEvents]
    })
  }, [])

  const startOver = useCallback(() => {
    setStep('venues')
    setSelectedVenues([])
    setPreviewEvents({})
    setSelectedEventIds({})
    setScrapedEvents([])
    setImportStatuses({})
  }, [])

  const totalSelectedEvents = Object.values(selectedEventIds).reduce(
    (sum, set) => sum + set.size,
    0
  )

  const totalAvailableEvents = Object.values(previewEvents).reduce(
    (sum, events) => sum + events.length,
    0
  )

  const value: WizardContextValue = {
    step,
    selectedVenues,
    previewEvents,
    selectedEventIds,
    scrapedEvents,
    importStatuses,
    totalSelectedEvents,
    totalAvailableEvents,
    setStep,
    selectVenues,
    handlePreviewComplete,
    toggleEvent,
    selectAllEvents,
    clearEventSelection,
    addScrapedEvents,
    setImportStatuses,
    startOver,
  }

  return (
    <WizardContext.Provider value={value}>
      {children}
    </WizardContext.Provider>
  )
}

export function useWizard() {
  const context = useContext(WizardContext)
  if (!context) {
    throw new Error('useWizard must be used within a WizardProvider')
  }
  return context
}
