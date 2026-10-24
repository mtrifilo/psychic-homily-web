import { STACKED_OFF_LINE } from '@/components/shared/stackedTable'

/**
 * A cell's place in a stacked row below `sm` (`stackedTableLayout`), read off
 * its grid classes: "line 1, column 2", "line 2, full width" or "no line".
 */
export function stackedPlace(cell: HTMLElement): string {
  if (cell.classList.contains(STACKED_OFF_LINE)) return 'no line'
  const line = cell.className.match(/max-sm:row-start-(\d)/)?.[1]
  if (cell.classList.contains('max-sm:col-span-full')) {
    return `line ${line}, full width`
  }
  const column = cell.className.match(/max-sm:col-start-(\d)/)?.[1]
  return `line ${line}, column ${column}`
}
