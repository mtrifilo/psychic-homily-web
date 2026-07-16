import type { ReactNode } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'

export function ChartModule({
  title,
  context,
  rowCount,
  isLoading,
  isError,
  hasData,
  testId,
  children,
  fullListHref,
}: {
  title: string
  context: string
  rowCount: number
  isLoading: boolean
  isError: boolean
  hasData: boolean
  testId: string
  children: ReactNode
  fullListHref?: string
}) {
  if (!isLoading && rowCount === 0 && (!isError || hasData)) return null

  return (
    <section data-testid={testId} className="min-w-0 overflow-hidden">
      <header className="flex min-h-6 items-start gap-2 border-b-2 border-foreground pb-1.5 font-mono leading-none">
        <h2 className="min-w-0 flex-1 text-[11px] font-bold uppercase tracking-[0.06em]">
          {title}
        </h2>
        <span className="shrink-0 text-[10px] text-muted-foreground">
          {context}
        </span>
        {fullListHref ? (
          <Link
            href={fullListHref}
            className="shrink-0 text-[10px] text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            full list →
          </Link>
        ) : null}
      </header>
      {isLoading ? (
        <div aria-label={`Loading ${title}`} role="status">
          {Array.from({ length: 4 }, (_, index) => (
            <div
              key={index}
              className="grid min-h-11 grid-cols-[1rem_minmax(0,1fr)_3rem] items-start gap-2 border-b border-border py-2"
            >
              <span className="h-3 animate-pulse rounded-sm bg-muted" />
              <span className="h-7 animate-pulse rounded-sm bg-muted" />
              <span className="h-3 animate-pulse rounded-sm bg-muted" />
            </div>
          ))}
        </div>
      ) : isError && !hasData ? (
        <p className="border-b border-border py-3 text-xs text-destructive">
          Unable to load this chart.
        </p>
      ) : (
        <ol>{children}</ol>
      )}
    </section>
  )
}

export function ChartRow({
  rank,
  primary,
  meta,
  stat,
  action,
  className,
}: {
  rank: number | null
  primary: ReactNode
  meta: ReactNode
  stat?: ReactNode
  action: ReactNode
  className?: string
}) {
  return (
    <li
      className={cn(
        'grid min-h-11 grid-cols-[1rem_minmax(0,1fr)_auto_auto] items-start gap-2 border-b border-border py-1.5',
        className
      )}
    >
      <span className="pt-0.5 font-mono text-xs text-muted-foreground">
        {rank ?? '—'}
      </span>
      <div className="min-w-0">
        <div className="truncate text-[13px] font-medium leading-4">
          {primary}
        </div>
        <div className="mt-px truncate text-[11px] leading-[13px] text-muted-foreground">
          {meta}
        </div>
      </div>
      {stat != null ? (
        <span className="whitespace-nowrap pt-0.5 text-right font-mono text-xs tabular-nums">
          {stat}
        </span>
      ) : null}
      <span className="pt-0.5 leading-none">{action}</span>
    </li>
  )
}
