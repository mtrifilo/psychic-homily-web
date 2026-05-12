'use client'

/**
 * NotificationList renders a vertical list of NotificationLogEntry rows.
 * Used by both the header bell popover (`variant="popover"`, compact) and
 * the `/notifications` inbox page (`variant="page"`, roomier). PSY-595.
 *
 * Rendering modes:
 *
 *  - Comment-driven rows (entity_type = comment_reply | comment_mention)
 *    show "<commenter> <verb> on <entity>" + a one-line excerpt, deep-
 *    linked to `comment_url` (anchors to #comment-<id>). The popover row
 *    auto-fires onItemClick on click so the parent can mark-as-read and
 *    close the popover.
 *  - Show-filter rows (entity_type = show, channel = email) show the
 *    filter name + an "Open show" link. Kept lightweight — the original
 *    inbox endpoint already supported these.
 *
 * Mark-read concern is OWNED BY THE PARENT (Bell / inbox page) via
 * onItemClick + the optional `markRead` callback. This component stays
 * presentational so the popover can mark-on-mount while the inbox page
 * can mark-on-click.
 */

import Link from 'next/link'
import { MessageCircle, AtSign, Calendar, BellRing, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatTimeAgo } from '../types'
import type { NotificationLogEntry } from '../types'

type Variant = 'popover' | 'page'

export interface NotificationListProps {
  /** Rows to render (already sorted newest-first by the server). */
  entries: NotificationLogEntry[]
  /** Visual variant. `popover` is dense, `page` is roomier. */
  variant?: Variant
  /** Optional callback fired when the user clicks a notification row. */
  onItemClick?: (entry: NotificationLogEntry) => void
}

export function NotificationList({
  entries,
  variant = 'page',
  onItemClick,
}: NotificationListProps) {
  if (entries.length === 0) {
    return (
      <div
        className={cn(
          'flex items-center justify-center text-sm text-muted-foreground',
          variant === 'popover' ? 'h-32 px-4' : 'h-40 rounded-lg border border-dashed border-border/50 px-4'
        )}
      >
        You&apos;re all caught up
      </div>
    )
  }

  return (
    <ul className="divide-y divide-border/50">
      {entries.map(entry => (
        <NotificationRow
          key={entry.id}
          entry={entry}
          variant={variant}
          onItemClick={onItemClick}
        />
      ))}
    </ul>
  )
}

interface RowProps {
  entry: NotificationLogEntry
  variant: Variant
  onItemClick?: (entry: NotificationLogEntry) => void
}

function NotificationRow({ entry, variant, onItemClick }: RowProps) {
  const isComment =
    entry.entity_type === 'comment_reply' ||
    entry.entity_type === 'comment_mention'
  const unread = entry.read_at == null

  const padding = variant === 'popover' ? 'px-3 py-2.5' : 'px-4 py-3'

  // Comment row — either comment_reply or comment_mention
  if (isComment) {
    const href = entry.comment_url ?? '#'
    const verb =
      entry.entity_type === 'comment_mention'
        ? 'mentioned you'
        : 'replied'
    const commenter = entry.commenter_name || 'Someone'
    const entityName = entry.comment_entity_name || 'a conversation'
    const Icon = entry.entity_type === 'comment_mention' ? AtSign : MessageCircle
    return (
      <li>
        <Link
          href={href}
          onClick={() => onItemClick?.(entry)}
          className={cn(
            'flex items-start gap-3 transition-colors hover:bg-accent/40',
            padding,
            unread && 'bg-accent/20'
          )}
        >
          <div
            className={cn(
              'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full',
              entry.entity_type === 'comment_mention'
                ? 'bg-primary/15 text-primary'
                : 'bg-muted text-muted-foreground'
            )}
            aria-hidden
          >
            <Icon className="h-3.5 w-3.5" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sm leading-snug">
              <span className="font-medium">{commenter}</span>{' '}
              <span className="text-muted-foreground">{verb} on</span>{' '}
              <span className="font-medium">{entityName}</span>
            </p>
            {entry.comment_excerpt && (
              <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                {entry.comment_excerpt}
              </p>
            )}
            <p className="mt-1 text-[11px] uppercase tracking-wide text-muted-foreground/70">
              {formatTimeAgo(entry.sent_at)}
            </p>
          </div>
          {unread && (
            <span
              className="mt-2 h-2 w-2 shrink-0 rounded-full bg-primary"
              aria-label="Unread"
            />
          )}
        </Link>
      </li>
    )
  }

  // Show-filter row — fall back to the legacy shape. There's currently no
  // surface that deep-links into the filter's matched show by ID alone
  // (the email is the canonical surface for those). We render a stub
  // pointing to /shows so the inbox page doesn't 404.
  const showHref = `/shows`
  return (
    <li>
      <Link
        href={showHref}
        onClick={() => onItemClick?.(entry)}
        className={cn(
          'flex items-start gap-3 transition-colors hover:bg-accent/40',
          padding,
          unread && 'bg-accent/20'
        )}
      >
        <div
          className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
          aria-hidden
        >
          {entry.filter_name ? (
            <BellRing className="h-3.5 w-3.5" />
          ) : (
            <Calendar className="h-3.5 w-3.5" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm leading-snug">
            {entry.filter_name ? (
              <>
                <span className="text-muted-foreground">New match for</span>{' '}
                <span className="font-medium">{entry.filter_name}</span>
              </>
            ) : (
              <span className="font-medium">{entry.entity_type}</span>
            )}
          </p>
          <p className="mt-1 flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground/70">
            {formatTimeAgo(entry.sent_at)}
            <ExternalLink className="h-3 w-3" />
          </p>
        </div>
        {unread && (
          <span
            className="mt-2 h-2 w-2 shrink-0 rounded-full bg-primary"
            aria-label="Unread"
          />
        )}
      </Link>
    </li>
  )
}
