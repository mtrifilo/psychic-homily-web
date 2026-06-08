import Link from 'next/link'

/**
 * LatestRadioShows (PSY-389) — three station preview cards on the logged-out
 * homepage (Figma `491:29`, section "Latest radio shows").
 *
 * Per the PSY-389 decision these cards LINK to `/radio` and DO NOT depend on
 * the Radio D2 station panel (PSY-1016, built in parallel) — coupling to that
 * in-flight component would conflict. Each card is a static newcomer-facing
 * teaser of one of the three real stations (KEXP / WFMU / NTS) that introduces
 * the station to someone who's never heard of it, then sends them to the full
 * `/radio` page to go deeper.
 *
 * The full live now-playing / vibe / recent-artists experience is the `/radio`
 * page itself; these cards are the lobby's invitation into it.
 */

interface StationPreview {
  /** Station call sign (KEXP / WFMU / NTS). */
  station: string
  /** Home city, shown next to the call sign. */
  city: string
  /** A representative show name. */
  showName: string
  /** One-line "what it's like" for a newcomer. */
  vibe: string
  /** A few representative artists, as a single middot-joined line. */
  artists: string
  /** Whether to flag the station as currently on air. */
  live?: boolean
}

const STATIONS: ReadonlyArray<StationPreview> = [
  {
    station: 'KEXP',
    city: 'Seattle',
    showName: 'Variety Mix',
    vibe: 'Eclectic host’s-choice',
    artists: 'Sleater-Kinney · Wipers · Unwound',
    live: true,
  },
  {
    station: 'WFMU',
    city: 'Jersey City',
    showName: 'Wake and Bake',
    vibe: 'Freeform, anything-goes',
    artists: 'Stereolab · Broadcast · Pram',
  },
  {
    station: 'NTS',
    city: 'London',
    showName: 'Charlie Bones',
    vibe: 'Global morning show',
    artists: 'Sun Ra · Alice Coltrane · Pharoah',
  },
]

function StationCard({ preview }: { preview: StationPreview }) {
  return (
    <Link
      href="/radio"
      aria-label={`${preview.station} · ${preview.city} — ${preview.showName}. Browse radio.`}
      className="flex flex-1 flex-col gap-[7px] rounded-xl border border-border bg-card p-[18px] transition-colors hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-[7px]">
          <span className="font-mono text-sm font-bold text-foreground">
            {preview.station}
          </span>
          <span className="text-xs text-muted-foreground">{preview.city}</span>
        </div>
        {preview.live && (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="text-[8px] text-primary" aria-hidden>
              ●
            </span>
            live
          </span>
        )}
      </div>
      <p className="text-[15px] font-semibold text-foreground">{preview.showName}</p>
      <p className="text-xs text-muted-foreground">{preview.vibe}</p>
      <p className="text-[13px] font-medium text-foreground">{preview.artists}</p>
    </Link>
  )
}

export function LatestRadioShows() {
  return (
    <section aria-labelledby="home-radio-heading" className="flex w-full flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2
          id="home-radio-heading"
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          Latest radio shows
        </h2>
        <Link
          href="/radio"
          className="text-sm font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
        >
          Browse all stations →
        </Link>
      </div>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-stretch">
        {STATIONS.map(preview => (
          <StationCard key={preview.station} preview={preview} />
        ))}
      </div>
    </section>
  )
}
