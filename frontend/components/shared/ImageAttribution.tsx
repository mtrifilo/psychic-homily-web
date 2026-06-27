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
  author,
  license,
}: {
  source?: string | null
  sourceUrl?: string | null
  kind?: Kind
  className?: string
  /** Photographer credit for a Commons (CC) image (PSY-1232). */
  author?: string | null
  /** License name for a Commons (CC) image, e.g. "CC BY-SA 4.0" (PSY-1232). */
  license?: string | null
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
  // Commons (CC): the license requires crediting the author + the specific license
  // alongside the source link. Render them when present; fall back to the plain
  // "via" form for a legacy/incomplete Commons record.
  if (source === 'commons' && license) {
    return (
      <p className={wrapClass}>
        {author ? `${noun(kind)}: ${author} · ` : ''}
        {license} · via {link}
      </p>
    )
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
