# shellcheck shell=bash
# scripts/lib/sql_strip_comments.sh
#
# Sourced by the migration gates (check_migration_lock_timeout.sh and
# check_schema_assertion_types.sh) so both read a migration's SQL by one rule.
# Defines functions only; sourcing it runs nothing.

# sql_strip_comments <file>: prints the SQL in <file> with its comments
# removed, as one line, so a statement split across lines still matches. A
# line break reads as one space. A `--` comment runs to the end of its line; a
# `/* */` comment nests, may span lines, and reads as one space. Inside a
# block comment `--` is plain text, and inside a line comment `/*` is. A
# single-quoted literal stays in the output, and comment markers inside it are
# text. Not parsed: backslash escapes in E'' literals, and dollar quoting (a $$
# body is scanned as SQL, which matches a function body's comments). Case is
# kept.
sql_strip_comments() {
  awk '
    # first_of(a, b): the smaller of two index() results, ignoring 0 (absent).
    function first_of(a, b) {
      if (a == 0) return b
      if (b == 0 || a < b) return a
      return b
    }
    {
      rest = $0
      while (rest != "") {
        if (in_quote) {
          quote_at = index(rest, "\047")
          if (quote_at == 0) { printf "%s", rest; break }
          printf "%s", substr(rest, 1, quote_at)
          rest = substr(rest, quote_at + 1)
          in_quote = 0
          continue
        }
        open_at = index(rest, "/*")
        if (depth > 0) {
          close_at = index(rest, "*/")
          if (close_at == 0 && open_at == 0) break
          if (open_at > 0 && (close_at == 0 || open_at < close_at)) {
            depth++
            rest = substr(rest, open_at + 2)
          } else {
            depth--
            rest = substr(rest, close_at + 2)
            if (depth == 0) printf " "
          }
        } else {
          dash_at = index(rest, "--")
          quote_at = index(rest, "\047")
          next_at = first_of(first_of(open_at, dash_at), quote_at)
          if (next_at == 0) { printf "%s", rest; break }
          printf "%s", substr(rest, 1, next_at - 1)
          if (next_at == dash_at) break
          if (next_at == quote_at) {
            printf "\047"
            in_quote = 1
            rest = substr(rest, quote_at + 1)
          } else {
            depth = 1
            rest = substr(rest, open_at + 2)
          }
        }
      }
      printf " "
    }
    END { print "" }
  ' "$1"
}
