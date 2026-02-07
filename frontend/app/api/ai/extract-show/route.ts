import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import * as Sentry from '@sentry/nextjs'
import type {
  ExtractShowRequest,
  ExtractShowResponse,
  ExtractedShowData,
  ExtractedArtist,
  ExtractedVenue,
  MatchSuggestion,
  VenueMatchSuggestion,
} from '@/lib/types/extraction'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

// System prompt for Claude to extract show information
const EXTRACTION_SYSTEM_PROMPT = `You are a show information extractor. Given text or an image of a show flyer, extract structured information.

Output ONLY valid JSON with no additional text or markdown formatting:
{
  "artists": [{"name": "Artist Name", "is_headliner": true}],
  "venue": {"name": "Venue Name", "city": "City", "state": "AZ"},
  "date": "YYYY-MM-DD",
  "time": "HH:MM",
  "cost": "$20",
  "ages": "21+"
}

Rules:
- First artist listed is usually the headliner (is_headliner: true), others are is_headliner: false
- Convert dates to YYYY-MM-DD format (assume current year if not specified)
- Convert time to 24-hour format (default to 20:00 if "doors" time is given but show time is ambiguous)
- State should be 2-letter abbreviation (default to AZ for Arizona venues)
- Omit fields if not found (don't include null or empty values)
- For cost, include the dollar sign if it's a paid show, or use "Free" if explicitly stated as free
- For ages, common formats are "21+", "18+", "All Ages"
- If multiple dates are shown, extract only the first/primary date
- Return ONLY the JSON object, no explanation or markdown code blocks`

interface UserProfile {
  success: boolean
  user?: {
    id: string
    email?: string
  }
}

interface ArtistSearchResult {
  artists: Array<{
    id: number
    name: string
    slug: string
  }>
  count: number
}

interface VenueSearchResult {
  venues: Array<{
    id: number
    name: string
    slug: string
    city: string
    state: string
  }>
  count: number
}

/**
 * Verify the user is authenticated (any user, not admin-only)
 */
async function getAuthenticatedUser(
  authToken: string
): Promise<UserProfile | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/auth/profile`, {
      headers: {
        Cookie: `auth_token=${authToken}`,
      },
    })

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch {
    return null
  }
}

/**
 * Search for an artist in the database
 */
async function searchArtist(name: string): Promise<ArtistSearchResult | null> {
  try {
    const response = await fetch(
      `${BACKEND_URL}/artists/search?q=${encodeURIComponent(name)}`
    )

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch {
    return null
  }
}

/**
 * Search for a venue in the database
 */
async function searchVenue(name: string): Promise<VenueSearchResult | null> {
  try {
    const response = await fetch(
      `${BACKEND_URL}/venues/search?q=${encodeURIComponent(name)}`
    )

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch {
    return null
  }
}

/**
 * Match extracted artists against the database
 */
async function matchArtists(
  rawArtists: Array<{ name: string; is_headliner?: boolean }>
): Promise<ExtractedArtist[]> {
  const matchedArtists: ExtractedArtist[] = []

  for (const artist of rawArtists) {
    const result: ExtractedArtist = {
      name: artist.name,
      is_headliner: artist.is_headliner ?? false,
    }

    // Try to find an exact match in the database
    const searchResult = await searchArtist(artist.name)
    if (searchResult && searchResult.artists.length > 0) {
      // Look for an exact match (case-insensitive)
      const exactMatch = searchResult.artists.find(
        a => a.name.toLowerCase() === artist.name.toLowerCase()
      )

      if (exactMatch) {
        result.matched_id = exactMatch.id
        result.matched_name = exactMatch.name
        result.matched_slug = exactMatch.slug
      } else {
        // No exact match — include top 3 results as suggestions
        result.suggestions = searchResult.artists.slice(0, 3).map(
          (a): MatchSuggestion => ({
            id: a.id,
            name: a.name,
            slug: a.slug,
          })
        )
      }
    }

    matchedArtists.push(result)
  }

  return matchedArtists
}

/**
 * Match extracted venue against the database
 */
async function matchVenue(
  rawVenue: { name: string; city?: string; state?: string } | undefined
): Promise<ExtractedVenue | undefined> {
  if (!rawVenue || !rawVenue.name) {
    return undefined
  }

  const result: ExtractedVenue = {
    name: rawVenue.name,
    city: rawVenue.city,
    state: rawVenue.state,
  }

  // Try to find an exact match in the database
  const searchResult = await searchVenue(rawVenue.name)
  if (searchResult && searchResult.venues.length > 0) {
    // Look for an exact match (case-insensitive)
    const exactMatch = searchResult.venues.find(
      v => v.name.toLowerCase() === rawVenue.name.toLowerCase()
    )

    if (exactMatch) {
      result.matched_id = exactMatch.id
      result.matched_name = exactMatch.name
      result.matched_slug = exactMatch.slug
      // Use database city/state if we matched
      result.city = exactMatch.city
      result.state = exactMatch.state
    } else {
      // No exact match — include top 3 results as suggestions
      result.suggestions = searchResult.venues.slice(0, 3).map(
        (v): VenueMatchSuggestion => ({
          id: v.id,
          name: v.name,
          slug: v.slug,
          city: v.city,
          state: v.state,
        })
      )
    }
  }

  return result
}

/**
 * Parse Claude's JSON response
 */
function parseExtractionResponse(text: string): Record<string, unknown> | null {
  try {
    // Try direct JSON parse first
    return JSON.parse(text)
  } catch {
    // Try to extract JSON from markdown code block
    const jsonMatch = text.match(/```(?:json)?\s*([\s\S]*?)```/)
    if (jsonMatch) {
      try {
        return JSON.parse(jsonMatch[1].trim())
      } catch {
        return null
      }
    }

    // Try to find JSON object in the response
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
  // Check authentication
  const cookieStore = await cookies()
  const authCookie = cookieStore.get('auth_token')

  if (!authCookie) {
    return NextResponse.json<ExtractShowResponse>(
      { success: false, error: 'Authentication required' },
      { status: 401 }
    )
  }

  // Verify user is authenticated (any user, not admin-only)
  const profile = await getAuthenticatedUser(authCookie.value)
  if (!profile?.success) {
    return NextResponse.json<ExtractShowResponse>(
      { success: false, error: 'Authentication required' },
      { status: 401 }
    )
  }

  // Check if Anthropic API key is configured
  const apiKey = process.env.ANTHROPIC_API_KEY
  if (!apiKey) {
    const error = new Error('ANTHROPIC_API_KEY not configured')
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'extract-show' },
    })
    return NextResponse.json<ExtractShowResponse>(
      { success: false, error: 'AI service not configured' },
      { status: 503 }
    )
  }

  // Parse request body
  let body: ExtractShowRequest
  try {
    body = await request.json()
  } catch {
    return NextResponse.json<ExtractShowResponse>(
      { success: false, error: 'Invalid request body' },
      { status: 400 }
    )
  }

  // Validate request
  const validMediaTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp']

  if (body.type === 'text') {
    if (!body.text || body.text.trim().length === 0) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: 'Text content is required' },
        { status: 400 }
      )
    }
    if (body.text.length > 10000) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: 'Text content exceeds maximum length of 10,000 characters' },
        { status: 400 }
      )
    }
  } else if (body.type === 'image' || body.type === 'both') {
    // Image is required for both 'image' and 'both' types
    if (!body.image_data) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: 'Image data is required' },
        { status: 400 }
      )
    }
    if (!body.media_type) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: 'Image media type is required' },
        { status: 400 }
      )
    }
    if (!validMediaTypes.includes(body.media_type)) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: `Invalid image type. Supported formats: ${validMediaTypes.join(', ')}` },
        { status: 400 }
      )
    }
    // For 'both' type, validate text if provided
    if (body.type === 'both' && body.text && body.text.length > 10000) {
      return NextResponse.json<ExtractShowResponse>(
        { success: false, error: 'Text content exceeds maximum length of 10,000 characters' },
        { status: 400 }
      )
    }
  } else {
    return NextResponse.json<ExtractShowResponse>(
      { success: false, error: 'Invalid request type. Use "text", "image", or "both"' },
      { status: 400 }
    )
  }

  try {
    // Initialize Anthropic client
    const anthropic = new Anthropic({ apiKey })

    // Build the message content based on input type
    let userContent: Anthropic.MessageParam['content']

    if (body.type === 'text') {
      // Text only
      userContent = body.text!
    } else if (body.type === 'image') {
      // Image only with default prompt
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
          text: 'Extract show information from this flyer image.',
        },
      ]
    } else {
      // Both image and text - include user's additional context
      const contextText = body.text?.trim()
        ? `Extract show information from this flyer image. Additional context from user: ${body.text}`
        : 'Extract show information from this flyer image.'

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

    // Call Claude to extract information
    const response = await anthropic.messages.create({
      model: 'claude-haiku-4-5-20251001',
      max_tokens: 1024,
      system: EXTRACTION_SYSTEM_PROMPT,
      messages: [
        {
          role: 'user',
          content: userContent,
        },
      ],
    })

    // Extract text response
    let responseText = ''
    for (const block of response.content) {
      if (block.type === 'text') {
        responseText += block.text
      }
    }

    // Parse the JSON response
    const parsed = parseExtractionResponse(responseText)
    if (!parsed) {
      return NextResponse.json<ExtractShowResponse>(
        {
          success: false,
          error: 'Failed to parse AI response',
          warnings: ['The AI response could not be parsed as JSON. Please try again.'],
        },
        { status: 500 }
      )
    }

    const warnings: string[] = []

    // Extract and match artists
    const rawArtists = Array.isArray(parsed.artists) ? parsed.artists : []
    if (rawArtists.length === 0) {
      warnings.push('No artists were found in the input')
    }
    const matchedArtists = await matchArtists(rawArtists)

    // Track match statistics for warnings
    const matchedArtistCount = matchedArtists.filter(a => a.matched_id).length
    const suggestedArtistCount = matchedArtists.filter(a => !a.matched_id && a.suggestions?.length).length
    const newArtistCount = matchedArtists.length - matchedArtistCount - suggestedArtistCount
    if (newArtistCount > 0 || suggestedArtistCount > 0) {
      const parts: string[] = []
      if (matchedArtistCount > 0) parts.push(`${matchedArtistCount} matched`)
      if (suggestedArtistCount > 0) parts.push(`${suggestedArtistCount} with suggestions`)
      if (newArtistCount > 0) parts.push(`${newArtistCount} new`)
      warnings.push(`Artists: ${parts.join(', ')}`)
    }

    // Extract and match venue
    const rawVenue = parsed.venue as { name: string; city?: string; state?: string } | undefined
    const matchedVenue = await matchVenue(rawVenue)

    if (matchedVenue && !matchedVenue.matched_id) {
      if (matchedVenue.suggestions?.length) {
        warnings.push(`Venue "${matchedVenue.name}" not found — similar venues available`)
      } else {
        warnings.push(`Venue "${matchedVenue.name}" will be created as new`)
      }
    }

    // Build the extracted data
    const extractedData: ExtractedShowData = {
      artists: matchedArtists,
      venue: matchedVenue,
      date: typeof parsed.date === 'string' ? parsed.date : undefined,
      time: typeof parsed.time === 'string' ? parsed.time : undefined,
      cost: typeof parsed.cost === 'string' ? parsed.cost : undefined,
      ages: typeof parsed.ages === 'string' ? parsed.ages : undefined,
      description: typeof parsed.description === 'string' ? parsed.description : undefined,
    }

    return NextResponse.json<ExtractShowResponse>({
      success: true,
      data: extractedData,
      warnings: warnings.length > 0 ? warnings : undefined,
    })
  } catch (error) {
    console.error('Show extraction error:', error)

    if (error instanceof Anthropic.APIError) {
      const message = error.message.toLowerCase()
      if (message.includes('credit') || message.includes('billing') || message.includes('balance')) {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'extract-show', error_type: 'credits_exhausted' },
        })
        return NextResponse.json<ExtractShowResponse>(
          {
            success: false,
            error: 'AI service temporarily unavailable. Please try again later.',
          },
          { status: 503 }
        )
      }

      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'extract-show', error_type: 'api_error' },
      })
      return NextResponse.json<ExtractShowResponse>(
        {
          success: false,
          error: 'AI service error. Please try again.',
        },
        { status: 503 }
      )
    }

    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'extract-show' },
    })
    return NextResponse.json<ExtractShowResponse>(
      {
        success: false,
        error: 'An unexpected error occurred. Please try again.',
      },
      { status: 500 }
    )
  }
}
