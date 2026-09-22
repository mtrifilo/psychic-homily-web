'use client'

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
// Concrete module path, not the `@/components/shared` barrel: this component
// is reachable from the home route. See features/sharedChunkBarrelGuard.test.ts.
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'
import { cn } from '@/lib/utils'
import { useFlipReorder } from '../homeLayoutMotion'
import {
  useHomeLayout,
  useHomeLayoutWriteFailed,
  usePersistHomeLayout,
} from '../hooks/useHomeLayout'
import {
  moveHomeSection,
  setHomeSectionVisibility,
  toHomeLayoutDocument,
  type HomeLayoutDocument,
  type HomeSectionId,
  type ResolvedHomeSection,
} from '../sections'

/**
 * The visibility change about to commit, or `null` for a reorder or a reset.
 *
 * Handed to the page behind the list BEFORE the layout state moves, because
 * the two answer it differently: a reorder slides the sections, a show or hide
 * transitions one section's height.
 */
export type HomeVisibilityChange = {
  id: HomeSectionId
  visible: boolean
} | null

type MoveDirection = 'up' | 'down'

/** Shared with the toolbar, which reports the same failure when the popover
 *  has already closed. */
export const SAVE_FAILED_MESSAGE = 'Could not save your layout. Try again.'

function focusKey(id: HomeSectionId, direction: MoveDirection): string {
  return `${id}:${direction}`
}

/**
 * The show / hide / reorder rows, shared verbatim by the home popover and the
 * Settings "Home page" card so the two cannot drift.
 *
 * Both read and write the same profile cache entry, so an edit in one is
 * already an edit in the other; nothing syncs them explicitly.
 */
export function HomeSectionList({
  initialLayout,
  onBeforeChange,
  footerAction,
  className,
}: {
  /** The document the server read for this request, so the first paint renders
   *  the viewer's own order. Omitted on surfaces with no server read. */
  initialLayout?: HomeLayoutDocument | null
  onBeforeChange?: (change: HomeVisibilityChange) => void
  /** Trailing footer affordance, opposite "Reset to default". The popover puts
   *  its "All settings →" link here; the settings card is already there. */
  footerAction?: ReactNode
  className?: string
}) {
  const { sections, isReady, hasStoredLayout } = useHomeLayout(initialLayout)
  const { persist, reset, isResetting } = usePersistHomeLayout()
  const hasError = useHomeLayoutWriteFailed()
  const { register, capture } = useFlipReorder(
    sections.map(section => section.id)
  )

  // Mounted unconditionally and updated in place: assistive tech announces
  // changes WITHIN a region already on the page, so a region inserted together
  // with its text is announced unreliably. ONE region for the whole list;
  // per-row regions would each claim the same move.
  const [announcement, setAnnouncement] = useState('')

  const buttons = useRef(new Map<string, HTMLButtonElement>())
  const pendingFocus = useRef<{
    id: HomeSectionId
    direction: MoveDirection
    fromIndex: number
  } | null>(null)

  const registerButton = useCallback(
    (key: string) => (node: HTMLButtonElement | null) => {
      if (node) buttons.current.set(key, node)
      else buttons.current.delete(key)
    },
    []
  )

  // Focus must ride the section, not the slot: a second click has to keep
  // moving the same row. React reorders the keyed nodes rather than recreating
  // them, but a DOM move can drop focus, so it is restored explicitly. A
  // button that just became disabled hands focus to its sibling on the same
  // row rather than letting it fall to <body>.
  //
  // Gated on the row having ACTUALLY moved, not merely on a click: the write
  // is optimistic but still asynchronous, so the render right after the click
  // still carries the old order, and restoring focus there would pin it to a
  // button that is about to be disabled.
  useLayoutEffect(() => {
    const pending = pendingFocus.current
    if (!pending) return
    const index = sections.findIndex(section => section.id === pending.id)
    if (index === -1 || index === pending.fromIndex) return
    pendingFocus.current = null
    const target = buttons.current.get(focusKey(pending.id, pending.direction))
    if (target && !target.disabled) {
      target.focus()
      return
    }
    const sibling = buttons.current.get(
      focusKey(pending.id, pending.direction === 'up' ? 'down' : 'up')
    )
    sibling?.focus()
  })

  const handleMove = useCallback(
    (section: ResolvedHomeSection, direction: MoveDirection) => {
      if (!isReady) return
      const next = moveHomeSection(sections, section.id, direction)
      // Null means the row was already at that end. Nothing to announce,
      // animate or persist.
      if (!next) return
      const fromIndex = sections.findIndex(entry => entry.id === section.id)
      const position = next.findIndex(entry => entry.id === section.id) + 1
      pendingFocus.current = { id: section.id, direction, fromIndex }
      setAnnouncement(
        `${section.title} moved ${direction}, now ${position} of ${next.length}.`
      )
      onBeforeChange?.(null)
      capture()
      persist(toHomeLayoutDocument(next))
    },
    [capture, isReady, onBeforeChange, persist, sections]
  )

  const handleToggle = useCallback(
    (section: ResolvedHomeSection, visible: boolean) => {
      if (!isReady) return
      setAnnouncement(`${section.title} ${visible ? 'shown' : 'hidden'}`)
      // No capture: the slot's own height transition is what moves the
      // sections below it, and a slide on top of that would move them twice.
      onBeforeChange?.({ id: section.id, visible })
      persist(
        toHomeLayoutDocument(
          setHomeSectionVisibility(sections, section.id, visible)
        )
      )
    },
    [isReady, onBeforeChange, persist, sections]
  )

  const handleReset = useCallback(() => {
    if (!isReady) return
    setAnnouncement('Home layout reset to default.')
    onBeforeChange?.(null)
    capture()
    reset()
  }, [capture, isReady, onBeforeChange, reset])

  // Enabled while a document is STORED, not merely while the list differs from
  // the shipped order. A viewer who customized and changed their mind back
  // still holds a row that only DELETE clears, and leaving them no way to
  // clear it would pin them to today's default after it changes.
  const canReset = isReady && hasStoredLayout

  return (
    <div className={className}>
      <span className="sr-only" role="status">
        {announcement}
      </span>

      <ul aria-label="Home sections" className="flex flex-col">
        {sections.map((section, index) => (
          <li
            key={section.id}
            ref={register(section.id)}
            className="flex items-start gap-3 border-b border-border px-4 py-3 last:border-b-0"
          >
            <div className="flex shrink-0 flex-col pt-0.5">
              <MoveButton
                ref={registerButton(focusKey(section.id, 'up'))}
                direction="up"
                title={section.title}
                disabled={!isReady || index === 0}
                onClick={() => handleMove(section, 'up')}
              />
              <MoveButton
                ref={registerButton(focusKey(section.id, 'down'))}
                direction="down"
                title={section.title}
                disabled={!isReady || index === sections.length - 1}
                onClick={() => handleMove(section, 'down')}
              />
            </div>

            <Checkbox
              checked={section.visible}
              disabled={!isReady}
              onCheckedChange={checked => handleToggle(section, checked === true)}
              aria-labelledby={`home-section-title-${section.id}`}
              aria-describedby={`home-section-description-${section.id}`}
              className="mt-1 shrink-0"
            />

            <div className="min-w-0 flex-1">
              <p
                id={`home-section-title-${section.id}`}
                className={cn(
                  'text-sm font-medium',
                  section.visible ? 'text-foreground' : 'text-muted-foreground'
                )}
              >
                {section.title}
              </p>
              <p
                id={`home-section-description-${section.id}`}
                className="mt-0.5 text-xs text-muted-foreground"
              >
                {section.description}
              </p>
            </div>

            {!section.visible && (
              <span
                className="shrink-0 self-center font-mono text-[11px] lowercase text-muted-foreground"
                aria-hidden="true"
              >
                hidden
              </span>
            )}
          </li>
        ))}
      </ul>

      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border px-4 py-3">
        <Button
          variant="link"
          size="sm"
          onClick={handleReset}
          disabled={!canReset || isResetting}
          className="h-auto p-0 text-sm text-muted-foreground no-underline hover:text-primary"
        >
          Reset to default
        </Button>
        <span className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
          Changes apply immediately
        </span>
        {footerAction}
      </div>

      {hasError && (
        <div className="px-4 pb-3">
          <InlineErrorBanner>{SAVE_FAILED_MESSAGE}</InlineErrorBanner>
        </div>
      )}
    </div>
  )
}

/**
 * One step, one direction. Rendered disabled at the ends rather than removed:
 * a control that disappears under the cursor moves every row below it and
 * costs the viewer the focus they were using.
 */
function MoveButton({
  ref,
  direction,
  title,
  disabled,
  onClick,
}: {
  ref: (node: HTMLButtonElement | null) => void
  direction: MoveDirection
  title: string
  disabled: boolean
  onClick: () => void
}) {
  const Icon = direction === 'up' ? ChevronUp : ChevronDown
  return (
    <Button
      ref={ref}
      variant="ghost"
      size="sm"
      onClick={onClick}
      disabled={disabled}
      aria-label={`Move ${title} ${direction}`}
      // 24px square: below that, the two stacked arrows are close enough on a
      // touch screen that a mis-tap hits the OTHER one, which moves the
      // section the wrong way and persists it. WCAG 2.5.8.
      className="h-6 w-6 p-0 text-muted-foreground hover:text-primary disabled:opacity-30"
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
    </Button>
  )
}
