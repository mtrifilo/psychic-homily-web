'use client'

import { useMutation } from '@tanstack/react-query'
import type {
  ExtractCollectionRequest,
  ExtractCollectionResponse,
} from '@/lib/types/extraction'

/**
 * Hook for the AI-assisted collection extraction. Same shape as
 * `useShowExtraction` — a thin useMutation wrapper around the Next.js
 * route. The route handles auth + Anthropic SDK + backend matching; the
 * hook surfaces the response (or thrown error) to the picker.
 */
async function extractCollectionInfo(
  request: ExtractCollectionRequest
): Promise<ExtractCollectionResponse> {
  const response = await fetch('/api/ai/extract-collection', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify(request),
  })

  const data: ExtractCollectionResponse = await response.json()

  if (!response.ok || !data.success) {
    throw new Error(data.error || 'Failed to extract collection items')
  }

  return data
}

export function useCollectionExtraction() {
  return useMutation({
    mutationFn: extractCollectionInfo,
  })
}
