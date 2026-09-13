import Link from 'next/link'
import {
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ORDER,
  sceneWindowHref,
  type SceneWindowKey,
} from '../sceneWindow'

/**
 * The window family's navigation: one component for every route in it.
 *
 * The scene root, the four rolling windows and both dated permalinks draw the
 * same strip from here. Each of those surfaces used to carry its own copy, and
 * the copies had already drifted — two windows pointed at one href, one strip
 * marked a window current that its route was not, and the day and week pages
 * grew a second, differently-styled row of their own.
 *
 * Register per the locked frame (`1665:2`): mono at 10px with 8% tracking, the
 * links in the accent tone, the active window in the foreground tone and
 * underlined, a direction with nothing behind it muted and unlinked.
 */

/** Mono micro-caps, shared by every item in both rows so the register is one. */
const CHIP_CLASS = 'rounded-sm py-1 font-mono text-[10px] uppercase tracking-[0.08em]'

/** Links carry the focus ring; offset, since these are inline text links. */
const LINK_CLASS = `${CHIP_CLASS} underline-offset-4 transition-colors hover:underline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring`

/**
 * One direction of the prev/next row.
 *
 * A null `href` is a direction this site cannot serve — past the edge of the
 * servable window, or a key the route would reject. It renders as muted text
 * rather than a link, because a chip pointing at a URL that 404s is a worse
 * answer than a plain statement that there is nothing there.
 */
export interface SceneWindowNavStep {
  label: string
  href: string | null
}

/** The prev/next row the day and week routes add below the chips. */
export interface SceneWindowNavSteps {
  /** Accessible name for the row, e.g. `Adjacent days`. */
  label: string
  prev: SceneWindowNavStep
  next: SceneWindowNavStep
}

/** The muted labels for the two ends of the servable window. */
export const SCENE_NAV_START_EDGE = 'Start of listings'
export const SCENE_NAV_END_EDGE = 'End of listings'

function NavStep({
  step,
  direction,
}: {
  step: SceneWindowNavStep
  direction: 'prev' | 'next'
}) {
  // The arrow belongs to the LINK. A muted edge with an arrow would draw the
  // eye toward a destination that does not exist.
  if (!step.href) {
    return <span className={`${CHIP_CLASS} text-muted-foreground`}>{step.label}</span>
  }

  return (
    <Link href={step.href} rel={direction} className={`${LINK_CLASS} text-primary`}>
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
