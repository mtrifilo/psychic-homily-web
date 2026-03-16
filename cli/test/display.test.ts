import { describe, test, expect } from "bun:test";
import { join } from "path";
import { tmpdir } from "os";

describe("ansi NO_COLOR", () => {
  test("color functions strip ANSI when NO_COLOR is set", async () => {
    const scriptPath = join(tmpdir(), `ph-ansi-test-${Date.now()}.ts`);
    await Bun.write(
      scriptPath,
      `
      import { bold, green, gray } from "${join(import.meta.dir, "..", "src", "lib", "ansi.ts")}";
      const results = [
        bold("test") === "test",
        green("test") === "test",
        gray("test") === "test",
      ];
      process.stdout.write(JSON.stringify(results));
    `,
    );

    const proc = Bun.spawn(["bun", "run", scriptPath], {
      stdout: "pipe",
      stderr: "pipe",
      env: { ...process.env, NO_COLOR: "1", TERM: "dumb" },
    });

    const output = await new Response(proc.stdout as ReadableStream).text();
    await proc.exited;

    const results = JSON.parse(output);
    expect(results).toEqual([true, true, true]);

    // Cleanup
    await Bun.write(scriptPath, ""); // Can't unlink easily, just empty it
  });
});
