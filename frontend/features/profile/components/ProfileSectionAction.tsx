'use client'

import Link from 'next/link'

interface ProfileSectionActionProps {
  /** Visible label, e.g. "Edit", "Manage", "View all →". */
  label: string
  /** Navigation target. When provided renders a <Link>; otherwise a button. */
  href?: string
  onClick?: () => void
  ariaLabel?: string
}

/**
 * The plain orange section action the profile redesign boards use in
 * main-column section headers ("Edit" / "Manage" / "View all →"). The boards
 * draw these as unbracketed primary-colored text links — deliberately NOT the
 * entity-page BracketLink treatment (PSY-1062).
 */
export function ProfileSectionAction({
  label,
  href,
  onClick,
  ariaLabel,
}: ProfileSectionActionProps) {
  const className = 'text-sm font-medium text-primary hover:underline'

  if (href) {
    return (
      <Link href={href} className={className} aria-label={ariaLabel}>
        {label}
      </Link>
    )
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className={className}
      aria-label={ariaLabel}
    >
      {label}
    </button>
  )
}
