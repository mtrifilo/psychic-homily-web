'use client'

import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import {
  animateSlotHeight,
  useFlipReorder,
  useReducedMotion,
} from '../homeLayoutMotion'
import { useHomeLayout } from '../hooks/useHomeLayout'
import {
  resolveCityLinkSlot,
  type HomeLayoutDocument,
  type HomeSectionId,
} from '../sections'
import { CustomizeHomeToolbar } from './CustomizeHomeToolbar'
import { HomeCityLinkSlotProvider } from './HomeCityShowsLink'
import type { HomeVisibilityChange } from './HomeSectionList'

/** A section mid-show or mid-hide, and which way it is going. */
type SlotTransition = 'collapse' | 'expand'

/**
 * The signed-in home's client shell: the toolbar, and the sections in this
 * viewer's order.
 *
 * `sections` arrives already rendered by the SERVER component above, one entry
 * per registry id. Every section is handed over whether or not it is visible,
 * because showing one has to paint immediately; only the ones this component
 * renders actually mount, so a hidden section issues no request.
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

  // Staged by the gesture that caused them, never derived from the layout: a
  // change arriving from another device should apply instantly, not play an
  // animation nobody asked for. A MAP, not one id, so toggling a second
  // section does not yank the first out mid-animation.
  const [transitions, setTransitions] = useState<
    ReadonlyMap<HomeSectionId, SlotTransition>
  >(() => new Map())

  const settleTransition = useCallback((id: HomeSectionId) => {
    setTransitions(current => {
      if (!current.has(id)) return current
      const next = new Map(current)
      next.delete(id)
      return next
    })
  }, [])

  // A section on its way out stays mounted until its height reaches zero.
  const rendered = layout.filter(
    section => section.visible || transitions.get(section.id) === 'collapse'
  )
  const { register, capture } = useFlipReorder(
    rendered.map(section => section.id)
  )

  const handleBeforeChange = useCallback(
    (change: HomeVisibilityChange) => {
      if (!change) {
        // A reorder or a reset slides the sections between their old and new
        // positions. A show or hide does not: the slot's own height transition
        // is what moves everything below it.
        capture()
        return
      }
      setTransitions(current => {
        const next = new Map(current)
        next.set(change.id, change.visible ? 'expand' : 'collapse')
        return next
      })
    },
    [capture]
  )

  // Read from `rendered`, not `layout`: a section still collapsing is on
  // screen, and calling the page empty while it animates would unmount it
  // before it finished and strand it in the transition map forever.
  const allHidden = rendered.length === 0
  const cityLinkSlot = resolveCityLinkSlot(layout)

  return (
    <HomeCityLinkSlotProvider value={cityLinkSlot}>
      <div className="flex w-full flex-col">
        <CustomizeHomeToolbar
          open={isPopoverOpen}
          onOpenChange={setPopoverOpen}
          initialLayout={initialLayout}
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
                id={section.id}
                registerRef={register(section.id)}
                transition={transitions.get(section.id)}
                onSettled={settleTransition}
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
  id,
  registerRef,
  transition,
  onSettled,
  children,
}: {
  id: HomeSectionId
  registerRef: (node: HTMLElement | null) => void
  transition: SlotTransition | undefined
  onSettled: (id: HomeSectionId) => void
  children: ReactNode
}) {
  const node = useRef<HTMLDivElement | null>(null)
  const reducedMotion = useReducedMotion()

  const setNode = (element: HTMLDivElement | null) => {
    node.current = element
    registerRef(element)
  }

  useLayoutEffect(() => {
    if (!transition || !node.current) return
    const animation = animateSlotHeight(
      node.current,
      transition,
      reducedMotion,
      () => onSettled(id)
    )
    if (!animation) {
      onSettled(id)
      return
    }
    // A collapse holds the slot at zero height after it finishes. If the write
    // failed and the section is staying after all, this slot is still mounted
    // and would be pinned invisible; cancelling releases it, and the cancel
    // handler settles the transition either way.
    return () => animation.cancel()
  }, [id, onSettled, reducedMotion, transition])

  return (
    <div
      ref={setNode}
      // Clipped only while a height is being animated: a section's own content
      // (popovers, the graph's hover chrome) may legitimately overflow at rest.
      style={transition ? { overflow: 'hidden' } : undefined}
      // A collapsing section is leaving; keep it out of the a11y tree and out
      // of the tab order for the frames it is still painted.
      aria-hidden={transition === 'collapse' || undefined}
      className="w-full"
    >
      {children}
    </div>
  )
}
