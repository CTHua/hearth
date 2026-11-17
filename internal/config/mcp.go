package config

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Transports an MCP server may use. sse is left out: Claude Code calls it
// legacy and Codex does not read it.
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
)

// Field names of an [[mcp]] entry that [[repos]] and [[skills]] do not have;
// name and url are shared.
const (
	FieldTransport      = "transport"
	FieldCommand        = "command"
	FieldArgs           = "args"
	FieldEnvVars        = "env_vars"
	FieldBearerTokenEnv = "bearer_token_env"
	FieldHeaderEnvs     = "header_envs"
)

const (
	keyMCP              = "mcp"
	authorizationHeader = "Authorization"
	schemeHTTPS         = "https"
	schemeHTTP          = "http"
)

var (
	stdioFields = []string{FieldCommand, FieldArgs, FieldEnvVars}
	httpFields  = []string{FieldURL, FieldBearerTokenEnv, FieldHeaderEnvs}
	mcpFields   = slices.Concat([]string{FieldName, FieldTransport}, stdioFields, httpFields)
	// No '.': [mcp_servers.a.b] would be a nested table in Codex's config.
	mcpNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	envNameRE = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	// An HTTP token (RFC 9110 section 5.6.2).
	headerNameRE = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	// Plain http only reaches this machine; compared with Hostname() exactly,
	// so 127.0.0.10 and localhost.example.invalid are not local.
	localHosts = []string{"localhost", "127.0.0.1"}
)

// HeaderEnv is one header_envs pair: the header is sent with the value of
// the environment variable Var.
type HeaderEnv struct {
	Header string
	Var    string
}

// MCP is one [[mcp]] entry of the team file. Secrets are only ever named by
// environment variable; the file has no place for a literal env or header
// value. Stdio servers use Command, Args and EnvVars; http servers use URL,
// BearerTokenEnv and HeaderEnvs.
type MCP struct {
	Name           string
	Transport      string
	Command        string
	Args           []string
	EnvVars        []string
	URL            string
	BearerTokenEnv string
	HeaderEnvs     []HeaderEnv
}

// EnvNames lists the environment variables the server needs, in file order:
// env_vars, then the bearer token, then each header's variable.
func (m MCP) EnvNames() []string {
	names := slices.Clone(m.EnvVars)
	if m.BearerTokenEnv != "" {
		names = append(names, m.BearerTokenEnv)
	}
	for _, h := range m.HeaderEnvs {
		names = append(names, h.Var)
	}
	return names
}

// FindMCP returns the MCP server called name, compared case-insensitively.
func (t Team) FindMCP(name string) (MCP, bool) {
	for _, server := range t.MCP {
		if strings.EqualFold(server.Name, name) {
			return server, true
		}
	}
	return MCP{}, false
}

// MCPNames lists the MCP server names in file order.
func (t Team) MCPNames() []string {
	names := make([]string, 0, len(t.MCP))
	for _, server := range t.MCP {
		names = append(names, server.Name)
	}
	return names
}

// WithMCP returns t with server appended. The error is FieldErrors when
// server breaks a rule or its name is taken.
func (t Team) WithMCP(server MCP) (Team, error) {
	next := t
	next.MCP = append(slices.Clone(t.MCP), server)
	if problems := checkMCP(next.MCP, nil); len(problems) > 0 {
		return t, problems
	}
	return next, nil
}

// WithoutMCP returns t without the MCP server called name, and whether it
// was there.
func (t Team) WithoutMCP(name string) (Team, bool) {
	next := t
	next.MCP = slices.DeleteFunc(slices.Clone(t.MCP), func(m MCP) bool { return strings.EqualFold(m.Name, name) })
	return next, len(next.MCP) != len(t.MCP)
}

// WithMCPChoices returns p with the chosen MCP server names replaced.
func (p Personal) WithMCPChoices(names []string) Personal {
	next := p.started()
	next.MCP = slices.Clone(names)
	return next
}

// MCPNameProblem explains why name cannot be an MCP server name; it is
// empty when it can.
func MCPNameProblem(name string) string {
	if name == "" {
		return "must be a non-empty string"
	}
	if !mcpNameRE.MatchString(name) {
		return fmt.Sprintf("%q must match %s ('.' is not allowed)", name, mcpNameRE.String())
	}
	return ""
}

// checkMCP validates every value, then names across entries. Fields in
// reported already have a problem from decoding and are not checked again;
// their entries take no part in the duplicate-name check.
func checkMCP(servers []MCP, reported FieldErrors) FieldErrors {
	skip := map[fieldKey]bool{}
	valid := make([]bool, len(servers))
	for i := range valid {
		valid[i] = true
	}
	for _, e := range reported {
		skip[fieldKey{index: e.Index, field: baseField(e.Field)}] = true
		valid[e.Index] = false
	}
	var problems FieldErrors
	names := make([]string, len(servers))
	for i, server := range servers {
		names[i] = server.Name
		for _, problem := range serverProblems(i, server, skip) {
			problems = append(problems, problem)
			valid[i] = false
		}
	}
	return append(problems, duplicateNames(keyMCP, names, valid)...)
}

// locatedItem names item i of the array field, such as "args[1]".
func locatedItem(field string, i int) string {
	return fmt.Sprintf("%s[%d]", field, i)
}

// baseField is the field a located problem such as "args[1]" belongs to.
func baseField(field string) string {
	base, _, _ := strings.Cut(field, "[")
	return base
}

func serverProblems(index int, m MCP, skip map[fieldKey]bool) FieldErrors {
	var problems FieldErrors
	add := func(field string, message string) {
		if message != "" && !skip[fieldKey{index: index, field: baseField(field)}] {
			problems = append(problems, FieldError{Table: keyMCP, Index: index, Field: field, Message: message})
		}
	}
	add(FieldName, MCPNameProblem(m.Name))
	switch m.Transport {
	case TransportStdio:
		add(FieldCommand, requiredProblem(m.Command))
		// A repeated name would be a repeated key in the generated files.
		seen := map[string]int{}
		for i, name := range m.EnvVars {
			field := locatedItem(FieldEnvVars, i)
			if message := envProblem(name); message != "" {
				add(field, message)
				continue
			}
			if first, dup := seen[name]; dup {
				add(field, fmt.Sprintf("duplicate %q (also %s[%d])", name, FieldEnvVars, first))
				continue
			}
			seen[name] = i
		}
		for _, field := range setFields(m, httpFields) {
			add(field, mixedProblem(TransportStdio))
		}
	case TransportHTTP:
		message := requiredProblem(m.URL)
		if message == "" {
			message = urlProblem(m.URL)
		}
		add(FieldURL, message)
		if m.BearerTokenEnv != "" {
			add(FieldBearerTokenEnv, envProblem(m.BearerTokenEnv))
		}
		add(FieldHeaderEnvs, headerProblem(m.HeaderEnvs))
		for _, field := range setFields(m, stdioFields) {
			add(field, mixedProblem(TransportHTTP))
		}
	case "":
		add(FieldTransport, "must be a non-empty string")
	default:
		add(FieldTransport, fmt.Sprintf("%q must be %q or %q", m.Transport, TransportStdio, TransportHTTP))
	}
	return problems
}

func requiredProblem(value string) string {
	if value == "" {
		return "must be a non-empty string"
	}
	return ""
}

func mixedProblem(transport string) string {
	return fmt.Sprintf("must not be set when transport is %q", transport)
}

// setFields lists which of fields hold a value in m.
func setFields(m MCP, fields []string) []string {
	values := map[string]bool{
		FieldCommand:        m.Command != "",
		FieldArgs:           len(m.Args) > 0,
		FieldEnvVars:        len(m.EnvVars) > 0,
		FieldURL:            m.URL != "",
		FieldBearerTokenEnv: m.BearerTokenEnv != "",
		FieldHeaderEnvs:     len(m.HeaderEnvs) > 0,
	}
	var set []string
	for _, field := range fields {
		if values[field] {
			set = append(set, field)
		}
	}
	return set
}

func envProblem(name string) string {
	if !envNameRE.MatchString(name) {
		return fmt.Sprintf("%q must be an environment variable name matching %s", name, envNameRE.String())
	}
	return ""
}

// headerProblem returns the first problem of header_envs. Header names are
// compared case-insensitively, as HTTP does, and Authorization is left to
// bearer_token_env in every spelling.
func headerProblem(headers []HeaderEnv) string {
	seen := map[string]string{}
	for _, h := range headers {
		if !headerNameRE.MatchString(h.Header) {
			return fmt.Sprintf("header %q must be an HTTP token matching %s", h.Header, headerNameRE.String())
		}
		if strings.EqualFold(h.Header, authorizationHeader) {
			return fmt.Sprintf("header %q is not allowed (compared case-insensitively); name the token with %s", h.Header, FieldBearerTokenEnv)
		}
		lower := strings.ToLower(h.Header)
		if first, dup := seen[lower]; dup {
			return fmt.Sprintf("duplicate header %q (also %q, header names are compared case-insensitively)", h.Header, first)
		}
		seen[lower] = h.Header
		if message := envProblem(h.Var); message != "" {
			return fmt.Sprintf("header %q: %s", h.Header, message)
		}
	}
	return ""
}

// urlProblem accepts https with a host, and plain http only to this
// machine. A URL with user info is refused without echoing it, since the
// password would be printed.
func urlProblem(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "is not a valid URL"
	}
	if u.User != nil {
		return "must not contain a user name or password; name the secret with bearer_token_env or header_envs"
	}
	if u.Scheme == schemeHTTPS && u.Hostname() != "" {
		return ""
	}
	if u.Scheme == schemeHTTP && slices.Contains(localHosts, u.Hostname()) {
		return ""
	}
	return fmt.Sprintf("%q must be https://<host>[:port][/path], or http:// to %s with an optional port and path",
		raw, strings.Join(localHosts, " or "))
}
