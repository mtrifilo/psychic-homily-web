/**
 * Rendering the verdict — for the Actions log and for Discord.
 *
 * The alert carries the actual numbers, not just a pass/fail. A bare "check
 * failed" is the kind of alert that gets muted, and a muted alert is the same
 * as no alert — which is the state this monitor exists to leave behind.
 */

import type { Comparison, Report } from './evaluate'

/** Matches the backend's Discord embed palette in services/notification/discord.go. */
const COLOR_RED = 0xff0000
const COLOR_GREEN = 0x00ff00
const COLOR_ORANGE = 0xffa500

// Discord's documented embed limits. Exceeding any of them makes the webhook
// return 400, i.e. the alert is silently lost at exactly the moment it matters.
const MAX_DESCRIPTION = 4096
const MAX_FIELD_VALUE = 1024
const MAX_TITLE = 256

export interface DiscordEmbedField {
  name: string
  value: string
  inline?: boolean
}

export interface DiscordEmbed {
  title: string
  description?: string
  color: number
  fields?: DiscordEmbedField[]
  timestamp?: string
}

export interface DiscordPayload {
  embeds: DiscordEmbed[]
}

export function truncate(value: string, limit: number): string {
  if (value.length <= limit) return value
  const ellipsis = '…'
  return value.slice(0, Math.max(0, limit - ellipsis.length)) + ellipsis
}

/** `<index> (10 shards)` — shared so the two formatters cannot word it differently. */
function shapeLabel(report: Report): string {
  return `<${report.shape}>${report.shape === 'index' ? ` (${report.shardCount} shards)` : ''}`
}

/** `9/10` reachable sampled URLs. */
function sampleTally(report: Report): string {
  return `${report.samples.filter(s => s.ok).length}/${report.samples.length}`
}

/**
 * `ok   shows  sitemap 1458 / api 1458  (+0, tol ±292)` — one aligned line per
 * labelled comparison, so the family table and the per-document table below it
 * cannot word the same numbers differently.
 */
function comparisonLines(rows: ReadonlyArray<{ label: string } & Comparison>): string[] {
  if (rows.length === 0) return []
  const width = Math.max(...rows.map(row => row.label.length))
  return rows.map(row => {
    const mark = row.ok ? 'ok  ' : 'FAIL'
    const sign = row.delta > 0 ? '+' : ''
    const note = row.vanished ? ' VANISHED' : ''
    return `${mark} ${row.label.padEnd(width)}  sitemap ${row.observed} / api ${row.expected}  (${sign}${row.delta}, tol ±${row.allowed})${note}`
  })
}

/** One line per family. */
function familyLines(report: Report): string[] {
  return comparisonLines(report.families.map(c => ({ ...c, label: c.family })))
}

/**
 * One line per sub-shard document.
 *
 * Written to the Actions log only. The Discord embed carries the family table
 * plus the failure list: a per-document table is one line per bucket of every
 * sub-sharded family, which is noise in a phone notification, and the failing
 * documents are named in the failure list anyway.
 */
function shardLines(report: Report): string[] {
  return comparisonLines(report.shards.map(c => ({ ...c, label: c.shard })))
}

/** The full human-readable report written to the Actions log. */
export function formatConsoleReport(report: Report): string {
  const lines: string[] = [
    `Sitemap freshness — ${report.ok ? 'PASS' : 'FAIL'}`,
    `target:  ${report.target}`,
    `shape:   ${shapeLabel(report)}`,
    // `unclassified` only means anything for the single-document shape: on the
    // sharded path every shard id resolves to a family through the shard table,
    // so nothing is ever classified and a printed `unclassified 0` would read
    // as evidence that every URL was recognised.
    `totals:  sitemap ${report.observedTotal} locs (entities ${report.expectedTotal} per API, pages ${report.observedPages}${report.shape === 'urlset' ? `, unclassified ${report.observedOther}` : ''})`,
    '',
    ...familyLines(report),
    '',
    ...shardLines(report),
    ...(report.shards.length > 0 ? [''] : []),
    `${report.futureShows.ok ? 'ok  ' : 'FAIL'} upcoming shows  ${report.futureShows.observed} (need ≥ ${report.futureShows.required})`,
    `${report.samples.every(s => s.ok) ? 'ok  ' : 'FAIL'} sampled URLs    ${sampleTally(report)} reachable`,
  ]

  const badSamples = report.samples.filter(s => !s.ok)
  if (badSamples.length > 0) {
    lines.push('', 'unreachable:')
    for (const sample of badSamples) {
      lines.push(`  ${sample.url} → ${sample.error ?? sample.status}`)
    }
  }

  if (report.failures.length > 0) {
    lines.push('', 'failures:')
    for (const failure of report.failures) lines.push(`  - ${failure}`)
  }

  return lines.join('\n')
}

/**
 * The Discord alert.
 *
 * The per-family table goes in the description as a code block so the numbers
 * stay aligned and readable on mobile, where field columns wrap badly. Every
 * string is truncated to Discord's limit — an over-long payload is rejected
 * with a 400 and the alert never arrives.
 */
export function formatDiscordPayload(report: Report, now: Date): DiscordPayload {
  const headline = report.ok
    ? 'Sitemap freshness check passed'
    : 'Sitemap freshness check FAILED'

  const table = ['```', ...familyLines(report), '```'].join('\n')
  const summary = [
    `**${report.target}** — \`${shapeLabel(report)}\``,
    `${report.observedTotal} sitemap URLs vs ${report.expectedTotal} API entities`,
    `Upcoming shows: **${report.futureShows.observed}** (need ≥ ${report.futureShows.required})`,
    `Sampled URLs reachable: **${sampleTally(report)}**`,
    '',
    table,
  ].join('\n')

  const fields: DiscordEmbedField[] = []
  if (report.failures.length > 0) {
    fields.push({
      name: `Failures (${report.failures.length})`,
      value: truncate(report.failures.map(f => `• ${f}`).join('\n'), MAX_FIELD_VALUE),
    })
  }

  return {
    embeds: [
      {
        title: truncate(headline, MAX_TITLE),
        description: truncate(summary, MAX_DESCRIPTION),
        color: report.ok ? COLOR_GREEN : COLOR_RED,
        fields: fields.length > 0 ? fields : undefined,
        timestamp: now.toISOString(),
      },
    ],
  }
}

/**
 * The alert sent when the check could not run at all — an unreachable API feed,
 * a sitemap that is neither shape, a malformed threshold.
 *
 * Lives here beside the other formatter so Discord's limits have exactly one
 * owner; when this was inlined in cli.ts it carried its own, different cap.
 * Reported as loudly as a drift failure: "the monitor crashed" and "the sitemap
 * is fine" must never look the same from the outside.
 */
export function formatCrashPayload(
  target: string,
  message: string,
  now: Date
): DiscordPayload {
  const wrapper = `**${target}**\n\`\`\`\n\n\`\`\``
  return {
    embeds: [
      {
        title: 'Sitemap freshness check could not run',
        description: `**${target}**\n\`\`\`\n${truncate(message, MAX_DESCRIPTION - wrapper.length)}\n\`\`\``,
        color: COLOR_ORANGE,
        timestamp: now.toISOString(),
      },
    ],
  }
}
