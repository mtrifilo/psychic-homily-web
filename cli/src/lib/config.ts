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

/** Get the config file path (for display purposes). */
export function getConfigPath(): string {
  return getConfigFile();
}
