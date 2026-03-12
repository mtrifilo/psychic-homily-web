'use client'

import { MapPin, Navigation, Info } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface VenueLocationCardProps {
  name: string
  address?: string | null
  city: string
  state: string
  zipcode?: string | null
  verified?: boolean
  className?: string
}

/**
 * Build a Google Maps search query string for the venue.
 */
function getMapQuery(venue: VenueLocationCardProps): string {
  const parts: string[] = [venue.name]

  if (venue.address) {
    parts.push(venue.address)
  }

  parts.push(venue.city, venue.state)

  if (venue.zipcode) {
    parts.push(venue.zipcode)
  }

  return parts.join(', ')
}

/**
 * Build a Google Maps search URL for the venue.
 * Uses the venue name and address/city/state to search.
 */
function getGoogleMapsUrl(venue: VenueLocationCardProps): string {
  const query = encodeURIComponent(getMapQuery(venue))
  return `https://www.google.com/maps/search/?api=1&query=${query}`
}

/**
 * Build a Google Maps embed URL for the venue.
 * Uses the legacy embed format that works without an API key.
 */
function getGoogleMapsEmbedUrl(venue: VenueLocationCardProps): string {
  const query = encodeURIComponent(getMapQuery(venue))
  return `https://maps.google.com/maps?q=${query}&t=&z=15&ie=UTF8&iwloc=&output=embed`
}

/**
 * Format the address for display
 */
function formatAddress(venue: VenueLocationCardProps): string[] {
  const lines: string[] = []

  if (venue.address) {
    lines.push(venue.address)
  }

  const cityStateLine = [venue.city, venue.state].filter(Boolean).join(', ')
  if (venue.zipcode) {
    lines.push(`${cityStateLine} ${venue.zipcode}`)
  } else {
    lines.push(cityStateLine)
  }

  return lines
}

export function VenueLocationCard(props: VenueLocationCardProps) {
  const { className, verified = true } = props
  const addressLines = formatAddress(props)
  const mapsUrl = getGoogleMapsUrl(props)
  const embedUrl = getGoogleMapsEmbedUrl(props)

  // For unverified venues, only show city/state
  const showFullAddress = verified

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <MapPin className="h-4 w-4" />
          Location
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {showFullAddress ? (
          <>
            {/* Embedded Map - only for verified venues */}
            <div className="relative w-full h-[180px] lg:h-[280px] rounded-md overflow-hidden bg-muted">
              <iframe
                title="Venue location map"
                src={embedUrl}
                className="absolute inset-0 w-full h-full border-0"
                loading="lazy"
                referrerPolicy="no-referrer-when-downgrade"
              />
            </div>

            <div className="text-sm">
              {addressLines.map((line, index) => (
                <p key={index} className={index === 0 ? '' : 'text-muted-foreground'}>
                  {line}
                </p>
              ))}
            </div>

            <Button asChild className="w-full" variant="outline">
              <a
                href={mapsUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                <Navigation className="h-4 w-4 mr-2" />
                Get Directions
              </a>
            </Button>
          </>
        ) : (
          /* Unverified venue - show city only with tooltip */
          <div className="text-sm">
            <div className="flex items-center gap-1.5">
              <p>{props.city}, {props.state}</p>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Info className="h-4 w-4 text-muted-foreground cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Full address available after venue verification</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
