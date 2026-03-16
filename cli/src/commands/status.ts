import { APIClient } from "../lib/api";
import { readConfig, resolveEnvironment, getConfigPath } from "../lib/config";
import * as display from "../lib/display";
import { green, red, dim, gray } from "../lib/ansi";

/**
 * CLI entry point for `ph status`.
 * Shows current configuration, API reachability, and auth status.
 */
export async function runStatus(envOverride?: string): Promise<void> {
  const config = await readConfig();
  const resolved = resolveEnvironment(config, envOverride);

  display.header("PH CLI Status");

  // Config file location
  display.kv("Config", getConfigPath());

  // Environment info
  if (!resolved) {
    const envName = envOverride || config.default_environment || "(not set)";
    display.kv("Environment", `${envName} ${red("(not configured)")}`);
    display.info('Run "ph init --url <url> --token <token>" to configure.');
    displayAvailableCommands();
    return;
  }

  display.kv("Environment", `${resolved.name}`);
  display.kv("API URL", resolved.env.url);
  display.kv("Token", maskToken(resolved.env.token));

  // Check API reachability
  const client = new APIClient(resolved.env);

  const healthy = await client.healthCheck();
  if (!healthy) {
    display.kv("API Status", red("unreachable"));
    display.warn(`Cannot connect to ${resolved.env.url}`);
    displayAvailableCommands();
    return;
  }

  display.kv("API Status", green("reachable"));

  // Check auth
  const user = await client.verifyAuth();
  if (user) {
    display.kv("Authenticated as", `${user.username} (ID: ${user.id})`);
    if (user.is_admin) {
      display.kv("Role", "admin");
    }
  } else {
    display.kv("Auth", red("token invalid or expired"));
  }

  displayAvailableCommands();
}

/**
 * Mask a token for display, showing only first 8 chars.
 */
function maskToken(token: string): string {
  if (token.length <= 8) return token;
  return `${token.slice(0, 8)}${"*".repeat(Math.min(token.length - 8, 16))}`;
}

/**
 * Display available CLI commands.
 */
function displayAvailableCommands(): void {
  display.header("Available Commands");
  const commands = [
    ["ph init", "Configure an API environment"],
    ["ph config show", "Display current configuration"],
    ["ph search <type> <query>", "Search for entities"],
    ["ph submit <type> <json>", "Submit entities for creation"],
    ["ph batch <file>", "Batch submit from JSON file"],
    ["ph status", "Show this status"],
  ];

  for (const [cmd, desc] of commands) {
    process.stderr.write(`  ${cmd.padEnd(30)} ${dim(desc)}\n`);
  }
}
