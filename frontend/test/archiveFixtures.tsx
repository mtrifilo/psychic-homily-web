/**
 * The past-show archive's shared test fixture (PSY-1842).
 *
 * Three suites exercise the same archive from different heights — the shared
 * component (`features/shows/components/PastShowsArchive.test.tsx`) and the two
 * wrappers that mount it — and all three assert the SAME expected page labels.
 * Those strings are a function of the histogram below and the page size: change
 * one bucket and every expectation in all three files means something different,
 * silently. Sharing the numbers is what makes that a compile-or-fail rather than
 * a quiet rewrite of what the suites are checking.
 *
 * Same reasoning as `test/layoutContracts.ts`, applied to a fixture rather than
 * to a Tailwind literal.
 */

import { vi } from 'vitest'

/** The page size both archives request. */
export const ARCHIVE_PAGE_SIZE = 50

/**
 * 161 shows across five months of 2025, newest first — four pages of 50.
 *
 * The month boundaries deliberately do NOT line up with the page boundaries:
 * page 1 straddles Sep/Aug, page 2 straddles Jul/Jun. A histogram walk that
 * dropped or double-counted a bucket would produce a well-formed but WRONG span,
 * which is the failure mode worth pinning — a wrong label is announced into a
 * live region and never corrected.
 */
export const ARCHIVE_MONTHS = [
  { year: 2025, month: 9, count: 20 },
  { year: 2025, month: 8, count: 30 },
  { year: 2025, month: 7, count: 40 },
  { year: 2025, month: 6, count: 30 },
  { year: 2025, month: 5, count: 41 },
] as const

/** The row count {@link ARCHIVE_MONTHS} sums to. The list envelope must agree. */
export const ARCHIVE_TOTAL = 161

/**
 * What each page link's accessible name must read, given the histogram above.
 *
 * Labels run CHRONOLOGICALLY — earlier month first — even though the list itself
 * is newest-first. That is a locked user decision (PSY-1769): a reversed
 * "Sep–Aug" scans as a wrap-around rather than as a page span.
 */
export const ARCHIVE_PAGE_LABELS = [
  [1, 'Aug–Sep 2025'],
  [2, 'Jun–Jul 2025'],
  [3, 'May–Jun 2025'],
  [4, 'May 2025'],
] as const

/**
 * The `@/components/shared` barrel, stubbed down to what an archive suite needs.
 *
 * The barrel is stubbed to keep unrelated shared components out of these suites,
 * but the PAGER is the thing every one of them observes behaviour through, so the
 * real one — live region and all — is spliced back in from its own module.
 *
 * Call it from inside an ASYNC `vi.mock` factory via a dynamic import, never a
 * top-level one: `vi.mock` is hoisted above the import block, so a statically
 * imported binding is not initialised yet when the factory runs.
 *
 *   vi.mock('@/components/shared', async () => {
 *     const kit = await import('@/test/archiveFixtures')
 *     return kit.archiveSharedBarrelMock()
 *   })
 */
export async function archiveSharedBarrelMock({
  yearStripTestId = 'year-strip',
}: { yearStripTestId?: string } = {}) {
  const pagination = await vi.importActual<
    typeof import('@/components/shared/Pagination')
  >('@/components/shared/Pagination')
  const chrome = await vi.importActual<
    typeof import('@/components/shared/paginationChrome')
  >('@/components/shared/paginationChrome')

  return {
    Pagination: pagination.Pagination,
    paginationWindow: pagination.paginationWindow,
    usePaginationFocusTarget: pagination.usePaginationFocusTarget,
    formatCount: chrome.formatCount,
    SectionHeader: ({
      title,
      action,
      headingProps,
    }: {
      title: string
      action?: React.ReactNode
      headingProps?: Record<string, unknown>
    }) => (
      <>
        <h2 {...headingProps}>{title}</h2>
        <div data-testid="section-action">{action}</div>
      </>
    ),
    YearStrip: ({ years }: { years: unknown[] }) => (
      <div data-testid={yearStripTestId}>{years.length} years</div>
    ),
  }
}
