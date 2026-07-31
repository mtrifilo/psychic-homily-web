/**
 * Entry point for the scheduled sitemap freshness check.
 *
 *   bun run sitemap:check
 *
 * Lives under lib/ rather than scripts/ on purpose: tsconfig.json excludes
 * `scripts`, so a monitor written there would be exempt from `bun run
 * typecheck` and `bun run lint` — an unchecked checker.
 *
 * Runs from a GitHub Actions schedule, deliberately OUTSIDE the application.
 * The original stale-sitemap defect was invisible from inside the system:
 * every component reported healthy and the only way to see it was to fetch
 * production from outside and count. An in-app health check would have been
 * green throughout, and a Go background ticker would have been starved by the
 * deploy cadence (pattern_ticker_starved_by_deploys.md).
 *
 * Alert-only. It never gates a deploy — production observation is external
 * state, and coupling releases to it trades one outage for another.
 */

import { resolveConfig, type MonitorConfig } from './config'
import { countFutureShows, evaluate, isoDate, pickSample, type Report } from './evaluate'
import { fetchExpectedCounts, sampleUrls, walkSitemap } from './fetch'
import {
  formatConsoleReport,
  formatCrashPayload,
  formatDiscordPayload,
  type DiscordPayload,
} from './format'

export async function runCheck(config: MonitorConfig, now: Date): Promise<Report> {
  // Sequential, not Promise.all: if the API feed is down there is nothing to
  // compare against, and crawling every shard first would waste a minute of
  // runner time to reach the same conclusion.
  const expectedByFamily = await fetchExpectedCounts(config)
  const observation = await walkSitemap(config)

  const sampled = pickSample(observation.locs, config.sampleSize)
  const samples = await sampleUrls(sampled, config)

  return evaluate(
    {
      shape: observation.shape,
      shardCount: observation.shardCount,
      observedByFamily: observation.observedByFamily,
      observedPages: observation.observedPages,
      observedOther: observation.observedOther,
      expectedByFamily,
      futureShowCount: countFutureShows(observation.showDates, isoDate(now)),
      samples,
      errors: observation.errors,
    },
    config
  )
}

/**
 * Post the alert.
 *
 * Returns false when the webhook itself failed, so the caller can say so in
 * the log. A monitor whose alerting is broken is indistinguishable from a
 * healthy system, which is precisely the trap being closed here.
 */
async function postToDiscord(
  webhookUrl: string,
  payload: DiscordPayload,
  timeoutMs: number
): Promise<boolean> {
  try {
    const response = await fetch(webhookUrl, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(timeoutMs),
    })
    if (!response.ok) {
      // Never log the URL: the webhook path carries a secret token.
      console.error(`Discord webhook returned ${response.status}`)
      return false
    }
    return true
  } catch (error) {
    console.error(`Discord webhook failed: ${(error as Error).message}`)
    return false
  }
}

export async function main(env: NodeJS.ProcessEnv, now: Date): Promise<number> {
  let config: MonitorConfig
  try {
    config = resolveConfig(env)
  } catch (error) {
    // No config means no webhook URL to alert on; the non-zero exit and the
    // red Actions run are the whole signal here.
    console.error(`configuration error: ${(error as Error).message}`)
    return 2
  }

  let report: Report
  try {
    report = await runCheck(config, now)
  } catch (error) {
    const message = (error as Error).message
    console.error(`sitemap freshness check could not run: ${message}`)
    if (config.discordWebhookUrl) {
      await postToDiscord(
        config.discordWebhookUrl,
        formatCrashPayload(config.target, message, now),
        config.fetchTimeoutMs
      )
    }
    return 2
  }

  console.log(formatConsoleReport(report))

  if (config.discordWebhookUrl && (!report.ok || config.notifyOnSuccess)) {
    const delivered = await postToDiscord(
      config.discordWebhookUrl,
      formatDiscordPayload(report, now),
      config.fetchTimeoutMs
    )
    if (!delivered) return 2
  } else if (!config.discordWebhookUrl && !report.ok) {
    console.error('DISCORD_WEBHOOK_URL is not set — failure was not alerted anywhere')
  }

  return report.ok ? 0 : 1
}

// `import.meta.main` is true only when this file is the entry point, so the
// module stays importable from tests without executing.
if (import.meta.main) {
  process.exitCode = await main(process.env, new Date())
}
