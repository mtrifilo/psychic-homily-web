import { Fragment } from 'react'
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

/**
 * A scene-page link in the accent register: `SceneSectionHeading`'s mono
 * micro-caps type at the same size, toned `text-primary`.
 *
 * Hover UNDERLINES rather than recolouring. Against this primary base,
 * `hover:text-foreground` is a step DOWN in contrast, so hovering would make
 * the link recede.
 *
 * The focus ring is offset rather than inset because these are inline text
 * links with no box of their own to ring inside.
 *
 * `--primary` against `--background` in LIGHT mode measures about 3.7:1, under
 * the 4.5:1 AA floor for text at this size. Dark mode clears it comfortably.
 * The tone is the wave's locked accent register rather than this constant's
 * choice, so changing it is a design decision about every surface that uses it.
 */
export const SCENE_ACCENT_LINK_CLASS =
  'rounded-sm font-mono text-[11px] uppercase tracking-widest text-primary underline-offset-4 transition-colors hover:underline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring'

/**
 * One named entity, linked to its own page when it has one.
 *
 * The guard is the point, and it is why this is a component rather than three
 * ternaries. Entity slugs are NULLABLE and `GenerateSlug` can return `""`, and
 * `/artists/` or `/venues/` with an empty slug resolves to the INDEX page
 * rather than 404ing (PSY-1754) — so the naive `href={`/artists/${slug}`}`
 * silently sends the reader to a directory that never mentions the thing they
 * clicked. `entityHref` below carries that rule for this module; this component
 * is the case where the link body is the name itself.
 *
 * A row is still NAMED when it cannot be linked. Dropping an unlinkable entity
 * would misstate the list it belongs to — the rooms list discloses which rooms
 * a page speaks for, and a missing one makes the coverage claim false.
 */
export function EntityNameLink({
  name,
  slug,
  basePath,
  className = 'font-medium hover:underline',
  unlinkedClassName = 'font-medium',
}: {
  name: string
  slug?: string | null
  /** Route prefix WITHOUT a trailing slash, e.g. `/artists`. */
  basePath: string
  className?: string
  unlinkedClassName?: string
}) {
  const href = entityHref(basePath, slug)
  if (!href) return <span className={unlinkedClassName}>{name}</span>
  return (
    <Link href={href} className={className}>
      {name}
    </Link>
  )
}

/**
 * `/artists/gatecreeper`, or null when the entity has no usable slug.
 *
 * The guard `EntityNameLink` documents above, in a form a caller whose link
 * body is not a bare name can reach for instead of writing a second copy. Null
 * means "do not link this", never "link to the index".
 *
 * Two slug shapes resolve to the INDEX rather than to the entity, and both
 * answer null:
 *
 *  - empty or whitespace, which is what a NULLABLE slug column and a
 *    `GenerateSlug` that can return `""` produce;
 *  - `.` and `..`, which `encodeURIComponent` leaves untouched, so
 *    `/collections/..` walks back up to `/collections`.
 *
 * Anything else is encoded, so the slug can only ever be ONE path segment.
 * Slugs are generated server-side and are `[a-z0-9-]` in practice, which
 * survives encoding untouched — this costs nothing on every real row and stops
 * a stored `/` from splicing a second segment onto a route the caller chose.
 */
export function entityHref(
  /** Route prefix WITHOUT a trailing slash, e.g. `/artists`. */
  basePath: string,
  slug?: string | null
): string | null {
  const trimmed = slug?.trim()
  if (!trimmed || trimmed === '.' || trimmed === '..') return null
  return `${basePath}/${encodeURIComponent(trimmed)}`
}

/**
 * A scene-page section heading, with an optional muted qualifier and one
 * trailing control.
 *
 * The type treatment is the wave's visual contract — letterspaced mono small
 * caps, the qualifier greyed inside the same heading — and it now appears on
 * every module of the detail page. Kept feature-local rather than reaching for
 * the shared `SectionHeader` primitive, whose `title` is a plain string and
 * whose caps variant is a different family and scale entirely.
 */
export function SceneSectionHeading({
  title,
  note,
  action,
}: {
  title: React.ReactNode
  /** Rendered muted inside the heading, after a middot. */
  note?: React.ReactNode
  /** A single trailing control, right-aligned on the heading's baseline. */
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-2">
      <h2 className="font-mono text-[11px] uppercase tracking-widest">
        {title}
        {note != null && (
          <>
            {' '}
            <span className="text-muted-foreground">· {note}</span>
          </>
        )}
      </h2>
      {action}
    </div>
  )
}

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
 * Thinner than `SceneTrackedVenue` because the footer's locked destination is
 * `/venues/{slug}` (PSY-1733) — an external website is never the href.
 */
export type TrackedRoom = {
  name: string
  slug?: string
}

/** Rooms read as prose links, unlike the medium-weight entity names elsewhere. */
const ROOM_LINK_CLASS = 'underline underline-offset-4 hover:text-primary'

/** Enough to name an entity and, when it has a slug, link to its own page. */
export type NamedEntity = {
  name: string
  slug?: string | null
  /** Preferred over the name for the React key when the caller has one. */
  id?: number | string
}

/**
 * `A · B · C`, with each entity linked when it has a slug.
 *
 * Returns a FRAGMENT rather than owning a block element, because a caller that
 * follows the names with an elision and a control needs all three inside one
 * flowing line. The separator sits in its own muted span rather than inside a
 * name, so a middot never lands in a link's hit area or its accessible name.
 *
 * An `id` is the only key that is unique by construction, so it wins where the
 * caller has one. Slugs are NULLABLE and can generate as `""`, which is why the
 * fallback tests truthiness rather than nullishness. The name is the last
 * resort and can still repeat: an id-less caller with two identically named,
 * slugless items keys them the same, which is a real state for venue and band
 * names and the reason `id` is on the type at all.
 */
export function EntityNameList({
  items,
  basePath,
  className,
  unlinkedClassName,
}: {
  items: NamedEntity[]
  /** Route prefix WITHOUT a trailing slash, e.g. `/artists`. */
  basePath: string
  className?: string
  unlinkedClassName?: string
}) {
  return (
    <>
      {items.map((item, i) => (
        <Fragment key={item.id ?? (item.slug || item.name)}>
          {i > 0 && <span className="text-muted-foreground"> · </span>}
          <EntityNameLink
            name={item.name}
            slug={item.slug}
            basePath={basePath}
            className={className}
            unlinkedClassName={unlinkedClassName}
          />
        </Fragment>
      ))}
    </>
  )
}

/** `A · B · C`, with each room linked when it has a slug. */
export function RoomList({ venues }: { venues: TrackedRoom[] }) {
  return (
    <p className="mt-2 text-sm leading-relaxed">
      <EntityNameList
        items={venues}
        basePath="/venues"
        className={ROOM_LINK_CLASS}
        unlinkedClassName=""
      />
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
 * When a room has a slug it links to `/venues/{slug}` (PSY-1733). Rooms without
 * a slug stay plain text.
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
