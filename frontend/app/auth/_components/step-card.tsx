import type { ReactNode } from 'react'

/** The bordered card the sign-up steps and their terminal states sit in. */
export function StepCard({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-col gap-5 rounded-lg border bg-card p-5 text-card-foreground sm:p-8">
      {children}
    </div>
  )
}

/**
 * A step card's eyebrow, heading and optional subline. With `focusOnMount`
 * the heading takes focus as it mounts, so a card that replaces a submitted
 * form is announced rather than left behind a focus that fell to the page.
 */
export function StepHeading({
  eyebrow,
  title,
  focusOnMount = false,
  children,
}: {
  eyebrow: string
  title: string
  focusOnMount?: boolean
  children?: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2">
      <p className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
        {eyebrow}
      </p>
      <h1
        ref={focusOnMount ? heading => heading?.focus() : undefined}
        tabIndex={focusOnMount ? -1 : undefined}
        className="text-2xl font-bold leading-[30px] text-foreground outline-none"
      >
        {title}
      </h1>
      {children}
    </div>
  )
}
