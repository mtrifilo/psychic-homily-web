import { Command } from "commander";
import { version } from "../package.json";
import { readConfig, resolveEnvironment } from "./lib/config";
import { APIError } from "./lib/api";
import * as display from "./lib/display";
import { runInit } from "./commands/init";
import { runConfigShow, runConfigSet } from "./commands/config";
import { runSearch } from "./commands/search";
import { runSubmitArtist } from "./commands/submit-artist";
import { runSubmitVenue } from "./commands/submit-venue";
import { runSubmitShow } from "./commands/submit-show";
import { submitReleases } from "./commands/submit-release";
import { runSubmitLabel } from "./commands/submit-label";
import { runSubmitFestival } from "./commands/submit-festival";
import { runBatch } from "./commands/batch";
import { runStatus } from "./commands/status";
import { runFestivalLinkArtists, runFestivalUnlinkArtist } from "./commands/festival";
import { runShowAddArtist, runShowRemoveArtist } from "./commands/show";

const program = new Command();

program
  .name("ph")
  .description("CLI for rapid knowledge graph data entry into Psychic Homily")
  .version(version, "-V, --version")
  .option("--env <environment>", "Target environment (default: from config)")
  .option("-v, --verbose", "Log full HTTP request/response details to stderr")
  .showHelpAfterError();

// ─── ph init ───────────────────────────────────────────────────────────────────

program
  .command("init")
  .description("Configure an API environment (URL + token)")
  .requiredOption("--url <url>", "API base URL")
  .requiredOption("--token <token>", "API token (phk_...)")
  .option("--name <name>", "Environment name", "production")
  .option("--set-default", "Set this environment as the default")
  .action(async function (this: Command) {
    await runInit(this.opts());
  });

// ─── ph config ─────────────────────────────────────────────────────────────────

const configCmd = program
  .command("config")
  .description("View or edit CLI configuration");

configCmd
  .command("show")
  .description("Display current configuration")
  .action(async () => {
    await runConfigShow();
  });

configCmd
  .command("set <key> <value>")
  .description("Set a configuration value (e.g., default_environment)")
  .action(async (key: string, value: string) => {
    await runConfigSet(key, value);
  });

// ─── ph search ─────────────────────────────────────────────────────────────────

program
  .command("search <entity-type> <query>")
  .description(
    "Search for existing entities (artist, venue, release, label, festival, show)",
  )
  .action(async (entityType: string, query: string) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runSearch(entityType, query, env);
  });

// ─── ph submit ────────────────────────────────────────────────────────────────

const SUBMIT_TYPES = ["artist", "venue", "show", "release", "label", "festival"];

program
  .command("submit <entity-type> [json]")
  .description("Submit entities for creation/update (artist, venue, show, release, label, festival)")
  .option("--confirm", "Actually submit (default is dry-run)")
  .option("--force", "Skip duplicate checking (force create)")
  .action(async (entityType: string, json: string | undefined, opts: { confirm?: boolean; force?: boolean }) => {
    if (!SUBMIT_TYPES.includes(entityType)) {
      display.error(
        `Invalid entity type "${entityType}". Must be one of: ${SUBMIT_TYPES.join(", ")}`,
      );
      process.exit(1);
    }

    const env = await resolveEnvOrExit(program.opts().env);

    switch (entityType) {
      case "artist":
        await runSubmitArtist(json, env, { confirm: opts.confirm, force: opts.force });
        break;
      case "venue":
        await runSubmitVenue(json, opts, env);
        break;
      case "show":
        await runSubmitShow(json, env, !!opts.confirm);
        break;
      case "release":
        await submitReleases(json ?? "", env, !!opts.confirm);
        break;
      case "label":
        await runSubmitLabel(json, opts, env);
        break;
      case "festival":
        await runSubmitFestival(json, env, !!opts.confirm);
        break;
      default:
        display.warn(
          `"ph submit ${entityType}" is not yet implemented.`,
        );
        process.exit(1);
    }
  });

// ─── ph batch ────────────────────────────────────────────────────────────────

program
  .command("batch <file>")
  .description("Submit a mixed-entity JSON file for batch creation/update")
  .option("--confirm", "Actually submit (default is dry-run)")
  .action(async (file: string, opts: { confirm?: boolean }) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runBatch(file, env, !!opts.confirm);
  });

// ─── ph festival ─────────────────────────────────────────────────────────────

const festivalCmd = program
  .command("festival")
  .description("Manage festival artist links");

festivalCmd
  .command("link-artists <festival> [json]")
  .description("Link artists to an existing festival by slug or ID")
  .option("--file <path>", "Read artist JSON from file")
  .option("--replace", "Remove existing artist links first")
  .option("--confirm", "Execute changes (default is dry-run)")
  .action(async (festival: string, json: string | undefined, opts: { file?: string; replace?: boolean; confirm?: boolean }) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runFestivalLinkArtists(festival, json, env, opts);
  });

festivalCmd
  .command("unlink-artist <festival> <artist>")
  .description("Remove an artist link from a festival")
  .option("--confirm", "Execute changes (default is dry-run)")
  .action(async (festival: string, artist: string, opts: { confirm?: boolean }) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runFestivalUnlinkArtist(festival, artist, env, !!opts.confirm);
  });

// ─── ph show ─────────────────────────────────────────────────────────────────

const showCmd = program
  .command("show")
  .description("Manage show artist links");

showCmd
  .command("add-artist <show-id> [json]")
  .description("Add artists to an existing show by ID")
  .option("--file <path>", "Read artist JSON from file")
  .option("--confirm", "Execute changes (default is dry-run)")
  .action(async (showId: string, json: string | undefined, opts: { file?: string; confirm?: boolean }) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runShowAddArtist(showId, json, env, opts);
  });

showCmd
  .command("remove-artist <show-id> <artist>")
  .description("Remove an artist from a show")
  .option("--confirm", "Execute changes (default is dry-run)")
  .action(async (showId: string, artist: string, opts: { confirm?: boolean }) => {
    const env = await resolveEnvOrExit(program.opts().env);
    await runShowRemoveArtist(showId, artist, env, !!opts.confirm);
  });

// ─── ph status ───────────────────────────────────────────────────────────────

program
  .command("status")
  .description("Show CLI configuration, API connectivity, and auth status")
  .action(async () => {
    await runStatus(program.opts().env, program.opts().verbose);
  });

// ─── Helpers ───────────────────────────────────────────────────────────────────

async function resolveEnvOrExit(
  envOverride?: string,
): Promise<{ url: string; token: string; verbose?: boolean }> {
  const config = await readConfig();
  const resolved = resolveEnvironment(config, envOverride);

  if (!resolved) {
    const envName = envOverride || config.default_environment || "(not set)";
    display.error(
      `Environment "${envName}" not found. Run "ph init" to configure one.`,
    );
    process.exit(1);
  }

  const verbose = program.opts().verbose ?? false;
  return { ...resolved.env, verbose };
}

// ─── Run ───────────────────────────────────────────────────────────────────────

try {
  await program.parseAsync();
} catch (err) {
  if (err instanceof APIError) {
    display.error(`API error (${err.status}): ${err.message}`);
    if (err.requestId) {
      display.kv("Request ID", err.requestId);
    }
  } else {
    const message =
      err instanceof Error ? err.message : "Unexpected error.";
    display.error(message);
  }
  process.exit(1);
}
