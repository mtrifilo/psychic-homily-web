import Link from 'next/link'

/**
 * MobileGraphTeaser — the sub-`GRAPH_BREAKPOINT_PX` form of a graph section.
 *
 * No graph canvas renders below the breakpoint on any surface, so whatever the
 * section puts in that slot is the entire section at that width. This is that
 * slot, and it is one line of body text whose sentence is the cross-link into
 * the knowledge graph: no heading, no scale line, no reserved box.
 *
 * The caller supplies the sentence and the href; the trailing arrow and the
 * type/link treatment belong to the component, so the surfaces that share it
 * cannot drift apart on the glyph, the spacing, or the underline.
 *
 * The arrow is decorative: the sentence alone is the accessible name.
 */
export function MobileGraphTeaser({
  href,
  children,
}: {
  href: string
  children: string
}) {
  return (
    <p className="text-xs text-muted-foreground">
      <Link href={href} className="underline underline-offset-4 hover:text-foreground">
        {children} <span aria-hidden="true">→</span>
      </Link>
    </p>
  )
}
