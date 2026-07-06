import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";
import { rematchRadioPlaysChunked } from "../lib/radio";
import { dim } from "../lib/ansi";

export interface RadioRematchCliOptions {
  stationId?: string;
  showSlug?: string;
  artistName?: string[];
  dryRun?: boolean;
  limit?: string;
}

function parsePositiveInt(value: string | undefined, label: string): number | undefined {
  if (value === undefined || value === "") return undefined;
  const n = Number(value);
  if (!Number.isInteger(n) || n <= 0) {
    throw new Error(`${label} must be a positive integer, got "${value}"`);
  }
  return n;
}

/** CLI entry for `ph radio rematch`. */
export async function runRadioRematch(
  env: EnvironmentConfig,
  opts: RadioRematchCliOptions,
): Promise<void> {
  const stationId = parsePositiveInt(opts.stationId, "--station");
  const maxGroups = parsePositiveInt(opts.limit, "--limit");
  const artistNames = opts.artistName?.map((n) => n.trim()).filter(Boolean);

  const client = new APIClient(env);

  if (opts.dryRun) {
    display.info("Dry run — listing artist names that would be rematched.");
  } else if (opts.showSlug) {
    display.info(`Rematching unmatched plays for show "${opts.showSlug}"…`);
  } else if (stationId) {
    display.info(`Rematching unmatched plays for station ${stationId}…`);
  } else if (artistNames?.length) {
    display.info(`Rematching ${artistNames.length} artist name(s)…`);
  } else {
    display.info("Rematching all unmatched radio plays (chunked by artist name)…");
  }

  const result = await rematchRadioPlaysChunked(client, {
    stationId,
    showSlug: opts.showSlug,
    artistNames,
    dryRun: opts.dryRun,
    maxGroups,
    onProgress: opts.dryRun
      ? undefined
      : (name, index, total) => {
          display.info(`[${index}/${total}] ${name}`);
        },
  });

  if (opts.dryRun) {
    const names = result.names ?? [];
    if (names.length === 0) {
      display.warn("No unmatched artist names found.");
      return;
    }
    for (const name of names) {
      process.stdout.write(`  ${name}\n`);
    }
    display.info(`${names.length} distinct name(s) would be rematched.`);
    display.warn("Dry run. Re-run without --dry-run to execute.");
    return;
  }

  if (result.matched > 0) {
    display.success(
      `Linked ${result.matched} play(s) across ${result.namesProcessed} artist name(s) ` +
        `(${result.unmatched} still unmatched of ${result.total} plays scanned).`,
    );
  } else {
    display.info(
      `No new play links across ${result.namesProcessed} name(s) ` +
        `(${result.total} unmatched plays scanned).`,
    );
  }

  if ((result.persist_errors ?? 0) > 0) {
    display.warn(`${result.persist_errors} persist error(s) — check server logs.`);
  }

  if (!opts.dryRun) {
    display.kv("Names processed", String(result.namesProcessed));
    display.kv("Plays scanned", String(result.total));
    display.kv("Newly linked", dim(String(result.matched)));
  }
}
