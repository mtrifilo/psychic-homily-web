'use client'

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { animateSlotHeight, useFlipReorder } from '../homeLayoutMotion'
import { useHomeLayout } from '../hooks/useHomeLayout'
import {
  HOME_SECTIONS,
  resolveCityLinkSlot,
  type HomeLayoutDocument,
  type HomeSectionId,
} from '../sections'
import { CustomizeHomeToolbar } from './CustomizeHomeToolbar'
import { HomeCityLinkSlotProvider } from './HomeCityShowsLink'
import type { HomeLayoutChange } from './HomeSectionList'

/**
 * The signed-in home's client shell: the toolbar, and the sections in this
 * viewer's order.
 *
 * `sections` arrives already rendered by the SERVER component above, one entry
 * per registry id. Every section is handed over whether or not it is visible,
 * because showing one has to paint immediately; only the ones this component
 * renders actually mount, so a hidden section still issues no requests.
 *
 * First paint carries the viewer's own order: `initialLayout` is the document
 * the server read on this request, and the profile query it defers to is
 * hydrated from the same read, so hydration finds the order it rendered.
 */
export function HomeLayoutRuntime({
  initialLayout,
  sections,
}: {
  initialLayout: HomeLayoutDocument | null
  sections: Record<HomeSectionId, ReactNode>
}) {
  const layout = useHomeLayout(initialLayout)
  const [isPopoverOpen, setPopoverOpen] = useState(false)

  // Staged by the gesture that caused them, never derived from the layout:
  // a change arriving from another device should apply instantly, not play an
  // animation nobody asked for.
  const [collapsingId, setCollapsingId] = useState<HomeSectionId | null>(null)
  const [enteringId, setEnteringId] = useState<HomeSectionId | null>(null)

  // A section on its way out stays mounted until its height reaches zero.
  const rendered = layout.filter(
    section => section.visible || section.id === collapsingId
  )
  const { register, capture } = useFlipReorder(
    rendered.map(section => section.id).join('|')
  )

  const handleBeforeChange = useCallback(
    (change: HomeLayoutChange) => {
      if (change.kind !== 'visibility') {
        // A reorder slides the sections between their old and new positions.
        // A show or hide does not: the slot's own height transition is what
        // moves everything below it, and adding a slide on top would move
        // those sections twice.
        capture()
        setCollapsingId(null)
        setEnteringId(null)
        return
      }
      setCollapsingId(change.visible ? null : change.id)
      setEnteringId(change.visible ? change.id : null)
    },
    [capture]
  )

  const handleCollapsed = useCallback(() => setCollapsingId(null), [])
  const handleEntered = useCallback(() => setEnteringId(null), [])

  const allHidden = layout.every(section => !section.visible)
  const cityLinkSlot = resolveCityLinkSlot(layout)

  return (
    <HomeCityLinkSlotProvider value={cityLinkSlot}>
      <div className="flex w-full flex-col">
        <CustomizeHomeToolbar
          sectionCount={HOME_SECTIONS.length}
          open={isPopoverOpen}
          onOpenChange={setPopoverOpen}
          fallback={initialLayout}
          onBeforeChange={handleBeforeChange}
          withCityLink={cityLinkSlot === 'toolbar'}
        />

        {allHidden ? (
          <p className="mt-6 text-sm text-muted-foreground">
            You have hidden every section.{' '}
            <button
              type="button"
              onClick={() => setPopoverOpen(true)}
              className="font-medium text-primary transition-colors hover:underline underline-offset-4"
            >
              Customize home →
            </button>
          </p>
        ) : (
          <div className="mt-4 flex w-full flex-col gap-14">
            {rendered.map(section => (
              <HomeSectionSlot
                key={section.id}
                registerRef={register(section.id)}
                collapsing={section.id === collapsingId}
                entering={section.id === enteringId}
                onCollapsed={handleCollapsed}
                onEntered={handleEntered}
              >
                {sections[section.id]}
              </HomeSectionSlot>
            ))}
          </div>
        )}
      </div>
    </HomeCityLinkSlotProvider>
  )
}

/**
 * One section's place in the column, and the only element that animates on a
 * show or hide.
 *
 * The wrapper exists so the height transition has something to animate that is
 * not the section's own layout: sections set their own flex column and gaps,
 * and overriding those from outside would be a different bug on each one.
 */
function HomeSectionSlot({
  registerRef,
  collapsing,
  entering,
  onCollapsed,
  onEntered,
  children,
}: {
  registerRef: (node: HTMLElement | null) => void
  collapsing: boolean
  entering: boolean
  onCollapsed: () => void
  onEntered: () => void
  children: ReactNode
}) {
  const node = useRef<HTMLDivElement | null>(null)

  const setNode = useCallback(
    (element: HTMLDivElement | null) => {
      node.current = element
      registerRef(element)
    },
    [registerRef]
  )

  useLayoutEffect(() => {
    if (!collapsing || !node.current) return
    const animation = animateSlotHeight(node.current, 'collapse', onCollapsed)
    if (!animation) {
      onCollapsed()
      return
    }
    // A collapse holds the slot at zero height after it finishes. If the write
    // failed and the section is staying after all, this slot is still mounted
    // and would be pinned invisible; cancelling releases it.
    return () => animation.cancel()
  }, [collapsing, onCollapsed])

  useLayoutEffect(() => {
    if (!entering || !node.current) return
    const animation = animateSlotHeight(node.current, 'expand', onEntered)
    if (!animation) {
      onEntered()
      return
    }
    return () => animation.cancel()
  }, [entering, onEntered])

  return (
    <div
      ref={setNode}
      // Clipped only while a height is being animated: a section's own content
      // (popovers, the graph's hover chrome) may legitimately overflow at rest.
      style={collapsing || entering ? { overflow: 'hidden' } : undefined}
      // A collapsing section is leaving; keep it out of the a11y tree and out
      // of the tab order for the frames it is still painted.
      aria-hidden={collapsing || undefined}
      className="w-full"
    >
      {children}
    </div>
  )
}
