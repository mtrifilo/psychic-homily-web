'use client'

import { useId, useMemo } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { addressesAnEntity } from '@/lib/entity-slug'
// Deep-imported, not through the `@/features/tags` barrel — see the note in
// features/shows/components/index.ts.
import { useEntityTags } from '@/features/tags/hooks'
import {
  compareEntityTagsByConfidence,
  getCategoryChipClasses,
  isDescriptiveTagCategory,
  TAG_CATEGORY_CREW,
} from '@/features/tags/types'

/**
 * The row's type register: mono micro-caps on the muted tone, the same 11px
 * the shipped crew chip wears on the scene page. Label and chip share it, as
 * Figma `1666:2` draws them at one size.
 */
const CREW_ROW_TEXT_CLASS = 'font-mono text-[11px] uppercase tracking-[0.04em]'

/**
 * The chip, composed once.
 *
 * The crew category's own look (no fill, square corners, mono uppercase
 * letterspaced, muted) comes from the shared treatment, which carries no font
 * size so each surface keeps its own density. This surface adds the border
 * width, the 8px/4px padding of the frame, and the register above.
 *
 * `break-words` is the guard on a crew name long enough to exceed the content
 * column on a narrow screen: `tags.name` allows 100 characters, and a single
 * unbroken token that cannot fit a line would otherwise widen the page.
 */
const CREW_CHIP_CLASS = cn(
  'inline-flex max-w-full items-center break-words border px-2 py-1 leading-none',
  CREW_ROW_TEXT_CLASS,
  getCategoryChipClasses(TAG_CATEGORY_CREW)
)

/** Hover and focus, which the chip wears only when it is a link. */
const CREW_CHIP_LINK_CLASS = cn(
  CREW_CHIP_CLASS,
  'transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring'
)

/**
 * The bookers this listing is credited to: the show's crew-category tags,
 * drawn as the first row of the tags and provenance footer per Figma `1666:2`.
 *
 * A crew is a TAG, not an entity, so a chip lands on `/tags/{slug}`; a crew
 * whose slug cannot address one is still NAMED rather than dropped, because
 * the name is what the chip is for.
 *
 * The row is absent entirely — no label, no scaffold — on a show carrying no
 * crew tag, which is most of them. It reads the same query key the tag list
 * below it reads, so the pair costs one request.
 *
 * `role="list"` is explicit on the chip list because the label depends on it:
 * a list whose markers are off loses its list semantics in WebKit, and a
 * generic element drops `aria-labelledby` with them. The label element carries
 * the copy in sentence case and is uppercased in CSS, so a screen reader says
 * the phrase rather than spelling it.
 */
export function ShowCrewAttribution({ showId }: { showId: number }) {
  const { data } = useEntityTags('show', showId)
  const labelId = useId()

  // A nameless crew is dropped rather than drawn: a chip with no accessible
  // name is an empty box in the tab order.
  //
  // Sorted, because `ListEntityTags` returns rows in no defined order — the
  // same ranking the tag list below applies to this payload, so the two rows
  // cannot disagree about which of two crews leads.
  const crews = useMemo(
    () =>
      (data?.tags ?? [])
        .filter(tag => !isDescriptiveTagCategory(tag.category) && tag.name.trim())
        .sort(compareEntityTagsByConfidence),
    [data?.tags]
  )

  if (!crews.length) return null

  return (
    <div
      className="flex flex-wrap items-center gap-x-2 gap-y-1.5"
      data-testid="show-crew-attribution"
    >
      <span id={labelId} className={cn(CREW_ROW_TEXT_CLASS, 'text-muted-foreground')}>
        Presented by
      </span>
      <ul
        role="list"
        aria-labelledby={labelId}
        className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5"
      >
        {crews.map(crew => {
          const href = addressesAnEntity(crew.slug.trim())
            ? `/tags/${encodeURIComponent(crew.slug.trim())}`
            : null
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
