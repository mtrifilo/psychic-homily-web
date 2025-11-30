interface SoundCloudProps {
  url: string
  title?: string
  artist?: string
  artist_url?: string
  track_url?: string
}

export function SoundCloud({
  url,
  title,
  artist,
  artist_url,
  track_url,
}: SoundCloudProps) {
  return (
    <div className="mb-6">
      <iframe
        width="100%"
        height="166"
        scrolling="no"
        frameBorder="no"
        allow="autoplay"
        src={url}
        style={{ borderRadius: '3px' }}
        title={title || 'SoundCloud Player'}
      />
      {(artist || title) && (
        <div
          style={{
            fontSize: '10px',
            color: '#cccccc',
            lineBreak: 'anywhere',
            wordBreak: 'normal',
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
            fontFamily:
              'Interstate, Lucida Grande, Lucida Sans Unicode, Lucida Sans, Garuda, Verdana, Tahoma, sans-serif',
            fontWeight: 100,
          }}
        >
          {artist_url && artist ? (
            <a
              href={artist_url}
              title={artist}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: '#cccccc', textDecoration: 'none' }}
            >
              {artist}
            </a>
          ) : (
            artist
          )}
          {artist && title && ' · '}
          {track_url && title ? (
            <a
              href={track_url}
              title={title}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: '#cccccc', textDecoration: 'none' }}
            >
              {title}
            </a>
          ) : (
            title
          )}
        </div>
      )}
    </div>
  )
}

