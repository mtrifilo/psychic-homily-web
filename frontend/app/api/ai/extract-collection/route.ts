/**
 * AI-assisted collection extraction.
 *
 * Mirrors the extract-show route: same auth check, same Anthropic SDK
 * pattern, same matched/suggestions response shape. Differences:
 *   - System prompt asks the model for a LIST of {artist_name, release_title?}
 *     rows in source order, not a single show payload.
 *   - Match strategy: per-row artist match against the existing
 *     `/artists/search` endpoint (case-insensitive exact match → matched_id,
 *     else top 3 suggestions). Release matching deferred — V1 carries
 *     release_title through to the response verbatim and the bulk-add
 *     pipeline commits only matched artist rows.
 *   - No autonomous entity creation. Per `feedback_human_verify_ai_entity_data.md`,
 *     unmatched rows go through the existing human-review path (a follow-up
 *     will wire the queue-for-review per-tier action; V1 surfaces unmatched
 *     rows as suggestions only).
 */

import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import * as Sentry from '@sentry/nextjs'
import type {
  ExtractCollectionRequest,
  ExtractCollectionResponse,
  ExtractedCollectionData,
  ExtractedCollectionItem,
  MatchSuggestion,
} from '@/lib/types/extraction'
import { getAuthenticatedUser } from '@/lib/auth-profile'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

/** Max payload sizes — mirror extract-show's gates so the surfaces stay aligned. */
const MAX_TEXT_LENGTH = 30000 // canon-list articles run longer than show flyers
const MAX_ITEMS_PER_EXTRACTION = 250 // bounded so a runaway prompt can't return 5,000 rows

/**
 * System prompt for collection extraction. Tuned for canon-list articles
 * (Pitchfork's 200 best albums, AOTY top-N, weekly Post-Trash digests).
 */
function getExtractionSystemPrompt(): string {
  return `You are a music-list extractor. Given text or an image of an article describing a list of musical works, extract structured information.

Output ONLY valid JSON with no additional text or markdown formatting:
{
  "source": "Pitchfork's 200 Best Albums of the 2010s",
  "description": "One-paragraph summary of the list's editorial framing, if present.",
  "items": [
    {"artist_name": "Kendrick Lamar", "release_title": "To Pimp a Butterfly"},
    {"artist_name": "Frank Ocean", "release_title": "Blonde"},
    {"artist_name": "Boris", "release_title": "Pink"}
  ]
}

Rules:
- Items must be in the order they appear in the source (top to bottom for lists; numbered or unranked both fine).
- For album lists, every item has artist_name + release_title. For pure artist lists, omit release_title.
- Use the canonical spelling the source uses (e.g. "Kendrick Lamar" not "kendrick lamar"). Do not invent capitalizations.
- If the source has both ranked + unranked sections, extract ranked first then unranked in source order.
- "source" should be the article's title or list name (e.g. "Pitchfork's 200 Best Albums of the 2010s"). Omit if unclear.
- "description" should summarize the editorial framing in 1-2 sentences if the article has one. Omit if it's just a list with no editorial.
- Skip any non-music entries (interludes, single tracks listed separately from their album, etc.).
- Cap the items array at ${MAX_ITEMS_PER_EXTRACTION} rows even if the source is longer — return the top N in source order.
- Return ONLY the JSON object, no explanation or markdown code blocks.`
}

interface ArtistSearchResult {
  artists: Array<{
    id: number
    name: string
    slug: string
  }>
  count: number
}

async function searchArtist(name: string): Promise<ArtistSearchResult | null> {
  try {
    const response = await fetch(
      `${BACKEND_URL}/artists/search?q=${encodeURIComponent(name)}`
    )
    if (!response.ok) return null
    return await response.json()
  } catch {
    return null
  }
}

/**
 * Match extracted items against the artists table. Sequential (not parallel)
 * so we don't fan out N=250 simultaneous backend requests at the upper end of
 * the cap. Backend search is cheap (~10ms) so the total is bounded at
 * canon-list scale (~2.5s at 250 items). If we ever hit 1000+ items, batch
 * the search calls via a new endpoint instead of parallelizing N HTTP fetches.
 */
async function matchItems(
  rawItems: Array<{ artist_name?: string; release_title?: string }>
): Promise<ExtractedCollectionItem[]> {
  const matched: ExtractedCollectionItem[] = []

  for (const raw of rawItems) {
    const artistName = typeof raw.artist_name === 'string' ? raw.artist_name.trim() : ''
    if (!artistName) continue

    const item: ExtractedCollectionItem = {
      artist_name: artistName,
      release_title:
        typeof raw.release_title === 'string' && raw.release_title.trim().length > 0
          ? raw.release_title.trim()
          : undefined,
    }

    const searchResult = await searchArtist(artistName)
    if (searchResult && Array.isArray(searchResult.artists) && searchResult.artists.length > 0) {
      // Same-name-band collision guard (`feedback_human_verify_ai_entity_data.md`):
      // canon-list articles produce names like "Boris" that match multiple
      // distinct PH artists (Japanese drone band vs American hardcore band).
      // If MULTIPLE candidates exact-match (case-insensitive), don't
      // auto-pick — surface as Pick suggestions so the user verifies. Only
      // auto-match when there's exactly one candidate with the name.
      const exactMatches = searchResult.artists.filter(
        a => a.name.toLowerCase() === artistName.toLowerCase()
      )
      if (exactMatches.length === 1) {
        item.matched_artist_id = exactMatches[0].id
        item.matched_artist_name = exactMatches[0].name
        item.matched_artist_slug = exactMatches[0].slug
      } else {
        item.artist_suggestions = searchResult.artists.slice(0, 3).map(
          (a): MatchSuggestion => ({
            id: a.id,
            name: a.name,
            slug: a.slug,
          })
        )
      }
    }

    matched.push(item)
  }

  return matched
}

function parseExtractionResponse(text: string): Record<string, unknown> | null {
  try {
    return JSON.parse(text)
  } catch {
    const jsonMatch = text.match(/```(?:json)?\s*([\s\S]*?)```/)
    if (jsonMatch) {
      try {
        return JSON.parse(jsonMatch[1].trim())
      } catch {
        return null
      }
    }
    const objectMatch = text.match(/\{[\s\S]*\}/)
    if (objectMatch) {
      try {
        return JSON.parse(objectMatch[0])
      } catch {
        return null
      }
    }
    return null
  }
}

export async function POST(request: NextRequest) {
  const cookieStore = await cookies()
  const authCookie = cookieStore.get('auth_token')

  if (!authCookie) {
    return NextResponse.json<ExtractCollectionResponse>(
      { success: false, error: 'Authentication required' },
      { status: 401 }
    )
  }

  const profile = await getAuthenticatedUser(authCookie.value)
  if (!profile?.success) {
    return NextResponse.json<ExtractCollectionResponse>(
      { success: false, error: 'Authentication required' },
      { status: 401 }
    )
  }

  const apiKey = process.env.ANTHROPIC_API_KEY
  if (!apiKey) {
    const error = new Error('ANTHROPIC_API_KEY not configured')
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'extract-collection' },
    })
    return NextResponse.json<ExtractCollectionResponse>(
      { success: false, error: 'AI service not configured' },
      { status: 503 }
    )
  }

  let body: ExtractCollectionRequest
  try {
    body = await request.json()
  } catch {
    return NextResponse.json<ExtractCollectionResponse>(
      { success: false, error: 'Invalid request body' },
      { status: 400 }
    )
  }

  const validMediaTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp']

  if (body.type === 'text') {
    if (!body.text || body.text.trim().length === 0) {
      return NextResponse.json<ExtractCollectionResponse>(
        { success: false, error: 'Text content is required' },
        { status: 400 }
      )
    }
    if (body.text.length > MAX_TEXT_LENGTH) {
      return NextResponse.json<ExtractCollectionResponse>(
        {
          success: false,
          error: `Text content exceeds maximum length of ${MAX_TEXT_LENGTH.toLocaleString()} characters`,
        },
        { status: 400 }
      )
    }
  } else if (body.type === 'image' || body.type === 'both') {
    if (!body.image_data) {
      return NextResponse.json<ExtractCollectionResponse>(
        { success: false, error: 'Image data is required' },
        { status: 400 }
      )
    }
    if (!body.media_type) {
      return NextResponse.json<ExtractCollectionResponse>(
        { success: false, error: 'Image media type is required' },
        { status: 400 }
      )
    }
    if (!validMediaTypes.includes(body.media_type)) {
      return NextResponse.json<ExtractCollectionResponse>(
        {
          success: false,
          error: `Invalid image type. Supported formats: ${validMediaTypes.join(', ')}`,
        },
        { status: 400 }
      )
    }
    if (body.type === 'both' && body.text && body.text.length > MAX_TEXT_LENGTH) {
      return NextResponse.json<ExtractCollectionResponse>(
        {
          success: false,
          error: `Text content exceeds maximum length of ${MAX_TEXT_LENGTH.toLocaleString()} characters`,
        },
        { status: 400 }
      )
    }
  } else {
    return NextResponse.json<ExtractCollectionResponse>(
      { success: false, error: 'Invalid request type. Use "text", "image", or "both"' },
      { status: 400 }
    )
  }

  try {
    const anthropic = new Anthropic({ apiKey })

    let userContent: Anthropic.MessageParam['content']

    if (body.type === 'text') {
      userContent = body.text!
    } else if (body.type === 'image') {
      userContent = [
        {
          type: 'image',
          source: {
            type: 'base64',
            media_type: body.media_type!,
            data: body.image_data!,
          },
        },
        {
          type: 'text',
          text: 'Extract the list of musical works (artists / releases) from this image. Items in source order.',
        },
      ]
    } else {
      const contextText = body.text?.trim()
        ? `Extract the list of musical works (artists / releases) from this image. Items in source order. Additional context from user: ${body.text}`
        : 'Extract the list of musical works (artists / releases) from this image. Items in source order.'

      userContent = [
        {
          type: 'image',
          source: {
            type: 'base64',
            media_type: body.media_type!,
            data: body.image_data!,
          },
        },
        {
          type: 'text',
          text: contextText,
        },
      ]
    }

    // 8192 vs extract-show's 1024 — canon lists at the 250-item upper end
    // produce ~25 tokens of JSON per row plus envelope, so a 200-item
    // Pitchfork list runs ~5,500-6,500 tokens of response. 4096 truncates
    // mid-JSON and parseExtractionResponse silently fails. Haiku's actual
    // ceiling is much higher; 8192 leaves headroom without changing cost
    // semantics (we're billed on actual output tokens, not the ceiling).
    const response = await anthropic.messages.create({
      model: 'claude-haiku-4-5-20251001',
      max_tokens: 8192,
      system: getExtractionSystemPrompt(),
      messages: [
        {
          role: 'user',
          content: userContent,
        },
      ],
    })

    let responseText = ''
    for (const block of response.content) {
      if (block.type === 'text') {
        responseText += block.text
      }
    }

    const parsed = parseExtractionResponse(responseText)
    if (!parsed) {
      return NextResponse.json<ExtractCollectionResponse>(
        {
          success: false,
          error: 'Failed to parse AI response',
          warnings: ['The AI response could not be parsed as JSON. Please try again.'],
        },
        { status: 500 }
      )
    }

    const warnings: string[] = []
    const rawItems = Array.isArray(parsed.items) ? parsed.items : []
    if (rawItems.length === 0) {
      warnings.push('No items were found in the input')
    }

    // Cap server-side too — defends against an over-eager model returning
    // 1000 rows when we asked for 250.
    const cappedItems = rawItems.slice(0, MAX_ITEMS_PER_EXTRACTION)
    if (rawItems.length > MAX_ITEMS_PER_EXTRACTION) {
      warnings.push(
        `Extracted ${rawItems.length} items; truncated to the first ${MAX_ITEMS_PER_EXTRACTION}.`
      )
    }

    const matched = await matchItems(cappedItems)

    const matchedCount = matched.filter(m => m.matched_artist_id).length
    const suggestionCount = matched.filter(
      m => !m.matched_artist_id && m.artist_suggestions?.length
    ).length
    const newCount = matched.length - matchedCount - suggestionCount
    if (newCount > 0 || suggestionCount > 0) {
      const parts: string[] = []
      if (matchedCount > 0) parts.push(`${matchedCount} matched`)
      if (suggestionCount > 0) parts.push(`${suggestionCount} with suggestions`)
      if (newCount > 0) parts.push(`${newCount} new`)
      warnings.push(`Items: ${parts.join(', ')}`)
    }

    const extractedData: ExtractedCollectionData = {
      source: typeof parsed.source === 'string' ? parsed.source : undefined,
      description: typeof parsed.description === 'string' ? parsed.description : undefined,
      items: matched,
    }

    return NextResponse.json<ExtractCollectionResponse>({
      success: true,
      data: extractedData,
      warnings: warnings.length > 0 ? warnings : undefined,
    })
  } catch (error) {
    console.error('Collection extraction error:', error)

    if (error instanceof Anthropic.APIError) {
      const message = error.message.toLowerCase()
      if (message.includes('credit') || message.includes('billing') || message.includes('balance')) {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'extract-collection', error_type: 'credits_exhausted' },
        })
        return NextResponse.json<ExtractCollectionResponse>(
          {
            success: false,
            error: 'AI service temporarily unavailable. Please try again later.',
          },
          { status: 503 }
        )
      }

      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'extract-collection', error_type: 'api_error' },
      })
      return NextResponse.json<ExtractCollectionResponse>(
        {
          success: false,
          error: 'AI service error. Please try again.',
        },
        { status: 503 }
      )
    }

    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'extract-collection' },
    })
    return NextResponse.json<ExtractCollectionResponse>(
      {
        success: false,
        error: 'An unexpected error occurred. Please try again.',
      },
      { status: 500 }
    )
  }
}
