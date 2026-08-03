import Link from 'next/link'

/**
 * The chrome the weekly and nightly city pages share.
 *
 * These two pages are deliberately siblings — each links to the other in its
 * header, so a reader walks between them in one click — which makes any visual
 * disagreement between them immediately obvious and slightly disorienting. Each
 * piece below had two copies before this module existed; the risk was never
 * that one copy was wrong, but that a later restyle would land on one page and
 * not the other.
 *
 * Deliberately NOT here: the show row. The week groups by day and leads with
 * the bill; the night is a schedule and leads with the time. Merging them would
 * mean a component with a mode switch, which is the thing this module exists to
 * avoid growing.
 */

/**
 * The header's adjacent-period chips. Each page adds its own width behaviour —
 * the week's three chips must share a row at mobile widths, the day's need not.
 */
export const SCENE_NAV_CHIP_CLASS =
  'rounded border border-border px-3 py-2 text-center font-mono text-xs text-muted-foreground transition-colors hover:bg-muted/50'

/** `Scenes / Chicago, IL` */
export function SceneBreadcrumb({ slug, sceneName }: { slug: string; sceneName: string }) {
  return (
    <nav aria-label="Breadcrumb" className="font-mono text-[11px] text-muted-foreground">
      <Link href="/scenes" className="hover:underline">
        Scenes
      </Link>
      {'  /  '}
      <Link href={`/scenes/${slug}`} className="hover:underline">
        {sceneName}
      </Link>
    </nav>
  )
}

/**
 * City at display scale, state in mono alongside.
 *
 * Both pages are built for cold arrivals from a shared link, where "Columbus"
 * or "Portland" are genuinely ambiguous — so the state has to be on the page,
 * not only in the breadcrumb. Setting it at display size would blunt the one
 * element that must survive a skim.
 */
export function SceneCityHeading({ city, state }: { city: string; state?: string | null }) {
  return (
    <h1 className="flex items-baseline gap-3 text-4xl font-bold tracking-tight md:text-5xl">
      {city}
      <span className="font-mono text-base font-normal tracking-wide text-muted-foreground">
        {state}
      </span>
    </h1>
  )
}

/** CANCELLED / SOLD OUT. Both are warnings, so both are destructive-toned. */
export function ShowStatusBadge({ label }: { label: string }) {
  return (
    <span className="shrink-0 rounded-sm border border-destructive px-1.5 py-px font-mono text-[10px] leading-4 tracking-wide text-destructive">
      {label}
    </span>
  )
}

/**
 * Enough to name a tracked room and, when we have it, link to its page here.
 *
 * Deliberately thinner than the day API's `SceneTrackedVenue`: the week payload
 * still sends bare names, and the footer's locked destination is `/venues/{slug}`
 * (PSY-1733) — an external website is never the href.
 */
export type TrackedRoom = {
  name: string
  slug?: string
}

/** One tracked room → `/venues/{slug}` when we have a slug, else plain text. */
export function RoomLink({ venue }: { venue: TrackedRoom }) {
  const slug = venue.slug?.trim()
  if (slug) {
    return (
      <Link
        href={`/venues/${slug}`}
        className="underline underline-offset-4 hover:text-primary"
      >
        {venue.name}
      </Link>
    )
  }
  // No page of its own: name it anyway. The list's job is to tell the reader
  // WHICH rooms this page speaks for, and dropping one would misstate the
  // coverage it is there to disclose.
  return <span>{venue.name}</span>
}

/** `A · B · C`, with each room linked when it has a slug. */
export function RoomList({ venues }: { venues: TrackedRoom[] }) {
  return (
    <p className="mt-2 text-sm leading-relaxed">
      {venues.map((venue, i) => (
        <span key={venue.slug || venue.name}>
          {i > 0 && <span className="text-muted-foreground"> · </span>}
          <RoomLink venue={venue} />
        </span>
      ))}
    </p>
  )
}

/**
 * The rooms a page speaks for, named in full.
 *
 * Load-bearing, not filler: coverage is a curated slice (11 rooms in Chicago,
 * not all of Chicago). A page that implied full city coverage would be false,
 * and a local would notice immediately.
 *
 * When a room has a slug it links to `/venues/{slug}` (PSY-1733). Week callers
 * that only have bare names still render unlinked — the week API has not yet
 * been enriched to `SceneTrackedVenue[]`.
 */
export function TrackedRoomsFooter({ city, rooms }: { city: string; rooms: TrackedRoom[] }) {
  if (rooms.length === 0) return null
  return (
    <footer className="mt-12">
      <div className="border-t-2 border-foreground" />
      <h2 className="pt-4 font-mono text-[11px] tracking-widest text-muted-foreground">
        ROOMS WE TRACK IN {city.toUpperCase()}
      </h2>
      <RoomList venues={rooms} />
      <Link
        href="/contribute"
        className="mt-2 inline-block text-sm text-muted-foreground hover:underline"
      >
        Missing a room? Suggest a venue →
      </Link>
    </footer>
  )
}
