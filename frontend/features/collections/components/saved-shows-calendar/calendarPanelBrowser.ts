/**
 * The browser side effects a calendar row performs. Each runs inside the
 * click that asked for it, with the address already in hand: a browser
 * refuses a popup, a clipboard write or a calendar hand-off that arrives
 * after waiting on the server.
 */

export interface OpenedTab {
  close: () => void
}

/**
 * Opens `url` in a new tab. Opened blank and sent on, rather than with
 * `noopener`, which returns no handle: the caller keeps a way to close the
 * tab, and the opener is cut while the blank tab is still same-origin, so the
 * page it lands on cannot reach back into this one.
 */
export function openTab(url: string): OpenedTab {
  const tab = window.open('', '_blank')
  if (!tab) {
    window.open(url, '_blank', 'noopener,noreferrer')
    return { close: () => {} }
  }
  tab.opener = null
  tab.location.href = url
  return { close: () => tab.close() }
}

/**
 * Copies `text`. The write starts synchronously, so it is still inside the
 * click; rejects when the browser has no clipboard or refuses the write.
 */
export function copyText(text: string): Promise<void> {
  try {
    return navigator.clipboard.writeText(text)
  } catch (error) {
    return Promise.reject(error)
  }
}
