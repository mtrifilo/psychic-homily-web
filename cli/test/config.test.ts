import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import {
  readConfig,
  writeConfig,
  resolveEnvironment,
  suggestEnvironment,
} from "../src/lib/config";
import { join } from "path";
import { tmpdir } from "os";
import { mkdtemp, rm } from "fs/promises";

describe("config", () => {
  let tmpDir: string;
  let originalEnv: string | undefined;

  beforeEach(async () => {
    tmpDir = await mkdtemp(join(tmpdir(), "ph-test-"));
    originalEnv = process.env.PH_CONFIG_PATH;
    process.env.PH_CONFIG_PATH = tmpDir;
  });

  afterEach(async () => {
    if (originalEnv !== undefined) {
      process.env.PH_CONFIG_PATH = originalEnv;
    } else {
      delete process.env.PH_CONFIG_PATH;
    }
    await rm(tmpDir, { recursive: true, force: true });
  });

  test("readConfig returns defaults when no config file exists", async () => {
    const config = await readConfig();
    expect(config.default_environment).toBe("production");
    expect(config.environments).toEqual({});
  });

  test("writeConfig and readConfig roundtrip", async () => {
    const config = {
      environments: {
        production: {
          url: "https://api.psychichomily.com",
          token: "phk_test123",
        },
        local: {
          url: "http://localhost:8080",
          token: "phk_local456",
        },
      },
      default_environment: "production",
    };

    await writeConfig(config);
    const read = await readConfig();

    expect(read.default_environment).toBe("production");
    expect(read.environments.production.url).toBe(
      "https://api.psychichomily.com",
    );
    expect(read.environments.production.token).toBe("phk_test123");
    expect(read.environments.local.url).toBe("http://localhost:8080");
    expect(read.environments.local.token).toBe("phk_local456");
  });

  test("resolveEnvironment returns the default environment", () => {
    const config = {
      environments: {
        production: {
          url: "https://api.psychichomily.com",
          token: "phk_test",
        },
      },
      default_environment: "production",
    };

    const result = resolveEnvironment(config);
    expect(result).not.toBeNull();
    expect(result!.name).toBe("production");
    expect(result!.env.url).toBe("https://api.psychichomily.com");
  });

  test("resolveEnvironment uses override when provided", () => {
    const config = {
      environments: {
        production: {
          url: "https://api.psychichomily.com",
          token: "phk_prod",
        },
        local: { url: "http://localhost:8080", token: "phk_local" },
      },
      default_environment: "production",
    };

    const result = resolveEnvironment(config, "local");
    expect(result).not.toBeNull();
    expect(result!.name).toBe("local");
    expect(result!.env.url).toBe("http://localhost:8080");
  });

  test("resolveEnvironment returns null for unknown environment", () => {
    const config = {
      environments: {
        production: {
          url: "https://api.psychichomily.com",
          token: "phk_test",
        },
      },
      default_environment: "production",
    };

    const result = resolveEnvironment(config, "staging");
    expect(result).toBeNull();
  });

  describe("suggestEnvironment", () => {
    const configured = ["local", "stage", "production"];

    test("suggests stage for staging (edit distance 3, not a substring)", () => {
      // The PSY-975 motivating case. "staging" does NOT contain "stage"
      // (s-t-a-g-i-n-g), so this is caught by the edit-distance fallback at
      // distance 3 (drop "ing", substitute to "e") — which is why the
      // threshold is 3, not 2.
      expect(suggestEnvironment("staging", configured)).toBe("stage");
    });

    test("suggests the substring match for prod -> production", () => {
      // "production" starts with "prod", so this hits the substring branch.
      expect(suggestEnvironment("prod", configured)).toBe("production");
    });

    test("suggests the nearest name by edit distance for a small typo", () => {
      // "stge" contains no configured name as a substring, so this exercises
      // the Levenshtein fallback (distance 1) rather than the substring branch.
      expect(suggestEnvironment("stge", configured)).toBe("stage");
    });

    test("returns null when nothing is close enough", () => {
      expect(suggestEnvironment("xyzzy", configured)).toBeNull();
    });

    test("returns null for an empty configured list", () => {
      expect(suggestEnvironment("stage", [])).toBeNull();
    });
  });
});
