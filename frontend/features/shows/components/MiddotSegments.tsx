import { Fragment } from 'react'

interface MiddotSegmentsProps {
  /**
   * The surviving segments, in order. Callers filter absences BEFORE this
   * component so the separators always sit BETWEEN segments — a conditional
   * chain grows a stray leading middot the first time its leftmost segment
   * is absent.
   *
   * Segments may be plain strings or elements. Elements should carry their
   * own stable `key`; pass it via the parallel `keys` array so an async
   * insertion mid-list (the provenance line's revision fragments) reconciles
   * by identity instead of position — index-keying there unmounts whatever
   * trails the insertion point and throws away its state (an open dialog).
   */
  segments: React.ReactNode[]
  /**
   * Stable key per segment, same order. Optional for all-string callers,
   * where content identity is fine.
   */
  keys?: string[]
  className?: string
  'data-testid'?: string
}

/**
 * A middot-separated fact line: `CAP ~3,600 · 17+ · DOORS 7PM`.
 *
 * One renderer for the venue facts line, the ticket line, and the provenance
 * byline, because the separator's accessibility treatment is subtle and must
 * not drift between them: the middot glyph is decoration (`aria-hidden`), but
 * the spaces around it are REAL text nodes outside the hidden span — remove
 * them and a screen reader announces the neighbours run together
 * ("8:00 PMON SALE"). Renders nothing for an empty list.
 */
export function MiddotSegments({
  segments,
  keys,
  className,
  'data-testid': testId,
}: MiddotSegmentsProps) {
  if (segments.length === 0) return null
  return (
    <div data-testid={testId} className={className}>
      {segments.map((segment, index) => (
        <Fragment key={keys?.[index] ?? `segment-${index}`}>
          {index > 0 && (
            <>
              {' '}
              <span aria-hidden="true" className="text-muted-foreground/60">
                &middot;
              </span>{' '}
            </>
          )}
          {/* String segments get their own span so each fact is one element
              (addressable, non-breaking within itself); element segments
              already are one. */}
          {typeof segment === 'string' ? <span>{segment}</span> : segment}
        </Fragment>
      ))}
    </div>
  )
}
