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
 * The rooms a page speaks for, named in full.
 *
 * Load-bearing, not filler: coverage is a curated slice (11 rooms in Chicago,
 * not all of Chicago). A page that implied full city coverage would be false,
 * and a local would notice immediately.
 *
 * Names only, unlinked, deliberately. A page that HAS listings has already
 * given the reader somewhere to go; this footer's job is to state the scope of
 * what they just read, and eleven competing links under a listing would argue
 * with it. The nightly page's empty state does the opposite — see its
 * "check the rooms directly" block, where linking each room IS the offer.
 */
export function TrackedRoomsFooter({ city, roomNames }: { city: string; roomNames: string[] }) {
  if (roomNames.length === 0) return null
  return (
    <footer className="mt-12">
      <div className="border-t-2 border-foreground" />
      <h2 className="pt-4 font-mono text-[11px] tracking-widest text-muted-foreground">
        ROOMS WE TRACK IN {city.toUpperCase()}
      </h2>
      <p className="mt-2 text-sm leading-relaxed">{roomNames.join(' · ')}</p>
      <Link
        href="/contribute"
        className="mt-2 inline-block text-sm text-muted-foreground hover:underline"
      >
        Missing a room? Suggest a venue →
      </Link>
    </footer>
  )
}
