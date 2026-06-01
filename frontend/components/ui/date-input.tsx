import * as React from "react"

import { cn } from "@/lib/utils"

/**
 * DateInput — a styled wrapper over the native `<input type="date">`.
 *
 * Deliberately wraps the native control (no calendar library) so the OS-native
 * date picker, keyboard handling, and screen-reader semantics are preserved.
 * The visual treatment mirrors the DS `Input` primitive exactly (hairline
 * border, transparent bg, orange focus ring, aria-invalid destructive state,
 * disabled opacity) so date fields sit flush with the rest of a form.
 *
 * `type` is fixed to "date" and intentionally not overridable — for free-text
 * or other input types, use `Input`.
 */
function DateInput({
  className,
  ...props
}: Omit<React.ComponentProps<"input">, "type">) {
  return (
    <input
      type="date"
      data-slot="date-input"
      className={cn(
        "file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-base transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
        "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
        "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
        className
      )}
      {...props}
    />
  )
}

export { DateInput }
