import Link from 'next/link'
import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { EN_DASH } from '../showArchive'
import { splitBill } from '../utils'

/** Stands in for a bill nobody has recorded. */
const ABSENT = EN_DASH

/**
 * The bill fields a dense show row renders.
 *
 * `set_type` and `is_headliner` are optional because not every endpoint serves
 * them: the artist-shows payload carries only id/slug/name, and `splitBill`
 * then reads the list in the order the API returned it, which is bill position
 * (`show_artists.position ASC`). That is the flyer reading, and the same answer
 * the flags would give for a correctly ordered bill.
 */
export interface ShowBillArtist {
  id: number
  slug: string
  name: string
  set_type?: string | null
  is_headliner?: boolean | null
}

function ArtistLink({
  artist,
  className,
}: {
  artist: ShowBillArtist
  className?: string
}) {
  // Unlinked without a slug. It is nullable in the DB and the API sends "" for
  // a missing one, and `/artists/` is not a 404 — it is the artists INDEX, so
  // an unguarded link would quietly take the reader off the page they are on
  // rather than failing visibly.
  if (!artist.slug) return <span className={className}>{artist.name}</span>
  return (
    <Link href={`/artists/${artist.slug}`} className={className}>
      {artist.name}
    </Link>
  )
}

export interface ShowBillProps {
  artists: ShowBillArtist[]
  /** Struck through, and suppresses the sold-out badge. */
  isCancelled: boolean
  isSoldOut: boolean
  /**
   * Rendered between the bill and the status badges, muted. The artist archive
   * puts the venue and city here, because its rows span venues; the venue
   * archive has nothing to add and omits it.
   */
  afterBill?: ReactNode
}

/**
 * Who played, in one dense table cell: the acts at the top in the foreground,
 * everyone under them as "w/ …", and the show's status as badges.
 *
 * Shared by the venue and artist archives (PSY-1753, PSY-1754) so "who
 * headlines this show" and "how a cancelled date reads" have one answer across
 * both, rather than two implementations that drift the first time either is
 * restyled.
 */
export function ShowBill({
  artists,
  isCancelled,
  isSoldOut,
  afterBill,
}: ShowBillProps) {
  const { headliners, support } = splitBill(artists)

  return (
    /* The badges sit OUTSIDE the bill branch on purpose. A show can reach this
       cell with an empty `artists` array (the backend's minimum-one validation
       tag is inert, and its artist resolution skips ids it cannot resolve), and
       a cancelled show with no bill is the one row where the status is the only
       thing the row has to say. */
    <span className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
      {headliners.length > 0 ? (
        <>
          <span
            className={cn(
              'font-medium text-foreground',
              isCancelled && 'line-through'
            )}
          >
            {headliners.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && ', '}
                <ArtistLink
                  artist={artist}
                  className="hover:text-primary hover:underline"
                />
              </span>
            ))}
          </span>
          {support.length > 0 && (
            <span className="text-muted-foreground">
              w/{' '}
              {support.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && ', '}
                  <ArtistLink
                    artist={artist}
                    className="hover:text-foreground hover:underline"
                  />
                </span>
              ))}
            </span>
          )}
        </>
      ) : (
        <span className="text-muted-foreground">{ABSENT}</span>
      )}
      {afterBill}
      {isCancelled && (
        <Badge variant="destructive" className="text-[10px]">
          CANCELLED
        </Badge>
      )}
      {/* A cancelled show's ticket status is moot, and two badges on one row
          read as two separate facts about a show that is not happening. */}
      {!isCancelled && isSoldOut && (
        <Badge variant="outline" className="text-[10px]">
          SOLD OUT
        </Badge>
      )}
    </span>
  )
}
