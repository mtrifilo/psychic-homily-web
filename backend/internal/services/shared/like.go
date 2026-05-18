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
