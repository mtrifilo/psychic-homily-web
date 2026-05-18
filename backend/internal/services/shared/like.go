package shared

import "strings"

// likeEscaper escapes the characters that would otherwise have wildcard
// meaning inside a SQL LIKE / ILIKE pattern. Postgres treats `\` as the
// default escape character, so escaped wildcards become literal characters
// without needing an explicit ESCAPE clause.
//
//	\  → \\   (escape the escape character first)
//	%  → \%   (any-substring wildcard becomes literal)
//	_  → \_   (single-character wildcard becomes literal)
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikePattern produces a substring-match LIKE / ILIKE pattern for the given
// user-supplied search term. Escapes wildcard characters so the input is
// matched literally, then wraps with `%…%`. Use anywhere a user-entered
// search string is interpolated into an ILIKE.
//
//	shared.LikePattern("indie")     // "%indie%"
//	shared.LikePattern("foo%bar")   // "%foo\%bar%"
//	shared.LikePattern("a_b")       // "%a\_b%"
//
// Callers are still responsible for trimming whitespace and short-circuiting
// the empty-string case before reaching the DB — an empty term here would
// produce `%%` and widen the result set.
func LikePattern(searchTerm string) string {
	return "%" + likeEscaper.Replace(searchTerm) + "%"
}

// LikePrefixPattern produces a starts-with LIKE / ILIKE pattern. Same escape
// guarantees as LikePattern but only appends a trailing `%`, so the input is
// matched as a literal prefix. Use for autocomplete-style "name starts
// with X" lookups where the DB has (or should have) a btree index on
// `LOWER(column)` that the optimizer can use only when the leading
// character is a literal.
//
//	shared.LikePrefixPattern("foo")     // "foo%"
//	shared.LikePrefixPattern("foo%")    // "foo\%%"
//	shared.LikePrefixPattern("a_")      // "a\_%"
//
// Same empty-string caveat as LikePattern — an empty term here produces `%`
// which matches every row.
func LikePrefixPattern(searchTerm string) string {
	return likeEscaper.Replace(searchTerm) + "%"
}
