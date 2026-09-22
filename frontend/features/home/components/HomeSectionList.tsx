'use client'

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'
import { useFlipReorder } from '../homeLayoutMotion'
import { useHomeLayout, usePersistHomeLayout } from '../hooks/useHomeLayout'
import {
  isDefaultHomeLayout,
  moveHomeSection,
  setHomeSectionVisibility,
  toHomeLayoutDocument,
  type HomeLayoutDocument,
  type HomeSectionId,
  type ResolvedHomeSection,
} from '../sections'

/**
 * What is about to change, handed to the page behind the list BEFORE the
 * layout state moves, so it can capture positions and stage its own
 * transitions against the same gesture.
 */
export type HomeLayoutChange =
  | { kind: 'move'; id: HomeSectionId }
  | { kind: 'visibility'; id: HomeSectionId; visible: boolean }
  | { kind: 'reset' }

type MoveDirection = 'up' | 'down'

const SAVE_FAILED_MESSAGE = 'Could not save your layout. Try again.'

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
  fallback,
  onBeforeChange,
  footerAction,
  className,
}: {
  /** The document the server read for this request, so the first paint renders
   *  the viewer's own order. Omitted on surfaces with no server read. */
  fallback?: HomeLayoutDocument | null
  onBeforeChange?: (change: HomeLayoutChange) => void
  /** Trailing footer affordance, opposite "Reset to default". The popover puts
   *  its "All settings →" link here; the settings card is already there. */
  footerAction?: ReactNode
  className?: string
}) {
  const sections = useHomeLayout(fallback)
  const { persist, reset, isResetting, hasError } = usePersistHomeLayout()
  const { register, capture } = useFlipReorder(
    sections.map(section => section.id).join('|')
  )

  // Mounted unconditionally and updated in place: assistive tech announces
  // changes WITHIN a region already on the page, so a region inserted together
  // with its text is announced unreliably. ONE region for the whole list —
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

  const commit = useCallback(
    (next: ResolvedHomeSection[], change: HomeLayoutChange) => {
      onBeforeChange?.(change)
      // Only a reorder moves these rows. A visibility toggle swaps a checkbox
      // and a tag in place, so capturing for it would leave a measurement
      // waiting for a move that never comes.
      if (change.kind !== 'visibility') capture()
      persist(toHomeLayoutDocument(next))
    },
    [capture, onBeforeChange, persist]
  )

  const handleMove = useCallback(
    (section: ResolvedHomeSection, direction: MoveDirection) => {
      const next = moveHomeSection(sections, section.id, direction)
      // Same reference means the row was already at that end. Nothing to
      // announce, animate or persist.
      if (next === sections) return
      const fromIndex = sections.findIndex(entry => entry.id === section.id)
      const position = next.findIndex(entry => entry.id === section.id) + 1
      pendingFocus.current = { id: section.id, direction, fromIndex }
      setAnnouncement(
        `${section.title} moved ${direction}, now ${position} of ${next.length}.`
      )
      commit(next, { kind: 'move', id: section.id })
    },
    [commit, sections]
  )

  const handleToggle = useCallback(
    (section: ResolvedHomeSection, visible: boolean) => {
      setAnnouncement(`${section.title} ${visible ? 'shown' : 'hidden'}`)
      commit(setHomeSectionVisibility(sections, section.id, visible), {
        kind: 'visibility',
        id: section.id,
        visible,
      })
    },
    [commit, sections]
  )

  const handleReset = useCallback(() => {
    setAnnouncement('Home layout reset to default.')
    onBeforeChange?.({ kind: 'reset' })
    capture()
    reset()
  }, [capture, onBeforeChange, reset])

  const isDefault = isDefaultHomeLayout(sections)

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
                disabled={index === 0}
                onClick={() => handleMove(section, 'up')}
              />
              <MoveButton
                ref={registerButton(focusKey(section.id, 'down'))}
                direction="down"
                title={section.title}
                disabled={index === sections.length - 1}
                onClick={() => handleMove(section, 'down')}
              />
            </div>

            <Checkbox
              checked={section.visible}
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
        <button
          type="button"
          onClick={handleReset}
          disabled={isDefault || isResetting}
          className="text-sm text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:text-muted-foreground disabled:hover:no-underline"
        >
          Reset to default
        </button>
        {footerAction}
      </div>

      {hasError && (
        <p role="alert" className="px-4 pb-3 text-sm text-destructive">
          {SAVE_FAILED_MESSAGE}
        </p>
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
    <button
      ref={ref}
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={`Move ${title} ${direction}`}
      className="flex h-4 w-5 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:text-muted-foreground"
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
    </button>
  )
}
