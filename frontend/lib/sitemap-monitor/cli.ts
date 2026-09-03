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

import { resolveConfig, type Env, type MonitorConfig } from './config'
import {
  countFutureShows,
  evaluate,
  isoDate,
  pickStratifiedSample,
  type Report,
} from './evaluate'
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
  // runner time to reach the same conclusion. The two phases hit different
  // origins, so overlapping them would be safe for the rate limiters and would
  // save the count phase's waves; the fail-fast is kept deliberately, and it is
  // now several waves rather than a single request.
  const expected = await fetchExpectedCounts(config)
  const observation = await walkSitemap(config)

  const sampled = pickStratifiedSample(observation.locsByBucket, config.sampleSize)
  const samples = await sampleUrls(sampled, config)

  return evaluate(
    {
      shape: observation.shape,
      shardCount: observation.shardCount,
      observedByFamily: observation.observedByFamily,
      observedPages: observation.observedPages,
      observedOther: observation.observedOther,
      expectedByFamily: expected.byFamily,
      observedByShard: observation.observedByShard,
      expectedByShard: expected.byShard,
      unservedShards: expected.unservedShards,
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
    // Scrub the URL out of the message before printing. A malformed webhook
    // URL makes fetch throw "Failed to parse URL from <the-url>", which would
    // print the secret token straight into the Actions log — the same hazard
    // the backend's utils.RedactErrorURL exists for.
    const message = (error as Error).message.split(webhookUrl).join('[redacted]')
    console.error(`Discord webhook failed: ${message}`)
    return false
  }
}

/**
 * Exit codes:
 *   0 — the sitemap is healthy.
 *   1 — the sitemap FAILED an assertion (alerted, if a webhook is configured).
 *   2 — the check could not run, or could not deliver its alert.
 *
 * 1 and 2 are kept distinct on purpose: "the sitemap is stale" and "the monitor
 * is broken" need different responses, and conflating them is how a broken
 * monitor gets mistaken for a quiet one.
 */
export async function main(env: Env, now: Date): Promise<number> {
  let config: MonitorConfig
  try {
    config = resolveConfig(env)
  } catch (error) {
    const message = (error as Error).message
    console.error(`configuration error: ${message}`)
    // Alert from the RAW env rather than giving up. A typo'd threshold set from
    // the GitHub UI throws here, and without this the monitor would be dead
    // every day with nothing but a red scheduled run to show for it — and
    // GitHub disables scheduled workflows after 60 days of repo inactivity, so
    // even that signal decays. resolveConfig never throws on the webhook URL
    // itself, so it is still usable at this point.
    const webhookUrl = env.DISCORD_WEBHOOK_URL?.trim()
    if (webhookUrl) {
      await postToDiscord(
        webhookUrl,
        formatCrashPayload(
          env.SITEMAP_MONITOR_TARGET?.trim() || '(target unresolved)',
          `configuration error: ${message}`,
          now
        ),
        30_000
      )
    }
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
    // A failed delivery must not overwrite a real sitemap failure with the
    // "monitor broken" code — the stale sitemap is still the finding, and 1 is
    // what says so.
    if (!delivered && report.ok) return 2
  } else if (!config.discordWebhookUrl && !report.ok) {
    console.error('DISCORD_WEBHOOK_URL is not set — failure was not alerted anywhere')
  }

  return report.ok ? 0 : 1
}

// `import.meta.main` is true only when this file is the entry point, so the
// module stays importable from tests without executing.
//
// Cast because `main` is a Bun extension to ImportMeta that the standard lib
// does not declare — `tsc` in CI rejects the bare property access even though
// it resolves locally, so the cast is what keeps the two in agreement.
if ((import.meta as ImportMeta & { main?: boolean }).main) {
  process.exitCode = await main(process.env, new Date())
}
