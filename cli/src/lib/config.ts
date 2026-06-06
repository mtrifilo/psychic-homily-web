import { join } from "path";
import { homedir } from "os";
import type { EnvironmentConfig, PHConfig } from "./types";

const DEFAULT_CONFIG: PHConfig = {
  environments: {},
  default_environment: "production",
};

function getConfigDir(): string {
  return process.env.PH_CONFIG_PATH || join(homedir(), ".psychic-homily");
}

function getConfigFile(): string {
  return join(getConfigDir(), "config.json");
}

/** Read the config file, returning defaults if it doesn't exist. */
export async function readConfig(): Promise<PHConfig> {
  try {
    const file = Bun.file(getConfigFile());
    if (!(await file.exists())) {
      return { ...DEFAULT_CONFIG };
    }
    const raw = await file.text();
    return JSON.parse(raw) as PHConfig;
  } catch {
    return { ...DEFAULT_CONFIG };
  }
}

/** Write the config file, creating the directory if needed. */
export async function writeConfig(config: PHConfig): Promise<void> {
  const { mkdir } = await import("fs/promises");
  await mkdir(getConfigDir(), { recursive: true });
  await Bun.write(getConfigFile(), JSON.stringify(config, null, 2) + "\n");
}

/** Resolve which environment to use, applying the --env flag override. */
export function resolveEnvironment(
  config: PHConfig,
  envOverride?: string,
): { name: string; env: EnvironmentConfig } | null {
  const name = envOverride || config.default_environment;
  const env = config.environments[name];
  if (!env) return null;
  return { name, env };
}

/**
 * Suggest the configured environment name closest to a mistyped one, for
 * "did you mean" error hints. Prefers a substring match in either direction
 * (catches `staging`↔`stage`, `prod`↔`production`), then falls back to the
 * nearest name by edit distance within a small threshold. Returns null when
 * nothing is close enough to suggest.
 */
export function suggestEnvironment(
  input: string,
  configured: string[],
): string | null {
  const target = input.toLowerCase();

  const substringMatch = configured.find((name) => {
    const lower = name.toLowerCase();
    return lower.includes(target) || target.includes(lower);
  });
  if (substringMatch) return substringMatch;

  let best: string | null = null;
  let bestDistance = Infinity;
  for (const name of configured) {
    const distance = levenshtein(target, name.toLowerCase());
    if (distance < bestDistance) {
      bestDistance = distance;
      best = name;
    }
  }
  // The threshold must be >= 3: the motivating case is `staging` -> `stage`,
  // which is distance 3 (drop "ing", substitute to "e") and is NOT a substring
  // match ("staging" does not contain "stage"). The substring branch above only
  // covers prefix/suffix cases like `prod` -> `production`. Keep it at 3 so the
  // tail typos still get a hint without suggesting wildly different names.
  return best !== null && bestDistance <= 3 ? best : null;
}

/** Levenshtein edit distance between two strings (single-row DP). */
function levenshtein(a: string, b: string): number {
  if (a === b) return 0;
  if (a.length === 0) return b.length;
  if (b.length === 0) return a.length;

  let prev = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 0; i < a.length; i++) {
    const curr = [i + 1];
    for (let j = 0; j < b.length; j++) {
      const cost = a[i] === b[j] ? 0 : 1;
      curr.push(Math.min(prev[j + 1] + 1, curr[j] + 1, prev[j] + cost));
    }
    prev = curr;
  }
  return prev[b.length];
}

/** Get the config file path (for display purposes). */
export function getConfigPath(): string {
  return getConfigFile();
}
