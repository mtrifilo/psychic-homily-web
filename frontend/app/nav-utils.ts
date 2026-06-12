export const navLinks = [
  { href: '/shows', label: 'Shows' },
  { href: '/artists', label: 'Artists' },
  { href: '/venues', label: 'Venues' },
  { href: '/blog', label: 'Blog' },
  { href: '/dj-sets', label: 'DJ Sets' },
  {
    href: 'https://psychichomily.substack.com/',
    label: 'Substack',
    external: true,
    prefetch: false as const,
  },
  // /submissions = contributor pending-edits feedback loop (PSY-600).
  // Show submission has its own page at /shows/submit and is reachable
  // from the Sidebar / CommandPalette / Contribute dashboard.
  { href: '/submissions', label: 'My Submissions', prefetch: false as const },
]

export type NavLink = (typeof navLinks)[number]

export function isExternal(link: NavLink): boolean {
  return 'external' in link && link.external === true
}

export function getUserInitials(user: {
  display_name?: string
  first_name?: string
  last_name?: string
  email: string
}): string {
  // display_name leads, mirroring the attribution chain (PSY-1063) — it's
  // the only name the profile form edits now.
  if (user.display_name) {
    const parts = user.display_name.trim().split(/\s+/)
    const first = parts[0][0]?.toUpperCase() ?? ''
    const last = parts.length > 1 ? (parts[parts.length - 1][0]?.toUpperCase() ?? '') : ''
    return first + last || (user.email?.[0]?.toUpperCase() ?? '?')
  }
  if (user.first_name) {
    const first = user.first_name[0].toUpperCase()
    const last = user.last_name ? user.last_name[0].toUpperCase() : ''
    return first + last
  }
  return user.email?.[0]?.toUpperCase() || '?'
}

export function getUserDisplayName(user: {
  display_name?: string
  first_name?: string
  last_name?: string
}): string | null {
  if (user.display_name) return user.display_name
  if (user.first_name && user.last_name)
    return `${user.first_name} ${user.last_name}`
  if (user.first_name) return user.first_name
  return null
}
