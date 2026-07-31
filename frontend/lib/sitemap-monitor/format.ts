/**
 * Rendering the verdict — for the Actions log and for Discord.
 *
 * The alert carries the actual numbers, not just a pass/fail. A bare "check
 * failed" is the kind of alert that gets muted, and a muted alert is the same
 * as no alert — which is the state this monitor exists to leave behind.
 */

import type { Report } from './evaluate'

/** Matches the backend's Discord embed palette in services/notification/discord.go. */
const COLOR_RED = 0xff0000
const COLOR_GREEN = 0x00ff00

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

/** `shows  1458 / 1458  (+0, ±292)` — one aligned line per family. */
function familyLines(report: Report): string[] {
  const width = Math.max(...report.families.map(c => c.family.length))
  return report.families.map(c => {
    const mark = c.ok ? 'ok  ' : 'FAIL'
    const sign = c.delta > 0 ? '+' : ''
    return `${mark} ${c.family.padEnd(width)}  sitemap ${c.observed} / api ${c.expected}  (${sign}${c.delta}, tol ±${c.allowed})`
  })
}

/** The full human-readable report written to the Actions log. */
export function formatConsoleReport(report: Report): string {
  const lines: string[] = [
    `Sitemap freshness — ${report.ok ? 'PASS' : 'FAIL'}`,
    `target:  ${report.target}`,
    `shape:   <${report.shape}>${report.shape === 'index' ? ` (${report.shardCount} shards)` : ''}`,
    `totals:  sitemap ${report.observedTotal} locs (entities ${report.expectedTotal} per API, pages ${report.observedPages}, unclassified ${report.observedOther})`,
    '',
    ...familyLines(report),
    '',
    `${report.futureShows.ok ? 'ok  ' : 'FAIL'} upcoming shows  ${report.futureShows.observed} (need ≥ ${report.futureShows.required})`,
    `${report.samples.every(s => s.ok) ? 'ok  ' : 'FAIL'} sampled URLs    ${report.samples.filter(s => s.ok).length}/${report.samples.length} reachable`,
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
    `**${report.target}** — \`<${report.shape}>\`${report.shape === 'index' ? ` (${report.shardCount} shards)` : ''}`,
    `${report.observedTotal} sitemap URLs vs ${report.expectedTotal} API entities`,
    `Upcoming shows: **${report.futureShows.observed}** (need ≥ ${report.futureShows.required})`,
    `Sampled URLs reachable: **${report.samples.filter(s => s.ok).length}/${report.samples.length}**`,
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
