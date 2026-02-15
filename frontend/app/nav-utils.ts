export const navLinks = [
  { href: '/shows', label: 'Shows' },
  { href: '/venues', label: 'Venues' },
  { href: '/blog', label: 'Blog' },
  { href: '/dj-sets', label: 'DJ Sets' },
  {
    href: 'https://psychichomily.substack.com/',
    label: 'Substack',
    external: true,
    prefetch: false as const,
  },
  { href: '/submissions', label: 'Submissions', prefetch: false as const },
]

export type NavLink = (typeof navLinks)[number]

export function isExternal(link: NavLink): boolean {
  return 'external' in link && link.external === true
}

export function getUserInitials(user: {
  first_name?: string
  last_name?: string
  email: string
}): string {
  if (user.first_name) {
    const first = user.first_name[0].toUpperCase()
    const last = user.last_name ? user.last_name[0].toUpperCase() : ''
    return first + last
  }
  return user.email?.[0]?.toUpperCase() || '?'
}

export function getUserDisplayName(user: {
  first_name?: string
  last_name?: string
}): string | null {
  if (user.first_name && user.last_name)
    return `${user.first_name} ${user.last_name}`
  if (user.first_name) return user.first_name
  return null
}
