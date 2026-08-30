'use client'

import Link from 'next/link'

export interface UserAttributionProps {
  /**
   * Display name as resolved by the backend.
   *
   * TWO FAMILIES OF CALLER, and they must not be confused (PSY-1940):
   *
   * - AUTHORED-CONTENT surfaces (comments, collections, the request board)
   *   resolve through `shared.ResolvePublicUserName`, which is never empty and
   *   bottoms out at "Anonymous". Their author slot has to say something, so
   *   {@link fallback} is a real terminal for them.
   * - CONTRIBUTION bylines (the show submitter credit, revision attribution)
   *   resolve through `shared.ResolvePublicContributorCredit`, which returns
   *   NOTHING for a contributor who hid their contributions or whose only
   *   resolvable name would come from their email address. Those callers must
   *   render no byline at all, and so must GUARD ON THE NAME BEFORE REACHING
   *   THIS COMPONENT — see ShowProvenanceLine's `datedCredit`, AttributionLine,
   *   and RevisionHistory. Handing an absent contribution name straight to this
   *   component would print "Anonymous", which asserts a person the backend
   *   deliberately declined to name.
   *
   * The default {@link fallback} therefore serves the first family only.
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

  /**
   * Terminal label when {@link name} is missing. Defaults to "Anonymous".
   *
   * Only meaningful for the authored-content family described on {@link name}.
   * A contribution byline must never rely on it: for those, an absent name means
   * "we may not say", and this default would answer with a person instead.
   */
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
 * a plain `<span>`. Never renders `User #${id}` — leaking the internal DB row
 * id reads like placeholder content, and the backend always resolves a label
 * for the surfaces that use this component's fallback.
 *
 * This component never decides whether a person may be NAMED; that is the
 * backend's call and, for contribution bylines, the caller's guard. See the
 * two families documented on {@link UserAttributionProps.name}.
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
