import { useState, useCallback } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from './ui/tabs'
import { Alert, AlertDescription } from './ui/alert'
import { ShowsTab } from './export/ShowsTab'
import { ArtistsTab } from './export/ArtistsTab'
import { VenuesTab } from './export/VenuesTab'
import { ImportSection } from './export/ImportSection'
import { useRemoteShowExistence } from '../lib/hooks/useExport'
import { useWizard } from '../context/WizardContext'
import { AlertTriangle, ChevronDown, ChevronUp, Settings } from 'lucide-react'
import type {
  AppSettings,
  ExportedShow,
  ExportedArtist,
  ExportedVenue,
} from '../lib/types'

interface Props {
  settings: AppSettings
}

export function DataExport({ settings }: Props) {
  const { setStep } = useWizard()
  const [showHelp, setShowHelp] = useState(false)

  // Data state - store loaded data for import
  const [shows, setShows] = useState<ExportedShow[]>([])
  const [artists, setArtists] = useState<ExportedArtist[]>([])
  const [venues, setVenues] = useState<ExportedVenue[]>([])

  // Selection state - use string IDs instead of indices
  const [selectedShowIds, setSelectedShowIds] = useState<Set<string>>(new Set())
  const [selectedArtistIds, setSelectedArtistIds] = useState<Set<string>>(new Set())
  const [selectedVenueIds, setSelectedVenueIds] = useState<Set<string>>(new Set())

  const hasLocalToken = Boolean(settings.localToken?.length)
  const hasStageToken = Boolean(settings.stageToken?.length)
  const hasProductionToken = Boolean(settings.productionToken?.length)

  const missingTokens = []
  if (!hasLocalToken) missingTokens.push('Local')
  if (!hasStageToken && !hasProductionToken) missingTokens.push('Stage or Production')

  // Check which shows exist on remote environments
  const { stageShowIds, productionShowIds } = useRemoteShowExistence(
    settings,
    shows.length > 0
  )

  // Get selected data for import by filtering using IDs
  const selectedShows = shows.filter((s) =>
    selectedShowIds.has(`${s.title}-${s.eventDate}`)
  )
  const selectedArtists = artists.filter((a) => selectedArtistIds.has(a.name))
  const selectedVenues = venues.filter((v) =>
    selectedVenueIds.has(`${v.name}-${v.city}-${v.state}`)
  )

  // Callbacks for loading data
  const handleShowsLoaded = useCallback((loadedShows: ExportedShow[]) => {
    setShows(loadedShows)
  }, [])

  const handleArtistsLoaded = useCallback((loadedArtists: ExportedArtist[]) => {
    setArtists(loadedArtists)
  }, [])

  const handleVenuesLoaded = useCallback((loadedVenues: ExportedVenue[]) => {
    setVenues(loadedVenues)
  }, [])

  return (
    <div className="space-y-4">
      {/* Header with help toggle */}
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-lg font-semibold text-foreground">Data Export</h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            Load from local database, upload to Stage or Production
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowHelp(!showHelp)}
          className="text-muted-foreground"
        >
          Help
          {showHelp ? (
            <ChevronUp className="ml-1 h-4 w-4" />
          ) : (
            <ChevronDown className="ml-1 h-4 w-4" />
          )}
        </Button>
      </div>

      {/* Collapsible help */}
      {showHelp && (
        <div className="text-sm text-muted-foreground bg-muted/50 rounded-lg p-3 space-y-2">
          <p className="font-medium text-foreground">How it works:</p>
          <ol className="list-decimal list-inside space-y-1 ml-1">
            <li>Start your local Go backend (localhost:8080)</li>
            <li>Shows auto-load from your local database</li>
            <li>Select items to upload</li>
            <li>Choose target (Stage, Production, or Both) then preview or import</li>
          </ol>
        </div>
      )}

      {/* Setup required banner - only show if tokens missing */}
      {missingTokens.length > 0 && (
        <Alert className="bg-amber-50 border-amber-200 dark:bg-amber-950/50 dark:border-amber-900">
          <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-500" />
          <AlertDescription className="flex items-center justify-between">
            <span className="text-amber-800 dark:text-amber-200">
              Configure {missingTokens.join(' and ')} API token{missingTokens.length > 1 ? 's' : ''} in Settings to continue
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setStep('settings')}
              className="ml-4 shrink-0"
            >
              <Settings className="h-4 w-4 mr-1" />
              Settings
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Tabs */}
      <Tabs defaultValue="shows" className="space-y-4">
        <TabsList>
          <TabsTrigger value="shows">
            Shows
            {selectedShowIds.size > 0 && (
              <Badge variant="secondary" className="ml-2 bg-primary/10 text-primary">
                {selectedShowIds.size}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="artists">
            Artists
            {selectedArtistIds.size > 0 && (
              <Badge variant="secondary" className="ml-2 bg-primary/10 text-primary">
                {selectedArtistIds.size}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="venues">
            Venues
            {selectedVenueIds.size > 0 && (
              <Badge variant="secondary" className="ml-2 bg-primary/10 text-primary">
                {selectedVenueIds.size}
              </Badge>
            )}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="shows">
          <ShowsTab
            selectedIds={selectedShowIds}
            onSelectionChange={setSelectedShowIds}
            onDataLoaded={handleShowsLoaded}
            hasLocalToken={hasLocalToken}
            stageShowIds={stageShowIds}
            productionShowIds={productionShowIds}
          />
        </TabsContent>

        <TabsContent value="artists">
          <ArtistsTab
            selectedIds={selectedArtistIds}
            onSelectionChange={setSelectedArtistIds}
            onDataLoaded={handleArtistsLoaded}
            hasLocalToken={hasLocalToken}
          />
        </TabsContent>

        <TabsContent value="venues">
          <VenuesTab
            selectedIds={selectedVenueIds}
            onSelectionChange={setSelectedVenueIds}
            onDataLoaded={handleVenuesLoaded}
            hasLocalToken={hasLocalToken}
          />
        </TabsContent>
      </Tabs>

      {/* Import section */}
      <ImportSection
        selectedShows={selectedShows}
        selectedArtists={selectedArtists}
        selectedVenues={selectedVenues}
        settings={settings}
      />
    </div>
  )
}
