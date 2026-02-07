import { useMutation } from '@tanstack/react-query'
import { importEvents, importData } from '../api'
import type {
  ScrapedEvent,
  ImportResult,
  DataImportResult,
  ExportedShow,
  ExportedArtist,
  ExportedVenue,
} from '../types'

// Import scraped events to the backend
export function useImportEvents() {
  return useMutation<
    ImportResult,
    Error,
    { events: ScrapedEvent[]; dryRun?: boolean }
  >({
    mutationFn: ({ events, dryRun = false }) => importEvents(events, dryRun),
  })
}

// Import data (shows, artists, venues) to remote backend
export function useDataImport() {
  return useMutation<
    DataImportResult,
    Error,
    {
      shows?: ExportedShow[]
      artists?: ExportedArtist[]
      venues?: ExportedVenue[]
      dryRun?: boolean
    }
  >({
    mutationFn: ({ shows, artists, venues, dryRun = false }) =>
      importData({ shows, artists, venues }, dryRun),
  })
}
