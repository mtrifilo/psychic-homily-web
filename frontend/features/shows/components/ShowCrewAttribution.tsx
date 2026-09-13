'use client'

import { useId, useMemo } from 'react'
import Link from 'next/link'
import { entityHref } from '@/lib/entity-slug'
// Deep-imported: `@/features/tags` is a `'use client'` barrel reachable from
// the root layout, so importing through it would put this module in the one
// client chunk every route loads.
import { useEntityTags } from '@/features/tags/hooks'
import {
  compareEntityTagsByConfidence,
  isCrewTagCategory,
  CREW_CHIP_CLASS,
  CREW_CHIP_LINK_CLASS,
} from '@/features/tags/types'

/**
 * The label's type register per Figma `1666:2`: mono micro-caps on the muted
 * tone, at the size the crew chip beside it wears.
 */
const CREW_ROW_LABEL_CLASS =
  'font-mono text-[11px] uppercase tracking-[0.04em] text-muted-foreground'

/**
 * The bookers this listing is credited to: the show's crew-category tags,
 * drawn as the first row of the tags and provenance footer per Figma `1666:2`.
 *
 * A crew is a TAG, not an entity, so a chip lands on `/tags/{slug}`; a crew
 * whose slug cannot address one is still NAMED rather than dropped, because
 * the name is what the chip is for.
 *
 * The row is absent entirely on a show carrying no crew tag, which is most
 * of them: no label, no scaffold. It reads the same query key the tag list
 * below it reads, so the pair costs one request.
 *
 * A chip is a name and a link and nothing else. It carries none of the vote,
 * remove or attribution-card controls a tag list chip carries, and the tag
 * list beside it on this page is told to omit crew tags, so a crew credit on
 * the show page has no in-page correction path.
 *
 * `role="list"` is explicit on the chip list because the label depends on it:
 * a list whose markers are off can lose its list semantics, and a generic
 * element drops `aria-labelledby` with them. The label element carries the
 * copy in sentence case and is uppercased in CSS, so a screen reader says the
 * phrase rather than spelling it.
 */
export function ShowCrewAttribution({ showId }: { showId: number }) {
  const { data } = useEntityTags('show', showId)
  const labelId = useId()

  // A nameless crew is dropped rather than drawn: a chip with no accessible
  // name is an empty box in the tab order.
  //
  // Sorted, because the tag read returns rows in no defined order: without
  // this, two reads of the same show can name its crews in two different
  // orders.
  const crews = useMemo(
    () =>
      (data?.tags ?? [])
        .filter(tag => isCrewTagCategory(tag.category) && tag.name.trim())
        .sort(compareEntityTagsByConfidence),
    [data?.tags]
  )

  if (!crews.length) return null

  return (
    <div
      className="flex flex-wrap items-center gap-x-2 gap-y-1.5"
      data-testid="show-crew-attribution"
    >
      <span id={labelId} className={CREW_ROW_LABEL_CLASS}>
        Presented by
      </span>
      <ul
        role="list"
        aria-labelledby={labelId}
        className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5"
      >
        {crews.map(crew => {
          const href = entityHref('/tags', crew.slug)
          return (
            <li key={crew.tag_id}>
              {href ? (
                <Link href={href} className={CREW_CHIP_LINK_CLASS}>
                  {crew.name}
                </Link>
              ) : (
                <span className={CREW_CHIP_CLASS}>{crew.name}</span>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
