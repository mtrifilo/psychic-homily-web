/**
 * Lightweight ANSI color helpers — zero dependencies.
 * Respects NO_COLOR (https://no-color.org) and TERM=dumb.
 */

function isEnabled(): boolean {
  return (
    !process.env.NO_COLOR &&
    process.env.TERM !== "dumb" &&
    (process.stdout.isTTY === true || "FORCE_COLOR" in process.env)
  );
}

const wrap =
  (open: string, close: string) =>
  (s: string) =>
    isEnabled() ? `${open}${s}${close}` : s;

// Attributes
export const bold = wrap("\x1b[1m", "\x1b[22m");
export const dim = wrap("\x1b[2m", "\x1b[22m");

// Colors
export const green = wrap("\x1b[38;5;114m", "\x1b[39m");
export const red = wrap("\x1b[38;5;196m", "\x1b[39m");
export const yellow = wrap("\x1b[38;5;214m", "\x1b[39m");
export const blue = wrap("\x1b[38;5;117m", "\x1b[39m");
export const gray = wrap("\x1b[38;5;245m", "\x1b[39m");
export const white = wrap("\x1b[38;5;253m", "\x1b[39m");
export const cyan = wrap("\x1b[38;5;80m", "\x1b[39m");
