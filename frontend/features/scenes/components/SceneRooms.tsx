'use client'

import { useState } from 'react'
import Link from 'next/link'
import { BracketLink } from '@/components/shared'
import {
  defaultRoomOrder,
  orderRooms,
  roomHref,
  roomLocationLabel,
  roomWebsite,
  type RoomOrder,
} from '../sceneRooms'
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
  const href = roomHref(room)
  const location = roomLocationLabel(room, sceneState)
  const website = roomWebsite(room)

  return (
    <li className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 border-b border-border/40 py-2 last:border-b-0">
      {href ? (
        <Link href={href} className="font-medium hover:underline">
          {room.name}
        </Link>
      ) : (
        // No page of its own: name it anyway. The list's job is to say WHICH
        // rooms this page speaks for, and dropping one would misstate the
        // coverage it exists to disclose.
        <span className="font-medium">{room.name}</span>
      )}

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

      <span className="hidden flex-1 sm:block" aria-hidden="true" />

      {showCount && (
        <span className="font-mono text-xs tabular-nums text-muted-foreground">
          {room.upcoming_show_count} show{room.upcoming_show_count === 1 ? '' : 's'}
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
  const order = chosenOrder ?? naturalOrder

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
        <h2 className="font-mono text-[11px] uppercase tracking-widest">
          Rooms / none tracked yet
        </h2>
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
      <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-2">
        <h2 className="font-mono text-[11px] uppercase tracking-widest">
          Rooms / {rooms.length} tracked{' '}
          {/* Names the order the list is ACTUALLY in, so it follows the escape
              hatch. `showCounts` is a separate question and deliberately does
              not move with it. */}
          <span className="text-muted-foreground">
            · {order === 'ranked' ? 'ordered by upcoming shows' : 'alphabetical'}
          </span>
        </h2>

        {/* The escape hatch only exists where there is something to escape.
            A scene already listed alphabetically because its counts order
            nothing has no second order to offer. */}
        {naturalOrder === 'ranked' && (
          <BracketLink
            label={order === 'ranked' ? 'Alphabetical' : 'By upcoming shows'}
            onClick={() =>
              setChosenOrder(order === 'ranked' ? 'alphabetical' : 'ranked')
            }
          />
        )}
      </div>

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
