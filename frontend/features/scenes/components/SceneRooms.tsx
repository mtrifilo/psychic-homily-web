'use client'

import { useState } from 'react'
// Deep-imported, not through `@/components/shared`: that barrel is reachable
// from the root layout, so importing a name through it is the habit that puts
// unrelated modules in the chunk every route loads (PSY-1772).
import { BracketLink } from '@/components/shared/BracketLink'
import { plural } from '../sceneCalendar'
import {
  defaultRoomOrder,
  orderRooms,
  roomLocationLabel,
  roomWebsite,
  type RoomOrder,
} from '../sceneRooms'
import { EntityNameLink, SceneSectionHeading } from './sceneChrome'
import type { SceneDetail, SceneVenue } from '../types'

/**
 * The rooms this scene speaks for, named and ordered by what is coming up.
 *
 * The count is ORDERING, not a KPI tile: the prior art's own leaderboard prints
 * `Baby's All Right 93 shows` to answer "which room should I check first", and
 * offers alphabetical as the escape hatch. The number is never a headline, and
 * on a scene where it cannot order anything it is not printed at all — a figure
 * that has to be hidden when small should never have been a headline when large.
 *
 * Load-bearing, not filler: coverage is a curated slice (12 rooms in Phoenix,
 * not all of Phoenix). A page that implied full city coverage would be false,
 * and a local would notice immediately.
 */

function RoomRow({
  room,
  sceneState,
  showCount,
}: {
  room: SceneVenue
  sceneState: string
  showCount: boolean
}) {
  const location = roomLocationLabel(room, sceneState)
  const website = roomWebsite(room)

  return (
    <li className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 border-b border-border/40 py-2 last:border-b-0">
      <EntityNameLink name={room.name} slug={room.slug} basePath="/venues" />

      {location && (
        <span className="font-mono text-xs text-muted-foreground">({location})</span>
      )}

      {website && (
        <a
          href={website}
          target="_blank"
          rel="noopener noreferrer"
          className="font-mono text-xs text-muted-foreground transition-colors hover:text-foreground"
          aria-label={`${room.name} website (opens in a new tab)`}
        >
          [site ↗]
        </a>
      )}

      {showCount && (
        <span className="font-mono text-xs tabular-nums text-muted-foreground sm:ml-auto">
          {plural(room.upcoming_show_count, 'show')}
        </span>
      )}
    </li>
  )
}

/** `Missing a room? [Suggest a venue →]` — the ask, in every state. */
function SuggestARoom() {
  return (
    <p className="mt-3 flex flex-wrap items-baseline gap-2 text-sm text-muted-foreground">
      Missing a room?
      <BracketLink label="Suggest a venue →" href="/contribute" />
    </p>
  )
}

export function SceneRooms({ scene }: { scene: SceneDetail }) {
  const rooms = scene.venues ?? []
  const naturalOrder = defaultRoomOrder(rooms)

  // null = "the reader has not chosen", so the default tracks the data rather
  // than freezing whatever it was on first render.
  const [chosenOrder, setChosenOrder] = useState<RoomOrder | null>(null)
  // A choice only survives while the choice EXISTS. If a refetch drops the
  // scene below the rankable threshold, the toggle disappears with it, and a
  // retained `ranked` would leave the list sorted by counts it no longer prints
  // and no control to get back from.
  const order =
    naturalOrder === 'ranked' ? (chosenOrder ?? 'ranked') : 'alphabetical'

  // Tied to whether the counts can ORDER the list, not to the current sort. A
  // reader on a dense scene who flips to alphabetical is changing the order,
  // not asking us to withhold a real per-room figure.
  const showCounts = naturalOrder === 'ranked'

  // The EMPTY state SUBSTITUTES rather than scaffolds. A titled section with a
  // `0 tracked` header over blank space is the anti-pattern the sparse matrix
  // exists to forbid; the ask is the only thing here worth a reader's attention.
  if (rooms.length === 0) {
    return (
      <section className="border-t border-border pt-4">
        <SceneSectionHeading title="Rooms / none tracked yet" />
        <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
          Everything on this page&apos;s calendar comes from rooms we track, and we
          track none in {scene.city} yet.
        </p>
        <SuggestARoom />
      </section>
    )
  }

  const ordered = orderRooms(rooms, order)

  return (
    <section className="border-t border-border pt-4">
      <SceneSectionHeading
        title={`Rooms / ${rooms.length} tracked`}
        // Names the order the list is ACTUALLY in, so it follows the escape
        // hatch. `showCounts` is a separate question and deliberately does not
        // move with it.
        note={order === 'ranked' ? 'ordered by upcoming shows' : 'alphabetical'}
        // The escape hatch only exists where there is something to escape. A
        // scene already listed alphabetically because its counts order nothing
        // has no second order to offer.
        action={
          naturalOrder === 'ranked' ? (
            <BracketLink
              label={order === 'ranked' ? 'Alphabetical' : 'By upcoming shows'}
              onClick={() =>
                setChosenOrder(order === 'ranked' ? 'alphabetical' : 'ranked')
              }
            />
          ) : undefined
        }
      />

      <ul className="mt-2">
        {ordered.map(room => (
          <RoomRow
            key={room.id}
            room={room}
            sceneState={scene.state}
            showCount={showCounts}
          />
        ))}
      </ul>

      <SuggestARoom />
    </section>
  )
}
