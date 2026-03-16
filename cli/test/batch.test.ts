import { describe, test, expect } from "bun:test";
import {
  parseBatchInput,
  validateBatchItems,
  groupByType,
} from "../src/commands/batch";

// -- parseBatchInput --

describe("parseBatchInput", () => {
  test("parses a valid JSON array", () => {
    const input = JSON.stringify([
      { entity_type: "artist", name: "Nina Hagen" },
      { entity_type: "label", name: "Numero" },
    ]);
    const result = parseBatchInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].entity_type).toBe("artist");
    expect(result[1].entity_type).toBe("label");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseBatchInput("not json at all")).toThrow("Invalid JSON");
  });

  test("throws on non-array JSON (object)", () => {
    expect(() =>
      parseBatchInput('{"entity_type": "artist", "name": "Test"}'),
    ).toThrow("must contain a JSON array");
  });

  test("throws on non-array JSON (string)", () => {
    expect(() => parseBatchInput('"just a string"')).toThrow(
      "must contain a JSON array",
    );
  });

  test("parses an empty array", () => {
    const result = parseBatchInput("[]");
    expect(result).toHaveLength(0);
  });
});

// -- validateBatchItems --

describe("validateBatchItems", () => {
  test("returns no errors for valid items", () => {
    const items = [
      { entity_type: "artist", name: "Test" },
      { entity_type: "label", name: "Test Label" },
      { entity_type: "release", title: "Test Release" },
      { entity_type: "venue", name: "Test Venue" },
      { entity_type: "festival", name: "Test Fest" },
      { entity_type: "show", event_date: "2026-06-01" },
    ];
    const errors = validateBatchItems(items);
    expect(errors).toHaveLength(0);
  });

  test("reports missing entity_type", () => {
    const items = [{ name: "No Type" } as any];
    const errors = validateBatchItems(items);
    expect(errors).toHaveLength(1);
    expect(errors[0].index).toBe(0);
    expect(errors[0].error).toContain("entity_type");
  });

  test("reports invalid entity_type", () => {
    const items = [{ entity_type: "widget", name: "Bad" }];
    const errors = validateBatchItems(items);
    expect(errors).toHaveLength(1);
    expect(errors[0].error).toContain("widget");
    expect(errors[0].error).toContain("Must be one of");
  });

  test("reports non-string entity_type", () => {
    const items = [{ entity_type: 42, name: "Numeric type" } as any];
    const errors = validateBatchItems(items);
    expect(errors).toHaveLength(1);
    expect(errors[0].error).toContain("must be a string");
  });

  test("reports multiple errors with correct indices", () => {
    const items = [
      { entity_type: "artist", name: "Valid" },
      { name: "Missing type" } as any,
      { entity_type: "bogus", name: "Invalid type" },
    ];
    const errors = validateBatchItems(items);
    expect(errors).toHaveLength(2);
    expect(errors[0].index).toBe(1);
    expect(errors[1].index).toBe(2);
  });
});

// -- groupByType --

describe("groupByType", () => {
  test("groups items by entity_type in processing order", () => {
    const items = [
      { entity_type: "release", title: "Satori" },
      { entity_type: "label", name: "Numero" },
      { entity_type: "artist", name: "Nina Hagen" },
      { entity_type: "artist", name: "Flower Travelin' Band" },
      { entity_type: "label", name: "Phoenix" },
    ];

    const groups = groupByType(items);

    // Labels should come first in the map iteration order
    const keys = [...groups.keys()];
    expect(keys).toEqual([
      "label",
      "artist",
      "release",
      "venue",
      "festival",
      "show",
    ]);

    // Check group sizes
    expect(groups.get("label")).toHaveLength(2);
    expect(groups.get("artist")).toHaveLength(2);
    expect(groups.get("release")).toHaveLength(1);
    expect(groups.get("venue")).toHaveLength(0);
    expect(groups.get("festival")).toHaveLength(0);
    expect(groups.get("show")).toHaveLength(0);
  });

  test("strips entity_type from grouped items", () => {
    const items = [
      { entity_type: "artist", name: "Nina Hagen", city: "Berlin" },
    ];

    const groups = groupByType(items);
    const artists = groups.get("artist")!;

    expect(artists).toHaveLength(1);
    expect(artists[0]).toEqual({ name: "Nina Hagen", city: "Berlin" });
    expect(artists[0]).not.toHaveProperty("entity_type");
  });

  test("preserves order within each group", () => {
    const items = [
      { entity_type: "artist", name: "First" },
      { entity_type: "artist", name: "Second" },
      { entity_type: "artist", name: "Third" },
    ];

    const groups = groupByType(items);
    const artists = groups.get("artist")!;

    expect(artists[0].name).toBe("First");
    expect(artists[1].name).toBe("Second");
    expect(artists[2].name).toBe("Third");
  });

  test("handles empty array", () => {
    const groups = groupByType([]);
    for (const type of [
      "label",
      "artist",
      "release",
      "venue",
      "festival",
      "show",
    ] as const) {
      expect(groups.get(type)).toHaveLength(0);
    }
  });

  test("dependency ordering: labels before artists before releases", () => {
    const items = [
      { entity_type: "release", title: "Satori" },
      { entity_type: "artist", name: "FTB" },
      { entity_type: "label", name: "Numero" },
    ];

    const groups = groupByType(items);
    const orderedKeys = [...groups.keys()].filter(
      (k) => groups.get(k)!.length > 0,
    );

    // Labels must come before artists, artists before releases
    const labelIdx = orderedKeys.indexOf("label");
    const artistIdx = orderedKeys.indexOf("artist");
    const releaseIdx = orderedKeys.indexOf("release");

    expect(labelIdx).toBeLessThan(artistIdx);
    expect(artistIdx).toBeLessThan(releaseIdx);
  });
});

// -- CLI integration tests --

describe("ph batch CLI integration", () => {
  const { join } = require("path");
  const { tmpdir } = require("os");
  const { mkdtemp, rm, writeFile } = require("fs/promises");

  const CLI_PATH = join(__dirname, "..", "src", "entry.ts");

  async function runCli(
    args: string[],
    env?: Record<string, string>,
  ): Promise<{ stdout: string; stderr: string; exitCode: number }> {
    const proc = Bun.spawn(["bun", "run", CLI_PATH, ...args], {
      stdout: "pipe",
      stderr: "pipe",
      env: { ...process.env, ...env },
    });

    const [stdout, stderr] = await Promise.all([
      new Response(proc.stdout as ReadableStream).text(),
      new Response(proc.stderr as ReadableStream).text(),
    ]);
    const exitCode = await proc.exited;

    return { stdout, stderr, exitCode };
  }

  test("batch with nonexistent file shows error", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-batch-test-"));
    const configDir = await mkdtemp(join(tmpdir(), "ph-batch-config-"));

    // Write a minimal config so env resolution works
    await writeFile(
      join(configDir, "config.json"),
      JSON.stringify({
        environments: {
          production: {
            url: "http://localhost:9999",
            token: "phk_test",
          },
        },
        default_environment: "production",
      }),
    );

    try {
      const { stderr, exitCode } = await runCli(
        ["batch", join(tmpDir, "nonexistent.json")],
        { PH_CONFIG_PATH: configDir },
      );
      expect(exitCode).toBe(1);
      expect(stderr).toContain("not found");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
      await rm(configDir, { recursive: true, force: true });
    }
  });

  test("batch with invalid entity_type shows error", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-batch-test-"));
    const configDir = await mkdtemp(join(tmpdir(), "ph-batch-config-"));

    await writeFile(
      join(configDir, "config.json"),
      JSON.stringify({
        environments: {
          production: {
            url: "http://localhost:9999",
            token: "phk_test",
          },
        },
        default_environment: "production",
      }),
    );

    const batchFile = join(tmpDir, "batch.json");
    await writeFile(
      batchFile,
      JSON.stringify([{ entity_type: "widget", name: "Bad" }]),
    );

    try {
      const { stderr, exitCode } = await runCli(["batch", batchFile], {
        PH_CONFIG_PATH: configDir,
      });
      expect(exitCode).toBe(1);
      expect(stderr).toContain("widget");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
      await rm(configDir, { recursive: true, force: true });
    }
  });

  test("batch with empty array shows warning", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-batch-test-"));
    const configDir = await mkdtemp(join(tmpdir(), "ph-batch-config-"));

    await writeFile(
      join(configDir, "config.json"),
      JSON.stringify({
        environments: {
          production: {
            url: "http://localhost:9999",
            token: "phk_test",
          },
        },
        default_environment: "production",
      }),
    );

    const batchFile = join(tmpDir, "batch.json");
    await writeFile(batchFile, "[]");

    try {
      const { stderr, exitCode } = await runCli(["batch", batchFile], {
        PH_CONFIG_PATH: configDir,
      });
      expect(exitCode).toBe(0);
      expect(stderr).toContain("nothing to process");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
      await rm(configDir, { recursive: true, force: true });
    }
  });
});
