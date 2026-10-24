import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'

/**
 * Breadth-first search of an UNRENDERED element tree, as a server page
 * component returns it, for the first element `match` accepts. Only
 * `props.children` is walked; nothing is rendered or invoked.
 */
export function findElement(
  root: ReactElement,
  match: (node: ReactElement) => boolean,
): ReactElement {
  const queue: unknown[] = [root]
  while (queue.length > 0) {
    const node = queue.shift()
    if (!isValidElement(node)) continue
    if (match(node)) return node
    queue.push(
      ...Children.toArray((node.props as { children?: ReactNode }).children),
    )
  }
  throw new Error('element not found in the page tree')
}
