import { describe, test, expect } from "bun:test";
import { join } from "path";
import { tmpdir } from "os";
import { mkdtemp, rm, writeFile } from "fs/promises";

const CLI_PATH = join(import.meta.dir, "..", "src", "entry.ts");

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

describe("ph status", () => {
  test("shows unconfigured state when no config exists", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-status-test-"));
    try {
      const { stderr, exitCode } = await runCli(["status"], {
        PH_CONFIG_PATH: tmpDir,
      });
      expect(exitCode).toBe(0);
      expect(stderr).toContain("Status");
      expect(stderr).toContain("not configured");
      expect(stderr).toContain("ph init");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("shows configured environment info", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-status-test-"));
    await writeFile(
      join(tmpDir, "config.json"),
      JSON.stringify({
        environments: {
          production: {
            url: "http://localhost:9999",
            token: "phk_test_token_1234567890",
          },
        },
        default_environment: "production",
      }),
    );

    try {
      const { stderr, exitCode } = await runCli(["status"], {
        PH_CONFIG_PATH: tmpDir,
      });
      expect(exitCode).toBe(0);
      expect(stderr).toContain("Status");
      expect(stderr).toContain("production");
      expect(stderr).toContain("http://localhost:9999");
      // Token should be masked
      expect(stderr).toContain("phk_test");
      expect(stderr).not.toContain("phk_test_token_1234567890");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("shows unreachable when API is down", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-status-test-"));
    await writeFile(
      join(tmpDir, "config.json"),
      JSON.stringify({
        environments: {
          production: {
            url: "http://localhost:9999",
            token: "phk_test_token_1234567890",
          },
        },
        default_environment: "production",
      }),
    );

    try {
      const { stderr, exitCode } = await runCli(["status"], {
        PH_CONFIG_PATH: tmpDir,
      });
      expect(exitCode).toBe(0);
      expect(stderr).toContain("unreachable");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("shows available commands", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-status-test-"));
    try {
      const { stderr, exitCode } = await runCli(["status"], {
        PH_CONFIG_PATH: tmpDir,
      });
      expect(exitCode).toBe(0);
      expect(stderr).toContain("Available Commands");
      expect(stderr).toContain("ph init");
      expect(stderr).toContain("ph search");
      expect(stderr).toContain("ph submit");
      expect(stderr).toContain("ph batch");
      expect(stderr).toContain("ph status");
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("--help shows status command", async () => {
    const { stdout, exitCode } = await runCli(["--help"]);
    expect(stdout).toContain("status");
    expect(exitCode).toBe(0);
  });
});
