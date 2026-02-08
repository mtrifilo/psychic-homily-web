import { useMutation } from '@tanstack/react-query'
import { importEvents, importData, importDataToEnv } from '../api'
import type {
  ScrapedEvent,
  ImportResult,
  DataImportResult,
  CombinedImportResult,
  ImportTarget,
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

// Import data to a specific target (Stage, Production, or Both)
export function useTargetedDataImport() {
  return useMutation<
    CombinedImportResult,
    Error,
    {
      shows?: ExportedShow[]
      artists?: ExportedArtist[]
      venues?: ExportedVenue[]
      dryRun?: boolean
      target: ImportTarget
    }
  >({
    mutationFn: async ({ shows, artists, venues, dryRun = false, target }) => {
      const payload = { shows, artists, venues }

      if (target === 'both') {
        const [stageResult, prodResult] = await Promise.allSettled([
          importDataToEnv(payload, dryRun, 'stage'),
          importDataToEnv(payload, dryRun, 'production'),
        ])
        return {
          stage: stageResult.status === 'fulfilled' ? stageResult.value : undefined,
          production: prodResult.status === 'fulfilled' ? prodResult.value : undefined,
        }
      }

      const result = await importDataToEnv(payload, dryRun, target)
      return { [target]: result }
    },
  })
}
