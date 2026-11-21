'use client'

import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import type React from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useDismissTimer } from '@/lib/hooks/common/useDismissTimer'
import { cn } from '@/lib/utils'
import type { CalendarSendControlProps } from './CalendarEmailStepsAction'
import { holdFocusInSurface } from './CalendarPanelParts'

/**
 * A Settings calendar card button: full width with a 44px target below `sm`,
 * the house small button from `sm` up.
 */
const CARD_BUTTON_CLASS = 'h-11 w-full sm:h-8 sm:w-auto'

type CardButtonProps = React.ComponentProps<typeof Button> & {
  /**
   * The action is in flight: the button waits, shows the spinner and reports
   * itself busy, with its label unchanged.
   */
  working?: boolean
}

export function CalendarCardButton({
  working = false,
  disabled,
  className,
  variant = 'outline',
  asChild,
  children,
  ...props
}: CardButtonProps) {
  return (
    <Button
      variant={variant}
      size="sm"
      asChild={asChild}
      disabled={disabled || working}
      aria-busy={working || undefined}
      className={cn(CARD_BUTTON_CLASS, className)}
      {...props}
    >
      {/* A slotted link takes exactly one child: it is never working. */}
      {asChild ? (
        children
      ) : (
        <>
          {working ? (
            <Loader2 className="animate-spin" aria-hidden="true" />
          ) : null}
          {children}
        </>
      )}
    </Button>
  )
}

/** "Email me the steps" and "Send again" as the card's outline buttons. */
export function CalendarCardSendButton({
  testId,
  ...props
}: CalendarSendControlProps) {
  return <CalendarCardButton data-testid={testId} {...props} />
}

/**
 * A ref for a block of the card that can leave while focus is inside it (a
 * refetch finds the link changed or gone, a question closes): focus moves to
 * the card first, so it never drops to the page. A layout cleanup runs while
 * the block's nodes are still in the page.
 */
export function useHoldFocusOnLeave<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  useLayoutEffect(() => {
    const block = ref.current
    return () => {
      if (block?.contains(document.activeElement)) holdFocusInSurface(block)
    }
  }, [])
  return ref
}

/**
 * Whether a control that just appeared may act yet: false for `delayMs` after
 * mount, then true. Read it in the control's handler, so the control looks,
 * focuses and announces itself exactly as it would without the wait.
 */
export function useArmedAfterMount(delayMs: number): () => boolean {
  const armed = useRef(false)
  const { schedule } = useDismissTimer(() => {
    armed.current = true
  }, delayMs)
  useEffect(() => {
    armed.current = false
    schedule()
  }, [schedule])
  return useCallback(() => armed.current, [])
}

/**
 * A block of the card that can leave under focus: an add button whose app
 * just connected, a stopped app that checked again. Focus inside it moves to
 * the card as it leaves.
 */
export function FocusHold({
  className,
  children,
}: {
  className?: string
  children: React.ReactNode
}) {
  const ref = useHoldFocusOnLeave<HTMLDivElement>()
  return (
    <div ref={ref} className={className}>
      {children}
    </div>
  )
}
