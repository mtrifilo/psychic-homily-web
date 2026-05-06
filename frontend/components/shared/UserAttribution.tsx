'use client'

import Link from 'next/link'

export interface UserAttributionProps {
  /**
   * Display name as resolved by the backend's `ResolveUserName` chain
   * (PSY-612). Should be non-empty for any user with ID > 0; falls through
   * to {@link fallback} when undefined or empty.
   */
  name?: string | null

  /**
   * URL-safe username slug. When set AND {@link linkable} is true, the byline
   * renders as a `<Link href="/users/${username}">`. Null / undefined renders
   * plain `<span>` — the user has no public profile to link to.
   *
   * The backend distinguishes nil-vs-set deliberately; do not coerce undefined
   * to an empty string upstream or the link gate breaks.
   */
  username?: string | null

  /** Terminal label when {@link name} is missing. Defaults to "Anonymous". */
  fallback?: string

  /**
   * Suppresses the link even when {@link username} is set. Useful inside
   * cards already wrapped in an outer `<Link>` — nesting two `<a>` elements
   * trips Playwright strict-mode resolution (CLAUDE.md "One link per entity
   * card"). Defaults to true.
   */
  linkable?: boolean

  className?: string

  /** Forwarded as `data-testid` onto the rendered element. */
  testId?: string
}

/**
 * Renders a user attribution byline. If `username` is set AND `linkable`
 * (default), renders `<Link href="/users/${username}">name</Link>`; otherwise
 * a plain `<span>`. Never renders `User #${id}` — the backend's canonical
 * resolver guarantees a usable display name, and leaking the internal DB row
 * id reads like placeholder content.
 */
export function UserAttribution({
  name,
  username,
  fallback = 'Anonymous',
  linkable = true,
  className,
  testId,
}: UserAttributionProps) {
  const displayName = name && name.length > 0 ? name : fallback
  const shouldLink = linkable && username && username.length > 0

  if (shouldLink) {
    return (
      <Link
        href={`/users/${username}`}
        className={className}
        data-testid={testId}
      >
        {displayName}
      </Link>
    )
  }

  return (
    <span className={className} data-testid={testId}>
      {displayName}
    </span>
  )
}
