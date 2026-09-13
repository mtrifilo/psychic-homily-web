import Link from 'next/link'
import {
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ORDER,
  sceneWindowHref,
  type SceneWindowKey,
} from '../sceneWindow'
import { SCENE_LINK_INTERACTION_CLASS } from './sceneChrome'

/**
 * The window family's navigation: one component for every route in it.
 *
 * The scene root, the four rolling windows and both dated permalinks draw the
 * same strip from here, so a reader walking between them sees one row that
 * changes only where it says they are.
 *
 * Register per the locked frame (`1665:2`): mono at 10px with 8% tracking, the
 * links in the accent tone, the active window in the foreground tone and
 * underlined, a direction with nothing behind it muted and unlinked.
 *
 * ROLLING routes mark their window current. A dated permalink marks none: it
 * names one night or one week, not a window that moves with the clock.
 */

/**
 * Mono micro-caps at the frame's own metrics, shared by every item in both rows
 * so the register is one. A step down in size from `SCENE_ACCENT_LINK_CLASS`,
 * whose interaction behaviour it composes rather than re-spells.
 *
 * The vertical padding is a TARGET, not spacing: at 10px these are standalone
 * nav targets, and 1.5 is what clears the 24px floor (WCAG 2.5.8) that the type
 * alone does not.
 */
const CHIP_CLASS =
  'inline-block rounded-sm py-1.5 font-mono text-[10px] uppercase tracking-[0.08em]'

const LINK_CLASS = `${CHIP_CLASS} ${SCENE_LINK_INTERACTION_CLASS}`

/** One direction of the prev/next row: a period this site can serve. */
export interface SceneWindowNavStep {
  label: string
  href: string
}

/** The prev/next row the day and week routes add below the chips. */
export interface SceneWindowNavSteps {
  /** Accessible name for the row, e.g. `Adjacent days`. */
  label: string
  /** Absent means the servable window ends here; the row says so, muted. */
  prev?: SceneWindowNavStep
  next?: SceneWindowNavStep
}

/** What each direction reads when there is nothing that way. */
const EDGE_LABEL = { prev: 'Start of listings', next: 'End of listings' } as const

function NavStep({
  step,
  direction,
}: {
  step: SceneWindowNavStep | undefined
  direction: 'prev' | 'next'
}) {
  // A direction with nothing behind it is muted text, never a link: a link to a
  // URL this site 404s is a worse answer than saying the listings end. It
  // carries no arrow either, which would point the eye at a destination that
  // does not exist.
  if (!step) {
    return (
      <span className={`${CHIP_CLASS} text-muted-foreground`}>{EDGE_LABEL[direction]}</span>
    )
  }

  return (
    <Link href={step.href} rel={direction} className={`${LINK_CLASS} text-primary`}>
      {/* The arrow carries the direction for a reader looking at the row, and
          `rel` carries it for a crawler, but neither reaches a screen reader as
          a word — an arrow may be announced as anything or as nothing. The
          accessible name says it. */}
      <span className="sr-only">{direction === 'prev' ? 'Previous: ' : 'Next: '}</span>
      {direction === 'prev' ? `← ${step.label}` : `${step.label} →`}
    </Link>
  )
}

export function SceneWindowNav({
  slug,
  current = null,
  steps,
}: {
  slug: string
  /**
   * The window this route IS, drawn as the active chip.
   *
   * Null on the scene ROOT, which is not one of these windows and so marks
   * none of them current, and on a dated day permalink, which names a single
   * night rather than a rolling window.
   */
  current?: SceneWindowKey | null
  /** Omitted by the routes that have no adjacent period to offer. */
  steps?: SceneWindowNavSteps
}) {
  return (
    <div>
      <nav aria-label="Show windows" className="flex flex-wrap items-center gap-x-3 gap-y-1">
        {SCENE_WINDOW_ORDER.map(key =>
          key === current ? (
            <span
              key={key}
              aria-current="page"
              className={`${CHIP_CLASS} text-foreground underline underline-offset-4`}
            >
              {SCENE_WINDOW_LABEL[key]}
            </span>
          ) : (
            <Link
              key={key}
              href={sceneWindowHref(slug, key)}
              className={`${LINK_CLASS} text-primary`}
            >
              {SCENE_WINDOW_LABEL[key]}
            </Link>
          )
        )}
      </nav>

      {steps && (
        <nav
          aria-label={steps.label}
          className="mt-1 flex items-center justify-between gap-3"
        >
          <NavStep step={steps.prev} direction="prev" />
          <NavStep step={steps.next} direction="next" />
        </nav>
      )}
    </div>
  )
}
