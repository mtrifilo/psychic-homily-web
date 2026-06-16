import { bandcampEmbedSrc } from '@/lib/bandcamp'

interface BandcampProps {
  album?: string
  track?: string
  artist: string
  title: string
  size?: 'large' | 'small'
  artwork?: 'small' | 'big'
  bgcol?: string
  linkcol?: string
  tracklist?: 'true' | 'false'
  height?: string
}

export function Bandcamp({
  album,
  track,
  artist,
  title,
  size = 'large',
  artwork = 'small',
  bgcol = 'ffffff',
  linkcol = '0687f5',
  tracklist = 'false',
  height = '120',
}: BandcampProps) {
  // A Bandcamp embed is an album OR a track; authors pass exactly one prop.
  const embedUrl = bandcampEmbedSrc({
    kind: album ? 'album' : 'track',
    id: album ?? track ?? '',
    size,
    bgcol,
    linkcol,
    artwork,
    tracklist: tracklist === 'true',
    transparent: true,
  })

  // Build the link URL. `artist` is an author-authored Bandcamp subdomain slug
  // (encodeURIComponent is invalid in a hostname), so only the title path
  // segment is encoded — keeps the href well-formed if a title isn't pre-slugged.
  const linkType = album ? 'album' : 'track'
  const linkUrl = `https://${artist}.bandcamp.com/${linkType}/${encodeURIComponent(title)}`

  return (
    <iframe
      title={`${title} by ${artist}`}
      style={{ border: 0, width: '100%', height: `${height}px` }}
      src={embedUrl}
      seamless
    >
      <a href={linkUrl}>
        {title} by {artist}
      </a>
    </iframe>
  )
}

