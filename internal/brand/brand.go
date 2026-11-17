// Package brand holds every user-visible string that carries the product name,
// so a rename touches only this file.
package brand

const (
	// Name is the command name users type.
	Name = "hearth"

	// TeamFile is the version-controlled repo list at the home-repo root.
	TeamFile = "hearth.toml"
	// LocalFile is the personal, gitignored settings file next to TeamFile.
	LocalFile = "hearth.local.toml"

	// MinVersionKey is the team-file key naming the lowest release that may read it.
	MinVersionKey = "min_hearth_version"

	// TeamFileHeader is the only comment kept when the team file is rewritten.
	TeamFileHeader = "# Managed by hearth. Edit with `hearth repo add` / `hearth repo remove`; comments are not kept."

	// Marker lines that fence the sections this tool owns in generated files.
	AgentsBegin    = "<!-- hearth:begin -->"
	AgentsEnd      = "<!-- hearth:end -->"
	GitignoreBegin = "# hearth:begin"
	GitignoreEnd   = "# hearth:end"

	// TeamFileChangedHint is printed after any command that rewrites TeamFile.
	TeamFileChangedHint = "hearth.toml is the team file: commit it and open a PR."

	// CloneTempPrefix prefixes the sibling directory a clone is made in before
	// it is renamed into place.
	CloneTempPrefix = ".hearth-"

	// StateDir is the gitignored home-repo directory for files this tool
	// generates, such as the built-in skill.
	StateDir = ".hearth"

	// BuiltinSkill is the name of the skill embedded in the binary; no other
	// skill may use it.
	BuiltinSkill = Name
)
