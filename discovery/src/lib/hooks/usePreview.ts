import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '../queryKeys'
import { previewVenueEvents, previewVenueEventsBatch, scrapeVenueEvents } from '../api'
import type { PreviewEvent, BatchPreviewResult, ScrapedEvent } from '../types'

// Preview events for a single venue
export function useVenuePreview(venueSlug: string, enabled = true) {
  return useQuery<PreviewEvent[], Error>({
    queryKey: queryKeys.preview.venue(venueSlug),
    queryFn: () => previewVenueEvents(venueSlug),
    enabled: enabled && Boolean(venueSlug),
  })
}

// Preview events for multiple venues in parallel
export function useBatchPreview(venueSlugs: string[], enabled = true) {
  return useQuery<BatchPreviewResult[], Error>({
    queryKey: queryKeys.preview.batch(venueSlugs),
    queryFn: () => previewVenueEventsBatch(venueSlugs),
    enabled: enabled && venueSlugs.length > 0,
  })
}

// Scrape selected events from a venue
export function useScrapeEvents() {
  const queryClient = useQueryClient()

  return useMutation<
    ScrapedEvent[],
    Error,
    { venueSlug: string; eventIds: string[] }
  >({
    mutationFn: ({ venueSlug, eventIds }) =>
      scrapeVenueEvents(venueSlug, eventIds),
    onSuccess: (_, variables) => {
      // Invalidate the preview cache for this venue after scraping
      queryClient.invalidateQueries({
        queryKey: queryKeys.preview.venue(variables.venueSlug),
      })
    },
  })
}
