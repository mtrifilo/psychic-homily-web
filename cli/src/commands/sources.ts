import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";

/** API representation of a source-config registry row (mirrors the backend). */
export interface SourceConfigInfo {
  id: number;
  entity_type: string;
  entity_id: number;
  source_url: string | null;
  last_refreshed_at: string | null;
  last_content_hash: string | null;
  consecutive_failures: number;
  created_at: string;
  updated_at: string;
}

const ENTITY_TYPES = ["venue", "label"];

export interface StaleOptions {
  limit?: string | number;
  maxFailures?: string | number;
}

/**
 * Validate and normalize an (entity_type, entity_id) pair. Throws on bad input
 * so the CLI wrappers can surface a clean error; the testable cores stay pure.
 */
export function parseEntity(entityType: string, entityId: string): { entityType: string; entityId: number } {
  if (!ENTITY_TYPES.includes(entityType)) {
    throw new Error(`Invalid entity type "${entityType}". Must be one of: ${ENTITY_TYPES.join(", ")}`);
  }
  const id = Number(entityId);
  if (!Number.isInteger(id) || id <= 0) {
    throw new Error(`entity_id must be a positive integer, got "${entityId}"`);
  }
  return { entityType, entityId: id };
}

// --- Testable cores (take a client) ---

/** Fetch the stalest registered sources (stalest first). */
export async function listStale(client: APIClient, opts: StaleOptions): Promise<SourceConfigInfo[]> {
  const params: Record<string, string> = {};
  if (opts.limit !== undefined && opts.limit !== "") params.limit = String(opts.limit);
  if (opts.maxFailures !== undefined && opts.maxFailures !== "") params.max_failures = String(opts.maxFailures);
  const res = await client.get<{ sources: SourceConfigInfo[]; count: number }>("/admin/sources", params);
  return res.sources ?? [];
}

/** Register or update a source for a venue/label. */
export async function registerSource(
  client: APIClient,
  entityType: string,
  entityId: string,
  sourceUrl?: string,
): Promise<SourceConfigInfo> {
  const { entityType: t, entityId: id } = parseEntity(entityType, entityId);
  const body: Record<string, unknown> = { entity_type: t, entity_id: id };
  if (sourceUrl) body.source_url = sourceUrl;
  return client.put<SourceConfigInfo>("/admin/sources", body);
}

/** Stamp a successful refresh (sets last_refreshed_at, resets failures). */
export async function recordRefresh(
  client: APIClient,
  entityType: string,
  entityId: string,
  contentHash?: string,
): Promise<SourceConfigInfo> {
  const { entityType: t, entityId: id } = parseEntity(entityType, entityId);
  const body: Record<string, unknown> = { entity_type: t, entity_id: id };
  if (contentHash) body.content_hash = contentHash;
  return client.post<SourceConfigInfo>("/admin/sources/refresh", body);
}

// --- CLI wrappers (build the client, render, exit on error) ---

function fail(err: unknown): never {
  display.error(err instanceof Error ? err.message : String(err));
  process.exit(1);
}

export async function runSourcesStale(env: EnvironmentConfig, opts: StaleOptions): Promise<void> {
  const client = new APIClient(env);
  try {
    const sources = await listStale(client, opts);
    if (sources.length === 0) {
      display.info("No source configs registered.");
      return;
    }
    const rows: string[][] = [["TYPE", "ID", "LAST REFRESHED", "FAILS", "SOURCE URL"]];
    for (const s of sources) {
      rows.push([
        s.entity_type,
        String(s.entity_id),
        s.last_refreshed_at ? s.last_refreshed_at.slice(0, 10) : "never",
        String(s.consecutive_failures),
        (s.source_url ?? "").slice(0, 60),
      ]);
    }
    display.table(rows);
    display.info(`${sources.length} source(s), stalest first`);
  } catch (err) {
    fail(err);
  }
}

export async function runSourcesRegister(
  entityType: string,
  entityId: string,
  sourceUrl: string | undefined,
  env: EnvironmentConfig,
): Promise<void> {
  const client = new APIClient(env);
  try {
    const out = await registerSource(client, entityType, entityId, sourceUrl);
    display.success(`Registered ${out.entity_type} ${out.entity_id}${out.source_url ? ` → ${out.source_url}` : ""}`);
  } catch (err) {
    fail(err);
  }
}

export async function runSourcesRefresh(
  entityType: string,
  entityId: string,
  env: EnvironmentConfig,
  opts: { contentHash?: string },
): Promise<void> {
  const client = new APIClient(env);
  try {
    const out = await recordRefresh(client, entityType, entityId, opts.contentHash);
    display.success(`Stamped refresh for ${out.entity_type} ${out.entity_id} (failures reset)`);
  } catch (err) {
    fail(err);
  }
}
