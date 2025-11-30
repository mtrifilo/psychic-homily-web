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
  // Build the embed URL
  const embedParts: string[] = []
  if (album) embedParts.push(`album=${album}`)
  if (track) embedParts.push(`track=${track}`)
  embedParts.push(`size=${size}`)
  embedParts.push(`bgcol=${bgcol}`)
  embedParts.push(`linkcol=${linkcol}`)
  embedParts.push(`tracklist=${tracklist}`)
  embedParts.push(`artwork=${artwork}`)
  embedParts.push('transparent=true')

  const embedUrl = `https://bandcamp.com/EmbeddedPlayer/${embedParts.join('/')}/`

  // Build the link URL
  const linkType = album ? 'album' : 'track'
  const linkUrl = `https://${artist}.bandcamp.com/${linkType}/${title}`

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

