'use client'

import { useMutation } from '@tanstack/react-query'
import type {
  ExtractShowRequest,
  ExtractShowResponse,
} from '@/lib/types/extraction'

/**
 * Extract show information from text or image using AI
 */
async function extractShowInfo(
  request: ExtractShowRequest
): Promise<ExtractShowResponse> {
  const response = await fetch('/api/ai/extract-show', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify(request),
  })

  const data: ExtractShowResponse = await response.json()

  // If the response indicates an error, throw to trigger mutation error state
  if (!response.ok || !data.success) {
    throw new Error(data.error || 'Failed to extract show information')
  }

  return data
}

/**
 * Hook for extracting show information from text or images
 *
 * @example
 * const { mutate, isPending, error, data } = useShowExtraction()
 *
 * // Extract from text
 * mutate({ type: 'text', text: 'The National at Valley Bar...' })
 *
 * // Extract from image
 * mutate({
 *   type: 'image',
 *   image_data: base64Data,
 *   media_type: 'image/jpeg'
 * })
 */
export function useShowExtraction() {
  return useMutation({
    mutationFn: extractShowInfo,
  })
}
