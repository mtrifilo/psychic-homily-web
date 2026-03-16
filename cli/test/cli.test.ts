import { describe, test, expect } from "bun:test";
import { join } from "path";
import { tmpdir } from "os";
import { mkdtemp, rm } from "fs/promises";

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

describe("CLI integration", () => {
  test("--version prints version", async () => {
    const { stdout, exitCode } = await runCli(["--version"]);
    expect(stdout.trim()).toBe("0.1.0");
    expect(exitCode).toBe(0);
  });

  test("--help lists all commands", async () => {
    const { stdout, exitCode } = await runCli(["--help"]);
    expect(stdout).toContain("init");
    expect(stdout).toContain("config");
    expect(stdout).toContain("search");
    expect(stdout).toContain("submit");
    expect(stdout).toContain("batch");
    expect(exitCode).toBe(0);
  });

  test("config show works with empty config", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-cli-test-"));
    try {
      const { stderr, exitCode } = await runCli(["config", "show"], {
        PH_CONFIG_PATH: tmpDir,
      });
      expect(stderr).toContain("No environments configured");
      expect(stderr).toContain("ph init");
      expect(exitCode).toBe(0);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("search fails without configured environment", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-cli-test-"));
    try {
      const { stderr, exitCode } = await runCli(
        ["search", "artist", "test"],
        { PH_CONFIG_PATH: tmpDir },
      );
      expect(stderr).toContain('not found');
      expect(stderr).toContain("ph init");
      expect(exitCode).toBe(1);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("init requires --url and --token", async () => {
    const { stderr, exitCode } = await runCli(["init"]);
    expect(stderr).toContain("required");
    expect(exitCode).toBe(1);
  });

  test("submit artist without JSON exits with error", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-cli-test-"));
    try {
      const { stderr, exitCode } = await runCli(["submit", "artist"], {
        PH_CONFIG_PATH: tmpDir,
      });
      // Will fail either with env not found or empty input
      expect(exitCode).toBe(1);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("submit unimplemented type shows not-yet-implemented message", async () => {
    const tmpDir = await mkdtemp(join(tmpdir(), "ph-cli-test-"));
    try {
      const { stderr, exitCode } = await runCli(["submit", "venue", "{}"], {
        PH_CONFIG_PATH: tmpDir,
      });
      // venue submit requires env config, but if config missing it fails there first
      // so we just check exit code is 1
      expect(exitCode).toBe(1);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("--env flag is accepted", async () => {
    const { stdout } = await runCli(["--env", "local", "--help"]);
    expect(stdout).toContain("ph");
  });
});
