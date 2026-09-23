/**
 * Fragment targets resolved inside one page's own tree.
 *
 * The document can hold more than one element with a given id: a route the
 * viewer left can stay mounted, hidden, beside the current one, carrying the
 * same ids. Both the browser's own fragment jump and `document.getElementById`
 * take the first match in the whole document, which can be the hidden one, so
 * a page resolves and jumps within its own root instead.
 */

import { revealAnchorTarget } from '@/features/auth/components/settings/useAnchorScroll'

export function findAnchorTarget(
  root: ParentNode,
  anchor: string
): HTMLElement | null {
  return root.querySelector<HTMLElement>(`[id="${CSS.escape(anchor)}"]`)
}

/**
 * Reveals the root's own target for `anchor` and records the fragment in the
 * address bar.
 */
export function jumpToAnchor(root: ParentNode | null, anchor: string): void {
  if (!root) return
  const target = findAnchorTarget(root, anchor)
  if (!target) return
  if (window.location.hash !== `#${anchor}`) {
    window.location.hash = anchor
  }
  revealAnchorTarget(target)
}
