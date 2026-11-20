'use client'

import type React from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * A Settings calendar card button: full width with a 44px target below `sm`,
 * the house small button from `sm` up.
 */
export const CARD_BUTTON_CLASS = 'h-11 w-full sm:h-8 sm:w-auto'

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
