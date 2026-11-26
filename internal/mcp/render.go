// Package mcp generates each chosen agent's project MCP config from the
// servers a person chose, and reports what those servers need from the
// environment. The generated formats live only here.
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/tomlstr"
)

// Generated files, relative to the home-repo root with "/" separators.
const (
	// ClaudeFile is the project MCP config Claude Code reads.
	ClaudeFile = ".mcp.json"
	// CodexFile is the project config Codex reads in a trusted project.
	CodexFile = ".codex/config.toml"
	// CopilotFile is the project MCP config GitHub Copilot reads.
	CopilotFile = ".copilot/config.json"
)

const (
	jsonIndent          = "  "
	codexTablePrefix    = "mcp_servers."
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "

	keyServers  = "mcpServers"
	keyType     = "type"
	keyCommand  = "command"
	keyArgs     = "args"
	keyEnv      = "env"
	keyURL      = "url"
	keyHeaders  = "headers"
	keyEnvVars  = "env_vars"
	keyBearer   = "bearer_token_env_var"
	keyHeaderTo = "env_http_headers"
)

// envRef is how Claude Code's .mcp.json expands a variable at start-up, so
// the file holds the name and never the value.
func envRef(name string) string {
	return "${" + name + "}"
}

// field is one key of a JSON object; object keeps keys in the order given,
// which a Go map would not.
type field struct {
	key   string
	value any
}

type object []field

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := encodeJSON(f.key)
		if err != nil {
			return nil, err
		}
		value, err := encodeJSON(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// encodeJSON encodes v compactly without HTML escaping, so "&" in an
// argument stays readable.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// renderClaude returns .mcp.json for servers, in their order.
func renderClaude(servers []config.MCP) ([]byte, error) {
	entries := object{}
	for _, server := range servers {
		entries = append(entries, field{server.Name, claudeServer(server)})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", jsonIndent)
	if err := enc.Encode(object{{keyServers, entries}}); err != nil {
		return nil, fmt.Errorf("cannot encode %s: %w", ClaudeFile, err)
	}
	return b.Bytes(), nil
}

func claudeServer(server config.MCP) object {
	if server.Transport == config.TransportHTTP {
		entry := object{{keyType, config.TransportHTTP}, {keyURL, server.URL}}
		headers := object{}
		if server.BearerTokenEnv != "" {
			headers = append(headers, field{authorizationHeader, bearerPrefix + envRef(server.BearerTokenEnv)})
		}
		for _, h := range server.HeaderEnvs {
			headers = append(headers, field{h.Header, envRef(h.Var)})
		}
		if len(headers) > 0 {
			entry = append(entry, field{keyHeaders, headers})
		}
		return entry
	}
	args := server.Args
	if args == nil {
		args = []string{}
	}
	entry := object{{keyType, config.TransportStdio}, {keyCommand, server.Command}, {keyArgs, args}}
	env := object{}
	for _, name := range server.EnvVars {
		env = append(env, field{name, envRef(name)})
	}
	if len(env) > 0 {
		entry = append(entry, field{keyEnv, env})
	}
	return entry
}

// renderCodex returns .codex/config.toml for servers, in their order.
// Codex forwards env_vars from its own environment, and reads the bearer
// token and headers from the named variables, so no value is written.
func renderCodex(servers []config.MCP) []byte {
	var b strings.Builder
	b.WriteString(brand.CodexConfigHeader + "\n")
	for _, server := range servers {
		b.WriteString("\n[" + codexTablePrefix + server.Name + "]\n")
		if server.Transport == config.TransportHTTP {
			b.WriteString(tomlstr.Line(keyURL, tomlstr.Quote(server.URL)))
			if server.BearerTokenEnv != "" {
				b.WriteString(tomlstr.Line(keyBearer, tomlstr.Quote(server.BearerTokenEnv)))
			}
			if len(server.HeaderEnvs) > 0 {
				b.WriteString(tomlstr.Line(keyHeaderTo, tomlstr.InlineTable(config.HeaderPairs(server.HeaderEnvs))))
			}
			continue
		}
		b.WriteString(tomlstr.Line(keyCommand, tomlstr.Quote(server.Command)))
		b.WriteString(tomlstr.Line(keyArgs, tomlstr.Array(server.Args)))
		if len(server.EnvVars) > 0 {
			b.WriteString(tomlstr.Line(keyEnvVars, tomlstr.Array(server.EnvVars)))
		}
	}
	return []byte(b.String())
}

// renderCopilot returns .copilot/config.json for servers, in their order.
// GitHub Copilot uses the same JSON format as Claude Code.
func renderCopilot(servers []config.MCP) ([]byte, error) {
	entries := object{}
	for _, server := range servers {
		entries = append(entries, field{server.Name, claudeServer(server)})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", jsonIndent)
	if err := enc.Encode(object{{keyServers, entries}}); err != nil {
		return nil, fmt.Errorf("cannot encode %s: %w", CopilotFile, err)
	}
	return b.Bytes(), nil
}
