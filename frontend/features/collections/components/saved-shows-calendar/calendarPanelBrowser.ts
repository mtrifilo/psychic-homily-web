/**
 * The browser side effects a calendar row performs: opening a tab, handing a
 * webcal address to the calendar app, copying the link. Each must start inside
 * the click that asked for it, because a browser refuses a popup or a
 * clipboard write that arrives after an await; the address itself may still
 * be on its way (the link is created or loaded on demand), so each takes the
 * address as a promise.
 */

export interface PendingTab {
  /** Sends the tab to `url`; a no-op once closed. */
  navigate: (url: string) => void
  close: () => void
}

/**
 * Opens a blank tab now, within the click, to be sent somewhere once the
 * address is known. The blank tab is same-origin until it navigates, which is
 * when its opener can still be cut, so the page it lands on cannot reach back
 * into this one. When the browser blocks it anyway, `navigate` falls back to a
 * plain new-tab open, which may be blocked too but costs nothing to try.
 */
export function openPendingTab(): PendingTab {
  const tab = window.open('', '_blank')
  if (tab) tab.opener = null
  return {
    navigate: url => {
      if (tab && !tab.closed) {
        tab.location.href = url
      } else {
        window.open(url, '_blank', 'noopener,noreferrer')
      }
    },
    close: () => tab?.close(),
  }
}

/**
 * Opens `url` in a new tab now. For a page whose address does not wait on the
 * link, so it never needs a pending tab.
 */
export function openTabNow(url: string): PendingTab {
  const pending = openPendingTab()
  pending.navigate(url)
  return pending
}

/** Hands a webcal address to the system calendar app from this tab. */
export function openInCalendarApp(webcalAddress: string): void {
  window.location.href = webcalAddress
}

export class ClipboardRefusedError extends Error {
  constructor(cause: unknown) {
    super('The browser refused to copy the calendar link.')
    this.name = 'ClipboardRefusedError'
    this.cause = cause
  }
}

/**
 * Copies the text `text` resolves to. Where the browser takes a clipboard item
 * whose content is still a promise, the write starts now, inside the click,
 * which is what Safari needs; otherwise, or if that write is refused, the
 * plain text write runs once the text arrives.
 *
 * Rejects with the promise's own error when the text never arrives, and with
 * ClipboardRefusedError when the text arrived but could not be written.
 */
export async function copyWhenResolved(text: Promise<string>): Promise<void> {
  let earlyWrite: Promise<void> | null = null
  if (
    typeof ClipboardItem !== 'undefined' &&
    typeof navigator.clipboard?.write === 'function'
  ) {
    try {
      earlyWrite = navigator.clipboard.write([
        new ClipboardItem({
          'text/plain': text.then(
            value => new Blob([value], { type: 'text/plain' })
          ),
        }),
      ])
      // Settled below; this only keeps an early rejection from surfacing as
      // unhandled while the text is awaited.
      earlyWrite.catch(() => {})
    } catch {
      earlyWrite = null
    }
  }

  const value = await text

  if (earlyWrite) {
    try {
      await earlyWrite
      return
    } catch {
      // Fall through to the plain write.
    }
  }
  try {
    await navigator.clipboard.writeText(value)
  } catch (error) {
    throw new ClipboardRefusedError(error)
  }
}
