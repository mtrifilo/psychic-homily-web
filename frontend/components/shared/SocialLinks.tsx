'use client'

import { Globe, Instagram, Facebook, Twitter, Youtube } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  renderableSocialLinks,
  type SocialLinkPlatform,
  type SocialLinkValues,
} from '@/lib/socialLinks'

interface SocialLinksProps {
  social?: SocialLinkValues | null
  className?: string
}

// Custom Spotify icon since lucide-react doesn't have one
function SpotifyIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <path d="M12 0C5.4 0 0 5.4 0 12s5.4 12 12 12 12-5.4 12-12S18.66 0 12 0zm5.521 17.34c-.24.359-.66.48-1.021.24-2.82-1.74-6.36-2.101-10.561-1.141-.418.122-.779-.179-.899-.539-.12-.421.18-.78.54-.9 4.56-1.021 8.52-.6 11.64 1.32.42.18.479.659.301 1.02zm1.44-3.3c-.301.42-.841.6-1.262.3-3.239-1.98-8.159-2.58-11.939-1.38-.479.12-1.02-.12-1.14-.6-.12-.48.12-1.021.6-1.141C9.6 9.9 15 10.561 18.72 12.84c.361.181.54.78.241 1.2zm.12-3.36C15.24 8.4 8.82 8.16 5.16 9.301c-.6.179-1.2-.181-1.38-.721-.18-.601.18-1.2.72-1.381 4.26-1.26 11.28-1.02 15.721 1.621.539.3.719 1.02.419 1.56-.299.421-1.02.599-1.559.3z" />
    </svg>
  )
}

// Custom Bandcamp icon
function BandcampIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <path d="M0 18.75l7.437-13.5H24l-7.438 13.5H0z" />
    </svg>
  )
}

// Custom SoundCloud icon
function SoundCloudIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <path d="M1.175 12.225c-.051 0-.094.046-.101.1l-.233 2.154.233 2.105c.007.058.05.098.101.098.05 0 .09-.04.099-.098l.255-2.105-.27-2.154c-.009-.06-.052-.1-.102-.1m-.899.828c-.06 0-.091.037-.104.094L0 14.479l.165 1.308c.014.057.045.094.09.094.043 0 .073-.037.085-.094l.195-1.308-.196-1.332c-.009-.057-.043-.094-.086-.094m1.83-1.229c-.059 0-.105.043-.112.109l-.21 2.563.21 2.458c.007.066.053.108.112.108.054 0 .1-.043.11-.108l.24-2.458-.24-2.563c-.01-.066-.056-.109-.11-.109m.89-.271c-.07 0-.12.047-.129.117l-.195 2.834.195 2.746c.009.07.059.117.129.117.069 0 .116-.046.126-.117l.224-2.746-.224-2.834c-.01-.07-.057-.117-.126-.117m.927-.239c-.076 0-.132.052-.14.127l-.178 3.073.178 2.956c.008.074.064.125.14.125.073 0 .128-.051.137-.125l.203-2.956-.203-3.073c-.009-.075-.063-.127-.137-.127m.935-.144c-.083 0-.14.057-.147.134l-.163 3.217.163 3.06c.007.08.064.133.147.133.08 0 .14-.053.148-.133l.184-3.06-.184-3.217c-.009-.077-.068-.134-.148-.134m.94-.04c-.089 0-.151.062-.16.141l-.149 3.257.149 3.048c.009.084.071.14.16.14.086 0 .148-.056.158-.14l.168-3.048-.168-3.257c-.01-.079-.072-.14-.158-.14m.94.026c-.094 0-.161.069-.168.149l-.135 3.19.135 3.014c.007.084.074.149.168.149.093 0 .16-.065.168-.149l.152-3.014-.152-3.19c-.008-.08-.075-.149-.168-.149m.966.088c-.097 0-.171.074-.179.158l-.12 3.144.12 2.997c.008.089.082.159.179.159.097 0 .171-.07.179-.159l.136-2.997-.136-3.144c-.008-.084-.082-.158-.179-.158m1.946-.206c-.1 0-.188.08-.188.188v6.063c0 .103.088.188.188.188h9.062c2.25 0 4.062-1.828 4.062-4.094s-1.812-4.094-4.062-4.094c-.562 0-1.094.125-1.578.344-.484-2.797-2.937-4.938-5.906-4.938-1.5 0-2.875.562-3.922 1.484-.312.281-.375.531-.375.797v6.063c.004.1.038.184.141.184" />
    </svg>
  )
}

/**
 * The glyph each platform is drawn with. It is keyed by the same platform keys
 * the gate registry declares, so a platform added there without a glyph fails
 * to typecheck rather than rendering an empty button.
 */
const SOCIAL_ICONS: Record<
  SocialLinkPlatform,
  React.ComponentType<{ className?: string }>
> = {
  website: Globe,
  instagram: Instagram,
  facebook: Facebook,
  twitter: Twitter,
  youtube: Youtube,
  spotify: SpotifyIcon,
  bandcamp: BandcampIcon,
  soundcloud: SoundCloudIcon,
}

/**
 * The stored social columns as outbound links, one glyph each.
 *
 * The href, the label and the order all come from `renderableSocialLinks`, so
 * this renders only what the host anchor accepts: a stored value whose host is
 * not the platform's produces no button at all, rather than an arbitrary
 * destination under a name the reader trusts. Six surfaces mount this component
 * with a raw contributor-writable column, so the gate lives here rather than at
 * the callers, where the next caller would silently skip it.
 */
export function SocialLinks({ social, className }: SocialLinksProps) {
  const links = renderableSocialLinks(social)
  if (links.length === 0) return null

  return (
    <div className={className}>
      <div className="flex flex-wrap gap-2">
        {links.map(({ platform, label, href }) => {
          const Icon = SOCIAL_ICONS[platform]

          return (
            <Button
              key={platform}
              variant="outline"
              size="icon"
              asChild
              className="h-9 w-9"
            >
              <a
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                title={label}
              >
                <Icon className="h-4 w-4" />
                <span className="sr-only">{label}</span>
              </a>
            </Button>
          )
        })}
      </div>
    </div>
  )
}
