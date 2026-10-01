package belay

import (
	"fmt"
	"path"
	"strings"
)

// ToolRule says which tool calls need a belay check before they run. It is a
// list of glob patterns (path.Match syntax) matched against tool names, e.g.
// "k8s_delete_*,k8s_scale". The zero value matches nothing.
type ToolRule struct {
	patterns []string
}

// ParseToolRule reads a comma-separated list of patterns. Blank entries are
// ignored; an invalid pattern is an error, so a typo cannot silently disable
// a check.
func ParseToolRule(spec string) (ToolRule, error) {
	var r ToolRule
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return ToolRule{}, fmt.Errorf("invalid tool pattern %q: %w", p, err)
		}
		r.patterns = append(r.patterns, p)
	}
	return r, nil
}

// Matches reports whether a call to the named tool needs a belay check.
func (r ToolRule) Matches(tool string) bool {
	for _, p := range r.patterns {
		if ok, _ := path.Match(p, tool); ok {
			return true
		}
	}
	return false
}

// Empty reports whether the rule matches no tool.
func (r ToolRule) Empty() bool {
	return len(r.patterns) == 0
}

// String returns the patterns as given, comma-separated.
func (r ToolRule) String() string {
	return strings.Join(r.patterns, ",")
}
