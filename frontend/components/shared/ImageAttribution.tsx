import { BracketLink } from './BracketLink'

/**
 * Caption that attributes a displayed image to its source provider (PSY-1175).
 *
 * Image providers (Spotify, Discogs, Cover Art Archive) require attribution + a
 * linkback when we display their hotlinked images. This renders the small caption
 * beneath a cover/photo, deriving the wording from the stored `source` id. Renders
 * nothing for an unknown/legacy source (null) or a plain contributor upload that
 * needs no provider credit.
 */

type Kind = 'cover' | 'photo' | 'image'

const PROVIDERS: Record<string, string> = {
  spotify: 'Spotify',
  discogs: 'Discogs',
  cover_art_archive: 'Cover Art Archive',
  commons: 'Wikimedia Commons',
  public_domain: 'Public domain',
  user: 'a contributor',
}

function noun(kind: Kind): string {
  return kind === 'cover' ? 'Cover' : kind === 'photo' ? 'Photo' : 'Image'
}

export function ImageAttribution({
  source,
  sourceUrl,
  kind = 'image',
  className,
}: {
  source?: string | null
  sourceUrl?: string | null
  kind?: Kind
  className?: string
}) {
  if (!source) return null
  const name = PROVIDERS[source]
  if (!name) return null // unknown source → no (mis)attribution

  const wrapClass = `text-xs text-muted-foreground ${className ?? ''}`
  const link = sourceUrl ? (
    <BracketLink label={`${name} ↗`} href={sourceUrl} ariaLabel={`${name} (opens in a new tab)`} />
  ) : (
    <span>{name}</span>
  )

  // Discogs requires the exact "Data provided by Discogs" phrasing.
  if (source === 'discogs') {
    return <p className={wrapClass}>Data provided by {link}</p>
  }
  // Non-provider sources: short credit, no "via".
  if (source === 'public_domain') {
    return <p className={wrapClass}>Public domain</p>
  }
  if (source === 'user') {
    return <p className={wrapClass}>Added by a contributor</p>
  }
  // Provider sources (spotify, cover_art_archive, commons): "Cover via [Name ↗]".
  return (
    <p className={wrapClass}>
      {noun(kind)} via {link}
    </p>
  )
}
