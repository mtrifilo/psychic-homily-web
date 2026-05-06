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
  // PSY-601: synthetic "_edit" discriminators reuse the underlying
  // entity icon so a "Suggested artist edit" row still gets the artist mic.
  venue_edit: MapPin,
  artist_edit: Mic2,
  release_edit: Disc3,
  label_edit: Tag,
  festival_edit: Tent,
}

function getEntityIcon(entityType: string): LucideIcon {
  return entityTypeIcons[entityType] || PenLine
}

function getEntityLink(entry: ContributionEntry): string | null {
  switch (entry.entity_type) {
    case 'show':
    case 'venue':
    case 'artist':
    case 'release':
    case 'label':
    case 'festival':
      return `/${entry.entity_type}s/${entry.entity_id}`
    case 'request':
      return `/requests/${entry.entity_id}`
    case 'collection':
      return `/collections/${entry.entity_id}`
    // PSY-601: pending_entity_edits surface with entity_type "<type>_edit"
    // (synthetic discriminator from the activity-feed UNION). Strip the
    // suffix so the row links back to the underlying entity.
    case 'venue_edit':
    case 'artist_edit':
    case 'release_edit':
    case 'label_edit':
    case 'festival_edit': {
      const baseType = entry.entity_type.slice(0, -'_edit'.length)
      return `/${baseType}s/${entry.entity_id}`
    }
    default:
      return null
  }
}

/**
 * Returns a human-readable label for the entity type, used as a fallback
 * when the backend doesn't return an entity name.
 */
const entityTypeLabels: Record<string, string> = {
  show: 'a show',
  venue: 'a venue',
  artist: 'an artist',
  release: 'a release',
  label: 'a label',
  festival: 'a festival',
  request: 'a request',
  collection: 'a collection',
  // PSY-601: synthetic "_edit" discriminators fall back to the underlying
  // entity copy. Used when entity_name is missing (e.g. entity hard-deleted
  // after the edit was submitted).
  venue_edit: 'a venue',
  artist_edit: 'an artist',
  release_edit: 'a release',
  label_edit: 'a label',
  festival_edit: 'a festival',
}

function getFallbackEntityLabel(entry: ContributionEntry): string {
  return entityTypeLabels[entry.entity_type] || entry.entity_type
}

/**
 * Maps raw action strings from the API into user-friendly display labels.
 * Actions come from audit_logs (e.g., "create", "report") and submission
 * sources (e.g., "submit_show", "submit_venue_edit").
 */
const actionLabels: Record<string, string> = {
  submit_show: 'Submitted show',
  submit_venue: 'Submitted venue',
  // PSY-601: actions emitted by the pending_entity_edits UNION. Without
  // these, the auto-formatted fallback ("Submit Artist Edit") leaked
  // through and looked like internal jargon.
  submit_artist_edit: 'Suggested artist edit',
  submit_venue_edit: 'Suggested venue edit',
  submit_release_edit: 'Suggested release edit',
  submit_label_edit: 'Suggested label edit',
  submit_festival_edit: 'Suggested festival edit',
  create: 'Created',
  update: 'Updated',
  delete: 'Deleted',
  report: 'Reported',
  suggest_edit: 'Suggested edit',
  approve: 'Approved',
  reject: 'Rejected',
  vote: 'Voted on',
  create_request: 'Created request',
  fulfill_request: 'Fulfilled request',
  create_collection: 'Created collection',
  update_collection: 'Updated collection',
  delete_collection: 'Deleted collection',
  add_collection_item: 'Added to collection',
  update_collection_item: 'Updated collection item',
  remove_collection_item: 'Removed from collection',
  set_collection_featured: 'Featured collection',
}

function formatAction(action: string): string {
  if (actionLabels[action]) return actionLabels[action]
  // Fallback: title-case with underscores replaced
  return action
    .replace(/_/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
}

/**
 * Sources that should not be displayed to users.
 * "web" is the default, "audit_log" and "submission" are internal labels.
 */
const hiddenSources = new Set(['web', 'audit_log', 'submission'])

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
                {entry.entity_name ? (
                  link ? (
                    <Link
                      href={link}
                      className="font-medium hover:underline"
                    >
                      {entry.entity_name}
                    </Link>
                  ) : (
                    <span className="font-medium">{entry.entity_name}</span>
                  )
                ) : link ? (
                  <Link
                    href={link}
                    className="text-muted-foreground hover:underline"
                  >
                    {getFallbackEntityLabel(entry)}
                  </Link>
                ) : (
                  <span className="text-muted-foreground">
                    {getFallbackEntityLabel(entry)}
                  </span>
                )}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">
                {formatRelativeTime(entry.created_at, { short: true })}
                {entry.source && !hiddenSources.has(entry.source) && (
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
