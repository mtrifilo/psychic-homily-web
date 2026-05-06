'use client'

import Link from 'next/link'

/**
 * Props for {@link UserAttribution}.
 *
 * The component takes the resolved display fields straight from the API and
 * decides whether to render a profile link. It deliberately does not perform
 * any name-resolution itself — that is the backend's job, via the canonical
 * `ResolveUserName` / `ResolveUserUsername` chain (PSY-612). Frontend
 * components should pass through the server fields verbatim.
 */
export interface UserAttributionProps {
  /**
   * Display name as resolved by the backend. Should be non-empty for users
   * with ID > 0; the canonical resolver chain falls back through username,
   * first/last name, email-prefix, and finally "Anonymous", so this string
   * is expected to be ready to render as-is.
   *
   * When undefined or empty, {@link fallback} is used instead.
   */
  name?: string | null

  /**
   * URL-safe username slug. When set AND {@link linkable} is true, the byline
   * renders as a `<Link href="/users/${username}">`. When null or undefined,
   * the byline renders as plain `<span>` — the user has no public profile to
   * link to.
   *
   * The backend distinguishes nil-vs-set deliberately; do not coerce undefined
   * to an empty string upstream or the link gate breaks.
   */
  username?: string | null

  /**
   * Terminal label when neither {@link name} nor {@link username} is available.
   * Defaults to "Anonymous" — matches the backend `ResolveUserName` terminal.
   */
  fallback?: string

  /**
   * When false, the username link is suppressed even if {@link username} is
   * set. Useful inside cards that are themselves wrapped in an outer link
   * where nesting two `<a>` elements would trip Playwright strict-mode
   * resolution (see CLAUDE.md memory entry on "One link per entity card").
   *
   * Defaults to true.
   */
  linkable?: boolean

  /**
   * Optional className applied to the rendered element (the `<Link>` when
   * linked, the `<span>` when plain). Lets call sites preserve the byline
   * styling they had before adopting this primitive.
   */
  className?: string

  /**
   * Optional `data-testid` for E2E / unit tests. Forwarded onto the rendered
   * element regardless of the linked / plain branch.
   */
  testId?: string
}

/**
 * Renders a user attribution byline with a single, consistent rule:
 *
 *   - If `username` is set AND `linkable` is true (default), render
 *     `<Link href="/users/${username}">{name ?? fallback}</Link>`.
 *   - Otherwise render plain `<span>{name ?? fallback}</span>`.
 *
 * The component never renders `User #${id}` debug fallbacks — those leak an
 * internal DB row id and read like placeholder content. The backend's
 * `ResolveUserName` chain (PSY-612 / `services/shared/user_resolver.go`)
 * guarantees `name` is non-empty for any user with `ID > 0`; for anonymous /
 * absent users we render {@link fallback} (default "Anonymous").
 *
 * Adoption history: PSY-353 introduced linkable bylines on collection cards;
 * PSY-560 extended the "plain when nil" gate to the suggest-edit + comment
 * paths. PSY-613 extracts the inline pattern into this primitive so every
 * subsequent attribution surface gets the same render path for free.
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
