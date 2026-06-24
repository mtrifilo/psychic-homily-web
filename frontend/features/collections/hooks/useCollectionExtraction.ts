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

  // Parse defensively: read the JSON body BEFORE checking response.ok, but
  // tolerate a non-JSON body. An upstream HTML 502 (Vercel/gateway error page)
  // would otherwise make response.json() reject with a SyntaxError, which the
  // caller surfaces as the opaque "Unexpected token '<'" instead of a friendly
  // message. PSY-855's 429 path keeps working — it returns valid JSON whose
  // `error` string is preserved by the throw below.
  let data: ExtractCollectionResponse | null = null
  try {
    data = (await response.json()) as ExtractCollectionResponse
  } catch {
    data = null
  }

  if (!response.ok || !data?.success) {
    // Prefer the server's `error` string (PSY-855 rate-limit hint, AI parse
    // failures). `||` (not `??`) so a missing OR empty `error` still falls
    // through — matching the pre-PSY-857 fallback. When the body was unusable
    // (HTML error page → data === null) report the HTTP status; otherwise the
    // domain-specific generic message.
    throw new Error(
      data?.error ||
        (data
          ? 'Failed to extract collection items'
          : `AI service error (HTTP ${response.status})`)
    )
  }

  return data
}

export function useCollectionExtraction() {
  return useMutation({
    mutationFn: extractCollectionInfo,
  })
}
