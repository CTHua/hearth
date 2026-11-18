package config

import (
	"fmt"
	"regexp"
	"slices"
)

// subagentNameRE is the name rule of a subagent. '-' is allowed, as in
// Claude Code's usual code-reviewer; whether Codex accepts it in a role
// name is not verified yet.
var subagentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// SubagentNameProblem explains why name cannot be a subagent name; it is
// empty when it can.
func SubagentNameProblem(name string) string {
	if name == "" {
		return "must be a non-empty string"
	}
	if !subagentNameRE.MatchString(name) {
		return fmt.Sprintf("%q must match %s", name, subagentNameRE.String())
	}
	return ""
}

// WithSubagents returns p with the chosen subagent names replaced.
func (p Personal) WithSubagents(names []string) Personal {
	next := p.started()
	next.Subagents = slices.Clone(names)
	return next
}
