package venueaddress

import (
	"regexp"
	"strings"
)

// robotsRules is the rule set robots.txt applies to this fetcher. A nil
// *robotsRules allows everything (no robots.txt, or no group that applies).
type robotsRules struct {
	rules []robotsRule
}

type robotsRule struct {
	allow   bool
	pattern string
}

// parseRobots reads the groups that apply to token, falling back to the "*"
// groups when none names it, per RFC 9309: a group is one or more user-agent
// lines followed by its allow/disallow lines, and every group that matches is
// merged. Unknown lines are ignored.
func parseRobots(body, token string) *robotsRules {
	token = strings.ToLower(token)
	var named, wildcard []robotsRule
	var groupAgents []string
	inRules := false
	var current []robotsRule
	flush := func() {
		for _, a := range groupAgents {
			switch {
			case a == "*":
				wildcard = append(wildcard, current...)
			case a != "" && strings.Contains(token, a):
				named = append(named, current...)
			}
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if inRules {
				flush()
				groupAgents, current, inRules = nil, nil, false
			}
			groupAgents = append(groupAgents, strings.ToLower(value))
		case "allow", "disallow":
			inRules = true
			if value == "" {
				continue // an empty disallow allows everything; an empty allow says nothing
			}
			current = append(current, robotsRule{allow: key == "allow", pattern: value})
		}
	}
	flush()
	if len(named) > 0 {
		return &robotsRules{rules: named}
	}
	if len(wildcard) > 0 {
		return &robotsRules{rules: wildcard}
	}
	return nil
}

// allows applies the most specific matching rule (the longest pattern), with
// allow winning a tie; a path no rule matches is allowed.
func (r *robotsRules) allows(path string) bool {
	if r == nil {
		return true
	}
	best := -1
	allowed := true
	for _, rule := range r.rules {
		if !robotsMatch(rule.pattern, path) {
			continue
		}
		n := len(rule.pattern)
		if n > best || (n == best && rule.allow) {
			best = n
			allowed = rule.allow
		}
	}
	return allowed
}

// robotsMatch reports whether path matches a robots.txt pattern: a prefix
// match where '*' matches any run of characters and a trailing '$' anchors the
// end.
func robotsMatch(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	expr := "^" + strings.Join(parts, ".*")
	if anchored {
		expr += "$"
	}
	re, err := regexp.Compile(expr)
	return err == nil && re.MatchString(path)
}
