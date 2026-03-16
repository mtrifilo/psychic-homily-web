import { bold, blue, gray, green, yellow, red, cyan, dim } from "./ansi";

/** Print a section header. */
export function header(text: string): void {
  process.stderr.write(`\n${bold(blue(text))}\n`);
}

/** Print a success message. */
export function success(text: string): void {
  process.stderr.write(`${green("✓")} ${text}\n`);
}

/** Print a warning message. */
export function warn(text: string): void {
  process.stderr.write(`${yellow("!")} ${text}\n`);
}

/** Print an error message. */
export function error(text: string): void {
  process.stderr.write(`${red("✗")} ${text}\n`);
}

/** Print an info message. */
export function info(text: string): void {
  process.stderr.write(`${blue("→")} ${text}\n`);
}

/** Print a key-value pair. */
export function kv(key: string, value: string): void {
  process.stderr.write(`  ${gray(key + ":")} ${value}\n`);
}

/**
 * Print a simple table to stderr.
 * Each row is an array of strings. First row is treated as header.
 */
export function table(rows: string[][]): void {
  if (rows.length === 0) return;

  // Calculate column widths
  const colWidths = rows[0].map((_, colIdx) =>
    Math.max(...rows.map((row) => (row[colIdx] || "").length)),
  );

  // Header
  const headerRow = rows[0]
    .map((cell, i) => bold(cell.padEnd(colWidths[i])))
    .join("  ");
  process.stderr.write(`  ${headerRow}\n`);

  // Separator
  const separator = colWidths.map((w) => "─".repeat(w)).join("──");
  process.stderr.write(`  ${dim(separator)}\n`);

  // Data rows
  for (let r = 1; r < rows.length; r++) {
    const row = rows[r]
      .map((cell, i) => cell.padEnd(colWidths[i]))
      .join("  ");
    process.stderr.write(`  ${row}\n`);
  }
}

/**
 * Print a field diff showing what would change in an update.
 * @param field - Field name
 * @param existing - Current value (empty string if not set)
 * @param proposed - Proposed value
 */
export function fieldDiff(
  field: string,
  existing: string,
  proposed: string,
): void {
  const fieldLabel = gray(field.padEnd(16));
  if (!existing && proposed) {
    // New info being added
    process.stderr.write(
      `  ${fieldLabel} ${dim("(empty)")} → ${green(proposed)}  ${cyan("← NEW")}\n`,
    );
  } else if (existing === proposed) {
    // Unchanged
    process.stderr.write(`  ${fieldLabel} ${dim(existing)}\n`);
  } else if (existing && proposed && existing !== proposed) {
    // Would conflict — show but don't overwrite
    process.stderr.write(
      `  ${fieldLabel} ${existing}  ${dim("(keeping existing)")}\n`,
    );
  }
}

/** Print a summary of planned operations. */
export function summary(creates: number, updates: number, skips: number): void {
  const parts: string[] = [];
  if (creates > 0) parts.push(green(`${creates} create${creates !== 1 ? "s" : ""}`));
  if (updates > 0) parts.push(yellow(`${updates} update${updates !== 1 ? "s" : ""}`));
  if (skips > 0) parts.push(gray(`${skips} skip${skips !== 1 ? "s" : ""}`));

  process.stderr.write(`\n${bold("Summary:")} ${parts.join(", ")}\n`);
}
