/**
 * The browser side effects a calendar row performs. Each runs inside the
 * click that asked for it, with the address already in hand: a browser
 * refuses a popup, a clipboard write or a calendar hand-off that arrives
 * after waiting on the server.
 */

/**
 * Opens `url` in a new tab, returning it, or null when the browser blocked
 * it. Opened blank and sent on, rather than with `noopener`, which returns no
 * handle even when it succeeds: the caller learns whether a tab opened and
 * keeps a way to close it, and the opener is cut while the blank tab is still
 * same-origin, so the page it lands on cannot reach back into this one.
 */
export function openTab(url: string): Window | null {
  const tab = window.open('', '_blank')
  if (!tab) return null
  tab.opener = null
  tab.location.href = url
  return tab
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
