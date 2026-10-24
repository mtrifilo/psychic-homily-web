import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'

/**
 * Breadth-first search of an UNRENDERED element tree, as a server page
 * component returns it, for every element `match` accepts, outermost first.
 * Only `props.children` is walked; nothing is rendered or invoked.
 */
export function findElements(
  root: ReactElement,
  match: (node: ReactElement) => boolean,
): ReactElement[] {
  const found: ReactElement[] = []
  const queue: unknown[] = [root]
  while (queue.length > 0) {
    const node = queue.shift()
    if (!isValidElement(node)) continue
    if (match(node)) found.push(node)
    queue.push(
      ...Children.toArray((node.props as { children?: ReactNode }).children),
    )
  }
  return found
}

/** The first element {@link findElements} would return; throws if none. */
export function findElement(
  root: ReactElement,
  match: (node: ReactElement) => boolean,
): ReactElement {
  const [first] = findElements(root, match)
  if (!first) throw new Error('element not found in the page tree')
  return first
}
