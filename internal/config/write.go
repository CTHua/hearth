package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/tomlstr"
)

const (
	filePerm fs.FileMode = 0o644
	// A group's subdirs are written one per line with a trailing comma.
	groupArrayOpen    = " = [\n"
	groupArrayIndent  = "  "
	groupArrayItemEnd = ",\n"
	groupArrayClose   = "]\n"
)

type schemaPart struct {
	SchemaVersion int64 `toml:"schema_version"`
}

type reposPart struct {
	Repos []Repo `toml:"repos,omitempty"`
}

type personalPart struct {
	SchemaVersion int64    `toml:"schema_version"`
	Protocol      string   `toml:"protocol,omitempty"`
	Agents        []string `toml:"agents,omitempty"`
	Skills        []string `toml:"skills,omitempty"`
	MCP           []string `toml:"mcp,omitempty"`
	Subagents     []string `toml:"subagents,omitempty"`
}

// WriteTeam replaces the whole team file under root. Only the fixed header
// comment survives; comments a person wrote are lost.
func WriteTeam(root string, t Team) error {
	var buf bytes.Buffer
	buf.WriteString(brand.TeamFileHeader + "\n")
	// Encoded in parts because the min version key is a brand constant and
	// cannot be a struct tag, and a map would sort it before schema_version.
	parts := []any{
		schemaPart{SchemaVersion: t.SchemaVersion},
		map[string]string{brand.MinVersionKey: t.MinVersion},
	}
	for _, part := range parts {
		if err := encode(&buf, part); err != nil {
			return apperr.New(apperr.Config, "cannot encode %s: %v", brand.TeamFile, err)
		}
	}
	// Repos come before skills, then MCP servers, each in the order they
	// were added.
	tables := []struct {
		present bool
		part    any
	}{
		{len(t.Repos) > 0, reposPart{Repos: t.Repos}},
	}
	for _, table := range tables {
		if !table.present {
			continue
		}
		buf.WriteString("\n")
		if err := encode(&buf, table.part); err != nil {
			return apperr.New(apperr.Config, "cannot encode %s: %v", brand.TeamFile, err)
		}
	}
	for _, skill := range t.Skills {
		buf.WriteString("\n")
		buf.WriteString(encodeSkill(skill))
	}
	for _, server := range t.MCP {
		buf.WriteString("\n")
		buf.WriteString(encodeMCP(server))
	}
	return writeFile(filepath.Join(root, brand.TeamFile), buf.Bytes())
}

// encodeSkill writes one [[skills]] table by hand: the encoder would put
// every member of a group on one line, and a pull request that adds or
// removes a member should change one line.
func encodeSkill(s Skill) string {
	var b strings.Builder
	b.WriteString("[[" + keySkills + "]]\n")
	b.WriteString(tomlstr.Line(FieldName, tomlstr.Quote(s.Name)))
	b.WriteString(tomlstr.Line(FieldURL, tomlstr.Quote(s.URL)))
	b.WriteString(tomlstr.Line(FieldCommit, tomlstr.Quote(s.Commit)))
	if !s.IsGroup() {
		b.WriteString(tomlstr.Line(FieldSubdir, tomlstr.Quote(s.Subdir)))
		return b.String()
	}
	b.WriteString(FieldSubdirs + groupArrayOpen)
	for _, subdir := range s.Subdirs {
		b.WriteString(groupArrayIndent + tomlstr.Quote(subdir) + groupArrayItemEnd)
	}
	b.WriteString(groupArrayClose)
	return b.String()
}

// encodeMCP writes one [[mcp]] table by hand: the encoder would sort
// header_envs and write it as a separate table instead of inline in the
// order it was read. Empty optional keys are left out.
func encodeMCP(m MCP) string {
	var b strings.Builder
	b.WriteString("[[" + keyMCP + "]]\n")
	b.WriteString(tomlstr.Line(FieldName, tomlstr.Quote(m.Name)))
	b.WriteString(tomlstr.Line(FieldTransport, tomlstr.Quote(m.Transport)))
	optional := []struct {
		field string
		set   bool
		value string
	}{
		{FieldCommand, m.Command != "", tomlstr.Quote(m.Command)},
		{FieldArgs, len(m.Args) > 0, tomlstr.Array(m.Args)},
		{FieldEnvVars, len(m.EnvVars) > 0, tomlstr.Array(m.EnvVars)},
		{FieldURL, m.URL != "", tomlstr.Quote(m.URL)},
		{FieldBearerTokenEnv, m.BearerTokenEnv != "", tomlstr.Quote(m.BearerTokenEnv)},
		{FieldHeaderEnvs, len(m.HeaderEnvs) > 0, tomlstr.InlineTable(HeaderPairs(m.HeaderEnvs))},
	}
	for _, key := range optional {
		if key.set {
			b.WriteString(tomlstr.Line(key.field, key.value))
		}
	}
	return b.String()
}

// HeaderPairs returns headers as header-variable pairs in order.
func HeaderPairs(headers []HeaderEnv) [][2]string {
	pairs := make([][2]string, 0, len(headers))
	for _, h := range headers {
		pairs = append(pairs, [2]string{h.Header, h.Var})
	}
	return pairs
}

// WritePersonal replaces the personal file under root with every key of p,
// so a command that changes one key keeps the others.
func WritePersonal(root string, p Personal) error {
	var buf bytes.Buffer
	part := personalPart{SchemaVersion: p.SchemaVersion, Protocol: p.Protocol, Agents: p.Agents, Skills: p.Skills, MCP: p.MCP, Subagents: p.Subagents}
	if err := encode(&buf, part); err != nil {
		return apperr.New(apperr.Config, "cannot encode %s: %v", brand.LocalFile, err)
	}
	return writeFile(filepath.Join(root, brand.LocalFile), buf.Bytes())
}

func encode(buf *bytes.Buffer, v any) error {
	enc := toml.NewEncoder(buf)
	enc.Indent = ""
	return enc.Encode(v)
}

// writeFile writes through a temporary file in the same directory and renames
// it, so a failed write never leaves a truncated config behind.
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return apperr.New(apperr.Config, "cannot write %s: %v", filepath.Base(path), err)
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	err = errors.Join(writeErr, closeErr)
	if err == nil {
		err = os.Chmod(tmp.Name(), filePerm)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		removeErr := os.Remove(tmp.Name())
		return apperr.New(apperr.Config, "cannot write %s: %v", filepath.Base(path), errors.Join(err, removeErr))
	}
	return nil
}
