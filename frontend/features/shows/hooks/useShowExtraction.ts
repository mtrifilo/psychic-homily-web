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

  // Parse defensively: read the JSON body BEFORE checking response.ok, but
  // tolerate a non-JSON body. An upstream HTML 502 (Vercel/gateway error page)
  // would otherwise make response.json() reject with a SyntaxError, which the
  // caller surfaces as the opaque "Unexpected token '<'" instead of a friendly
  // message. PSY-855's 429 path keeps working — it returns valid JSON whose
  // `error` string is preserved by the throw below.
  let data: ExtractShowResponse | null = null
  try {
    data = (await response.json()) as ExtractShowResponse
  } catch {
    data = null
  }

  // If the response indicates an error, throw to trigger mutation error state.
  // Prefer the server's `error` string (PSY-855 rate-limit hint, AI parse
  // failures). Fall back to the HTTP status only when the body was unusable
  // (HTML error page) — otherwise the domain-specific generic message.
  if (!response.ok || !data?.success) {
    throw new Error(
      data?.error ??
        (data
          ? 'Failed to extract show information'
          : `AI service error (HTTP ${response.status})`)
    )
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
