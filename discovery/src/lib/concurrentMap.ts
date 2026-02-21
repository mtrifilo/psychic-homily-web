/**
 * Concurrency-limited parallel executor with per-item callbacks.
 * Worker-pool pattern: N workers pull from a shared index.
 */
export async function concurrentMap<T, R>(
  items: T[],
  fn: (item: T, index: number) => Promise<R>,
  options: {
    concurrency?: number
    onItemComplete?: (result: R, index: number) => void
    onItemError?: (error: unknown, index: number) => void
  } = {},
): Promise<R[]> {
  const { concurrency = 5, onItemComplete, onItemError } = options
  const results: (R | undefined)[] = new Array(items.length)
  let nextIndex = 0

  async function worker() {
    while (nextIndex < items.length) {
      const i = nextIndex++
      try {
        const result = await fn(items[i], i)
        results[i] = result
        onItemComplete?.(result, i)
      } catch (err) {
        onItemError?.(err, i)
      }
    }
  }

  const workers = Array.from(
    { length: Math.min(concurrency, items.length) },
    () => worker(),
  )
  await Promise.all(workers)

  return results.filter((r): r is R => r !== undefined)
}
