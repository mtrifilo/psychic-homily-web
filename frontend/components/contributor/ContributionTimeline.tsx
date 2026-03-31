'use client'

import Link from 'next/link'
import {
  Calendar,
  MapPin,
  Disc3,
  Tag,
  Tent,
  Mic2,
  PenLine,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ContributionEntry } from '@/features/auth'
import { formatRelativeTime } from '@/lib/formatRelativeTime'

const entityTypeIcons: Record<string, LucideIcon> = {
  show: Calendar,
  venue: MapPin,
  release: Disc3,
  label: Tag,
  festival: Tent,
  artist: Mic2,
}

function getEntityIcon(entityType: string): LucideIcon {
  return entityTypeIcons[entityType] || PenLine
}

function getEntityLink(entry: ContributionEntry): string | null {
  // Build a link to the entity if possible
  const entityName = entry.entity_name
  if (!entityName) return null

  switch (entry.entity_type) {
    case 'show':
    case 'venue':
    case 'artist':
    case 'release':
    case 'label':
    case 'festival':
      return `/${entry.entity_type}s/${entry.entity_id}`
    default:
      return null
  }
}

function formatAction(action: string): string {
  return action
    .replace(/_/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
}

interface ContributionTimelineProps {
  contributions: ContributionEntry[]
}

export function ContributionTimeline({ contributions }: ContributionTimelineProps) {
  if (contributions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No recent contributions.
      </p>
    )
  }

  return (
    <div className="space-y-1">
      {contributions.map(entry => {
        const Icon = getEntityIcon(entry.entity_type)
        const link = getEntityLink(entry)

        return (
          <div
            key={entry.id}
            className="flex items-start gap-3 py-2.5 px-3 rounded-lg hover:bg-muted/30 transition-colors"
          >
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted">
              <Icon className="h-4 w-4 text-muted-foreground" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm">
                <span className="text-muted-foreground">
                  {formatAction(entry.action)}
                </span>{' '}
                {entry.entity_name && link ? (
                  <Link
                    href={link}
                    className="font-medium hover:underline"
                  >
                    {entry.entity_name}
                  </Link>
                ) : entry.entity_name ? (
                  <span className="font-medium">{entry.entity_name}</span>
                ) : (
                  <span className="text-muted-foreground">
                    {entry.entity_type} #{entry.entity_id}
                  </span>
                )}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">
                {formatRelativeTime(entry.created_at, { short: true })}
                {entry.source && entry.source !== 'web' && (
                  <span> &middot; via {entry.source}</span>
                )}
              </p>
            </div>
          </div>
        )
      })}
    </div>
  )
}
